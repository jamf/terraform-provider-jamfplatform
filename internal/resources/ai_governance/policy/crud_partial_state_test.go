// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/aischemas"
)

// Fixture values the stub server reports and the plans below are built from.
const (
	stubPolicyID      = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	stubToolID        = "com.anthropic.claudecode"
	stubSchemaVersion = "2026-08-14"
	stubSettings      = `{"verbose":true}`
	stubCreatedAt     = "2026-08-30T10:00:00Z"
	stubUpdatedAt     = "2026-08-30T10:05:00Z"
)

// policyStub is a stand-in for the AI Governance policies API, with the per-endpoint outcome each
// test needs. A zero value succeeds everywhere.
//
// The seam is the HTTP boundary rather than an injected interface, matching
// security_cloud/dns_zone's crud_partial_state_test.go: PolicyResource holds a concrete
// *aigovernance.Client, and an interface introduced only for a test would be a bigger change than
// the behaviour it pins. The stub is local rather than testhelpers.NewMockClient because
// testhelpers reaches the provider package under the acceptance build tag, and this package is one
// of the resources the provider registers — importing it from an in-package test makes that a cycle.
type policyStub struct {
	// name is the policy name the read reports, so an update can be seen to have landed.
	name string
	// status is the policy status the read reports. Empty means ACTIVE.
	status string
	// getStatus is the HTTP status the read answers with. Zero means 200 with the policy body.
	getStatus int
	// updateStatus is the HTTP status the update answers with. Zero means 204.
	updateStatus int
	// updateCode is the machine-readable code carried by a failing update.
	updateCode string
	// version is the optimistic-lock counter the read reports. Nil reports none, which is what a
	// policy the platform created before the counter shipped answers with.
	version *int64
	// ifMatch records the precondition the last update carried, as a sentHeader, so a test can see
	// what was sent and whether a header was sent at all.
	ifMatch atomic.Value
	// publishStatus is the HTTP status the publish answers with. Zero means 201.
	publishStatus int
	// publishCode is the machine-readable code carried by a failing publish.
	publishCode string
	// deleteStatus is the HTTP status the archive answers with. Zero means 204.
	deleteStatus int
	// publishCalls counts the publish requests the stub received.
	publishCalls atomic.Int64
	// draftsPublished counts the publishes the stub accepted, and drives what the read reports: a
	// policy whose draft has been published holds no draft and carries that version number.
	draftsPublished atomic.Int64
}

// clients starts the stub and returns the base client together with the AI Governance client built
// on it, so a test can point both the resource's API client and its schema cache at the same server.
// Retries are disabled so a deliberate 5xx is answered once rather than waited on.
func (s *policyStub) clients(t *testing.T) (*jamfplatform.Client, *aigovernance.Client) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(server.Close)
	base := jamfplatform.NewClient(server.URL, "test-id", "test-secret",
		jamfplatform.WithRetryPolicy(0, 0, 0),
		jamfplatform.WithMinRequestInterval(0),
	)
	return base, aigovernance.New(base)
}

// client starts the stub and returns an AI Governance client pointed at it.
func (s *policyStub) client(t *testing.T) *aigovernance.Client {
	t.Helper()
	_, api := s.clients(t)
	return api
}

// serve routes the four policy endpoints the resource reaches, the two catalogue reads plan-time
// validation makes, and the token endpoint every request authenticates against.
func (s *policyStub) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/auth/token":
		writeJSONBody(w, http.StatusOK, map[string]any{
			"access_token": "test-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/publish"):
		s.publishCalls.Add(1)
		if s.publishStatus != 0 {
			writeAPIError(w, s.publishStatus, s.publishCode, "the publish failed")
			return
		}
		version := s.draftsPublished.Add(1)
		writeJSONBody(w, http.StatusCreated, map[string]any{"id": "version-1", "versionNumber": version})
	case r.Method == http.MethodPost:
		writeJSONBody(w, http.StatusCreated, map[string]any{"id": stubPolicyID})
	case r.Method == http.MethodPatch:
		values, present := r.Header["If-Match"]
		s.ifMatch.Store(sentHeader{value: strings.Join(values, ", "), present: present})
		if s.updateStatus != 0 {
			writeAPIError(w, s.updateStatus, s.updateCode, "the update failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete:
		if s.deleteStatus != 0 {
			writeAPIError(w, s.deleteStatus, "", "the archive failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(r.URL.Path, "/tools"):
		writeJSONBody(w, http.StatusOK, map[string]any{
			"results": []map[string]any{{
				"id":             stubToolID,
				"displayName":    "Claude Code",
				"schemaVersion":  stubSchemaVersion,
				"schemaVersions": []string{stubSchemaVersion},
			}},
			"totalCount": 1,
		})
	case strings.Contains(r.URL.Path, "/schemas/"):
		writeJSONBody(w, http.StatusOK, map[string]any{
			"toolId":        stubToolID,
			"schemaVersion": stubSchemaVersion,
			"schema":        json.RawMessage(testSchema),
		})
	default:
		if s.getStatus != 0 {
			writeAPIError(w, s.getStatus, "", "the read-back failed")
			return
		}
		writeJSONBody(w, http.StatusOK, s.detail())
	}
}

// detail is the policy body the read answers with, defaulting the two fields a test varies.
func (s *policyStub) detail() map[string]any {
	name := s.name
	if name == "" {
		name = "unit-test-policy"
	}
	status := s.status
	if status == "" {
		status = aigovernance.PolicyDetailStatusActive
	}
	published := s.draftsPublished.Load()
	body := map[string]any{
		"id":            stubPolicyID,
		"name":          name,
		"toolId":        stubToolID,
		"schemaVersion": stubSchemaVersion,
		"settings":      json.RawMessage(stubSettings),
		"status":        status,
		"hasDraft":      published == 0,
		"schemaDrift":   false,
		"createdAt":     stubCreatedAt,
		"updatedAt":     stubUpdatedAt,
		"createdBy":     "actor",
		"updatedBy":     "actor",
	}
	if published > 0 {
		body["currentVersionNumber"] = published
	}
	if s.version != nil {
		body["version"] = *s.version
	}
	return body
}

// sentHeader is one request header as the stub saw it. Presence is recorded separately from the
// value because Header.Get answers the empty string for a header that was absent and for one that
// arrived empty, and the two are different requests: an empty If-Match is a precondition the
// platform validates, while an absent one is an unconditional update.
type sentHeader struct {
	value   string
	present bool
}

// sentIfMatch reports the precondition the last update carried, and whether a PATCH arrived at all.
func (s *policyStub) sentIfMatch() (sentHeader, bool) {
	header, ok := s.ifMatch.Load().(sentHeader)
	return header, ok
}

// writeJSONBody writes a JSON response body with the given status.
func writeJSONBody(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeAPIError writes the platform's structured error shape, so the SDK parses it into the
// machine-readable details the resource's error handling reads.
func writeAPIError(w http.ResponseWriter, status int, code, description string) {
	writeJSONBody(w, status, map[string]any{
		"httpStatus": status,
		"traceId":    "trace-1",
		"errors": []map[string]any{{
			"code":        code,
			"field":       "",
			"description": description,
		}},
	})
}

// policySchemas returns the resource and identity schemas the request and response objects below
// are built against.
func policySchemas(ctx context.Context, t *testing.T) (resourceschema.Schema, tfsdk.ResourceIdentity) {
	t.Helper()
	r := &PolicyResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)
	return schemaResp.Schema, tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema}
}

// policyRaw builds a policy object with every attribute null, then applies the given values. Null
// is the right starting point for both a plan and a state: an attribute a test does not name is one
// the operator left out.
func policyRaw(ctx context.Context, policySchema resourceschema.Schema, values map[string]tftypes.Value) tftypes.Value {
	object := policySchema.Type().TerraformType(ctx).(tftypes.Object)
	all := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		all[name] = tftypes.NewValue(attributeType, nil)
	}
	maps.Copy(all, values)
	return tftypes.NewValue(object, all)
}

// createPlanRaw builds a create plan: the required attributes set, `publish` carrying the schema
// default Terraform applies before Create runs, and every other Computed attribute Unknown as
// Terraform sends it.
func createPlanRaw(ctx context.Context, policySchema resourceschema.Schema) tftypes.Value {
	return policyRaw(ctx, policySchema, map[string]tftypes.Value{
		"name":              tftypes.NewValue(tftypes.String, "unit-test-policy"),
		"tool_id":           tftypes.NewValue(tftypes.String, stubToolID),
		"schema_version":    tftypes.NewValue(tftypes.String, stubSchemaVersion),
		"settings_json":     tftypes.NewValue(tftypes.String, stubSettings),
		"publish":           tftypes.NewValue(tftypes.Bool, true),
		"id":                tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"published_version": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"has_draft":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"schema_drift":      tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"created_at":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"updated_at":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})
}

// priorStateRaw builds the state an applied policy holds, with every value known.
func priorStateRaw(ctx context.Context, policySchema resourceschema.Schema) tftypes.Value {
	return policyRaw(ctx, policySchema, map[string]tftypes.Value{
		"name":              tftypes.NewValue(tftypes.String, "unit-test-policy"),
		"tool_id":           tftypes.NewValue(tftypes.String, stubToolID),
		"schema_version":    tftypes.NewValue(tftypes.String, stubSchemaVersion),
		"settings_json":     tftypes.NewValue(tftypes.String, stubSettings),
		"publish":           tftypes.NewValue(tftypes.Bool, true),
		"id":                tftypes.NewValue(tftypes.String, stubPolicyID),
		"published_version": tftypes.NewValue(tftypes.Number, 1),
		"has_draft":         tftypes.NewValue(tftypes.Bool, false),
		"schema_drift":      tftypes.NewValue(tftypes.Bool, false),
		"created_at":        tftypes.NewValue(tftypes.String, stubCreatedAt),
		"updated_at":        tftypes.NewValue(tftypes.String, stubCreatedAt),
	})
}

// updatePlanRaw builds an update plan that renames the policy: `id` and `created_at` carry the
// prior state through UseStateForUnknown, and the values the write changes arrive Unknown.
func updatePlanRaw(ctx context.Context, policySchema resourceschema.Schema, name string) tftypes.Value {
	return policyRaw(ctx, policySchema, map[string]tftypes.Value{
		"name":              tftypes.NewValue(tftypes.String, name),
		"tool_id":           tftypes.NewValue(tftypes.String, stubToolID),
		"schema_version":    tftypes.NewValue(tftypes.String, stubSchemaVersion),
		"settings_json":     tftypes.NewValue(tftypes.String, stubSettings),
		"publish":           tftypes.NewValue(tftypes.Bool, true),
		"id":                tftypes.NewValue(tftypes.String, stubPolicyID),
		"created_at":        tftypes.NewValue(tftypes.String, stubCreatedAt),
		"published_version": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"has_draft":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"schema_drift":      tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"updated_at":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})
}

// TestCreate_PublishFailureStillRecordsThePolicy pins the state a create writes when the policy is
// created and the publish that follows fails. The policy exists on the tenant by then, and because
// policy names are not unique a create that records nothing leaves it invisible and mints a second
// one on the next apply.
//
// The fully-known assertion is not incidental: Terraform answers an unknown value in the state a
// failed apply returns with an "invalid result object after apply" error of its own, which would
// bury the diagnostic below.
func TestCreate_PublishFailureStillRecordsThePolicy(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{publishStatus: http.StatusInternalServerError}
	r := &PolicyResource{client: stub.client(t)}

	policySchema, identity := policySchemas(ctx, t)
	raw := createPlanRaw(ctx, policySchema)
	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: policySchema},
		Identity: &identity,
	}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: raw},
		Config: tfsdk.Config{Schema: policySchema, Raw: raw},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed publish must still be reported as an error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the created policy must be recorded in state, or the apply orphans it on the tenant")
	}
	if !resp.State.Raw.IsFullyKnown() {
		t.Errorf("partial state must be wholly known, got %s", resp.State.Raw)
	}

	var state policyModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.ID.ValueString(); got != stubPolicyID {
		t.Errorf("id = %q, want %q", got, stubPolicyID)
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{stubPolicyID, "unpublished draft", "do not create it again", "next apply retries"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not mention %q", detail, want)
		}
	}
}

// TestCreate_FailedReadBackReportsTheNewID pins the diagnostic a create writes when the policy is
// created and the read-back fails, which is the other order the two calls can fail in.
//
// Only the diagnostic is asserted. Unlike security_cloud/dns_zone, this resource records nothing in
// state on a failed read-back — the five Computed attributes are still Unknown at that point, so
// committing the plan would need each one nulling first — and whether to do that is a separate call
// from the publish ordering this file exists to pin. What the test does hold is that the created
// policy is findable: the ID reaches the diagnostic, so the orphan can be reconciled by hand.
func TestCreate_FailedReadBackReportsTheNewID(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{getStatus: http.StatusInternalServerError}
	r := &PolicyResource{client: stub.client(t)}

	policySchema, identity := policySchemas(ctx, t)
	raw := createPlanRaw(ctx, policySchema)
	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: policySchema},
		Identity: &identity,
	}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: raw},
		Config: tfsdk.Config{Schema: policySchema, Raw: raw},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed read-back must be reported as an error")
	}
	var reported bool
	for _, err := range resp.Diagnostics.Errors() {
		if strings.Contains(err.Detail(), stubPolicyID) {
			reported = true
		}
	}
	if !reported {
		t.Errorf("no diagnostic names the created policy %q, so the operator cannot find it: %v", stubPolicyID, resp.Diagnostics.Errors())
	}
}

// TestUpdate_PublishFailureStillRecordsTheDraft pins the state an update writes when the draft is
// saved and the publish that follows fails. Returning before the state write would leave state at
// the values it held before the update, hiding both the rename and the draft the platform now holds.
func TestUpdate_PublishFailureStillRecordsTheDraft(t *testing.T) {
	ctx := context.Background()
	const renamed = "unit-test-policy-renamed"
	stub := &policyStub{name: renamed, publishStatus: http.StatusInternalServerError}
	r := &PolicyResource{client: stub.client(t)}

	policySchema, identity := policySchemas(ctx, t)
	plan := updatePlanRaw(ctx, policySchema, renamed)
	resp := resource.UpdateResponse{
		State:    tfsdk.State{Schema: policySchema, Raw: priorStateRaw(ctx, policySchema)},
		Identity: &identity,
	}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: plan},
		Config: tfsdk.Config{Schema: policySchema, Raw: plan},
		State:  tfsdk.State{Schema: policySchema, Raw: priorStateRaw(ctx, policySchema)},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed publish must still be reported as an error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("a live policy must not be removed from state because its publish failed")
	}
	if !resp.State.Raw.IsFullyKnown() {
		t.Errorf("committed state must be wholly known, got %s", resp.State.Raw)
	}

	var state policyModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.Name.ValueString(); got != renamed {
		t.Errorf("name = %q, want %q — the saved draft was not recorded", got, renamed)
	}
	if !state.HasDraft.ValueBool() {
		t.Error("has_draft = false, want true — state must record the draft the publish failed to publish")
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "previously published version") {
		t.Errorf("detail %q does not say what blueprints keep deploying", detail)
	}
	if !strings.Contains(detail, "next apply retries") {
		t.Errorf("detail %q does not promise the retry planPublishOutcome makes real", detail)
	}
}

// TestPublishIfNeeded_NoDraftToPublishIsSuccess pins the 409 the platform answers when nothing is
// staged as success. It diffs the settings itself, so an apply that changed only the name — or one
// that enabled publish on an already-published policy — lands here, and treating it as a failure
// would fail an apply that did exactly what was asked.
func TestPublishIfNeeded_NoDraftToPublishIsSuccess(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{publishStatus: http.StatusConflict, publishCode: codeNoDraftToPublish}
	r := &PolicyResource{client: stub.client(t)}

	if err := r.publishIfNeeded(ctx, stubPolicyID, true); err != nil {
		t.Fatalf("publishIfNeeded returned %v, want nil for a 409 saying there is no draft", err)
	}
	if got := stub.publishCalls.Load(); got != 1 {
		t.Errorf("publish requests = %d, want 1", got)
	}
}

// TestPublishIfNeeded_OtherFailuresAreReported pins that only the no-draft conflict is absorbed: any
// other failure has to reach the caller, or a policy that was never published is reported as applied.
func TestPublishIfNeeded_OtherFailuresAreReported(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{publishStatus: http.StatusInternalServerError}
	r := &PolicyResource{client: stub.client(t)}

	if err := r.publishIfNeeded(ctx, stubPolicyID, true); err == nil {
		t.Fatal("publishIfNeeded returned nil for a 500, so a failed publish would pass as applied")
	}
}

// TestPublishIfNeeded_DisabledMakesNoRequest pins `publish = false` as staging a draft rather than
// publishing one, since the request itself is what mints a version.
func TestPublishIfNeeded_DisabledMakesNoRequest(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{}
	r := &PolicyResource{client: stub.client(t)}

	if err := r.publishIfNeeded(ctx, stubPolicyID, false); err != nil {
		t.Fatalf("publishIfNeeded returned %v, want nil when publishing is disabled", err)
	}
	if got := stub.publishCalls.Load(); got != 0 {
		t.Errorf("publish requests = %d, want 0 — publishing is disabled", got)
	}
}

// TestRead_ArchivedPolicyIsAbsent pins an archived policy as gone. Archiving is a soft delete the
// API renders as a 404 today, but the spec declares ARCHIVED as a status the read may report, and a
// service that starts honouring it would otherwise leave every plan reporting no changes for a
// policy delivered to no device.
func TestRead_ArchivedPolicyIsAbsent(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{status: aigovernance.PolicyDetailStatusArchived}
	r := &PolicyResource{client: stub.client(t)}

	policySchema, identity := policySchemas(ctx, t)
	prior := priorStateRaw(ctx, policySchema)
	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}
	r.Read(ctx, resource.ReadRequest{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("an archived policy is absence, not an error: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Error("an archived policy must be removed from state, or the next plan reports no changes for a policy on no device")
	}
	if len(resp.Diagnostics.Warnings()) == 0 {
		t.Fatal("removing a resource from state silently gives the operator nothing to act on")
	}
	if detail := resp.Diagnostics.Warnings()[0].Detail(); !strings.Contains(detail, stubPolicyID) {
		t.Errorf("warning %q does not name the archived policy", detail)
	}
}

// TestRead_ActivePolicyIsRefreshed is the counterpart the archived case needs: without it, a status
// check that matched every value would look identical to a correct one.
func TestRead_ActivePolicyIsRefreshed(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{name: "unit-test-policy-renamed"}
	r := &PolicyResource{client: stub.client(t)}

	policySchema, identity := policySchemas(ctx, t)
	prior := priorStateRaw(ctx, policySchema)
	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}
	r.Read(ctx, resource.ReadRequest{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("reading an active policy: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("an active policy must stay in state")
	}

	var state policyModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.Name.ValueString(); got != "unit-test-policy-renamed" {
		t.Errorf("name = %q, want the value the platform reported", got)
	}
}

// TestDelete_WarnsThatBlueprintsMayStillReferenceThePolicy pins the one remedy this resource has
// for a destroy that breaks a blueprint. Jamf accepts the archive even when a deployed blueprint
// pins one of the policy's published versions, and the blueprint is then unwritable in full, so a
// destroy that reports clean success and says nothing leaves the operator to discover that on their
// next unrelated change to it.
func TestDelete_WarnsThatBlueprintsMayStillReferenceThePolicy(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{}
	r := &PolicyResource{client: stub.client(t)}

	policySchema, _ := policySchemas(ctx, t)
	prior := priorStateRaw(ctx, policySchema)
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: policySchema, Raw: prior}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: policySchema, Raw: prior}}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("archiving a policy: %v", resp.Diagnostics.Errors())
	}
	if len(resp.Diagnostics.Warnings()) != 1 {
		t.Fatalf("warnings = %d, want 1 naming the blueprint consequence", len(resp.Diagnostics.Warnings()))
	}
	detail := resp.Diagnostics.Warnings()[0].Detail()
	for _, want := range []string{stubPolicyID, "POLICY_ARCHIVED"} {
		if !strings.Contains(detail, want) {
			t.Errorf("warning %q does not mention %q", detail, want)
		}
	}
}

// TestDelete_FailedArchiveDoesNotWarn pins the other half of that warning: a policy the platform
// refused to archive is still there, so describing it as archived would be false.
func TestDelete_FailedArchiveDoesNotWarn(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{deleteStatus: http.StatusInternalServerError}
	r := &PolicyResource{client: stub.client(t)}

	policySchema, _ := policySchemas(ctx, t)
	prior := priorStateRaw(ctx, policySchema)
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: policySchema, Raw: prior}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: policySchema, Raw: prior}}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed archive must be reported as an error")
	}
	if got := len(resp.Diagnostics.Warnings()); got != 0 {
		t.Errorf("warnings = %d, want 0 — the policy was not archived: %v", got, resp.Diagnostics.Warnings())
	}
}

// TestPublishFailureIsRetriedByTheNextPlan chains the three calls the retry runs through, because
// each one alone looks correct while the loop between them is broken. It is the unit-seam form of
// what an acceptance test cannot reach: terraform-plugin-testing has no way to fail a publish on a
// live tenant, and the failure this pins is exactly a publish that fails.
//
// Create leaves the policy in state with has_draft true. The plan Terraform then makes carries no
// configuration change at all, so the only thing that can produce a diff is ModifyPlan — and without
// one Update is never called and blueprints deliver the previous version's settings for good. The
// retry publishes the draft as it stands, which is what the diagnostic promises.
func TestPublishFailureIsRetriedByTheNextPlan(t *testing.T) {
	ctx := context.Background()
	policySchema, identity := policySchemas(ctx, t)

	failing := &policyStub{publishStatus: http.StatusInternalServerError}
	creating := &PolicyResource{client: failing.client(t)}
	createRaw := createPlanRaw(ctx, policySchema)
	createResp := resource.CreateResponse{State: tfsdk.State{Schema: policySchema}, Identity: &identity}
	creating.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: createRaw},
		Config: tfsdk.Config{Schema: policySchema, Raw: createRaw},
	}, &createResp)

	if !createResp.Diagnostics.HasError() {
		t.Fatal("a failed publish must be reported as an error")
	}
	var created policyModel
	if diags := createResp.State.Get(ctx, &created); diags.HasError() {
		t.Fatalf("reading back the created state: %v", diags)
	}
	if !created.HasDraft.ValueBool() {
		t.Fatal("state must record the draft the failed publish left behind, or there is nothing to retry from")
	}
	if !created.PublishedVersion.IsNull() {
		t.Fatalf("published_version = %s, want null — nothing was published", created.PublishedVersion)
	}

	retrying := &policyStub{}
	base, api := retrying.clients(t)
	r := &PolicyResource{client: api, schemas: aischemas.NewCache(base)}

	unchanged := createResp.State.Raw
	planResp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: policySchema, Raw: unchanged}}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: unchanged},
		Config: tfsdk.Config{Schema: policySchema, Raw: unchanged},
		State:  tfsdk.State{Schema: policySchema, Raw: unchanged},
	}, planResp)

	if planResp.Diagnostics.HasError() {
		t.Fatalf("planning the retry: %v", planResp.Diagnostics.Errors())
	}
	if planResp.Plan.Raw.Equal(unchanged) {
		t.Fatal("the plan equals prior state, so Terraform reports no changes and the draft is never published")
	}

	updateResp := resource.UpdateResponse{
		State:    tfsdk.State{Schema: policySchema, Raw: unchanged},
		Identity: &identity,
	}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: planResp.Plan.Raw},
		Config: tfsdk.Config{Schema: policySchema, Raw: unchanged},
		State:  tfsdk.State{Schema: policySchema, Raw: unchanged},
	}, &updateResp)

	if updateResp.Diagnostics.HasError() {
		t.Fatalf("the retried apply: %v", updateResp.Diagnostics.Errors())
	}
	if got := retrying.publishCalls.Load(); got != 1 {
		t.Errorf("publish requests = %d, want 1 — the retry has to reach the publish route", got)
	}
	if !updateResp.State.Raw.IsFullyKnown() {
		t.Fatalf("committed state must be wholly known, got %s", updateResp.State.Raw)
	}

	var final policyModel
	if diags := updateResp.State.Get(ctx, &final); diags.HasError() {
		t.Fatalf("reading back the retried state: %v", diags)
	}
	if final.HasDraft.ValueBool() {
		t.Error("has_draft = true after the retry published the draft")
	}
	if got := final.PublishedVersion.ValueInt64(); got != 1 {
		t.Errorf("published_version = %d, want 1 — the retry mints the first version", got)
	}
}

// TestRead_IdentityImportKeepsTimeoutsTyped pins the typing of the model this Read builds when it
// has no prior state to decode. An `identity = { id = ... }` import block — the form
// `terraform query -generate-config-out` writes, so the form every generated config is imported
// with — leaves req.State.Raw null, and readIdentity constructs the model from the identity alone.
// A zero timeouts.Value carries an object type with no attributes, and the framework then refuses
// the state this Read writes with a "Value Conversion Error" naming timeouts.Type, taking the whole
// plan down with it.
func TestRead_IdentityImportKeepsTimeoutsTyped(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{name: "unit-test-policy"}
	r := &PolicyResource{client: stub.client(t)}

	policySchema, identity := policySchemas(ctx, t)
	identityType := identity.Schema.Type().TerraformType(ctx).(tftypes.Object)
	identity.Raw = tftypes.NewValue(identityType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, stubPolicyID),
	})

	nullPriorState := tfsdk.State{
		Schema: policySchema,
		Raw:    tftypes.NewValue(policySchema.Type().TerraformType(ctx), nil),
	}
	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: policySchema},
		Identity: &identity,
	}
	r.Read(ctx, resource.ReadRequest{State: nullPriorState, Identity: &identity}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("reading a policy addressed only by identity: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the policy must be written to state")
	}

	var state policyModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := len(state.Timeouts.AttributeTypes(ctx)); got != 4 {
		t.Errorf("timeouts carries %d attribute types, want the schema's 4", got)
	}
}

// recordingPrivateState is a stand-in for the framework's private-state surface. The concrete type
// is internal to terraform-plugin-framework, so a response built in a unit test carries none and
// there is no exported constructor to supply one — which is why the lock-token helpers take the two
// narrow interfaces this satisfies.
type recordingPrivateState struct {
	values map[string][]byte
}

// newRecordingPrivateState returns an empty private-state stand-in.
func newRecordingPrivateState() *recordingPrivateState {
	return &recordingPrivateState{values: map[string][]byte{}}
}

// GetKey returns a stashed value, or nil when the key was never written.
func (p *recordingPrivateState) GetKey(_ context.Context, key string) ([]byte, diag.Diagnostics) {
	return p.values[key], nil
}

// SetKey stashes a value, deleting the key for an empty one the way the framework does.
func (p *recordingPrivateState) SetKey(_ context.Context, key string, value []byte) diag.Diagnostics {
	if len(value) == 0 {
		delete(p.values, key)
		return nil
	}
	p.values[key] = value
	return nil
}

// TestHydrate_RecordsThePolicysVersionForTheNextUpdate pins where the precondition comes from. The
// value has to be read back *after* the write, because the write itself advances the counter — a
// token captured any earlier would make the following apply conflict with nothing but itself.
func TestHydrate_RecordsThePolicysVersionForTheNextUpdate(t *testing.T) {
	ctx := context.Background()
	version := int64(7)
	stub := &policyStub{version: &version}
	r := &PolicyResource{client: stub.client(t)}
	private := newRecordingPrivateState()

	var model policyModel
	var diags diag.Diagnostics
	if !r.hydrate(ctx, &model, stubPolicyID, private, &diags) {
		t.Fatalf("hydrating a live policy: %v", diags)
	}
	if diags.HasError() {
		t.Fatalf("hydrating a live policy: %v", diags)
	}

	token, tokenDiags := readLockToken(ctx, private)
	if tokenDiags.HasError() {
		t.Fatalf("reading the recorded token back: %v", tokenDiags)
	}
	if token != `"7"` {
		t.Errorf("recorded precondition = %q, want %q — the next update would not be conditional", token, `"7"`)
	}
}

// TestHydrate_ClearsTheTokenWhenThePolicyReportsNoVersion pins the legacy case. A policy the
// platform created before the counter shipped reports null and sends no ETag, so there is no
// precondition to make — and leaving a previously recorded one in place would send a value the
// platform can only refuse.
func TestHydrate_ClearsTheTokenWhenThePolicyReportsNoVersion(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{}
	r := &PolicyResource{client: stub.client(t)}
	private := newRecordingPrivateState()
	private.values[privateKeyVersion] = []byte(`"\"3\""`)

	var model policyModel
	var diags diag.Diagnostics
	if !r.hydrate(ctx, &model, stubPolicyID, private, &diags) {
		t.Fatalf("hydrating a live policy: %v", diags)
	}

	token, tokenDiags := readLockToken(ctx, private)
	if tokenDiags.HasError() {
		t.Fatalf("reading the recorded token back: %v", tokenDiags)
	}
	if token != "" {
		t.Errorf("recorded precondition = %q, want none — a stale token must never be sent", token)
	}
}

// TestUpdate_VersionConflictIsReportedAndChangesNothing pins the whole point of the precondition:
// the platform refused the write, so the resource must say what happened and leave state exactly as
// it found it. Removing the resource or committing the plan would both describe a policy that was
// never written.
func TestUpdate_VersionConflictIsReportedAndChangesNothing(t *testing.T) {
	ctx := context.Background()
	const renamed = "unit-test-policy-renamed"
	stub := &policyStub{
		name:         renamed,
		updateStatus: http.StatusConflict,
		updateCode:   codePolicyVersionConflict,
	}
	r := &PolicyResource{client: stub.client(t)}

	policySchema, identity := policySchemas(ctx, t)
	plan := updatePlanRaw(ctx, policySchema, renamed)
	prior := priorStateRaw(ctx, policySchema)
	resp := resource.UpdateResponse{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: plan},
		Config: tfsdk.Config{Schema: policySchema, Raw: plan},
		State:  tfsdk.State{Schema: policySchema, Raw: prior},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a refused update must be reported as an error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("a policy that still exists must not be removed from state")
	}
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("state changed on a refused update:\n got %s\nwant %s", resp.State.Raw, prior)
	}
	if stub.publishCalls.Load() != 0 {
		t.Error("a refused update must not go on to publish")
	}

	summary := resp.Diagnostics.Errors()[0].Summary()
	if !strings.Contains(summary, "not at the version Terraform last read") {
		t.Errorf("summary %q does not say what was observed", summary)
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "changed nothing") {
		t.Errorf("detail %q does not say the apply left the policy alone", detail)
	}
	if !strings.Contains(detail, "settings_json") {
		t.Errorf("detail %q does not say why a stale write is worth refusing", detail)
	}
	for _, want := range []string{"Make a new plan", "saved plan file", "two Terraform resources"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not mention %q", detail, want)
		}
	}
}

// TestUpdatePolicy_SendsThePreconditionAsAHeader pins the SDK boundary rather than this package's
// own logic, because the failure it guards against is silent. A header parameter the generator
// emitted as a query key would leave every update unconditional with nothing anywhere reporting it —
// the platform ignores the unknown key and answers 204 — so the value is asserted where it can be
// seen, on the wire.
func TestUpdatePolicy_SendsThePreconditionAsAHeader(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{}
	api := stub.client(t)

	request := &aigovernance.UpdatePolicyRequest{
		SchemaVersion: stubSchemaVersion,
		Settings:      json.RawMessage(stubSettings),
	}
	if err := api.UpdatePolicy(ctx, stubPolicyID, request, `"7"`); err != nil {
		t.Fatalf("updating with a precondition: %v", err)
	}
	got, patched := stub.sentIfMatch()
	if !patched {
		t.Fatal("the stub saw no update at all")
	}
	if !got.present {
		t.Fatal("the update carried no If-Match header")
	}
	if got.value != `"7"` {
		t.Errorf("If-Match = %q, want %q", got.value, `"7"`)
	}
}

// TestUpdatePolicy_OmitsThePreconditionWhenThereIsNone pins the other half: the empty string must
// send no header at all rather than an empty one, because the platform validates the header it is
// given and an empty value is not a version. The assertion is on presence rather than on the value,
// since a header that arrived empty and one that never arrived are the same value to Header.Get —
// which is what would let the SDK's own emptiness guard be deleted unnoticed.
func TestUpdatePolicy_OmitsThePreconditionWhenThereIsNone(t *testing.T) {
	ctx := context.Background()
	stub := &policyStub{}
	api := stub.client(t)

	request := &aigovernance.UpdatePolicyRequest{
		SchemaVersion: stubSchemaVersion,
		Settings:      json.RawMessage(stubSettings),
	}
	if err := api.UpdatePolicy(ctx, stubPolicyID, request, ""); err != nil {
		t.Fatalf("updating without a precondition: %v", err)
	}
	got, patched := stub.sentIfMatch()
	if !patched {
		t.Fatal("the stub saw no update at all")
	}
	if got.present {
		t.Errorf("If-Match arrived with value %q, want no header at all", got.value)
	}
}

// refusingPrivateState is a private-state stand-in whose writes fail, standing in for the
// framework's own error diagnostics — a reserved key, a value that is not valid JSON, or an
// uninitialized surface.
type refusingPrivateState struct {
	*recordingPrivateState
}

// SetKey refuses the write the way the framework does, with an error diagnostic.
func (p refusingPrivateState) SetKey(_ context.Context, _ string, _ []byte) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.AddError("Private state refused the write", "the stand-in refuses every write")
	return diags
}

// TestUpdate_SendsThePreconditionRecordedInPrivateState pins the wiring the whole feature rests on:
// the version a previous read recorded reaches the update as its precondition. Nothing else can pin
// it, because the framework populates the private state on a real operation only — so a unit test
// supplies the stand-in PolicyResource.privateOverride exists for, and asserts the header on the
// wire rather than the argument, which is where a precondition that never left is visible.
func TestUpdate_SendsThePreconditionRecordedInPrivateState(t *testing.T) {
	ctx := context.Background()
	const renamed = "unit-test-policy-renamed"
	version := int64(7)
	stub := &policyStub{name: renamed, version: &version}
	private := newRecordingPrivateState()
	private.values[privateKeyVersion] = []byte(`"\"7\""`)
	r := &PolicyResource{client: stub.client(t), privateOverride: private}

	policySchema, identity := policySchemas(ctx, t)
	plan := updatePlanRaw(ctx, policySchema, renamed)
	prior := priorStateRaw(ctx, policySchema)
	resp := resource.UpdateResponse{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: plan},
		Config: tfsdk.Config{Schema: policySchema, Raw: plan},
		State:  tfsdk.State{Schema: policySchema, Raw: prior},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("updating a policy whose version is recorded: %v", resp.Diagnostics.Errors())
	}
	got, patched := stub.sentIfMatch()
	if !patched {
		t.Fatal("the stub saw no update at all")
	}
	if !got.present {
		t.Fatal("the update carried no If-Match header, so it was unconditional")
	}
	if got.value != `"7"` {
		t.Errorf("If-Match = %q, want %q", got.value, `"7"`)
	}
	for _, warning := range resp.Diagnostics.Warnings() {
		if strings.Contains(warning.Summary(), "without the concurrent-edit check") {
			t.Errorf("a conditional update reported itself as unprotected: %q", warning.Detail())
		}
	}
}

// TestUpdate_WarnsWhenNoVersionIsRecorded pins the other outcome, which is the majority case on any
// estate with history: a policy the platform created before it kept the counter reports none, the
// update goes out unconditional, and the platform answers 204. Nothing else records that, so the
// warning is the only place an operator sees which of their policies the check does not cover.
func TestUpdate_WarnsWhenNoVersionIsRecorded(t *testing.T) {
	ctx := context.Background()
	const renamed = "unit-test-policy-renamed"
	stub := &policyStub{name: renamed}
	r := &PolicyResource{client: stub.client(t), privateOverride: newRecordingPrivateState()}

	policySchema, identity := policySchemas(ctx, t)
	plan := updatePlanRaw(ctx, policySchema, renamed)
	prior := priorStateRaw(ctx, policySchema)
	resp := resource.UpdateResponse{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: plan},
		Config: tfsdk.Config{Schema: policySchema, Raw: plan},
		State:  tfsdk.State{Schema: policySchema, Raw: prior},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a policy with no recorded version must still update: %v", resp.Diagnostics.Errors())
	}
	if got, patched := stub.sentIfMatch(); !patched || got.present {
		t.Errorf("If-Match presence = %v, want no header on an unconditional update", patched && got.present)
	}
	if !warningContains(resp.Diagnostics, "without the concurrent-edit check", stubPolicyID) {
		t.Errorf("an unconditional update was not reported: %v", resp.Diagnostics.Warnings())
	}
}

// TestUpdate_UndecodableVersionWarnsAndStillUpdates pins the strength of the milder token failure. A
// value that is not valid JSON carries no version at all, so there is nothing to compare and the
// update degrades to the unconditional write the resource performed before the key existed —
// blocking the apply would cost the operator their change for a piece of bookkeeping.
func TestUpdate_UndecodableVersionWarnsAndStillUpdates(t *testing.T) {
	ctx := context.Background()
	const renamed = "unit-test-policy-renamed"
	stub := &policyStub{name: renamed}
	private := newRecordingPrivateState()
	private.values[privateKeyVersion] = []byte(`{`)
	r := &PolicyResource{client: stub.client(t), privateOverride: private}

	policySchema, identity := policySchemas(ctx, t)
	plan := updatePlanRaw(ctx, policySchema, renamed)
	prior := priorStateRaw(ctx, policySchema)
	resp := resource.UpdateResponse{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: plan},
		Config: tfsdk.Config{Schema: policySchema, Raw: plan},
		State:  tfsdk.State{Schema: policySchema, Raw: prior},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("an undecodable version must not block the apply: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the updated policy must stay in state")
	}
	if !warningContains(resp.Diagnostics, "recorded version", "cannot decode") {
		t.Errorf("the undecodable version was not reported: %v", resp.Diagnostics.Warnings())
	}
}

// TestUpdate_RefusesAVersionThatWouldUpdateUnconditionally pins the one private-state value that
// fails open. Jamf reads `*` as "apply regardless of the version", so a value spelled that way would
// turn the check off and still report a successful apply, where every other unexpected spelling is
// refused on the wire.
func TestUpdate_RefusesAVersionThatWouldUpdateUnconditionally(t *testing.T) {
	ctx := context.Background()
	const renamed = "unit-test-policy-renamed"
	stub := &policyStub{name: renamed}
	private := newRecordingPrivateState()
	private.values[privateKeyVersion] = []byte(`"*"`)
	r := &PolicyResource{client: stub.client(t), privateOverride: private}

	policySchema, identity := policySchemas(ctx, t)
	plan := updatePlanRaw(ctx, policySchema, renamed)
	prior := priorStateRaw(ctx, policySchema)
	resp := resource.UpdateResponse{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: plan},
		Config: tfsdk.Config{Schema: policySchema, Raw: plan},
		State:  tfsdk.State{Schema: policySchema, Raw: prior},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a value that disables the check must be refused, not sent")
	}
	if _, patched := stub.sentIfMatch(); patched {
		t.Error("the update was sent anyway, so the check was disabled and the apply reported success")
	}
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("state changed on an update that was never sent:\n got %s\nwant %s", resp.State.Raw, prior)
	}
}

// TestReadLockToken_AcceptsTheEntityTagFormsAndRefusesTheRest pins the vocabulary either side of
// that error, since a pattern that matched everything and one that matched nothing both leave the
// `*` case passing.
func TestReadLockToken_AcceptsTheEntityTagFormsAndRefusesTheRest(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		stored  string
		want    string
		refused bool
	}{
		{name: "strong entity tag, the form this provider writes", stored: `"\"7\""`, want: `"7"`},
		{name: "bare counter, the form state written before the quoting holds", stored: `"7"`, want: "7"},
		{name: "weak validator", stored: `"W/\"7\""`, want: `W/"7"`},
		{name: "wildcard", stored: `"*"`, refused: true},
		{name: "not a counter", stored: `"garbage"`, refused: true},
		{name: "empty", stored: `""`, refused: true},
		{name: "comma-separated list", stored: `"\"7\", \"8\""`, refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			private := newRecordingPrivateState()
			private.values[privateKeyVersion] = []byte(tc.stored)
			token, diags := readLockToken(ctx, private)
			if tc.refused {
				if !diags.HasError() {
					t.Fatalf("%s was accepted, and would be sent as a precondition", tc.stored)
				}
				if token != "" {
					t.Errorf("token = %q, want none on a refused value", token)
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("%s was refused: %v", tc.stored, diags.Errors())
			}
			if token != tc.want {
				t.Errorf("token = %q, want %q", token, tc.want)
			}
		})
	}
}

// TestRead_RecordsThePolicysVersionForTheNextUpdate pins the other end of the round trip. A refresh
// is the only thing that records a version for a policy Terraform is not writing, so a read that
// stopped recording one would leave every subsequent update unconditional with nothing failing.
func TestRead_RecordsThePolicysVersionForTheNextUpdate(t *testing.T) {
	ctx := context.Background()
	version := int64(9)
	stub := &policyStub{version: &version}
	private := newRecordingPrivateState()
	r := &PolicyResource{client: stub.client(t), privateOverride: private}

	policySchema, identity := policySchemas(ctx, t)
	prior := priorStateRaw(ctx, policySchema)
	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}
	r.Read(ctx, resource.ReadRequest{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("reading an active policy: %v", resp.Diagnostics.Errors())
	}
	token, tokenDiags := readLockToken(ctx, private)
	if tokenDiags.HasError() {
		t.Fatalf("reading the recorded token back: %v", tokenDiags)
	}
	if token != `"9"` {
		t.Errorf("recorded precondition = %q, want %q — the next update would not be conditional", token, `"9"`)
	}
}

// TestCreate_PrivateStateFailureStillRecordsThePolicy pins the cost ordering. Recording the version
// is bookkeeping whose only consequence is that the next update goes out unconditional; failing to
// record a policy that exists is unrecoverable, because policy names are not unique and the next
// apply creates a second one. So the write that fails must not be the one that decides.
func TestCreate_PrivateStateFailureStillRecordsThePolicy(t *testing.T) {
	ctx := context.Background()
	version := int64(1)
	stub := &policyStub{version: &version}
	r := &PolicyResource{
		client:          stub.client(t),
		privateOverride: refusingPrivateState{recordingPrivateState: newRecordingPrivateState()},
	}

	policySchema, identity := policySchemas(ctx, t)
	raw := createPlanRaw(ctx, policySchema)
	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: policySchema},
		Identity: &identity,
	}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: policySchema, Raw: raw},
		Config: tfsdk.Config{Schema: policySchema, Raw: raw},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a private-state write must not fail the create: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the created policy must be recorded in state, or the apply orphans it on the tenant")
	}
	if !warningContains(resp.Diagnostics, "Unable to record the AI policy's version") {
		t.Errorf("a version that could not be recorded was not reported: %v", resp.Diagnostics.Warnings())
	}
	for _, warning := range resp.Diagnostics.Warnings() {
		if strings.Contains(warning.Detail(), "NewProviderData") {
			t.Errorf("the warning prescribes an implementation step: %q", warning.Detail())
		}
	}
}

// TestRead_PrivateStateFailureStillRefreshesState is the same ordering on the refresh path, where
// the cost of failing is a resource that reports an error on every plan for a policy that is fine.
func TestRead_PrivateStateFailureStillRefreshesState(t *testing.T) {
	ctx := context.Background()
	version := int64(4)
	stub := &policyStub{name: "unit-test-policy-renamed", version: &version}
	r := &PolicyResource{
		client:          stub.client(t),
		privateOverride: refusingPrivateState{recordingPrivateState: newRecordingPrivateState()},
	}

	policySchema, identity := policySchemas(ctx, t)
	prior := priorStateRaw(ctx, policySchema)
	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}
	r.Read(ctx, resource.ReadRequest{
		State:    tfsdk.State{Schema: policySchema, Raw: prior},
		Identity: &identity,
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a private-state write must not fail the refresh: %v", resp.Diagnostics.Errors())
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("an active policy must stay in state")
	}
	var state policyModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.Name.ValueString(); got != "unit-test-policy-renamed" {
		t.Errorf("name = %q, want the value the platform reported", got)
	}
}

// warningContains reports whether any warning carries all of the given fragments across its summary
// and detail.
func warningContains(diags diag.Diagnostics, fragments ...string) bool {
	for _, warning := range diags.Warnings() {
		text := warning.Summary() + " " + warning.Detail()
		matched := true
		for _, fragment := range fragments {
			if !strings.Contains(text, fragment) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
