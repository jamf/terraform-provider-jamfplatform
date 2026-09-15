// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_search_domain

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

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// writtenThenUnreadableClient returns a Jamf Security Cloud client pointed at a stub
// server that reports nothing configured, accepts the write, and then fails every
// read after it. That is the sequence Create's own doc comment describes: the tenant
// ends up configured while the handler cannot confirm it.
//
// The stub is local for the same reasons the one in crud_test.go is — see its doc
// comment — and differs from it only in changing its answer once the write lands.
func writtenThenUnreadableClient(t *testing.T) *securitycloud.Client {
	t.Helper()
	written := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/auth/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case r.Method != http.MethodGet:
			written = true
			w.WriteHeader(http.StatusNoContent)
		case written:
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "the confirming read failed"})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "SEARCH_DOMAIN_NOT_SET"})
		}
	}))
	t.Cleanup(server.Close)
	return securitycloud.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// TestCreate_FailedConfirmingReadRecordsTheDomain pins the partial state a create
// writes when the confirming read fails after the write has landed. Create's
// preflight forgives that case on the next apply, but only after the operator has
// been left with a configured tenant and no state to show it; recording the planned
// value under the singleton ID is what makes the recovery a refresh rather than an
// import.
func TestCreate_FailedConfirmingReadRecordsTheDomain(t *testing.T) {
	ctx := context.Background()
	r := &SearchDomainResource{client: writtenThenUnreadableClient(t)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Create(ctx, resource.CreateRequest{Plan: planWithDomain(ctx, schemaResp.Schema, "corp.example.com")}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed confirming read must still be reported as an error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the written search domain must be recorded in state, or the tenant is configured with nothing tracking it")
	}
	if !resp.State.Raw.IsFullyKnown() {
		t.Errorf("partial state must be wholly known, got %s", resp.State.Raw)
	}

	var state SearchDomainResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.ID.ValueString(); got != helpers.SingletonID {
		t.Errorf("id = %q, want %q", got, helpers.SingletonID)
	}
	if got := state.DomainName.ValueString(); got != "corp.example.com" {
		t.Errorf("domain_name = %q, want %q", got, "corp.example.com")
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{helpers.SingletonID, "no need to import it"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not mention %q", detail, want)
		}
	}
}
