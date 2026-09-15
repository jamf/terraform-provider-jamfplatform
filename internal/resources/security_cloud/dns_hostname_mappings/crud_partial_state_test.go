// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_hostname_mappings

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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// writtenThenUnreadableClient returns a Jamf Security Cloud client pointed at a stub
// server that reports an empty mapping set, accepts the replace, and then fails every
// read after it. That is the sequence Create's preflight has to forgive on the next
// apply: the tenant holds the mappings and the handler cannot confirm them.
//
// The seam is the HTTP boundary rather than an injected interface, matching
// dns_search_domain's crud_test.go: the handlers hold a concrete
// *securitycloud.Client, and an interface introduced only for a test would be a
// bigger change than the behaviour it pins. The stub is local rather than
// testhelpers.NewMockClient because testhelpers reaches the provider package under
// the acceptance build tag, and this package is one of the resources the provider
// registers — importing it from an in-package test makes that a cycle.
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
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "the read-back failed"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "totalCount": 0})
		}
	}))
	t.Cleanup(server.Close)
	return securitycloud.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// planWithMappings builds a create plan holding just the mapping set, which is the
// only attribute a configuration can set.
func planWithMappings(t *testing.T, ctx context.Context, mappingsSchema resourceschema.Schema, mappings types.Set) tfsdk.Plan {
	t.Helper()
	object := mappingsSchema.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	raw, err := mappings.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("converting the mapping set: %v", err)
	}
	values["mappings"] = raw
	values["id"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	return tfsdk.Plan{Schema: mappingsSchema, Raw: tftypes.NewValue(object, values)}
}

// TestCreate_FailedReadBackRecordsTheMappings pins the partial state a create writes
// when the read-back fails after the replace has landed. Without it the tenant holds
// the mappings and Terraform records nothing, so the operator's next apply re-enters
// Create — where the preflight has to recognise the provider's own write rather than
// refuse it as an administrator's.
//
// It is also the test for write's return value: the diagnostics cannot distinguish a
// replace that never happened from a read-back that failed after one did, so a create
// that committed state on every error would record mappings the tenant does not hold.
func TestCreate_FailedReadBackRecordsTheMappings(t *testing.T) {
	ctx := context.Background()
	r := &HostnameMappingsResource{client: writtenThenUnreadableClient(t)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	mappings := mappingSet(t, writeMappingObject(t, "corp.example.com", addressSet(t, "10.0.0.1"), types.SetNull(types.StringType), true, false))
	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Create(ctx, resource.CreateRequest{Plan: planWithMappings(t, ctx, schemaResp.Schema, mappings)}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed read-back must still be reported as an error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the written mappings must be recorded in state, or the tenant holds mappings nothing tracks")
	}
	if !resp.State.Raw.IsFullyKnown() {
		t.Errorf("partial state must be wholly known, got %s", resp.State.Raw)
	}

	var state HostnameMappingsResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.ID.ValueString(); got != helpers.SingletonID {
		t.Errorf("id = %q, want %q", got, helpers.SingletonID)
	}
	if got := len(state.Mappings.Elements()); got != 1 {
		t.Errorf("state holds %d mapping(s), want 1", got)
	}

	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{helpers.SingletonID, "nothing has to be re-created"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q does not mention %q", detail, want)
		}
	}
}

// TestCreate_FailedWriteRecordsNothing is the control for the case above: a replace
// that never landed must leave state empty, or Terraform would claim ownership of
// mappings the tenant does not hold and report a spurious drift on the next plan.
func TestCreate_FailedWriteRecordsNothing(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/auth/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}, "totalCount": 0})
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": "the write failed"})
		}
	}))
	t.Cleanup(server.Close)
	r := &HostnameMappingsResource{client: securitycloud.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	mappings := mappingSet(t, writeMappingObject(t, "corp.example.com", addressSet(t, "10.0.0.1"), types.SetNull(types.StringType), true, false))
	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Create(ctx, resource.CreateRequest{Plan: planWithMappings(t, ctx, schemaResp.Schema, mappings)}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed write must be reported as an error")
	}
	if !resp.State.Raw.IsNull() {
		t.Errorf("a create whose write never landed must record no state, got %s", resp.State.Raw)
	}
}
