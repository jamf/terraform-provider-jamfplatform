// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

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
// that leaves a provisioned gateway with nothing tracking it.
//
// The seam is the HTTP boundary rather than an injected interface, matching
// dns_search_domain's crud_test.go: the handlers hold a concrete
// *securitycloud.Client, and an interface introduced only for a test would be a
// bigger change than the behaviour it pins. The stub is local rather than
// testhelpers.NewMockClient because testhelpers reaches the provider package under
// the acceptance build tag, and this package is one of the resources the provider
// registers — importing it from an in-package test makes that a cycle.
func createdThenUnreadableClient(t *testing.T, gatewayID string) *securitycloud.Client {
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
			_ = json.NewEncoder(w).Encode(map[string]any{"id": gatewayID})
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "the read-back failed"})
		}
	}))
	t.Cleanup(server.Close)
	return securitycloud.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// internetGatewayRawPlan builds a create plan for a dedicated internet gateway: the
// required attributes set, the computed ones Unknown as Terraform sends them, and
// everything else null. The internet form is the one that carries every computed
// value — status and the dedicated egress addresses — with no `ipsec` block to build.
//
// createTimeout, when non-empty, populates the `timeouts` block's `create` value.
// Empty leaves the whole block null so the resource's own default applies. The
// readiness wait runs on that budget, so a test that needs it to run out sets a
// short one here rather than waiting out the real ten minutes.
func internetGatewayRawPlan(ctx context.Context, gatewaySchema resourceschema.Schema, createTimeout string) tftypes.Value {
	object := gatewaySchema.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}

	contactType := object.AttributeTypes["contact"].(tftypes.Object)
	values["contact"] = tftypes.NewValue(contactType, map[string]tftypes.Value{
		"name":  tftypes.NewValue(tftypes.String, "Network team"),
		"email": tftypes.NewValue(tftypes.String, "network@example.com"),
	})
	values["name"] = tftypes.NewValue(tftypes.String, "unit-test-gateway")
	values["egress_region"] = tftypes.NewValue(tftypes.String, egressRegionValues()[0])
	values["enabled"] = tftypes.NewValue(tftypes.Bool, true)
	values["tenant_ids"] = tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, []tftypes.Value{
		tftypes.NewValue(tftypes.String, "tenant-1"),
	})
	values["id"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	values["status"] = tftypes.NewValue(object.AttributeTypes["status"], tftypes.UnknownValue)
	values["dedicated_egress_ip_addresses"] = tftypes.NewValue(
		object.AttributeTypes["dedicated_egress_ip_addresses"], tftypes.UnknownValue)

	if createTimeout != "" {
		timeoutsType := object.AttributeTypes["timeouts"].(tftypes.Object)
		timeoutValues := make(map[string]tftypes.Value, len(timeoutsType.AttributeTypes))
		for name, attributeType := range timeoutsType.AttributeTypes {
			timeoutValues[name] = tftypes.NewValue(attributeType, nil)
		}
		timeoutValues["create"] = tftypes.NewValue(tftypes.String, createTimeout)
		values["timeouts"] = tftypes.NewValue(timeoutsType, timeoutValues)
	}

	return tftypes.NewValue(object, values)
}

// TestCreate_FailedReadBackKeepsTheNewID pins the partial state a create writes when
// the read-back fails. Without it the gateway is provisioned and Terraform records
// nothing, so a retry provisions a second one — taking another dedicated IP address
// from the account's allotment and leaving the first to be found in the admin UI.
//
// The fully-known assertion is not incidental. Terraform answers an unknown value in
// the state a failed apply returns with an "invalid result object after apply" error
// of its own, which would bury the diagnostic below; the status block and the
// dedicated egress addresses are Computed with no default, so both arrive Unknown in
// every create plan.
func TestCreate_FailedReadBackKeepsTheNewID(t *testing.T) {
	ctx := context.Background()
	const gatewayID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	r := &GatewayResource{client: createdThenUnreadableClient(t, gatewayID)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	raw := internetGatewayRawPlan(ctx, schemaResp.Schema, "")
	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed read-back must still be reported as an error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the created gateway must be recorded in state, or the apply leaves it provisioned and untracked")
	}
	if !resp.State.Raw.IsFullyKnown() {
		t.Errorf("partial state must be wholly known, got %s", resp.State.Raw)
	}

	var state GatewayResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.ID.ValueString(); got != gatewayID {
		t.Errorf("id = %q, want %q", got, gatewayID)
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{gatewayID, "do not re-create it"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not mention %q", detail, want)
		}
	}
}
