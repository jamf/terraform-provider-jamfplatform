// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/aischemas"
)

// TestRenderProblem pins the wording each finding produces, and that every one names the schema
// version it was checked against — a finding an operator cannot attribute to a schema version is a
// finding they cannot act on.
func TestRenderProblem(t *testing.T) {
	cases := []struct {
		name           string
		problem        aischemas.Problem
		wantSummary    string
		wantMentions   []string
		mentionsSchema bool
	}{
		{
			name:           "wrong type names the setting",
			problem:        aischemas.Problem{Kind: aischemas.WrongType, Path: "/verbose", Detail: "expected a boolean, found a string."},
			wantSummary:    "Setting does not match the tool's schema",
			wantMentions:   []string{"The setting at /verbose", "expected a boolean"},
			mentionsSchema: true,
		},
		{
			name:           "undeclared key points at a newer schema version",
			problem:        aischemas.Problem{Kind: aischemas.UnrecognisedKey, Path: "/nope", Detail: `"nope" is not declared.`},
			wantSummary:    "Setting is not declared for this schema version",
			wantMentions:   []string{"The setting at /nope", "newer schema version"},
			mentionsSchema: true,
		},
		{
			name:         "missing required key",
			problem:      aischemas.Problem{Kind: aischemas.MissingRequiredKey, Path: "", Detail: `"tier" is required.`},
			wantSummary:  "Required setting is missing",
			wantMentions: []string{"The settings", `"tier" is required.`},
		},
		{
			name:           "a finding about the whole object falls back to naming the settings",
			problem:        aischemas.Problem{Kind: aischemas.UndeclaredKey, Path: "", Detail: "not accepted."},
			wantSummary:    "Setting does not match the tool's schema",
			wantMentions:   []string{"The settings:"},
			mentionsSchema: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary, detail := renderProblem(c.problem, "2026-08-14")
			if summary != c.wantSummary {
				t.Errorf("summary = %q, want %q", summary, c.wantSummary)
			}
			for _, mention := range c.wantMentions {
				if !strings.Contains(detail, mention) {
					t.Errorf("detail does not mention %q: %s", mention, detail)
				}
			}
			if c.mentionsSchema && !strings.Contains(detail, "2026-08-14") {
				t.Errorf("detail must name the schema version: %s", detail)
			}
		})
	}
}

// TestSortExpressionValidatorAcceptsOnlySupportedProperties pins the set the platform accepts:
// anything else is refused with VALIDATION_FAILED naming the sort parameter, mid-read.
func TestSortExpressionValidatorAcceptsOnlySupportedProperties(t *testing.T) {
	pattern := mustCompile("^(" + joinAlternatives(sortableProperties) + "):(asc|desc)$")

	for _, good := range []string{"name:asc", "name:desc", "createdAt:asc", "updatedAt:desc"} {
		if !pattern.MatchString(good) {
			t.Errorf("%q should be accepted", good)
		}
	}
	for _, bad := range []string{"name", "name:", ":asc", "bogus:asc", "name:sideways", "NAME:asc", "name:asc,extra"} {
		if pattern.MatchString(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

// stubService describes an AI Governance catalogue and schema surface for a plan-time validation
// test, including the two ways it can be unavailable.
//
// The seam is the HTTP boundary rather than an injected interface, matching
// security_cloud/dns_zone/crud_partial_state_test.go: aischemas.Cache holds a concrete SDK client,
// and an interface introduced only for a test would be a bigger change than the behaviour it pins.
// The stub is local rather than testhelpers.NewMockClient because testhelpers reaches the provider
// package under the acceptance build tag, and the provider registers this package — importing it
// from an in-package test makes that a cycle.
type stubService struct {
	tools      []aigovernance.ToolSummary
	schema     string
	failTools  bool
	failSchema bool
}

// testSchema declares one boolean setting and one nested object, both open to extra keys — the
// Claude Code shape, where an undeclared key is stored and never applied.
const testSchema = `{"type":"object","additionalProperties":true,"properties":` +
	`{"verbose":{"type":"boolean"},"permissions":{"type":"object","additionalProperties":true,` +
	`"properties":{"allow":{"type":"array"}}}}}`

// claudeCode is the catalogue entry the settings tests validate against, carrying one superseded
// schema version so the drift warning has something to fire on.
func claudeCode() aigovernance.ToolSummary {
	return aigovernance.ToolSummary{
		ID:             "com.anthropic.claudecode",
		DisplayName:    "Claude Code",
		SchemaVersion:  "2026-08-14",
		SchemaVersions: []string{"2026-08-14", "2026-05-19"},
	}
}

// resource builds a PolicyResource whose schema cache is backed by the stub.
func (s stubService) resource(t *testing.T) *PolicyResource {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/auth/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case strings.Contains(r.URL.Path, "/schemas/"):
			if s.failSchema {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "the schema read failed"})
				return
			}
			_ = json.NewEncoder(w).Encode(aigovernance.ToolSchemaResponse{
				ToolID:        claudeCode().ID,
				SchemaVersion: claudeCode().SchemaVersion,
				Schema:        json.RawMessage(s.schema),
			})
		case strings.HasSuffix(r.URL.Path, "/tools"):
			if s.failTools {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{"message": "the catalogue read failed"})
				return
			}
			_ = json.NewEncoder(w).Encode(aigovernance.ToolListResponse{Results: s.tools, TotalCount: len(s.tools)})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client := jamfplatform.NewClient(server.URL, "test-id", "test-secret",
		jamfplatform.WithRetryPolicy(0, 0, 0),
		jamfplatform.WithMinRequestInterval(0),
	)
	return &PolicyResource{client: aigovernance.New(client), schemas: aischemas.NewCache(client)}
}

// diagnosticPath returns the attribute path a diagnostic names, or the empty string for one that
// names none.
func diagnosticPath(diagnostic diag.Diagnostic) string {
	if withPath, ok := diagnostic.(diag.DiagnosticWithPath); ok {
		return withPath.Path().String()
	}
	return ""
}

// assertCounts fails unless the diagnostics hold exactly the given number of errors and warnings,
// reporting what they actually said when they do not.
func assertCounts(t *testing.T, diags diag.Diagnostics, wantErrors, wantWarnings int) {
	t.Helper()
	if got := len(diags.Errors()); got != wantErrors {
		t.Errorf("got %d errors, want %d: %v", got, wantErrors, diags.Errors())
	}
	if got := len(diags.Warnings()); got != wantWarnings {
		t.Errorf("got %d warnings, want %d: %v", got, wantWarnings, diags.Warnings())
	}
}

// changedCatalogue is the severity a create carries: both catalogue-checked values are ones the
// operator has just written, so an unrecognised one is an error. The tests below that call
// checkCatalogue directly are about what it recognises, not about who wrote the value.
var changedCatalogue = catalogueChange{toolID: true, schemaVersion: true}

// TestCheckSettingsUndeclaredKeyIsAWarning pins the classification the whole feature turns on: a key
// the schema does not declare is stored by the platform and never applied by the tool, which is
// advisory — the key may simply postdate the schema version — so it must not block a plan. Inverting
// the Advisory branch fails here.
func TestCheckSettingsUndeclaredKeyIsAWarning(t *testing.T) {
	r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema}.resource(t)
	plan := policyModel{SettingsJSON: newJSONObjectValue(`{"permissions":{"allow":[],"allowlist":true}}`)}

	var diags diag.Diagnostics
	r.checkSettings(context.Background(), &diags, &plan, claudeCode().ID, claudeCode().SchemaVersion)

	assertCounts(t, diags, 0, 1)
	if len(diags.Warnings()) == 0 {
		return
	}
	warning := diags.Warnings()[0]
	if got := diagnosticPath(warning); got != "settings_json" {
		t.Errorf("warning path = %q, want settings_json", got)
	}
	if !strings.Contains(warning.Detail(), "The setting at /permissions/allowlist") {
		t.Errorf("the warning must locate the key by JSON pointer: %s", warning.Detail())
	}
}

// TestCheckSettingsWrongTypeIsAnError pins the other half of the split: a value of the wrong type is
// a write Jamf refuses, so reporting it as advisory would let the apply fail instead of the plan.
func TestCheckSettingsWrongTypeIsAnError(t *testing.T) {
	r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema}.resource(t)
	plan := policyModel{SettingsJSON: newJSONObjectValue(`{"verbose":"yes"}`)}

	var diags diag.Diagnostics
	r.checkSettings(context.Background(), &diags, &plan, claudeCode().ID, claudeCode().SchemaVersion)

	assertCounts(t, diags, 1, 0)
	if len(diags.Errors()) == 0 {
		return
	}
	if got := diagnosticPath(diags.Errors()[0]); got != "settings_json" {
		t.Errorf("error path = %q, want settings_json", got)
	}
}

// TestCheckSettingsAcceptsAValidBody pins that a body matching the schema produces nothing at all,
// so neither half of the split fires on a correct configuration.
func TestCheckSettingsAcceptsAValidBody(t *testing.T) {
	r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema}.resource(t)
	plan := policyModel{SettingsJSON: newJSONObjectValue(`{"verbose":true,"permissions":{"allow":["Read"]}}`)}

	var diags diag.Diagnostics
	r.checkSettings(context.Background(), &diags, &plan, claudeCode().ID, claudeCode().SchemaVersion)

	assertCounts(t, diags, 0, 0)
}

// TestCheckCatalogueWarnsOnASupersededSchemaVersion pins the drift warning. The platform keeps
// serving the older schema, so this cannot be an error — but schema_drift as a computed attribute
// only reports it after the apply.
func TestCheckCatalogueWarnsOnASupersededSchemaVersion(t *testing.T) {
	r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema}.resource(t)

	var diags diag.Diagnostics
	if !r.checkCatalogue(context.Background(), &diags, claudeCode().ID, "2026-05-19", changedCatalogue) {
		t.Error("a superseded schema version must still be checked against")
	}

	assertCounts(t, diags, 0, 1)
	if len(diags.Warnings()) == 0 {
		return
	}
	if got := diagnosticPath(diags.Warnings()[0]); got != "schema_version" {
		t.Errorf("warning path = %q, want schema_version", got)
	}
}

// TestCheckCatalogueRejectsAnUnknownTool pins the one plan-time hard error the catalogue produces,
// and that it names the attribute the operator wrote rather than the wire field.
func TestCheckCatalogueRejectsAnUnknownTool(t *testing.T) {
	r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema}.resource(t)

	var diags diag.Diagnostics
	if r.checkCatalogue(context.Background(), &diags, "com.example.nope", "2026-08-14", changedCatalogue) {
		t.Error("an unknown tool must stop the settings check")
	}

	assertCounts(t, diags, 1, 0)
	if len(diags.Errors()) == 0 {
		return
	}
	failure := diags.Errors()[0]
	if got := diagnosticPath(failure); got != "tool_id" {
		t.Errorf("error path = %q, want tool_id", got)
	}
	if !strings.Contains(failure.Detail(), claudeCode().ID) {
		t.Errorf("the error must list the identifiers the catalogue does offer: %s", failure.Detail())
	}
}

// TestCheckCatalogueRejectsAnUnknownSchemaVersion pins the second hard error, which the platform
// would otherwise report mid-apply as SCHEMA_VERSION_UNKNOWN.
func TestCheckCatalogueRejectsAnUnknownSchemaVersion(t *testing.T) {
	r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema}.resource(t)

	var diags diag.Diagnostics
	if r.checkCatalogue(context.Background(), &diags, claudeCode().ID, "1999-01-01", changedCatalogue) {
		t.Error("an unknown schema version must stop the settings check")
	}

	assertCounts(t, diags, 1, 0)
	if len(diags.Errors()) == 0 {
		return
	}
	if got := diagnosticPath(diags.Errors()[0]); got != "schema_version" {
		t.Errorf("error path = %q, want schema_version", got)
	}
}

// TestUnreadableCatalogueWarnsOncePerPlan pins the difference between a plan where validation passed
// and a plan where it never ran. The plan must still succeed — the platform validates the write —
// but silence would leave the two indistinguishable, while the resource documentation tells the
// operator the settings are checked during plan.
func TestUnreadableCatalogueWarnsOncePerPlan(t *testing.T) {
	r := stubService{schema: testSchema, failTools: true}.resource(t)

	var diags diag.Diagnostics
	if !r.checkCatalogue(context.Background(), &diags, claudeCode().ID, claudeCode().SchemaVersion, changedCatalogue) {
		t.Error("a catalogue that cannot be read must not stop the plan")
	}

	assertCounts(t, diags, 0, 1)
	if len(diags.Warnings()) == 0 {
		return
	}
	if got := diags.Warnings()[0].Summary(); got != "Settings validation unavailable" {
		t.Errorf("warning summary = %q", got)
	}

	if !r.checkCatalogue(context.Background(), &diags, claudeCode().ID, claudeCode().SchemaVersion, changedCatalogue) {
		t.Error("a catalogue that cannot be read must not stop the plan")
	}
	assertCounts(t, diags, 0, 1)
}

// TestUnreadableSchemaWarnsOnce pins the same notice on the other fetch. A role holding
// ai-policies:write without ai-policies:read reaches exactly this path.
func TestUnreadableSchemaWarnsOnce(t *testing.T) {
	r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema, failSchema: true}.resource(t)
	plan := policyModel{SettingsJSON: newJSONObjectValue(`{"verbose":"yes"}`)}

	var diags diag.Diagnostics
	r.checkSettings(context.Background(), &diags, &plan, claudeCode().ID, claudeCode().SchemaVersion)
	assertCounts(t, diags, 0, 1)

	r.checkSettings(context.Background(), &diags, &plan, claudeCode().ID, claudeCode().SchemaVersion)
	assertCounts(t, diags, 0, 1)
}

// policyRawPlan builds a plan with the three attributes plan-time validation reads set, and
// everything else null, the way Terraform sends a create for a configuration that sets only those.
func policyRawPlan(ctx context.Context, policySchema resourceschema.Schema, toolID, schemaVersion, settings string) tftypes.Value {
	object := policySchema.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	values["tool_id"] = tftypes.NewValue(tftypes.String, toolID)
	values["schema_version"] = tftypes.NewValue(tftypes.String, schemaVersion)
	values["settings_json"] = tftypes.NewValue(tftypes.String, settings)

	return tftypes.NewValue(object, values)
}

// TestModifyPlanClassifiesFindings pins the wiring from a plan to the diagnostics an operator sees:
// an advisory finding must reach them as a warning the plan survives, and everything else as an
// error that stops it. The acceptance suite cannot cover the warning half — terraform-plugin-testing
// has no warning assertion — so inverting the Advisory branch is caught only here.
func TestModifyPlanClassifiesFindings(t *testing.T) {
	cases := []struct {
		name          string
		toolID        string
		schemaVersion string
		settings      string
		wantErrors    int
		wantWarnings  int
	}{
		{
			name:          "undeclared key warns",
			toolID:        claudeCode().ID,
			schemaVersion: claudeCode().SchemaVersion,
			settings:      `{"permissions":{"allow":[],"allowlist":true}}`,
			wantWarnings:  1,
		},
		{
			name:          "wrong type errors",
			toolID:        claudeCode().ID,
			schemaVersion: claudeCode().SchemaVersion,
			settings:      `{"verbose":"yes"}`,
			wantErrors:    1,
		},
		{
			name:          "an unknown tool stops before the settings are read",
			toolID:        "com.example.nope",
			schemaVersion: claudeCode().SchemaVersion,
			settings:      `{"verbose":"yes"}`,
			wantErrors:    1,
		},
		{
			name:          "a valid body reports nothing",
			toolID:        claudeCode().ID,
			schemaVersion: claudeCode().SchemaVersion,
			settings:      `{"verbose":true}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema}.resource(t)

			var schemaResp resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

			var resp resource.ModifyPlanResponse
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{
				Plan: tfsdk.Plan{
					Schema: schemaResp.Schema,
					Raw:    policyRawPlan(ctx, schemaResp.Schema, c.toolID, c.schemaVersion, c.settings),
				},
			}, &resp)

			assertCounts(t, resp.Diagnostics, c.wantErrors, c.wantWarnings)
		})
	}
}

// publishFixture describes one side of a plan-prediction case — a prior state or a proposed plan —
// naming only the fields the cases vary. An empty string takes the stub fixture's value, so a case
// reads as the difference it is testing.
type publishFixture struct {
	name            string
	toolID          string
	schemaVersion   string
	settings        string
	hasDraft        bool
	publishDisabled bool
	publishUnknown  bool
	noVersionYet    bool
}

// orDefault returns the value, or the fallback when it is empty.
func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// publishValue renders the `publish` attribute the fixture asks for.
func (f publishFixture) publishValue() tftypes.Value {
	if f.publishUnknown {
		return tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue)
	}
	return tftypes.NewValue(tftypes.Bool, !f.publishDisabled)
}

// publishedVersionValue renders the `published_version` a prior state holds: 1 for a policy that has
// been published, null for one staged as a draft and never published.
func (f publishFixture) publishedVersionValue() tftypes.Value {
	if f.noVersionYet {
		return tftypes.NewValue(tftypes.Number, nil)
	}
	return tftypes.NewValue(tftypes.Number, 1)
}

// stateRaw renders the fixture as the state an applied policy holds, with every value known.
func (f publishFixture) stateRaw(ctx context.Context, policySchema resourceschema.Schema) tftypes.Value {
	return policyRaw(ctx, policySchema, map[string]tftypes.Value{
		"name":              tftypes.NewValue(tftypes.String, orDefault(f.name, "unit-test-policy")),
		"tool_id":           tftypes.NewValue(tftypes.String, orDefault(f.toolID, stubToolID)),
		"schema_version":    tftypes.NewValue(tftypes.String, orDefault(f.schemaVersion, stubSchemaVersion)),
		"settings_json":     tftypes.NewValue(tftypes.String, orDefault(f.settings, stubSettings)),
		"publish":           f.publishValue(),
		"id":                tftypes.NewValue(tftypes.String, stubPolicyID),
		"published_version": f.publishedVersionValue(),
		"has_draft":         tftypes.NewValue(tftypes.Bool, f.hasDraft),
		"schema_drift":      tftypes.NewValue(tftypes.Bool, false),
		"created_at":        tftypes.NewValue(tftypes.String, stubCreatedAt),
		"updated_at":        tftypes.NewValue(tftypes.String, stubCreatedAt),
	})
}

// planRaw renders the fixture as the plan the framework hands ModifyPlan for an update: id and
// created_at carried through by UseStateForUnknown, and every other Computed attribute Unknown,
// which is what MarkComputedNilsAsUnknown produces for any plan that changes something.
func (f publishFixture) planRaw(ctx context.Context, policySchema resourceschema.Schema) tftypes.Value {
	return policyRaw(ctx, policySchema, map[string]tftypes.Value{
		"name":              tftypes.NewValue(tftypes.String, orDefault(f.name, "unit-test-policy")),
		"tool_id":           tftypes.NewValue(tftypes.String, orDefault(f.toolID, stubToolID)),
		"schema_version":    tftypes.NewValue(tftypes.String, orDefault(f.schemaVersion, stubSchemaVersion)),
		"settings_json":     tftypes.NewValue(tftypes.String, orDefault(f.settings, stubSettings)),
		"publish":           f.publishValue(),
		"id":                tftypes.NewValue(tftypes.String, stubPolicyID),
		"created_at":        tftypes.NewValue(tftypes.String, stubCreatedAt),
		"published_version": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"has_draft":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"schema_drift":      tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"updated_at":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})
}

// noPriorState renders the prior state Terraform sends for a create: a wholly null object.
func noPriorState(ctx context.Context, policySchema resourceschema.Schema) tftypes.Value {
	return tftypes.NewValue(policySchema.Type().TerraformType(ctx), nil)
}

// runModifyPlan drives ModifyPlan the way fwserver does — resp.Plan seeded with the proposed plan,
// req.State carrying the prior one — and hands back the response so the planned values and the
// diagnostics can both be asserted.
func runModifyPlan(ctx context.Context, t *testing.T, r *PolicyResource, policySchema resourceschema.Schema, priorRaw, planRaw tftypes.Value) *resource.ModifyPlanResponse {
	t.Helper()
	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: policySchema, Raw: planRaw}}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: planRaw},
		Config: tfsdk.Config{Schema: policySchema, Raw: planRaw},
		State:  tfsdk.State{Schema: policySchema, Raw: priorRaw},
	}, resp)
	return resp
}

// plannedPublishedVersion reads published_version back off the planned object.
func plannedPublishedVersion(ctx context.Context, t *testing.T, resp *resource.ModifyPlanResponse) types.Int64 {
	t.Helper()
	var planned types.Int64
	if diags := resp.Plan.GetAttribute(ctx, path.Root("published_version"), &planned); diags.HasError() {
		t.Fatalf("reading the planned published_version: %v", diags)
	}
	return planned
}

// plannedHasDraft reads has_draft back off the planned object.
func plannedHasDraft(ctx context.Context, t *testing.T, resp *resource.ModifyPlanResponse) types.Bool {
	t.Helper()
	var planned types.Bool
	if diags := resp.Plan.GetAttribute(ctx, path.Root("has_draft"), &planned); diags.HasError() {
		t.Fatalf("reading the planned has_draft: %v", diags)
	}
	return planned
}

// TestModifyPlanRepublishesASurvivingDraft pins the retry the publish-failure diagnostics promise.
//
// Every case here is the plan Terraform makes when the configuration has not changed — the shape a
// failed publish leaves behind, and the one the whole finding turns on. The framework marks a
// Computed attribute unknown only when the proposed plan already differs from prior state, so in
// this shape has_draft and published_version arrive carrying their prior values: without ModifyPlan
// making them unknown the plan is empty, Update is never called, and blueprints deliver the previous
// version's settings for good.
//
// The publish = false case is the other half. Publication is then the operator's, and forcing a diff
// would publish a draft they staged deliberately.
func TestModifyPlanRepublishesASurvivingDraft(t *testing.T) {
	cases := []struct {
		name         string
		prior        publishFixture
		wantUnknowns bool
	}{
		{
			name:         "a draft that survived a failed publish is republished",
			prior:        publishFixture{hasDraft: true},
			wantUnknowns: true,
		},
		{
			name:         "a draft on a never-published policy is republished",
			prior:        publishFixture{hasDraft: true, noVersionYet: true},
			wantUnknowns: true,
		},
		{
			name:  "a draft staged with publishing disabled is left alone",
			prior: publishFixture{hasDraft: true, publishDisabled: true},
		},
		{
			name:  "a published policy with no draft plans no republish",
			prior: publishFixture{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema}.resource(t)
			policySchema, _ := policySchemas(ctx, t)

			unchanged := c.prior.stateRaw(ctx, policySchema)
			resp := runModifyPlan(ctx, t, r, policySchema, unchanged, unchanged)

			if resp.Diagnostics.HasError() {
				t.Fatalf("planning must not fail: %v", resp.Diagnostics.Errors())
			}
			hasDraft := plannedHasDraft(ctx, t, resp)
			publishedVersion := plannedPublishedVersion(ctx, t, resp)

			if c.wantUnknowns {
				if !hasDraft.IsUnknown() {
					t.Errorf("planned has_draft = %s, want unknown — without a diff Update is never called and the draft is never published", hasDraft)
				}
				if !publishedVersion.IsUnknown() {
					t.Errorf("planned published_version = %s, want unknown — the retry mints a version", publishedVersion)
				}
				return
			}
			if hasDraft.IsUnknown() {
				t.Error("planned has_draft is unknown, so an apply the operator did not ask for would publish their draft")
			}
			if got := hasDraft.ValueBool(); got != c.prior.hasDraft {
				t.Errorf("planned has_draft = %t, want the prior %t", got, c.prior.hasDraft)
			}
			if publishedVersion.IsUnknown() {
				t.Error("planned published_version is unknown, so a blueprint pinning it plans a write for a version that will not move")
			}
			if got := publishedVersion.ValueInt64(); got != 1 {
				t.Errorf("planned published_version = %d, want the prior 1", got)
			}
		})
	}
}

// TestModifyPlanRepublishesWhenPublishIsNotYetKnown pins the same republish for a `publish` value
// interpolated from something the plan cannot resolve yet. Reading an unknown as "not publishing"
// would leave the draft unpublished on a plan that goes on to publish it.
func TestModifyPlanRepublishesWhenPublishIsNotYetKnown(t *testing.T) {
	ctx := context.Background()
	r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema}.resource(t)
	policySchema, _ := policySchemas(ctx, t)

	resp := runModifyPlan(ctx, t, r, policySchema,
		publishFixture{hasDraft: true}.stateRaw(ctx, policySchema),
		publishFixture{hasDraft: true, publishUnknown: true}.stateRaw(ctx, policySchema))

	if resp.Diagnostics.HasError() {
		t.Fatalf("planning must not fail: %v", resp.Diagnostics.Errors())
	}
	if planned := plannedHasDraft(ctx, t, resp); !planned.IsUnknown() {
		t.Errorf("planned has_draft = %s, want unknown", planned)
	}
	if planned := plannedPublishedVersion(ctx, t, resp); !planned.IsUnknown() {
		t.Errorf("planned published_version = %s, want unknown", planned)
	}
}

// TestPlansToPublish pins the reading of `publish` the republish depends on. Null is the schema
// default, true, and an unknown cannot be ruled out — treating either as no would leave a surviving
// draft unpublished for a plan that will in fact publish it.
func TestPlansToPublish(t *testing.T) {
	cases := []struct {
		name  string
		value types.Bool
		want  bool
	}{
		{name: "explicitly enabled", value: types.BoolValue(true), want: true},
		{name: "explicitly disabled", value: types.BoolValue(false)},
		{name: "null takes the schema default", value: types.BoolNull(), want: true},
		{name: "unknown cannot be ruled out", value: types.BoolUnknown(), want: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := plansToPublish(&policyModel{Publish: c.value}); got != c.want {
				t.Errorf("plansToPublish(%s) = %t, want %t", c.value, got, c.want)
			}
		})
	}
}

// TestModifyPlanCreateLeavesBothAttributesUnknown pins the create case both predicates must leave
// alone: there is no prior state to hold a value from or to find a draft in, and version 1 is
// genuinely unknowable until the publish returns.
func TestModifyPlanCreateLeavesBothAttributesUnknown(t *testing.T) {
	ctx := context.Background()
	r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema}.resource(t)
	policySchema, _ := policySchemas(ctx, t)

	resp := runModifyPlan(ctx, t, r, policySchema,
		noPriorState(ctx, policySchema), createPlanRaw(ctx, policySchema))

	if resp.Diagnostics.HasError() {
		t.Fatalf("planning a create must not fail: %v", resp.Diagnostics.Errors())
	}
	if planned := plannedPublishedVersion(ctx, t, resp); !planned.IsUnknown() {
		t.Errorf("planned published_version = %s, want unknown on a create", planned)
	}
	if planned := plannedHasDraft(ctx, t, resp); !planned.IsUnknown() {
		t.Errorf("planned has_draft = %s, want unknown on a create", planned)
	}
}

// TestModifyPlanCatalogueSeverityFollowsWhoWroteTheValue pins the whole point of (20): a tool or
// schema version the catalogue does not list fails the plan when the configuration has just changed
// it — that is a typo, and catching it before an apply is the point — and only warns when it is
// unchanged. The served version lists are short and Jamf may withdraw one it still serves policies
// for, and a hard error would then fail plans whose real changes are elsewhere in the workspace.
func TestModifyPlanCatalogueSeverityFollowsWhoWroteTheValue(t *testing.T) {
	const withdrawn = "1999-01-01"
	const absentTool = "com.example.nope"

	cases := []struct {
		name         string
		create       bool
		prior        publishFixture
		plan         publishFixture
		wantErrors   int
		wantWarnings int
		wantPath     string
	}{
		{
			name:         "an unchanged withdrawn schema version warns",
			prior:        publishFixture{schemaVersion: withdrawn},
			plan:         publishFixture{schemaVersion: withdrawn},
			wantWarnings: 1,
			wantPath:     "schema_version",
		},
		{
			name:       "a schema version the operator just changed errors",
			prior:      publishFixture{},
			plan:       publishFixture{schemaVersion: withdrawn},
			wantErrors: 1,
			wantPath:   "schema_version",
		},
		{
			name:       "a create with an unknown schema version errors",
			create:     true,
			plan:       publishFixture{schemaVersion: withdrawn},
			wantErrors: 1,
			wantPath:   "schema_version",
		},
		{
			name:         "an unchanged absent tool warns",
			prior:        publishFixture{toolID: absentTool},
			plan:         publishFixture{toolID: absentTool},
			wantWarnings: 1,
			wantPath:     "tool_id",
		},
		{
			name:       "a tool the operator just changed errors",
			prior:      publishFixture{},
			plan:       publishFixture{toolID: absentTool},
			wantErrors: 1,
			wantPath:   "tool_id",
		},
		{
			name:       "a create with an unknown tool errors",
			create:     true,
			plan:       publishFixture{toolID: absentTool},
			wantErrors: 1,
			wantPath:   "tool_id",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			r := stubService{tools: []aigovernance.ToolSummary{claudeCode()}, schema: testSchema}.resource(t)
			policySchema, _ := policySchemas(ctx, t)

			prior := c.prior.stateRaw(ctx, policySchema)
			if c.create {
				prior = noPriorState(ctx, policySchema)
			}
			resp := runModifyPlan(ctx, t, r, policySchema, prior, c.plan.planRaw(ctx, policySchema))

			assertCounts(t, resp.Diagnostics, c.wantErrors, c.wantWarnings)

			reported := append(resp.Diagnostics.Errors(), resp.Diagnostics.Warnings()...)
			if len(reported) == 0 {
				return
			}
			if got := diagnosticPath(reported[0]); got != c.wantPath {
				t.Errorf("diagnostic path = %q, want %q", got, c.wantPath)
			}
			if c.wantWarnings == 1 && !strings.Contains(reported[0].Detail(), "unchanged from the last apply") {
				t.Errorf("the warning must say why it is not an error: %s", reported[0].Detail())
			}
		})
	}
}
