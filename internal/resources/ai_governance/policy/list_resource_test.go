// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"
)

const (
	listPolicyKeptID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	listPolicyGoneID = "9b2c1f0e-1111-4222-8333-444455556666"
)

// listPolicies runs List against a stub whose listing names two policies and whose per-policy read
// answers goneStatus for the second, and returns the display names streamed and every diagnostic
// the stream carried. A read status of zero answers 200 for both.
func listPolicies(t *testing.T, goneStatus int, includeResource bool) ([]string, diag.Diagnostics, bool) {
	t.Helper()
	ctx := context.Background()

	stub := &policyStub{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/token":
			stub.serve(w, r)
		case strings.HasSuffix(r.URL.Path, "/policies"):
			writeJSONBody(w, http.StatusOK, map[string]any{
				"totalCount": 2,
				"results": []map[string]any{
					{"id": listPolicyKeptID, "name": "kept", "toolId": stubToolID, "schemaVersion": stubSchemaVersion, "status": "ACTIVE"},
					{"id": listPolicyGoneID, "name": "gone", "toolId": stubToolID, "schemaVersion": stubSchemaVersion, "status": "ACTIVE"},
				},
			})
		case strings.HasSuffix(r.URL.Path, listPolicyGoneID) && goneStatus != 0:
			writeAPIError(w, goneStatus, "POLICY_NOT_FOUND", "Policy not found")
		default:
			stub.serve(w, r)
		}
	}))
	t.Cleanup(server.Close)

	base := jamfplatform.NewClient(server.URL, "test-id", "test-secret",
		jamfplatform.WithRetryPolicy(0, 0, 0),
		jamfplatform.WithMinRequestInterval(0),
	)
	r := &PolicyListResource{client: aigovernance.New(base)}

	managed := &PolicyResource{}
	var schemaResp resource.SchemaResponse
	managed.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	var identityResp resource.IdentitySchemaResponse
	managed.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, &identityResp)

	var configResp list.ListResourceSchemaResponse
	r.ListResourceConfigSchema(ctx, list.ListResourceSchemaRequest{}, &configResp)
	configType := configResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	configValues := map[string]tftypes.Value{}
	for name, attrType := range configType.AttributeTypes {
		configValues[name] = tftypes.NewValue(attrType, nil)
	}

	var stream list.ListResultsStream
	r.List(ctx, list.ListRequest{
		Config:                 tfsdk.Config{Schema: configResp.Schema, Raw: tftypes.NewValue(configType, configValues)},
		IncludeResource:        includeResource,
		ResourceSchema:         schemaResp.Schema,
		ResourceIdentitySchema: identityResp.IdentitySchema,
	}, &stream)
	if stream.Results == nil {
		t.Fatal("List left the stream unset, so Terraform receives neither results nor a diagnostic")
	}

	var names []string
	var diags diag.Diagnostics
	failed := false
	for result := range stream.Results {
		if result.Diagnostics.HasError() {
			failed = true
		}
		diags.Append(result.Diagnostics...)
		if result.DisplayName != "" {
			names = append(names, result.DisplayName)
		}
	}
	return names, diags, failed
}

// TestList_PolicyArchivedMidQueryIsSkippedWithWarning pins the race the acceptance suite hit on a
// shared environment: the listing names a policy that another run archives before this query reads
// it. That costs the entry, with a warning, rather than failing the whole query.
func TestList_PolicyArchivedMidQueryIsSkippedWithWarning(t *testing.T) {
	names, diags, failed := listPolicies(t, http.StatusNotFound, true)

	if failed {
		t.Fatalf("a policy archived mid-query failed the query: %v", diags)
	}
	if len(names) != 1 || names[0] != "kept" {
		t.Fatalf("streamed %v, want only the policy that could still be read", names)
	}
	if len(diags) != 1 || diags[0].Severity() != diag.SeverityWarning ||
		!strings.Contains(diags[0].Detail(), listPolicyGoneID) {
		t.Fatalf("diagnostics = %v, want one warning naming %s", diags, listPolicyGoneID)
	}
}

// TestList_PolicyReadFailureOtherThanNotFoundStillFails keeps the skip narrow: only a missing
// policy is ordinary, so a refusal is still reported rather than hidden as a short result.
func TestList_PolicyReadFailureOtherThanNotFoundStillFails(t *testing.T) {
	_, diags, failed := listPolicies(t, http.StatusForbidden, true)

	if !failed {
		t.Fatalf("a 403 on the per-policy read was swallowed: %v", diags)
	}
}

// TestList_WithoutIncludeResourceNeverReadsPolicies confirms a plain query costs no per-policy read,
// so the skip path is unreachable and both entries stream.
func TestList_WithoutIncludeResourceNeverReadsPolicies(t *testing.T) {
	names, diags, failed := listPolicies(t, http.StatusNotFound, false)

	if failed || len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(names) != 2 {
		t.Fatalf("streamed %v, want both policies", names)
	}
}
