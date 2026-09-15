// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_profile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// Paths the activation profile operations are mounted at, so the stub can route
// on method and path rather than on method alone: create, pause and the bulk
// delete are all POSTs under the same collection.
const (
	activationProfilesPath       = "/securitycloud/v1/activation-profiles"
	activationProfilesDeletePath = activationProfilesPath + "/delete-multiple"
	activationProfilePauseSuffix = "/pause"
)

// stubbedClient returns a Jamf Security Cloud client pointed at a stub server that
// answers the token request and hands everything else to handler.
//
// The seam is the HTTP boundary rather than an injected interface, matching
// dns_zone's crud_partial_state_test.go: the handlers hold a concrete
// *securitycloud.Client, and an interface introduced only for a test would be a
// bigger change than the behaviour it pins. The stub is local rather than
// testhelpers.NewMockClient because testhelpers reaches the provider package under
// the acceptance build tag, and this package is one of the resources the provider
// registers — importing it from an in-package test makes that a cycle.
func stubbedClient(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *securitycloud.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/auth/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return securitycloud.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// createdThenPauseRefusedClient accepts the create and refuses the pause, which is
// what a credential holding activation-profiles:create but not :update sees.
func createdThenPauseRefusedClient(t *testing.T, code string) *securitycloud.Client {
	t.Helper()
	return stubbedClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == activationProfilesPath:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": code})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, activationProfilePauseSuffix):
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "pausing was refused"})
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "unexpected request"})
		}
	})
}

// profileRaw builds a fully-known activation profile object for the resource
// schema, with `id` set to code or left Unknown when code is empty, the way
// Terraform sends a create plan.
func profileRaw(ctx context.Context, profileSchema resourceschema.Schema, code string, paused bool) tftypes.Value {
	object := profileSchema.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}

	capabilitiesType := object.AttributeTypes["capabilities"].(tftypes.Object)
	values["name"] = tftypes.NewValue(tftypes.String, "unit-test-profile")
	values["platforms"] = tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, []tftypes.Value{
		tftypes.NewValue(tftypes.String, "mac"),
	})
	values["capabilities"] = tftypes.NewValue(capabilitiesType, map[string]tftypes.Value{
		"content_controls": tftypes.NewValue(tftypes.Bool, false),
		"network_security": tftypes.NewValue(tftypes.Bool, true),
		"note":             tftypes.NewValue(tftypes.String, nil),
	})
	values["paused"] = tftypes.NewValue(tftypes.Bool, paused)
	values["id"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	if code != "" {
		values["id"] = tftypes.NewValue(tftypes.String, code)
	}

	return tftypes.NewValue(object, values)
}

// newProfileSchema returns the resource schema, built from the resource itself so
// the test does not depend on any other test file's helper.
func newProfileSchema(ctx context.Context, t *testing.T) resourceschema.Schema {
	t.Helper()
	var schemaResp resource.SchemaResponse
	(&ActivationProfileResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building the schema: %v", schemaResp.Diagnostics)
	}
	return schemaResp.Schema
}

// TestCreate_RefusedPauseCommitsARunningProfile pins the state a create writes when
// the profile is minted but the pause is refused. `paused` must be committed false:
// Read cannot refresh it and Update only acts on a plan/state difference, so
// recording the desired value instead would leave Terraform asserting a closed
// profile that is quietly accepting enrollments, with no later plan that ever
// corrects it.
func TestCreate_RefusedPauseCommitsARunningProfile(t *testing.T) {
	ctx := context.Background()
	const code = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	profileSchema := newProfileSchema(ctx, t)
	r := &ActivationProfileResource{client: createdThenPauseRefusedClient(t, code)}

	resp := resource.CreateResponse{State: tfsdk.State{Schema: profileSchema}}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: profileSchema, Raw: profileRaw(ctx, profileSchema, "", true)},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a refused pause must still be reported as an error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the created profile must be recorded in state, or the apply orphans it on the tenant")
	}
	if !resp.State.Raw.IsFullyKnown() {
		t.Errorf("partial state must be wholly known, got %s", resp.State.Raw)
	}

	var state ActivationProfileResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.ID.ValueString(); got != code {
		t.Errorf("id = %q, want %q", got, code)
	}
	if state.Paused.ValueBool() {
		t.Error("paused = true, want false: the pause was refused, so the profile is still accepting enrollments")
	}
}

// TestCreate_SuccessfulPauseRecordsPaused is the other half of the pair: only a
// pause the server accepted may promote state to paused.
func TestCreate_SuccessfulPauseRecordsPaused(t *testing.T) {
	ctx := context.Background()
	const code = "9c858901-8a57-4791-81fe-4c455b099bc9"
	profileSchema := newProfileSchema(ctx, t)
	paused := false
	client := stubbedClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == activationProfilesPath:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": code})
		case r.Method == http.MethodPost && r.URL.Path == activationProfilesPath+"/"+code+activationProfilePauseSuffix:
			paused = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "unexpected request"})
		}
	})
	r := &ActivationProfileResource{client: client}

	resp := resource.CreateResponse{State: tfsdk.State{Schema: profileSchema}}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: profileSchema, Raw: profileRaw(ctx, profileSchema, "", true)},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("creating a paused profile: %v", resp.Diagnostics)
	}
	if !paused {
		t.Error("the pause operation was never called")
	}

	var state ActivationProfileResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.ID.ValueString(); got != code {
		t.Errorf("id = %q, want %q", got, code)
	}
	if !state.Paused.ValueBool() {
		t.Error("paused = false, want true: the pause succeeded")
	}
}

// TestDelete_RefusalKeepsTheProfileInState pins the refusal contract: a delete the
// server refused deleted nothing, so the resource must stay in state. Dropping it
// would leave a live activation code accepting enrollments with no Terraform record
// of it.
func TestDelete_RefusalKeepsTheProfileInState(t *testing.T) {
	ctx := context.Background()
	const code = "7d4b1c2e-2f6a-4a0e-9c0f-5a1d3b8e6f21"
	profileSchema := newProfileSchema(ctx, t)
	client := stubbedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == activationProfilesDeletePath {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "deleting was refused"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "unexpected request"})
	})
	r := &ActivationProfileResource{client: client}

	priorState := tfsdk.State{Schema: profileSchema, Raw: profileRaw(ctx, profileSchema, code, false)}
	resp := resource.DeleteResponse{State: priorState}
	r.Delete(ctx, resource.DeleteRequest{State: priorState}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a refused delete must be reported as an error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("a refused delete must leave the profile in state, or a live activation code is left unmanaged")
	}

	var state ActivationProfileResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.ID.ValueString(); got != code {
		t.Errorf("id = %q, want %q", got, code)
	}
}

// TestRead_UnknownCodeRemovesTheResource covers the one thing Read can detect: a
// 404 means the code never existed, so the resource is gone from Terraform's point
// of view.
func TestRead_UnknownCodeRemovesTheResource(t *testing.T) {
	ctx := context.Background()
	const code = "0e2c9c1a-4f77-4f2a-9b18-08c3f5b46d77"
	profileSchema := newProfileSchema(ctx, t)
	client := stubbedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == activationProfilesPath+"/"+code {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "no such activation profile"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "unexpected request"})
	})
	r := &ActivationProfileResource{client: client}

	priorState := tfsdk.State{Schema: profileSchema, Raw: profileRaw(ctx, profileSchema, code, false)}
	resp := resource.ReadResponse{State: priorState}
	r.Read(ctx, resource.ReadRequest{State: priorState}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("a missing activation code must not be an error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Errorf("state = %s, want null: an activation code the server does not know is gone", resp.State.Raw)
	}
}
