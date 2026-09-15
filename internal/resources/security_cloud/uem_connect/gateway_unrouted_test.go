// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

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

// gatewayUnroutedClient returns a Jamf Security Cloud client pointed at a stub
// server shaped like the host that caused the state wipe: it issues a token and
// routes no API namespace, so every call after authentication meets the Jamf
// gateway's bare 404 — status 404, text/plain, body "404 page not found\n".
//
// Serving /auth/token is what makes the stub useful rather than merely 404-shaped.
// A server that refuses the token too fails the read during authentication, which
// errors and keeps state whether or not the guard exists, so the test would pass
// against a provider with the guard removed.
//
// The stub is local rather than shared with crud_partial_state_test.go's
// createdThenUnreadableClient because the two describe different servers: that one
// speaks JSON and accepts writes, this one serves nothing but the token. The seam
// is the HTTP boundary for the same reason as there — the handlers hold a concrete
// *securitycloud.Client, and testhelpers cannot be imported from an in-package
// test without a cycle.
func gatewayUnroutedClient(t *testing.T) *securitycloud.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/token" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("404 page not found\n"))
	}))
	t.Cleanup(server.Close)
	return securitycloud.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// managedPriorState builds the state a refresh of an already-managed integration
// arrives with: the identifier and the vendor set, every optional block null.
//
// The vendor matters because Read reads a null Required attribute as an import,
// and an import takes the identity branch instead of the prior-state one.
func managedPriorState(ctx context.Context, connectorSchema resourceschema.Schema, connectorID string) tftypes.Value {
	object := connectorSchema.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	values["id"] = tftypes.NewValue(tftypes.String, connectorID)
	values["uem_vendor"] = tftypes.NewValue(tftypes.String, vendorJamfPro)
	return tftypes.NewValue(object, values)
}

// TestRead_GatewayUnroutedKeepsStateAndErrors drives the real Read against a host
// that routes no Jamf service and pins both halves of what the guard in isNotFound
// buys: the failure is reported, and the integration stays in state.
//
// The classifier unit tests cannot see the second half. They prove
// helpers.IsGatewayUnrouted answers true for the bare 404, not that Read consults
// it before removing anything — a refactor that checked it after calling
// RemoveResource, or against the wrong error value on the far side of an
// errors.Join, would leave every one of them green. The regression that allows is
// the one this guard was added for: a base_url serving /auth/token but no API
// namespace made every Read receive that 404, isNotFound called it absence, and a
// single refresh emptied the state file while terraform plan reported a clean set
// of creates and exited 0.
//
// A framework Read signals that removal by calling resp.State.RemoveResource(ctx),
// which nulls resp.State.Raw and nothing else — so a null Raw below is exactly the
// state wipe, and asserting it is non-null is asserting the wipe did not happen.
func TestRead_GatewayUnroutedKeepsStateAndErrors(t *testing.T) {
	ctx := context.Background()
	const connectorID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	r := &UEMConnectResource{client: gatewayUnroutedClient(t)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	raw := managedPriorState(ctx, schemaResp.Schema, connectorID)
	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Read(ctx, resource.ReadRequest{
		State:    tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}, &resp)

	if resp.State.Raw.IsNull() {
		t.Fatal("the integration was removed from state; an unrouted 404 must not be read as absence")
	}
	if !resp.Diagnostics.HasError() {
		t.Fatal("a read against a host that routes no Jamf service must be reported as an error")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "404 page not found") {
		t.Errorf("the error should carry the gateway's reply, got %q", detail)
	}

	var state UEMConnectResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.ID.ValueString(); got != connectorID {
		t.Errorf("id = %q, want %q", got, connectorID)
	}
}
