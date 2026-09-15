// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// stubHandler is one recorded exchange the stub server should serve.
type stubHandler func(w http.ResponseWriter, r *http.Request)

// newStubClient returns a Jamf Account client pointed at a stub server driven by
// handle.
//
// The seam is the HTTP boundary rather than an injected interface, matching
// dns_zone's crud_partial_state_test.go: the CRUD methods hold a concrete
// *account.Client, and an interface introduced only for a test would be a bigger
// change than the behaviour it pins. The stub is local rather than
// testhelpers.NewMockClient because testhelpers reaches the provider package
// under the acceptance build tag, and this package is one the provider registers
// — importing it from an in-package test makes that a cycle.
func newStubClient(t *testing.T, handle stubHandler) *account.Client {
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
		handle(w, r)
	}))
	t.Cleanup(server.Close)
	return account.New(jamfplatform.NewClient(server.URL, "test-id", "test-secret", jamfplatform.WithRetryPolicy(0, 0, 0)))
}

// claimBody is the 201 body a claim answers with: the whole stored
// representation, which is why Create needs no read-back.
const claimBody = `{
	"id": "26917",
	"createdByName": null,
	"accountId": "001ABCDEFGHIJKLMNO",
	"domain": "tf-unit.example",
	"verificationKey": "verification-key-claim",
	"domainStatus": "PENDING",
	"createdDate": "2026-09-02T12:33:32.658Z",
	"lastModifiedDate": "2026-09-02T12:33:32.658Z",
	"lastVerificationDate": null,
	"verificationExpirationDate": "2026-09-16T12:33:32.658Z",
	"sharedDomain": false,
	"verifiedTldId": null
}`

// domainListBody is the collection envelope a read scans.
const domainListBody = `{"totalCount":1,"results":[` + claimBody + `]}`

// domainRawValue builds a resource object with domain set, every computed
// attribute Unknown as Terraform sends them on create, and timeouts null.
func domainRawValue(ctx context.Context, s resourceschema.Schema, domain string, computedKnown bool) tftypes.Value {
	object := s.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		switch {
		case name == "domain":
			values[name] = tftypes.NewValue(attributeType, domain)
		case name == "timeouts":
			values[name] = tftypes.NewValue(attributeType, nil)
		case computedKnown:
			values[name] = tftypes.NewValue(attributeType, nil)
		default:
			values[name] = tftypes.NewValue(attributeType, tftypes.UnknownValue)
		}
	}
	return tftypes.NewValue(object, values)
}

// resourceUnderTest wires a stub client into the resource and returns it with its
// schemas, so each test states only the exchange it cares about.
func resourceUnderTest(t *testing.T, handle stubHandler) (*DomainResource, resourceschema.Schema, *tfsdk.ResourceIdentity) {
	t.Helper()
	ctx := context.Background()
	r := &DomainResource{client: newStubClient(t, handle)}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	var identityResp resource.IdentitySchemaResponse
	r.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	return r, schemaResp.Schema, &tfsdk.ResourceIdentity{Schema: identityResp.IdentitySchema}
}

// TestCreate_AdoptsTheClaimResponse pins that Create populates state entirely
// from the claim response and issues no second call. The verification token is
// minted by that one call, and a claim can only be re-read by scanning the whole
// collection, so a read-back would cost a listing to learn nothing.
func TestCreate_AdoptsTheClaimResponse(t *testing.T) {
	ctx := context.Background()
	var calls []string

	r, s, identity := resourceUnderTest(t, func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(claimBody))
	})

	resp := resource.CreateResponse{
		State:    tfsdk.State{Schema: s},
		Identity: identity,
	}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: domainRawValue(ctx, s, "tf-unit.example", false)},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}
	if len(calls) != 1 || calls[0] != "POST /sso/v1/domains" {
		t.Errorf("create issued %v, want exactly one claim call", calls)
	}

	var state DomainResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if state.ID.ValueString() != "26917" {
		t.Errorf("id = %q, want %q", state.ID.ValueString(), "26917")
	}
	if state.VerificationTXTRecord.ValueString() != "jamf-site-verification=verification-key-claim" {
		t.Errorf("verification_txt_record = %q", state.VerificationTXTRecord.ValueString())
	}
	if !resp.State.Raw.IsFullyKnown() {
		t.Errorf("state must be wholly known after create, got %s", resp.State.Raw)
	}
}

// TestCreate_DuplicateClaimPointsAtTheDomain pins that the refusal a practitioner
// is most likely to hit lands on the attribute they can change, rather than as a
// bare conflict naming a state and no remedy.
func TestCreate_DuplicateClaimPointsAtTheDomain(t *testing.T) {
	ctx := context.Background()

	r, s, identity := resourceUnderTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"errors":[{"code":"CONFLICT","field":null,"description":"Domain is already added to your organization"}],"httpStatus":409}`))
	})

	resp := resource.CreateResponse{State: tfsdk.State{Schema: s}, Identity: identity}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: domainRawValue(ctx, s, "tf-unit.example", false)},
	}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a duplicate claim must be reported as an error")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a refused claim must write no state")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "import it rather than claiming it again") {
		t.Errorf("detail %q does not name the remedy", detail)
	}
}

// TestRead_MatchesTheCollectionOnTheDomainName pins the read path. Jamf Account
// exposes no read of a single claim, so a refresh is a scan of the collection
// matched on the name.
func TestRead_MatchesTheCollectionOnTheDomainName(t *testing.T) {
	ctx := context.Background()
	var calls []string

	r, s, identity := resourceUnderTest(t, func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(domainListBody))
	})

	resp := resource.ReadResponse{State: tfsdk.State{Schema: s}, Identity: identity}
	r.Read(ctx, resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: domainRawValue(ctx, s, "tf-unit.example", true)},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if len(calls) != 1 || calls[0] != "GET /sso/v1/domains" {
		t.Errorf("read issued %v, want exactly one collection read", calls)
	}

	var state DomainResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("reading back the state: %v", diags)
	}
	if state.ID.ValueString() != "26917" {
		t.Errorf("read did not hydrate the identifier: %q", state.ID.ValueString())
	}
}

// TestRead_WithdrawnClaimIsRemovedFromState pins drift recovery. A claim
// withdrawn outside Terraform is simply absent from the collection — there is no
// not-found status to branch on — so absence from the scan is the only signal.
func TestRead_WithdrawnClaimIsRemovedFromState(t *testing.T) {
	ctx := context.Background()

	r, s, identity := resourceUnderTest(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalCount":0,"results":[]}`))
	})

	resp := resource.ReadResponse{
		State:    tfsdk.State{Schema: s, Raw: domainRawValue(ctx, s, "tf-unit.example", true)},
		Identity: identity,
	}
	r.Read(ctx, resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: domainRawValue(ctx, s, "tf-unit.example", true)},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("a withdrawn claim must be removed from state so the next plan re-creates it")
	}
}

// TestUpdate_IssuesNoWrite pins the immutability of the claim at the call level.
// Every configurable attribute is RequiresReplace and Jamf exposes no update, so
// the only call Update may make is the listing that re-hydrates the read-only
// attributes.
func TestUpdate_IssuesNoWrite(t *testing.T) {
	ctx := context.Background()
	var calls []string

	r, s, identity := resourceUnderTest(t, func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(domainListBody))
	})

	resp := resource.UpdateResponse{State: tfsdk.State{Schema: s}, Identity: identity}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: domainRawValue(ctx, s, "tf-unit.example", true)},
		State: tfsdk.State{Schema: s, Raw: domainRawValue(ctx, s, "tf-unit.example", true)},
	}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics: %v", resp.Diagnostics)
	}
	for _, call := range calls {
		if !strings.HasPrefix(call, "GET ") {
			t.Errorf("update issued a write: %s", call)
		}
	}
	if len(calls) != 1 {
		t.Errorf("update issued %v, want exactly one collection read", calls)
	}
}

// TestDelete_TreatsAnAlreadyWithdrawnClaimAsSuccess pins the not-found branch
// STYLE_GUIDE §Delete semantics requires of every delete: the transport neither
// retries nor swallows a 404, so a claim already gone has to be recognised here.
func TestDelete_TreatsAnAlreadyWithdrawnClaimAsSuccess(t *testing.T) {
	ctx := context.Background()
	var calls []string

	r, s, _ := resourceUnderTest(t, func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"code":"NOT_FOUND","field":null,"description":"Unable to find domain by id: 26917"}],"httpStatus":404}`))
	})

	state := stateWithID(ctx, t, s, "tf-unit.example", "26917")
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: state}}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("an already-withdrawn claim must not fail the destroy: %v", resp.Diagnostics)
	}
	if len(calls) != 1 || calls[0] != "DELETE /sso/v1/domains/26917" {
		t.Errorf("delete issued %v, want one call keyed on the identifier", calls)
	}
}

// TestDelete_WithoutAnIdentifierSaysWhatToDo covers the state a partially-applied
// import can leave: the domain name is recorded but the identifier is not, and
// withdrawing a claim needs the identifier.
func TestDelete_WithoutAnIdentifierSaysWhatToDo(t *testing.T) {
	ctx := context.Background()
	var calls []string

	r, s, _ := resourceUnderTest(t, func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})

	state := domainRawValue(ctx, s, "tf-unit.example", true)
	resp := resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: state}}, &resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("a destroy with no identifier recorded must be reported, not silently skipped")
	}
	if len(calls) != 0 {
		t.Errorf("delete must issue nothing without an identifier, issued %v", calls)
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "-refresh-only") {
		t.Errorf("detail %q does not name the remedy", detail)
	}
}

// stateWithID builds a resource object carrying both the domain name and the
// Jamf-assigned identifier, which is the state a destroy runs against.
func stateWithID(ctx context.Context, t *testing.T, s resourceschema.Schema, domain, id string) tftypes.Value {
	t.Helper()
	object := s.Type().TerraformType(ctx).(tftypes.Object)
	values := make(map[string]tftypes.Value, len(object.AttributeTypes))
	for name, attributeType := range object.AttributeTypes {
		switch name {
		case "domain":
			values[name] = tftypes.NewValue(attributeType, domain)
		case "id":
			values[name] = tftypes.NewValue(attributeType, id)
		default:
			values[name] = tftypes.NewValue(attributeType, nil)
		}
	}
	return tftypes.NewValue(object, values)
}
