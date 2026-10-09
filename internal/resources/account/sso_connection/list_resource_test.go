// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// secondOIDCSummaryBody is a second manageable connection, so a limit can be
// asserted against something it has to cut short.
const secondOIDCSummaryBody = `{
	"id": "con_unittest0005",
	"name": "tf-unit-oidc-two",
	"type": "OIDC",
	"region": "EU",
	"domains": ["tf-unit-two.example"],
	"enabledApplications": ["ACCOUNT"],
	"easyConfig": false,
	"syncUserProfileAttributesAtLogin": true,
	"ticketUrl": null,
	"tokenEndpointAuthMethod": "CLIENT_SECRET_POST"
}`

// secondOIDCConnectionBody is the single read for that second connection.
const secondOIDCConnectionBody = `{
	"id": "con_unittest0005",
	"name": "tf-unit-oidc-two",
	"type": "OIDC",
	"region": "EU",
	"clientId": "probe-client-id-two",
	"consentFlow": false,
	"easyConfig": false,
	"domains": ["tf-unit-two.example"],
	"scopes": "openid",
	"pkceAuthType": "DISABLED",
	"tokenEndpointAuthMethod": "CLIENT_SECRET_POST",
	"sendNonce": false,
	"syncUserProfileAttributesAtLogin": true,
	"aliasLoginHintToIdp": true,
	"attributeMap": "{\"mapping_mode\":\"bind_all\"}",
	"sessionInfo": {"maxSessionTimeInMinutes": null, "maxInactivityTimeInMinutes": null},
	"oidcOptions": {
		"issuerUrl": "idp-two.example",
		"authorizationEndpoint": "idp-two.example/authorize",
		"tokenEndpoint": "idp-two.example/token",
		"jwksUri": "idp-two.example/keys",
		"userInfoEndpoint": null
	}
}`

// twoManageableListBody is a collection of two connections both of which can be
// managed.
const twoManageableListBody = `{"totalCount":2,"results":[` + oidcSummaryBody + `,` + secondOIDCSummaryBody + `]}`

// listConnections runs List against a stub serving body from the collection and
// dispatching each single read by identifier, and returns everything the stream
// pushed along with the diagnostics it carried.
//
// The schemas come from ConnectionResource because a list request carries the
// managed resource's own schema and identity schema — the list resource declares
// only its (empty) configuration.
func listConnections(t *testing.T, body string, limit int64, includeResource bool) ([]list.ListResult, diag.Diagnostics) {
	t.Helper()

	singles := map[string]string{
		unitConnectionID:   oidcConnectionBody,
		"con_unittest0002": consentFlowConnectionBody,
		"con_unittest0005": secondOIDCConnectionBody,
	}

	return listConnectionsWith(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(req.URL.Path, "/connections") {
			_, _ = w.Write([]byte(body))
			return
		}
		id := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
		single, known := singles[id]
		if !known {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(notFoundBody))
			return
		}
		_, _ = w.Write([]byte(single))
	}, limit, includeResource)
}

// listConnectionsWith is the same run against a caller-supplied stub, for the
// cases listConnections cannot express: it dispatches every unknown identifier
// as not-found, and a read Jamf refuses some other way is a separate branch.
func listConnectionsWith(t *testing.T, handle stubHandler, limit int64, includeResource bool) ([]list.ListResult, diag.Diagnostics) {
	t.Helper()
	ctx := context.Background()

	r := &ConnectionListResource{client: newStubClient(t, handle)}

	managed := &ConnectionResource{}
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
		IncludeResource:        includeResource,
		Limit:                  limit,
		ResourceSchema:         schemaResp.Schema,
		ResourceIdentitySchema: identityResp.IdentitySchema,
	}, &stream)

	if stream.Results == nil {
		t.Fatal("List left the stream unset, so Terraform receives neither results nor a diagnostic")
	}

	var results []list.ListResult
	var diags diag.Diagnostics
	for result := range stream.Results {
		if result.Diagnostics.HasError() {
			t.Fatalf("list diagnostics: %v", result.Diagnostics)
		}
		diags.Append(result.Diagnostics...)
		results = append(results, result)
	}
	return results, diags
}

// displayNames reduces a stream to the connections it named, which is the part a
// practitioner sees and the part the identity is built from.
//
// A result with no name is skipped, because the framework's diagnostics-only
// stream is a single nameless result: when every connection was left out there is
// nothing to import but there is still something to say, and that carrier is not
// a connection.
func displayNames(results []list.ListResult) []string {
	names := make([]string, 0, len(results))
	for _, result := range results {
		if result.DisplayName == "" {
			continue
		}
		names = append(names, result.DisplayName)
	}
	return names
}

// TestList_NoLimitStreamsEveryManageableConnection pins the unlimited case.
// Terraform sends 0 when it wants everything, and the clamp treats that as the
// collection length rather than as a limit of nothing.
func TestList_NoLimitStreamsEveryManageableConnection(t *testing.T) {
	results, _ := listConnections(t, twoManageableListBody, 0, true)

	got := displayNames(results)
	if len(got) != 2 || got[0] != unitConnectionName || got[1] != "tf-unit-oidc-two" {
		t.Fatalf("streamed %v, want both connections in Jamf's own order", got)
	}

	var identity connectionIdentityModel
	if diags := results[0].Identity.Get(context.Background(), &identity); diags.HasError() {
		t.Fatalf("reading back the identity: %v", diags)
	}
	if identity.ID.ValueString() != unitConnectionID {
		t.Errorf("identity id = %q, want the connection identifier", identity.ID.ValueString())
	}

	var state ConnectionResourceModel
	if diags := results[0].Resource.Get(context.Background(), &state); diags.HasError() {
		t.Fatalf("reading back the resource: %v", diags)
	}
	if state.Name.ValueString() != unitConnectionName {
		t.Errorf("resource name = %q, want the stored name adopted for a bulk import", state.Name.ValueString())
	}
	if state.GenericOIDC == nil {
		t.Error("a hydrated result must carry the settings block, which only the single read supplies")
	}
	if len(state.EnabledProductNames.Elements()) != 2 {
		t.Errorf("enabled_product_names = %s, want the products from the collection entry", state.EnabledProductNames)
	}
	if len(state.EnabledProducts) != 0 {
		t.Errorf("enabled_products = %v, want nothing — no read returns the tenants", state.EnabledProducts)
	}
}

// TestList_LimitCutsTheStreamShort pins the clamp on the side that matters: a
// limit below the collection size has to stop the stream, not just size the
// slice.
func TestList_LimitCutsTheStreamShort(t *testing.T) {
	results, _ := listConnections(t, twoManageableListBody, 1, false)

	if got := displayNames(results); len(got) != 1 || got[0] != unitConnectionName {
		t.Errorf("streamed %v, want exactly the first connection", got)
	}
}

// TestList_LimitAboveTheCollectionStreamsEverything is the other side of the
// clamp: a limit larger than the collection must not be read as a promise to
// produce that many results.
func TestList_LimitAboveTheCollectionStreamsEverything(t *testing.T) {
	results, _ := listConnections(t, twoManageableListBody, 50, false)

	if got := displayNames(results); len(got) != 2 {
		t.Errorf("streamed %v, want both connections", got)
	}
}

// TestList_LeavesOutWhatCannotBeImported pins the filter, and the warnings that
// make the omission visible.
//
// Neither omitted class is distinguishable from the collection entry alone — it
// carries no consent flag, and a connection that cannot be read individually
// looks entirely ordinary in it — which is why each entry is read.
func TestList_LeavesOutWhatCannotBeImported(t *testing.T) {
	results, diags := listConnections(t, mixedConnectionListBody, 0, true)

	got := displayNames(results)
	if len(got) != 1 || got[0] != unitConnectionName {
		t.Fatalf("streamed %v, want only the connection that can be managed", got)
	}

	warnings := diags.Warnings()
	if len(warnings) != 2 {
		t.Fatalf("carried %d warnings, want one for each connection left out: %v", len(warnings), warnings)
	}
	joined := strings.Join([]string{warnings[0].Summary() + warnings[0].Detail(), warnings[1].Summary() + warnings[1].Detail()}, "\n")
	for _, want := range []string{"con_unittest0002", "con_unittest0003", "admin-consent", "cannot read"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings do not mention %q:\n%s", want, joined)
		}
	}
}

// TestList_EmptyCollectionStreamsNoResults pins the empty branch. An organization
// that holds no connections is not an error, and the stream has to be set to the
// empty iterator rather than left unset — an unset stream reaches Terraform as
// neither results nor a diagnostic.
func TestList_EmptyCollectionStreamsNoResults(t *testing.T) {
	results, diags := listConnections(t, emptyConnectionListBody, 0, false)

	if got := displayNames(results); len(got) != 0 {
		t.Errorf("streamed %v, want nothing", got)
	}
	if len(diags) != 0 {
		t.Errorf("carried diagnostics for an empty organization: %v", diags)
	}
}

// TestList_OnlyUnmanageableConnectionsStillReportsWhy is the interaction of the
// two branches above, and the one an early return can get wrong: every entry
// filtered out has to reach an empty stream, and the warnings explaining the
// omissions must survive rather than being dropped with the results they were
// attached to.
func TestList_OnlyUnmanageableConnectionsStillReportsWhy(t *testing.T) {
	body := `{"totalCount":1,"results":[` + ghostSummaryBody + `]}`

	results, diags := listConnections(t, body, 0, false)

	if got := displayNames(results); len(got) != 0 {
		t.Errorf("streamed %v, want nothing importable", got)
	}
	if len(diags.Warnings()) != 1 {
		t.Fatalf("carried %d warnings, want the one explaining the omission: %v", len(diags.Warnings()), diags)
	}
	if !strings.Contains(diags.Warnings()[0].Detail(), "con_unittest0003") {
		t.Errorf("warning %q does not name the connection left out", diags.Warnings()[0].Detail())
	}
}

// TestList_TransientReadFailureKeepsTheOtherConnections is the finding this
// stream shape exists for. A per-connection read refused for any reason other
// than not-found used to replace the whole stream, so one connection answering a
// 500 — or a rate limit — cost the operator every connection already read. The
// readable connections have to survive, and the one left out has to be named.
func TestList_TransientReadFailureKeepsTheOtherConnections(t *testing.T) {
	body := `{"totalCount":3,"results":[` + oidcSummaryBody + `,` + ghostSummaryBody + `,` + secondOIDCSummaryBody + `]}`

	singles := map[string]string{
		unitConnectionID:   oidcConnectionBody,
		"con_unittest0005": secondOIDCConnectionBody,
	}

	results, diags := listConnectionsWith(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(req.URL.Path, "/connections") {
			_, _ = w.Write([]byte(body))
			return
		}
		id := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
		single, known := singles[id]
		if !known {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(upstreamErrorBody))
			return
		}
		_, _ = w.Write([]byte(single))
	}, 0, true)

	got := displayNames(results)
	if len(got) != 2 || got[0] != unitConnectionName || got[1] != "tf-unit-oidc-two" {
		t.Fatalf("streamed %v, want both readable connections despite the failure between them", got)
	}

	warnings := diags.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("carried %d warnings, want the one naming the connection left out: %v", len(warnings), diags)
	}
	detail := warnings[0].Summary() + warnings[0].Detail()
	for _, want := range []string{"con_unittest0003", "tf-unit-ghost"} {
		if !strings.Contains(detail, want) {
			t.Errorf("warning does not mention %q:\n%s", want, detail)
		}
	}
}

// TestList_SkippingTheLastConnectionStillWarns pins why the diagnostics-only
// result trails the stream rather than riding on the first result. Attaching the
// skips to results[0] reports nothing when the connection left out is the last
// one read, because the framework has already consumed that result.
func TestList_SkippingTheLastConnectionStillWarns(t *testing.T) {
	body := `{"totalCount":2,"results":[` + oidcSummaryBody + `,` + ghostSummaryBody + `]}`

	results, diags := listConnections(t, body, 0, false)

	if got := displayNames(results); len(got) != 1 || got[0] != unitConnectionName {
		t.Fatalf("streamed %v, want the one readable connection", got)
	}
	if len(diags.Warnings()) != 1 {
		t.Fatalf("carried %d warnings, want the one for the connection skipped last: %v", len(diags.Warnings()), diags)
	}
	if !strings.Contains(diags.Warnings()[0].Detail(), "con_unittest0003") {
		t.Errorf("warning %q does not name the connection left out", diags.Warnings()[0].Detail())
	}
}
