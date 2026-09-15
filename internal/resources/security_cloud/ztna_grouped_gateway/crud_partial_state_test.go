// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

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

// createdThenUnreadableClient returns a Jamf Security Cloud client pointed at a stub
// server that accepts the create and then fails every read, which is the sequence
// that leaves a group on the tenant with nothing tracking it.
//
// The seam is the HTTP boundary rather than an injected interface, matching
// dns_search_domain's crud_test.go: the handlers hold a concrete
// *securitycloud.Client, and an interface introduced only for a test would be a
// bigger change than the behaviour it pins. The stub is local rather than
// testhelpers.NewMockClient because testhelpers reaches the provider package under
// the acceptance build tag, and this package is one of the resources the provider
// registers — importing it from an in-package test makes that a cycle.
func createdThenUnreadableClient(t *testing.T, groupID string) *securitycloud.Client {
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
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": groupID})
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "the read-back failed"})
		}
	}))
	t.Cleanup(server.Close)
	return securitycloud.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// groupedGatewayRawPlan builds a create plan with the required attributes set, the
// computed ones Unknown as Terraform sends them, and everything else null.
func groupedGatewayRawPlan(ctx context.Context, groupSchema resourceschema.Schema) tftypes.Value {
	object := groupSchema.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}

	values["name"] = tftypes.NewValue(tftypes.String, "unit-test-group")
	values["gateway_ids"] = tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
		tftypes.NewValue(tftypes.String, "gateway-1"),
		tftypes.NewValue(tftypes.String, "gateway-2"),
	})
	values["routing_strategy"] = tftypes.NewValue(tftypes.String, routingStrategyValues()[0])
	values["required_gateway_stability"] = tftypes.NewValue(tftypes.String, gatewayStabilityValues()[0])
	values["tenant_ids"] = tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, []tftypes.Value{
		tftypes.NewValue(tftypes.String, "tenant-1"),
	})
	values["id"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	values["created_at"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)

	return tftypes.NewValue(object, values)
}

// TestCreate_FailedReadBackKeepsTheNewID pins the partial state a create writes when
// the read-back fails. Without it the group exists on the tenant and Terraform records
// nothing, so the retry builds a second group over the same member gateways and leaves
// the first unmanaged.
//
// The fully-known assertion is not incidental. Terraform answers an unknown value in
// the state a failed apply returns with an "invalid result object after apply" error
// of its own, which would bury the diagnostic below; created_at is Computed with no
// default, so it arrives Unknown in every create plan.
func TestCreate_FailedReadBackKeepsTheNewID(t *testing.T) {
	ctx := context.Background()
	const groupID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	r := &GroupedGatewayResource{client: createdThenUnreadableClient(t, groupID)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: groupedGatewayRawPlan(ctx, schemaResp.Schema)},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed read-back must still be reported as an error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the created grouped gateway must be recorded in state, or the apply orphans it on the tenant")
	}
	if !resp.State.Raw.IsFullyKnown() {
		t.Errorf("partial state must be wholly known, got %s", resp.State.Raw)
	}

	var state GroupedGatewayResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.ID.ValueString(); got != groupID {
		t.Errorf("id = %q, want %q", got, groupID)
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{groupID, "do not re-create it"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not mention %q", detail, want)
		}
	}
}
