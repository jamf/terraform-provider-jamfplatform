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

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// searchDomainClient returns a Jamf Security Cloud client pointed at a stub server
// that answers the token exchange, answers every read with the given status and
// body, and accepts a write or a clear with the 204 those endpoints return.
//
// The seam is the HTTP boundary rather than an injected interface: the handlers hold
// a concrete *securitycloud.Client, and an interface introduced only for a test
// would be a bigger change than the behaviour it pins. The stub is local rather than
// testhelpers.NewMockClient because testhelpers reaches the provider package under
// the acceptance build tag, and this package is one of the resources the provider
// registers — importing it from an in-package test makes that a cycle.
func searchDomainClient(t *testing.T, status int, body any) *securitycloud.Client {
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
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(server.Close)
	return securitycloud.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// nullObject builds a value of the given object type with every attribute null —
// the shape a data source configuration with no arguments arrives as, and the shape
// the state carries into the Read that follows an import (only the id is passed
// through, and even that is null here because these tests exercise the read of the
// remote value rather than the import path). A wholly null object cannot be decoded
// into a Go struct, so it has to be an object of nulls rather than a null object,
// which is also why a zero tftypes.Value will not serve: req.State.Get refuses it.
func nullObject(objectType tftypes.Type) tftypes.Value {
	object, ok := objectType.(tftypes.Object)
	if !ok {
		return tftypes.NewValue(objectType, nil)
	}
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	return tftypes.NewValue(object, values)
}

// TestResourceRead_EmptyValueIsAbsence pins that the resource Read agrees with
// Create's preflight, which already reads an empty value as nothing configured.
// Absence presents as a 404 as shipped, so this is defence: were the endpoint ever
// to answer 200 with no value, the alternative is committing "" to state and
// reporting a search domain the tenant does not have.
func TestResourceRead_EmptyValueIsAbsence(t *testing.T) {
	ctx := context.Background()
	r := &SearchDomainResource{client: searchDomainClient(t, http.StatusOK, map[string]any{"suffix": ""})}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	resp := resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: schemaResp.Schema, Raw: nullObject(schemaResp.Schema.Type().TerraformType(ctx))}}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("an empty value is absence, not an error: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Errorf("expected the resource to be removed from state, got %s", resp.State.Raw)
	}
}

// TestResourceRead_ValueIsKept is the positive control: without it the assertion
// above would pass just as well against a Read that removed the resource
// unconditionally.
func TestResourceRead_ValueIsKept(t *testing.T) {
	ctx := context.Background()
	r := &SearchDomainResource{client: searchDomainClient(t, http.StatusOK, map[string]any{"suffix": "corp.example.com"})}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: schemaResp.Schema, Raw: nullObject(schemaResp.Schema.Type().TerraformType(ctx))}}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("a configured search domain must stay in state")
	}
	var state SearchDomainResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.DomainName.ValueString(); got != "corp.example.com" {
		t.Errorf("domain_name = %q, want %q", got, "corp.example.com")
	}
}

// TestDataSourceRead_EmptyValueIsAnError pins the data source's half of the same
// agreement. It exists so the promise in its doc comment — that nothing referencing
// it is ever handed an empty string — holds for both shapes of absence, not only the
// 404 the endpoint sends today.
func TestDataSourceRead_EmptyValueIsAnError(t *testing.T) {
	ctx := context.Background()
	d := &SearchDomainDataSource{client: searchDomainClient(t, http.StatusOK, map[string]any{"suffix": ""})}

	var schemaResp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)

	config := tfsdk.Config{Schema: schemaResp.Schema, Raw: nullObject(schemaResp.Schema.Type().TerraformType(ctx))}
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: config}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("an empty value must be an error: a data source yielding \"\" feeds it into whatever referenced it")
	}
	want, _ := noSearchDomainConfiguredError()
	if got := resp.Diagnostics.Errors()[0].Summary(); got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

// planWithDomain builds a create plan holding just domain_name, which is the only
// attribute a configuration can set.
func planWithDomain(ctx context.Context, resourceSchema resourceschema.Schema, domain string) tfsdk.Plan {
	object := resourceSchema.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	values["domain_name"] = tftypes.NewValue(tftypes.String, domain)
	return tfsdk.Plan{Schema: resourceSchema, Raw: tftypes.NewValue(object, values)}
}

// createResource runs Create against a stub answering every request with the given
// stored search domain, and returns the response.
func createResource(t *testing.T, planned, stored string) resource.CreateResponse {
	t.Helper()
	ctx := context.Background()
	r := &SearchDomainResource{client: searchDomainClient(t, http.StatusOK, map[string]any{"suffix": stored})}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: schemaResp.Schema},
		Identity: &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema},
	}
	r.Create(ctx, resource.CreateRequest{Plan: planWithDomain(ctx, schemaResp.Schema, planned)}, &resp)
	return resp
}

// TestCreate_AdoptsAnIdenticalStoredValue is the retry case. Create's confirming
// read can fail after the write has landed, leaving the tenant configured and
// Terraform holding no state; the operator's only move is to apply again. A
// preflight testing presence alone refuses that second apply as if an administrator
// had set the value, so the tenant stays unmanageable without a manual import.
func TestCreate_AdoptsAnIdenticalStoredValue(t *testing.T) {
	resp := createResource(t, "corp.example.com", "corp.example.com")

	if resp.Diagnostics.HasError() {
		t.Fatalf("a stored value identical to the planned one is this handler's own write, not a clobber: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("the retry must record state, or the next apply faces the same problem")
	}
	var state SearchDomainResourceModel
	if diags := resp.State.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if got := state.DomainName.ValueString(); got != "corp.example.com" {
		t.Errorf("domain_name = %q, want %q", got, "corp.example.com")
	}
	if got := state.ID.ValueString(); got != helpers.SingletonID {
		t.Errorf("id = %q, want %q", got, helpers.SingletonID)
	}
}

// TestCreate_RefusesADifferentStoredValue is the other half: the refusal has to
// survive the retry fix, since nothing on the wire distinguishes a create from a
// silent takeover of a value someone else set.
//
// The detail names both reachable causes. Import is the fix for an administrator's
// value; a configuration declaring this one-per-tenant resource twice — legal HCL,
// and nothing here prevents it — needs the duplicate block removed instead, and
// would otherwise be told to import a value its own apply had just written.
func TestCreate_RefusesADifferentStoredValue(t *testing.T) {
	resp := createResource(t, "corp.example.com", "someone-else.example.com")

	if !resp.Diagnostics.HasError() {
		t.Fatal("a different stored value must be refused")
	}
	first := resp.Diagnostics.Errors()[0]
	if got, want := first.Summary(), "Search domain already configured"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
	for _, want := range []string{"someone-else.example.com", "terraform import", "duplicate"} {
		if !strings.Contains(first.Detail(), want) {
			t.Errorf("detail %q does not mention %q", first.Detail(), want)
		}
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a refused create must record no state")
	}
}
