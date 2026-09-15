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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// createdThenStuckClient returns a client pointed at a stub server that accepts the
// create and then answers every read with a gateway parked in the given state, which
// is what an apply sees when provisioning outlasts the timeout budget.
//
// The seam is the HTTP boundary rather than an injected interface, for the reasons
// createdThenUnreadableClient gives.
func createdThenStuckClient(t *testing.T, gatewayID, state string) *securitycloud.Client {
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
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":           gatewayID,
				"name":         "unit-test-gateway",
				"datacenter":   securitycloud.GatewayCreateRequestDatacenterAfSouth1,
				"enabled":      true,
				"tenantIds":    []string{"tenant-1"},
				"contact":      map[string]any{"name": "Network team", "email": "network@example.com"},
				"dedicatedIps": map[string]any{"enabled": true, "ips": []string{"198.51.100.10", "198.51.100.11"}},
				"status":       map[string]any{"state": state, "updatedAt": "2026-08-31T12:00:00Z"},
				"createdAt":    "2026-08-31T12:00:00Z",
				"updatedAt":    "2026-08-31T12:00:00Z",
			})
		}
	}))
	t.Cleanup(server.Close)
	return securitycloud.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// TestCreate_ExhaustedReadinessWaitWarnsAndKeepsState pins the whole point of making
// the wait advisory: a gateway that has not come up inside the budget is still a
// gateway the account is paying for, so the apply succeeds with a warning and the
// resource stays in state.
//
// An error here would be actively harmful rather than merely strict: Terraform marks
// a resource tainted when a create returns an error, and destroys and recreates it on
// the next apply — discarding a provisioned dedicated gateway, and the dedicated IP
// addresses it took from the account's allotment, for the offence of being slow.
//
// The two states are separate cases because the warning has to distinguish them.
// Still provisioning is usually slowness. Unreachable or degraded is a plausible
// fault, and telling an operator to wait it out would be the wrong advice.
//
// The budget is short and the poll interval is not, so the wait reads once, finds the
// gateway parked, and runs out — the same sequence as a real timeout, in milliseconds.
// That the state is populated at all also pins that the caller reuses the
// representation the wait read: another read on the by-then-dead context could only
// have failed.
func TestCreate_ExhaustedReadinessWaitWarnsAndKeepsState(t *testing.T) {
	cases := []struct {
		name     string
		state    string
		wantText string
	}{
		{
			name:     "still provisioning",
			state:    securitycloud.GatewayStatusStatePending,
			wantText: "taking longer",
		},
		{
			name:     "reported unreachable",
			state:    securitycloud.GatewayStatusStateDown,
			wantText: "more likely a fault than slowness",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			const gatewayID = "a1b2"
			r := &GatewayResource{client: createdThenStuckClient(t, gatewayID, tc.state)}

			var schemaResp resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
			var identityResp resource.IdentitySchemaResponse
			r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

			raw := internetGatewayRawPlan(ctx, schemaResp.Schema, "300ms")
			resp := resource.CreateResponse{
				State:    tfsdk.State{Schema: schemaResp.Schema},
				Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
			}
			r.Create(ctx, resource.CreateRequest{
				Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
			}, &resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("an exhausted readiness wait must not fail the apply: %v", resp.Diagnostics)
			}
			if resp.Diagnostics.WarningsCount() != 1 {
				t.Fatalf("warnings = %d, want 1: %v", resp.Diagnostics.WarningsCount(), resp.Diagnostics)
			}
			warning := resp.Diagnostics.Warnings()[0]
			if !strings.Contains(warning.Detail(), tc.wantText) {
				t.Errorf("warning %q does not distinguish this state (looking for %q)", warning.Detail(), tc.wantText)
			}
			if !strings.Contains(warning.Detail(), "created successfully") {
				t.Errorf("warning %q must say the gateway was created", warning.Detail())
			}

			if resp.State.Raw.IsNull() {
				t.Fatal("the gateway exists and is billable, so it must be recorded in state")
			}
			var state GatewayResourceModel
			if diags := resp.State.Get(ctx, &state); diags.HasError() {
				t.Fatalf("reading back the state: %v", diags)
			}
			if got := state.ID.ValueString(); got != gatewayID {
				t.Errorf("id = %q, want %q", got, gatewayID)
			}
			if state.Status.IsNull() || state.Status.IsUnknown() {
				t.Fatalf("status = %s, want the representation the wait already read", state.Status)
			}
			if got := state.Status.String(); !strings.Contains(got, tc.state) {
				t.Errorf("status = %s, want the state the wait observed (%s)", got, tc.state)
			}
			if state.DedicatedEgressIPAddresses.IsNull() || len(state.DedicatedEgressIPAddresses.Elements()) != 2 {
				t.Errorf("dedicated egress addresses = %s, want the two the gateway already holds",
					state.DedicatedEgressIPAddresses)
			}
		})
	}
}
