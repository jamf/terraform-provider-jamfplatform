// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateMobileDevicePrestageV3
//   pro.GetMobileDevicePrestageV3
//   pro.UpdateMobileDevicePrestageV3                       (subject to upstream Jamf server PUT-500 bug — handled by isPutSerializerBug + diffPlanAgainstGet workaround)
//   pro.DeleteMobileDevicePrestageV3
//   pro.ResolveMobileDevicePrestageV3ByName               (data source name lookup)
//   pro.ResolveMobileDevicePrestageV3IDByName             (data source name lookup ID-only path)
//   pro.ListMobileDevicePrestagesV3                       (list resource + data source name lookup paging)
//   pro.GetMobileDevicePrestageScopeV2                    (folded scope_serial_numbers read)
//   pro.ReplaceMobileDevicePrestageScopeV2                (folded scope_serial_numbers write)
//
// Mobile-device prestages diverge from the computer sibling in several ways
// reflected here:
//   - No readiness wait: mobile does NOT gate scope / PUT on profileUuid
//     populating (spike §F13). Create = POST → GET → assign → applyScope →
//     GET scope → state. profile_uuid is a plain Computed echo.
//   - Three versionLock layers only (root + locationInformation +
//     purchasingInformation); no accountSettings (spike §5/§7).
//   - PUT-500-with-commit bug reproduces; the GET-diff verifier SKIPS the
//     §9.1 server-authoritative exclusion set (storage_quota_size_megabytes,
//     use_storage_quota_size, temporary_session_only, temporary_session_timeout)
//     — their post-PUT drift is expected, not a silent rollback.
//     anchor_certificates, names and default_prestage stay IN the check.
//   - Mobile scope errors: ALREADY_SCOPED (serial on another prestage),
//     DEVICE_DOES_NOT_EXIST_ON_TOKEN (serial not on the ADE token).
//   - DELETE → GET returns a clean 404 (no computer 400 INVALID_ID quirk).
//
// Status: current. Last reviewed 2026-08-07.

package mobile_device_prestage_enrollment

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// ModifyPlan marks the "server-arbitrated" fields Unknown when their planned
// value is one the Jamf Pro server may silently override, so Terraform Core's
// plan→apply consistency check accepts the server's resolved value instead of
// erroring with "Provider produced inconsistent result after apply". Runs on
// create and update (skipped on destroy).
//
//   - use_storage_quota_size / temporary_session_only: mutually exclusive
//     Shared-iPad storage modes — when both resolve `true` the server forces
//     one to `false` (§F9), so both are rendered Unknown and the server picks.
//
// default_prestage is NOT in this set — see the note in the body.
//
// The user's intent is preserved for the wire body by restoreServerArbitrated
// in Create/Update (a bare Unknown would serialise as `false` and corrupt the
// write). diffPlanAgainstGet additionally excludes those two fields (§9.1).
func (r *MobileDevicePrestageEnrollmentResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return // destroy
	}
	var plan MobileDevicePrestageEnrollmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Prior state (null on create).
	var state MobileDevicePrestageEnrollmentResourceModel
	if !req.State.Raw.IsNull() {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// default_prestage is deliberately NOT rendered Unknown here. It is not
	// server-arbitrated: Jamf Pro never silently keeps it false. Claiming a
	// default another PreStage already holds is rejected outright —
	// 400 ALREADY_DEFAULT "Another prestage is already the default prestage" —
	// on both create and update, with nothing written. Wire-probed 2026-08-07
	// against Jamf Pro 11.30; see the transition table on
	// diagnoseAlreadyDefault. Stamping Unknown over a known config value is also
	// illegal for an Optional+Computed attribute: Terraform Core rejects the plan
	// with "planned value cty.UnknownVal(cty.Bool) does not match config value
	// cty.True", which made `default_prestage = true` impossible to set at all.

	// use_storage_quota_size ⊻ temporary_session_only: the server stores at most
	// one as true (§F9), so a resolved both-true is always a transition into the
	// conflict (steady state never has both true). Render both Unknown and let
	// the server pick; the next plan resolves to the one-true/one-false state and
	// round-trips cleanly.
	if isKnownTrue(plan.UseStorageQuotaSize) && isKnownTrue(plan.TemporarySessionOnly) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("use_storage_quota_size"), types.BoolUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("temporary_session_only"), types.BoolUnknown())...)
	}
	// timezone validity is enforced by the shared validators.IANATimeZone() on
	// the schema attribute (Go's embedded IANA database) — no plan-time API
	// call needed.
}

// isKnownTrue reports whether a Bool is a concrete true (not null/unknown).
func isKnownTrue(b types.Bool) bool {
	return !b.IsNull() && !b.IsUnknown() && b.ValueBool()
}

// restoreServerArbitrated reverses the Unknowns ModifyPlan stamped onto the
// server-arbitrated fields, sourcing the user's true intent (config first, then
// prior state) so the POST/PUT body sends it. The final persisted value still
// comes from the post-write GET via assignGetToResource, which is what keeps
// the result consistent with the Unknown plan. On create pass a zero-value
// state.
//
// default_prestage is absent because ModifyPlan no longer stamps it Unknown —
// its planned value is already the user's intent.
func restoreServerArbitrated(plan *MobileDevicePrestageEnrollmentResourceModel, cfg, state MobileDevicePrestageEnrollmentResourceModel) {
	plan.UseStorageQuotaSize = resolveIntentBool(plan.UseStorageQuotaSize, cfg.UseStorageQuotaSize, state.UseStorageQuotaSize)
	plan.TemporarySessionOnly = resolveIntentBool(plan.TemporarySessionOnly, cfg.TemporarySessionOnly, state.TemporarySessionOnly)
}

// diagnoseAlreadyDefault replaces the raw ALREADY_DEFAULT API error with one
// that says which knob to turn. Jamf Pro allows exactly one default mobile
// device PreStage per tenant and will not reassign it; the current holder has to
// give it up first. Wire-probed 2026-08-07 against Jamf Pro 11.30:
//
//	create default_prestage = true, no current holder   → accepted
//	create default_prestage = true, another holds it    → 400 ALREADY_DEFAULT, nothing created
//	update default_prestage = true, no current holder   → accepted
//	update default_prestage = true, this one holds it   → accepted (no-op)
//	update default_prestage = true, another holds it    → 400 ALREADY_DEFAULT, nothing written
//	update default_prestage = false (release)           → accepted, tenant left with no default
//
// Returns true when it recognised and reported the error.
func diagnoseAlreadyDefault(diags *diag.Diagnostics, cfg MobileDevicePrestageEnrollmentResourceModel, err error) bool {
	if err == nil || !strings.Contains(err.Error(), "ALREADY_DEFAULT") {
		return false
	}
	detail := "Jamf Pro allows only one default mobile device PreStage per tenant and will not move the default automatically: " +
		"another PreStage already holds it, so this write was rejected and nothing changed. Set default_prestage = false on the " +
		"PreStage that currently holds it, then apply again."
	if isKnownTrue(cfg.DefaultPrestage) {
		detail += " When both PreStages are managed by Terraform, the release has to land before the claim — Terraform does not " +
			"order the two updates on its own, so either apply the release on its own first, or add a depends_on from this " +
			"resource to the one giving up the default."
	}
	diags.AddAttributeError(path.Root("default_prestage"), "another PreStage is already the tenant default", detail)
	return true
}

// warnNamingNotConfigured flags a record whose stored deviceNamingConfigured is
// false despite a names block that asks for naming. Because that wire field is
// unmodelled (see namesSchema), Terraform sees no drift and reports "No
// changes", so the admin UI would go on hiding the naming payload silently. Any
// apply that touches this resource rewrites the flag correctly; this warning is
// what tells the user one is needed. Reachable for records written by provider
// releases before the flag was sent at all, or edited outside Terraform.
func warnNamingNotConfigured(diags *diag.Diagnostics, state MobileDevicePrestageEnrollmentResourceModel, got *pro.GetMobileDevicePrestageV3) {
	if got.Names == nil || !namingIntentBesidesMode(state.Names) {
		return
	}
	if got.Names.DeviceNamingConfigured != nil && *got.Names.DeviceNamingConfigured {
		return
	}
	diags.AddAttributeWarning(
		path.Root("names"),
		"device naming not applied in Jamf Pro",
		"This PreStage has a names block, but Jamf Pro has device naming marked unconfigured, so the "+
			"\"Mobile device names\" payload is hidden in the admin UI and the names are not applied to enrolling devices. "+
			"Re-apply this resource to correct it — for example `terraform apply -replace=<address>`, or any change to the "+
			"resource. Terraform reports no drift on its own because Jamf Pro's naming-configured flag is not a "+
			"Terraform-managed attribute.",
	)
}

func resolveIntentBool(planV, cfgV, stateV types.Bool) types.Bool {
	if !planV.IsUnknown() {
		return planV
	}
	if !cfgV.IsNull() && !cfgV.IsUnknown() {
		return cfgV
	}
	if !stateV.IsNull() && !stateV.IsUnknown() {
		return stateV
	}
	return types.BoolValue(false)
}

// Create provisions a new mobile device prestage enrollment on Jamf Pro.
func (r *MobileDevicePrestageEnrollmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg MobileDevicePrestageEnrollmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// ModifyPlan rendered the server-arbitrated fields Unknown; restore the
	// user's intent for the POST body (no prior state on create).
	restoreServerArbitrated(&plan, cfg, MobileDevicePrestageEnrollmentResourceModel{})

	createTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultCreateTimeout, plan.Timeouts.Create)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	createCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	post, d := buildPostInput(createCtx, plan, cfg)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	postResp, err := r.client.CreateMobileDevicePrestageV3(createCtx, post)
	if err != nil {
		if !diagnoseAlreadyDefault(&resp.Diagnostics, cfg, err) {
			resp.Diagnostics.AddError("Error creating Jamf Pro mobile device prestage enrollment", helpers.APIErrorDetail(err))
		}
		return
	}
	if postResp == nil || postResp.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing prestage ID",
			"Jamf Pro returned success on the POST but did not include an ID; cannot persist state.",
		)
		return
	}
	id := postResp.ID
	plan.ID = types.StringValue(id)

	// Refresh via GET (server-canonical values).
	got, err := r.client.GetMobileDevicePrestageV3(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro mobile device prestage enrollment after create", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignGetToResource(createCtx, &plan, plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Apply scope if user supplied any serial numbers. Mobile does not gate
	// on profile_uuid readiness (§F13) — apply immediately.
	if !plan.ScopeSerialNumbers.IsNull() && !plan.ScopeSerialNumbers.IsUnknown() {
		if d := applyScope(createCtx, r.client, id, plan.ScopeSerialNumbers); d.HasError() {
			resp.Diagnostics.Append(d...)
			return
		}
	}
	scope, err := r.client.GetMobileDevicePrestageScopeV2(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading prestage scope after create", helpers.APIErrorDetail(err))
		return
	}
	plan.ScopeSerialNumbers = scopeSerialsToSet(scope)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, MobileDevicePrestageEnrollmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro mobile device prestage enrollment", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state from the live Jamf Pro record.
func (r *MobileDevicePrestageEnrollmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MobileDevicePrestageEnrollmentResourceModel
	isImport := req.State.Raw.IsNull()
	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this prestage without existing state or identity data; cannot determine which record to read.",
			)
			return
		}
		var identity MobileDevicePrestageEnrollmentIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError("Missing prestage ID", "Identity 'id' attribute is empty.")
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(timeoutAttributeTypes)
		seedImportNestedSentinels(&state)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		// site_id is always populated by Create from the GET; a null value
		// with the ID set means this is the first Read after
		// ImportStatePassthroughID. Seed sentinel nested-block pointers so the
		// state-builder rebuilds them.
		if state.SiteID.IsNull() {
			seedImportNestedSentinels(&state)
		}
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro mobile device prestage enrollment without ID.")
		return
	}

	got, err := r.client.GetMobileDevicePrestageV3(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro mobile device prestage enrollment not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro mobile device prestage enrollment", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignGetToResource(readCtx, &state, state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	warnNamingNotConfigured(&resp.Diagnostics, state, got)

	scope, err := r.client.GetMobileDevicePrestageScopeV2(readCtx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading prestage scope", helpers.APIErrorDetail(err))
		return
	}
	state.ScopeSerialNumbers = scopeSerialsToSet(scope)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, MobileDevicePrestageEnrollmentIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies plan against state. PUT is full-replace. The Jamf server-side
// response-serializer bug returns HTTP 500 with an empty errors[] body even on
// successful writes (§F4); this function catches that signature, GETs the
// record, and verifies via diffPlanAgainstGet that every plan field (minus the
// §9.1 server-authoritative exclusion set) round-tripped.
func (r *MobileDevicePrestageEnrollmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg MobileDevicePrestageEnrollmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// ModifyPlan rendered the server-arbitrated fields Unknown; restore the
	// user's intent (config, else prior state) for the PUT body.
	restoreServerArbitrated(&plan, cfg, state)

	updateTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultUpdateTimeout, plan.Timeouts.Update)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	id := plan.ID.ValueString()
	if id == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot update without an ID.")
		return
	}

	// Pre-PUT GET to source the three versionLocks + nested ids.
	preGet, err := r.client.GetMobileDevicePrestageV3(updateCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading prestage before update", helpers.APIErrorDetail(err))
		return
	}

	put, d := buildPutInput(updateCtx, plan, cfg)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	injectVersionLocks(put, preGet)

	_, putErr := r.client.UpdateMobileDevicePrestageV3(updateCtx, id, put)
	putHitServerBug := false
	if putErr != nil {
		if !isPutSerializerBug(putErr) {
			if !diagnoseAlreadyDefault(&resp.Diagnostics, cfg, putErr) {
				resp.Diagnostics.AddError("Error updating Jamf Pro mobile device prestage enrollment", helpers.APIErrorDetail(putErr))
			}
			return
		}
		putHitServerBug = true
	}

	// GET-diff to verify the write committed (handles both the
	// 500-with-commit and 500-with-silent-rollback flavours of §F4b).
	postGet, err := r.client.GetMobileDevicePrestageV3(updateCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading prestage after update", helpers.APIErrorDetail(err))
		return
	}
	if unchanged := diffPlanAgainstGet(updateCtx, plan, postGet); len(unchanged) > 0 {
		if putHitServerBug {
			resp.Diagnostics.AddError(
				"Jamf Pro mobile device prestage update did not commit",
				fmt.Sprintf("The Jamf Pro PUT endpoint returned HTTP 500 (a known Jamf Pro defect) and the verifying GET shows the write was silently rolled back. %s — most often this is a server-side validation failure on `anchor_certificates` (Jamf Pro validates PEM content) or a `names.prestage_device_names` element missing its server-assigned id. Fix the offending input and re-run `terraform apply`.", fmtUnchangedFields(unchanged)),
			)
			return
		}
		// PUT returned success but state still diverges — surface for
		// investigation.
		resp.Diagnostics.AddWarning(
			"Jamf Pro mobile device prestage update partially applied",
			fmt.Sprintf("PUT returned 200 but a subsequent GET shows the following fields did not round-trip: %s", fmtUnchangedFields(unchanged)),
		)
	} else if putHitServerBug {
		tflog.Warn(updateCtx, putWorkaroundWarning, map[string]any{"id": id})
	}

	resp.Diagnostics.Append(assignGetToResource(updateCtx, &plan, state, postGet)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Scope reconciliation: replace the entire serial-number set if the plan
	// differs from the prior state.
	scopeApplyDiags := diag.Diagnostics{}
	if !setsEqual(plan.ScopeSerialNumbers, state.ScopeSerialNumbers) {
		scopeApplyDiags = applyScope(updateCtx, r.client, id, plan.ScopeSerialNumbers)
	}

	// Always re-read scope so the persisted state matches the server even
	// when the scope write failed (keeps Terraform's consistency check from
	// masking our diagnostic).
	scope, err := r.client.GetMobileDevicePrestageScopeV2(updateCtx, id)
	if err != nil {
		resp.Diagnostics.Append(scopeApplyDiags...)
		resp.Diagnostics.AddError("Error reading prestage scope after update", helpers.APIErrorDetail(err))
		return
	}
	plan.ScopeSerialNumbers = scopeSerialsToSet(scope)

	if scopeApplyDiags.HasError() {
		resp.Diagnostics.Append(scopeApplyDiags...)
		resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, MobileDevicePrestageEnrollmentIdentityModel{ID: plan.ID})...)
		_ = resp.State.Set(ctx, &plan)
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, MobileDevicePrestageEnrollmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the prestage. Server cascade removes the scope assignments
// (§F7); GET-after-delete returns a clean 404.
func (r *MobileDevicePrestageEnrollmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MobileDevicePrestageEnrollmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultDeleteTimeout, state.Timeouts.Delete)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot delete without ID.")
		return
	}

	if err := r.client.DeleteMobileDevicePrestageV3(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro mobile device prestage enrollment already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro mobile device prestage enrollment", helpers.APIErrorDetail(err))
	}
}

// seedImportNestedSentinels initialises empty model pointers for every
// Optional-only typed-pointer nested block. Used by the import code path so
// the state-builder rebuilds each block from the GET response. No
// account_settings — mobile has none.
func seedImportNestedSentinels(state *MobileDevicePrestageEnrollmentResourceModel) {
	state.SkipSetupItems = &SkipSetupItemsModel{}
	state.Names = &NamesModel{}
	state.LocationInformation = &LocationInformationModel{}
	state.PurchasingInformation = &PurchasingInformationModel{}
}

// applyScope drives a ReplaceMobileDevicePrestageScopeV2 call. Always GETs
// first to source the scope versionLock (independent of the parent lock, §F7).
//
// Jamf Pro returns `400 ALREADY_SCOPED` when a serial is currently scoped to a
// different PreStage, and `400 DEVICE_DOES_NOT_EXIST_ON_TOKEN` when a serial is
// not present on the ADE token. The provider rewraps the former with workflow
// guidance.
func applyScope(ctx context.Context, client *pro.Client, prestageID string, serials types.Set) diag.Diagnostics {
	var diags diag.Diagnostics

	scope, err := client.GetMobileDevicePrestageScopeV2(ctx, prestageID)
	if err != nil {
		diags.AddError("Error reading prestage scope before replace", helpers.APIErrorDetail(err))
		return diags
	}

	planSlice, d := stringSetToSlice(ctx, serials)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	if planSlice == nil {
		planSlice = []string{}
	}
	body := &pro.PrestageScopeUpdate{SerialNumbers: planSlice, VersionLock: scope.VersionLock}

	if _, err := client.ReplaceMobileDevicePrestageScopeV2(ctx, prestageID, body); err != nil {
		summary := "Error replacing prestage scope"
		detail := err.Error()
		switch {
		case strings.Contains(detail, "ALREADY_SCOPED"):
			summary = "Jamf Pro PreStage scope conflict (serial already assigned)"
			detail += "\n\nJamf Pro enforces single-PreStage-per-serial: at least one serial in `scope_serial_numbers` is currently assigned to a different PreStage. Jamf Pro does not move serials between PreStages transparently: remove the serial from the holding PreStage first (in two separate applies, via `depends_on` ordering, or via the Jamf Pro admin UI) and re-run."
		case strings.Contains(detail, "DEVICE_DOES_NOT_EXIST_ON_TOKEN"):
			summary = "Jamf Pro PreStage scope error (serial not on ADE token)"
			detail += "\n\nAt least one serial in `scope_serial_numbers` does not exist on the Automated Device Enrollment token backing this PreStage (`device_enrollment_program_instance_id`). Confirm the device is assigned to this ADE/MDM server in Apple Business/School Manager and that the token has synced."
		}
		diags.AddError(summary, detail)
	}
	return diags
}

func setsEqual(a, b types.Set) bool {
	if a.IsNull() && b.IsNull() {
		return true
	}
	if a.IsUnknown() || b.IsUnknown() {
		return false
	}
	return a.Equal(b)
}
