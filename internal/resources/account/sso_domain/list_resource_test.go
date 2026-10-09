// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// ownedDomainBody is a second claimed domain, so the collection fixtures can hold
// more than one entry and the limit can be asserted against something it has to
// cut short.
const ownedDomainBody = `{
	"id": "26918",
	"createdByName": "Someone Else",
	"accountId": "001ABCDEFGHIJKLMNO",
	"domain": "tf-unit-second.example",
	"verificationKey": "verification-key-second",
	"domainStatus": "VERIFIED",
	"createdDate": "2026-09-02T12:40:00.000Z",
	"lastModifiedDate": "2026-09-02T12:40:00.000Z",
	"lastVerificationDate": "2026-09-02T12:41:00.000Z",
	"verificationExpirationDate": "2026-09-16T12:41:00.000Z",
	"sharedDomain": false,
	"verifiedTldId": null
}`

// sharedDomainBody is a domain another organization owns and shares into this
// one. It is listed by the collection read exactly like an owned claim, and the
// only thing separating them is sharedDomain.
const sharedDomainBody = `{
	"id": "26919",
	"createdByName": null,
	"accountId": "001ZYXWVUTSRQPONM",
	"domain": "tf-unit-shared.example",
	"verificationKey": "verification-key-shared",
	"domainStatus": "VERIFIED",
	"createdDate": "2026-09-01T09:00:00.000Z",
	"lastModifiedDate": "2026-09-01T09:00:00.000Z",
	"lastVerificationDate": "2026-09-01T09:01:00.000Z",
	"verificationExpirationDate": "2026-09-15T09:01:00.000Z",
	"sharedDomain": true,
	"verifiedTldId": null
}`

// twoOwnedDomainsBody is a collection of two claims this organization owns.
const twoOwnedDomainsBody = `{"totalCount":2,"results":[` + claimBody + `,` + ownedDomainBody + `]}`

// ownedAndSharedDomainsBody is a collection holding one owned claim and one
// domain shared in by another organization.
const ownedAndSharedDomainsBody = `{"totalCount":2,"results":[` + sharedDomainBody + `,` + claimBody + `]}`

// emptyDomainListBody is the collection an organization holding nothing answers
// with.
const emptyDomainListBody = `{"totalCount":0,"results":[]}`

// listDomains runs List against a stub serving body from the collection endpoint,
// and returns everything the stream pushed.
//
// The schemas come from DomainResource because ListRequest carries the managed
// resource's own schema and identity schema — the list resource declares only its
// (empty) configuration.
func listDomains(t *testing.T, body string, limit int64) []list.ListResult {
	t.Helper()
	ctx := context.Background()

	r := &DomainListResource{client: newStubClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})}

	managed := &DomainResource{}
	var schemaResp resource.SchemaResponse
	managed.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	managed.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	var configResp list.ListResourceSchemaResponse
	r.ListResourceConfigSchema(ctx, list.ListResourceSchemaRequest{}, &configResp)
	configType := configResp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	var stream list.ListResultsStream
	r.List(ctx, list.ListRequest{
		Config: tfsdk.Config{Schema: configResp.Schema, Raw: tftypes.NewValue(configType, map[string]tftypes.Value{
			"timeouts": tftypes.NewValue(configType.AttributeTypes["timeouts"], nil),
		})},
		IncludeResource:        true,
		Limit:                  limit,
		ResourceSchema:         schemaResp.Schema,
		ResourceIdentitySchema: identityResp.IdentitySchema,
	}, &stream)

	if stream.Results == nil {
		t.Fatal("List left the stream unset, so Terraform receives neither results nor a diagnostic")
	}

	var results []list.ListResult
	for result := range stream.Results {
		if result.Diagnostics.HasError() {
			t.Fatalf("list diagnostics: %v", result.Diagnostics)
		}
		results = append(results, result)
	}
	return results
}

// displayNames reduces a stream to the domains it named, which is the part a
// practitioner sees and the part the identity is built from.
func displayNames(results []list.ListResult) []string {
	names := make([]string, 0, len(results))
	for _, result := range results {
		names = append(names, result.DisplayName)
	}
	return names
}

// TestList_NoLimitStreamsEveryOwnedDomain pins the unlimited case. Terraform
// sends 0 when it wants everything, and the clamp treats that as the collection
// length rather than as a limit of nothing.
func TestList_NoLimitStreamsEveryOwnedDomain(t *testing.T) {
	results := listDomains(t, twoOwnedDomainsBody, 0)

	if got := displayNames(results); len(got) != 2 {
		t.Fatalf("streamed %v, want both claimed domains", got)
	}
	if results[0].DisplayName != "tf-unit.example" || results[1].DisplayName != "tf-unit-second.example" {
		t.Errorf("streamed %v, want Jamf's own order preserved", displayNames(results))
	}

	var identity domainIdentityModel
	if diags := results[0].Identity.Get(context.Background(), &identity); diags.HasError() {
		t.Fatalf("reading back the identity: %v", diags)
	}
	if identity.Domain.ValueString() != "tf-unit.example" {
		t.Errorf("identity domain = %q, want the domain name", identity.Domain.ValueString())
	}

	var state DomainResourceModel
	if diags := results[0].Resource.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading back the resource: %v", diags)
	}
	if state.ID.ValueString() != "26917" {
		t.Errorf("resource id = %q, want the identifier hydrated for a bulk import", state.ID.ValueString())
	}
}

// TestList_LimitCutsTheStreamShort pins the clamp on the side that matters: a
// limit below the collection size has to stop the stream, not just size the
// slice.
func TestList_LimitCutsTheStreamShort(t *testing.T) {
	results := listDomains(t, twoOwnedDomainsBody, 1)

	if got := displayNames(results); len(got) != 1 || got[0] != "tf-unit.example" {
		t.Errorf("streamed %v, want exactly the first claimed domain", got)
	}
}

// TestList_LimitAboveTheCollectionStreamsEverything is the other side of the
// clamp: a limit larger than the collection must not be read as a promise to
// produce that many results.
func TestList_LimitAboveTheCollectionStreamsEverything(t *testing.T) {
	if got := displayNames(listDomains(t, twoOwnedDomainsBody, 50)); len(got) != 2 {
		t.Errorf("streamed %v, want both claimed domains", got)
	}
}

// TestList_ExcludesSharedDomains pins the filter. A shared domain is listed by
// the collection read exactly like an owned claim, but it cannot be changed or
// withdrawn, so importing one would leave an entry no destroy can remove.
func TestList_ExcludesSharedDomains(t *testing.T) {
	got := displayNames(listDomains(t, ownedAndSharedDomainsBody, 0))

	if len(got) != 1 || got[0] != "tf-unit.example" {
		t.Errorf("streamed %v, want only the domain this organization owns", got)
	}
}

// TestList_EmptyCollectionStreamsNoResults pins the empty branch. An
// organization that has claimed nothing is not an error, and the stream has to be
// set to the empty iterator rather than left nil — a nil stream reaches Terraform
// as neither results nor a diagnostic.
func TestList_EmptyCollectionStreamsNoResults(t *testing.T) {
	if got := displayNames(listDomains(t, emptyDomainListBody, 0)); len(got) != 0 {
		t.Errorf("streamed %v, want nothing", got)
	}
}

// TestList_OnlySharedDomainsStreamsNoResults is the interaction of the two
// branches above, and the one a filter added ahead of an early return can get
// wrong: every entry filtered out has to reach the same empty stream as an empty
// collection, not fall through with the stream unset.
func TestList_OnlySharedDomainsStreamsNoResults(t *testing.T) {
	body := `{"totalCount":1,"results":[` + sharedDomainBody + `]}`

	if got := displayNames(listDomains(t, body, 0)); len(got) != 0 {
		t.Errorf("streamed %v, want nothing importable", got)
	}
}
