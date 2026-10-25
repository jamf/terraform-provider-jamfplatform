// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/aischemas"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// ModifyPlan predicts what this apply will do to the two attributes publishing moves, then checks
// the tool, the schema version and the settings body against what Jamf publishes, during plan
// rather than at apply.
//
// The checks live here rather than in an attribute validator because they need the API: the tool
// catalogue and the vendor schema are server data, so there is no literal set to validate against
// and no generated SDK constant to alias. Attribute validators run before the provider is configured
// and have no client, which is what rules them out.
//
// Every check is best-effort by construction. A catalogue or schema that cannot be fetched produces
// no findings — the platform validates the write regardless, and a plan that failed because the
// provider could not reach an advisory endpoint would be worse than a plan that let the apply report
// the problem. It does not pass silently, though: the operator is told once per plan that no check
// ran, because the resource's own documentation promises the settings are checked during plan, so a
// clean plan otherwise reads as confirmation that they were.
//
// The prediction runs before the checks and independently of them, because it has to hold for a plan
// whose tool or schema version is still unknown — an unresolved interpolation stops the checks, and
// a policy whose publish state Terraform then failed to plan would be worse than an unchecked one.
func (r *PolicyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || r.client == nil {
		return
	}

	r.reportNameCollision(ctx, req, resp)

	var plan policyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	prior := priorPolicy(ctx, req.State, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	planPublishOutcome(ctx, resp, &plan, prior)
	if resp.Diagnostics.HasError() {
		return
	}

	if !helpers.IsConfiguredValue(plan.ToolID) || !helpers.IsConfiguredValue(plan.SchemaVersion) {
		return
	}

	toolID := plan.ToolID.ValueString()
	schemaVersion := plan.SchemaVersion.ValueString()
	if !r.checkCatalogue(ctx, &resp.Diagnostics, toolID, schemaVersion, catalogueChanges(&plan, prior)) {
		return
	}
	r.checkSettings(ctx, &resp.Diagnostics, &plan, toolID, schemaVersion)
}

// priorPolicy reads the prior state, reporting nil for a create — a plan with no prior state to
// compare against, where every value is one the operator has just written.
func priorPolicy(ctx context.Context, state tfsdk.State, diags *diag.Diagnostics) *policyModel {
	if state.Raw.IsNull() {
		return nil
	}
	var prior policyModel
	diags.Append(state.Get(ctx, &prior)...)
	if diags.HasError() {
		return nil
	}
	return &prior
}

// planPublishOutcome plans the two attributes a publish moves, `has_draft` and `published_version`.
//
// It exists because the framework marks a Computed attribute unknown only when the proposed new
// state differs from the prior state at all (fwserver.MarkComputedNilsAsUnknown, reached from
// PlanResourceChange only under that condition). So the two cases this function handles are the two
// the default gets wrong in opposite directions.
//
// A draft that survived into state with publishing enabled is planned as republished, both
// attributes unknown, so a diff exists and Update runs. Without it, the plan after a failed publish
// is empty: state records has_draft true, config equals state, nothing goes unknown, and Terraform
// never calls Update — leaving blueprints delivering the previous version's settings for good, with
// nothing anywhere to say so. Two facts make republishing safe rather than presumptuous. First, a
// persistent has_draft with config equal to state has essentially one cause, a failed publish:
// aigovernance.PolicyDetail's Settings and SchemaVersion are the *draft's* values, so a refresh
// pulls an admin's UI edits into state, config then differs from it, and the ordinary settings diff
// reverts them — a UI draft whose settings differ from the configuration never reaches this branch,
// and one whose settings match it is harmless to publish. Second, the retry works: wire-probed on
// 2026-08-30, a PATCH sending settings identical to an existing draft answers 204 and leaves
// hasDraft true — the draft survives an identical write — and the POST /publish that follows answers
// 201 with versionNumber 2 rather than 409 NO_DRAFT_TO_PUBLISH.
//
// The reverse optimisation — holding published_version at its prior value when the settings are
// semantically equal, so that a rename does not make every blueprint interpolating the number plan an
// in-place update — was implemented and then removed deliberately. Do not restore it. It can only
// ever fire when something else about the policy changed, because that is the sole condition under
// which the framework marks the number unknown at all; and that is exactly the condition under which
// Update runs and reads the real number back. So every firing of the hold is a firing where the
// prediction can be contradicted, and it is contradicted whenever a version was published outside
// Terraform since the state was last refreshed — an admin publishing in the admin UI is a supported
// workflow, not an edge case. The contradiction surfaces as "Provider produced inconsistent result
// after apply", which tells the operator the provider is broken when it is not. An unknown is the
// truthful plan for a number this provider does not own: UseStateForUnknown is safe on id and
// created_at because those cannot change, and published_version can.
func planPublishOutcome(ctx context.Context, resp *resource.ModifyPlanResponse, plan, prior *policyModel) {
	if prior == nil {
		return
	}
	if prior.HasDraft.ValueBool() && plansToPublish(plan) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("has_draft"), types.BoolUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("published_version"), types.Int64Unknown())...)
	}
}

// plansToPublish reports whether this apply will attempt to publish. A value that is not yet known
// counts as publishing: the attribute defaults to true, so null means yes, and an unknown cannot be
// ruled out — treating either as no would suppress the republish a surviving draft needs.
func plansToPublish(plan *policyModel) bool {
	return !helpers.IsConfiguredValue(plan.Publish) || plan.Publish.ValueBool()
}

// catalogueChange records, for each of the two attributes checked against the tool catalogue,
// whether this plan is what wrote the value.
type catalogueChange struct {
	toolID        bool
	schemaVersion bool
}

// catalogueChanges reports which catalogue-checked attributes this plan changes. A create counts as
// changing both: there is no prior value the operator can be said to have left alone.
func catalogueChanges(plan, prior *policyModel) catalogueChange {
	if prior == nil {
		return catalogueChange{toolID: true, schemaVersion: true}
	}
	return catalogueChange{
		toolID:        !prior.ToolID.Equal(plan.ToolID),
		schemaVersion: !prior.SchemaVersion.Equal(plan.SchemaVersion),
	}
}

// checkCatalogue validates the tool and its schema version against the catalogue, and reports
// whether the pair is good enough to go on and check the settings against.
//
// A schema version behind the tool's current one is a warning rather than an error: the platform
// keeps serving it, and its own schemaDrift flag exists to say so. Announcing it at plan time is
// the point — schema_drift as a computed attribute only tells an operator after the fact.
//
// A catalogue that cannot be read reports the pair as good enough to go on with, so the plan
// proceeds, and says so once per plan rather than once per policy.
//
// A value the catalogue does not offer is reported at the severity addCatalogueFinding chooses, and
// either way stops the settings check: a schema cannot be fetched for a tool or version the
// catalogue does not list, so there is nothing left to validate against.
func (r *PolicyResource) checkCatalogue(ctx context.Context, diags *diag.Diagnostics, toolID, schemaVersion string, changed catalogueChange) bool {
	tool, found, err := r.schemas.Tool(ctx, toolID)
	if err != nil {
		r.noteValidationUnavailable(diags, "The AI tool catalogue could not be read", err)
		return true
	}
	if !found {
		addCatalogueFinding(diags, path.Root("tool_id"), changed.toolID,
			"Unknown AI tool",
			fmt.Sprintf("The platform offers no AI tool with the identifier %q. %s", toolID, r.knownTools(ctx)),
		)
		return false
	}
	if !slices.Contains(tool.SchemaVersions, schemaVersion) {
		addCatalogueFinding(diags, path.Root("schema_version"), changed.schemaVersion,
			"Unknown settings schema version",
			fmt.Sprintf("%s does not offer a settings schema version %q. Accepted versions: %s.",
				tool.DisplayName, schemaVersion, strings.Join(tool.SchemaVersions, ", ")),
		)
		return false
	}
	if schemaVersion != tool.SchemaVersion {
		diags.AddAttributeWarning(
			path.Root("schema_version"),
			"Settings schema version is behind the current one",
			fmt.Sprintf("%s now publishes schema version %q, and this policy is written against %q. The policy "+
				"keeps working and Jamf keeps serving the older schema, but settings added since are unavailable "+
				"to it. Moving forward means setting schema_version to %q and reconciling settings_json with it.",
				tool.DisplayName, tool.SchemaVersion, schemaVersion, tool.SchemaVersion),
		)
	}
	return true
}

// catalogueUnchangedNote is appended to a catalogue finding the plan reports as a warning, so the
// operator can tell a value they mistyped from one the world moved out from under.
const catalogueUnchangedNote = "This value is unchanged from the last apply, so the plan reports it rather than " +
	"failing: Jamf keeps serving an existing policy written against a value it has withdrawn from the catalogue, " +
	"and refuses the write itself if it does not. The settings were not checked against a published schema during " +
	"this plan."

// addCatalogueFinding reports a value the tool catalogue does not offer, as an error when this plan
// is what wrote it and a warning when it is not.
//
// The split matters because both findings are derived from live remote data, ModifyPlan runs for
// every policy in the plan including the ones it changes nothing about, and the platform keeps
// serving an older schema version for an existing policy — which is what schema_drift exists to
// signal and what the resource documentation promises. The served version lists are short: Claude
// Code offers two and published both within three months. If that is a cap rather than a coincidence,
// the next schema retires the older one, and a hard error would then fail every plan touching a
// policy pinned to it — including plans whose real changes are elsewhere in the workspace, with no
// version pin to fall back on. So a create, or a tool_id or schema_version the operator has just
// changed, keeps the error, because catching a typo at plan time is the whole point; a value they did
// not touch is reported and left to the apply, where helpers.go already translates TOOL_ID_UNKNOWN
// and SCHEMA_VERSION_UNKNOWN into the same guidance. This is the reasoning CLAUDE.md records for
// the Apple profile schemas — freshness is a scheduled concern, not a plan-time one — which the hard errors
// contradicted.
func addCatalogueFinding(diags *diag.Diagnostics, attribute path.Path, changed bool, summary, detail string) {
	if changed {
		diags.AddAttributeError(attribute, summary, detail)
		return
	}
	diags.AddAttributeWarning(attribute, summary, detail+" "+catalogueUnchangedNote)
}

// knownTools renders the catalogue for a diagnostic, or says nothing when it cannot be read.
func (r *PolicyResource) knownTools(ctx context.Context) string {
	tools, err := r.schemas.Tools(ctx)
	if err != nil || len(tools) == 0 {
		return "Read the available identifiers from the jamfplatform_ai_governance_tools data source."
	}
	rendered := make([]string, 0, len(tools))
	for _, tool := range tools {
		rendered = append(rendered, fmt.Sprintf("%s (%s)", tool.ID, tool.DisplayName))
	}
	return "Available: " + strings.Join(rendered, ", ") + "."
}

// noteValidationUnavailable warns that plan-time settings validation did not run, once per plan.
//
// Not failing the plan is deliberate — the platform validates the write, and an advisory endpoint
// the provider could not reach is no reason to block one. Passing silently is not: a role without
// the ai-policies:read privilege these reads need, or a transient 5xx, would otherwise leave a plan
// in which nothing was checked indistinguishable from a plan in which everything passed. The notice
// fires once per configured provider instance, so a configuration holding twenty policies reports
// it once — the rule impact alerts already follow for a tenant that cannot be read.
func (r *PolicyResource) noteValidationUnavailable(diags *diag.Diagnostics, cause string, err error) {
	if !r.schemas.NoticeOnce() {
		return
	}
	diags.AddWarning(
		"Settings validation unavailable",
		fmt.Sprintf("%s, so settings_json was not checked against the tool's published schema during this plan: %s\n\n"+
			"The check is advisory, so the plan is unaffected; the platform validates the settings when they are written. "+
			"No further notices will be shown for this plan.", cause, err),
	)
}

// checkSettings validates the settings body against the tool's published schema.
//
// The error and warning split is the vendor schema's, not a choice: a setting of the wrong type or
// outside its accepted values is a write Jamf refuses, while an undeclared key is accepted and
// stored by a tool whose schema allows extras — and then never applied. See
// aischemas.Problem.Advisory.
func (r *PolicyResource) checkSettings(ctx context.Context, diags *diag.Diagnostics, plan *policyModel, toolID, schemaVersion string) {
	if !helpers.IsConfiguredValue(plan.SettingsJSON) {
		return
	}
	decoded, err := decodeJSON(plan.SettingsJSON.ValueString())
	if err != nil {
		return
	}
	settings, ok := decoded.(map[string]any)
	if !ok {
		return
	}

	document, err := r.schemas.Document(ctx, toolID, schemaVersion)
	if err != nil {
		r.noteValidationUnavailable(diags, fmt.Sprintf("The %s settings schema %s could not be read", toolID, schemaVersion), err)
		return
	}

	for _, problem := range document.Validate(settings) {
		summary, detail := renderProblem(problem, schemaVersion)
		if problem.Advisory() {
			diags.AddAttributeWarning(path.Root("settings_json"), summary, detail)
			continue
		}
		diags.AddAttributeError(path.Root("settings_json"), summary, detail)
	}
}

// renderProblem turns a schema finding into a diagnostic summary and detail.
//
// The path is quoted into the detail rather than turned into a Terraform attribute path: the
// settings are one string attribute, so there is no traversable path to point at, and the JSON
// pointer is what the platform's own failures use.
func renderProblem(problem aischemas.Problem, schemaVersion string) (string, string) {
	location := "The settings"
	if problem.Path != "" {
		location = "The setting at " + problem.Path
	}

	switch problem.Kind {
	case aischemas.UnrecognisedKey:
		return "Setting is not declared for this schema version",
			location + ": " + problem.Detail + " Checked against schema version " + schemaVersion +
				". If this tool has published a newer schema version that declares it, set schema_version to that instead."
	case aischemas.MissingRequiredKey:
		return "Required setting is missing", location + ": " + problem.Detail
	default:
		return "Setting does not match the tool's schema",
			location + ": " + problem.Detail + " Checked against schema version " + schemaVersion + "."
	}
}
