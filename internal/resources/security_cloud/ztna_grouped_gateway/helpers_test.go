// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// apiError builds the wrapped error shape the SDK returns, so the test exercises
// the same unwrap path the CRUD callers do.
func apiError(status int, code, description string) error {
	return fmt.Errorf("CreateZtnaGroupedGatewayV1: %w", &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "POST",
		URL:        "/api/securitycloud/v1/ztna/grouped-gateways",
		Errors: []jamfplatform.ErrorDetail{
			{Code: code, Description: description},
		},
	})
}

func TestAppendWriteDiagnostics_MapsMembershipCodes(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantPath path.Path
		wantText string
	}{
		{
			name:     "mixed tunnel types points at the member list",
			err:      apiError(422, codeMixedTunnelTypes, "All member gateways must have the same tunnel type (all IPSec or all non-IPSec)."),
			wantPath: path.Root("gateway_ids"),
			wantText: "same form",
		},
		{
			name:     "shared member points at the member list",
			err:      apiError(422, codeSharedGatewayMember, "Grouped Gateway members must be dedicated gateways."),
			wantPath: path.Root("gateway_ids"),
			wantText: "shared_gateways",
		},
		{
			// A member that does not exist and a member owned by someone else share
			// one code, so the diagnostic has to offer both readings.
			name:     "missing member points at the member list",
			err:      apiError(422, codeGatewayNotFound, "A gateway in gatewayIds does not exist or is not accessible to this customer."),
			wantPath: path.Root("gateway_ids"),
			wantText: "belongs to another customer",
		},
		{
			name:     "unknown id points at tenant_ids",
			err:      apiError(400, codeBadRequest, "No mapping found for one of the supplied ids"),
			wantPath: path.Root("tenant_ids"),
			wantText: "same organization",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			if !appendWriteDiagnostics(&diags, tc.err) {
				t.Fatalf("expected the error to be recognised, diagnostics: %v", diags)
			}
			withPath, ok := diags[0].(diag.DiagnosticWithPath)
			if !ok {
				t.Fatalf("diagnostic %T carries no path; it must point at the attribute that caused it", diags[0])
			}
			if !withPath.Path().Equal(tc.wantPath) {
				t.Errorf("path = %s, want %s", withPath.Path(), tc.wantPath)
			}
			if !strings.Contains(diags[0].Detail(), tc.wantText) {
				t.Errorf("detail %q does not explain the fix (looking for %q)", diags[0].Detail(), tc.wantText)
			}
		})
	}
}

// TestAppendWriteDiagnostics_UnrecognisedErrorsFallThrough is the important
// negative case: an unmapped code must be reported false so the caller emits the
// raw API error instead of swallowing it.
func TestAppendWriteDiagnostics_UnrecognisedErrorsFallThrough(t *testing.T) {
	var diags diag.Diagnostics

	if appendWriteDiagnostics(&diags, apiError(500, "SOMETHING_NEW", "boom")) {
		t.Error("an unmapped error code must not be treated as handled")
	}
	if appendWriteDiagnostics(&diags, errors.New("connection reset")) {
		t.Error("a non-API error must not be treated as handled")
	}
	if diags.HasError() {
		t.Errorf("unhandled errors must add no diagnostics of their own, got %v", diags)
	}
}

func TestAppendDeleteDiagnostics_ConflictExplainsOrdering(t *testing.T) {
	var diags diag.Diagnostics
	err := fmt.Errorf("DeleteZtnaGroupedGatewayV1: %w", &jamfplatform.APIResponseError{
		StatusCode: http.StatusConflict,
		Method:     "DELETE",
		URL:        "/api/securitycloud/v1/ztna/grouped-gateways/b6ed74d2",
	})

	if !appendDeleteDiagnostics(&diags, err) {
		t.Fatalf("a 409 on delete must be recognised, diagnostics: %v", diags)
	}
	if !strings.Contains(diags[0].Detail(), "separate apply") {
		t.Errorf("detail does not explain the ordering fix:\n%s", diags[0].Detail())
	}
}

func TestAppendDeleteDiagnostics_OtherStatusesFallThrough(t *testing.T) {
	var diags diag.Diagnostics
	if appendDeleteDiagnostics(&diags, apiError(http.StatusNotFound, "NOT_FOUND", "gone")) {
		t.Error("a 404 must not be handled here")
	}
	if appendDeleteDiagnostics(&diags, errors.New("connection reset")) {
		t.Error("a non-API error must not be handled here")
	}
}

// TestAppendWriteDiagnostics_NotEntitledIsNotAttributeScoped pins the one arm of
// the write switch that had no test. It was the only untested codeNotEntitled arm
// in the namespace — device_group, dns_zone, ztna_gateway and uem_connect all pin
// theirs — and nothing live can cover it, because an entitled tenant cannot be
// un-entitled. Hence a synthesized body.
func TestAppendWriteDiagnostics_NotEntitledIsNotAttributeScoped(t *testing.T) {
	var diags diag.Diagnostics

	if !appendWriteDiagnostics(&diags, apiError(403, codeNotEntitled, "Not entitled.")) {
		t.Fatal("NOT_ENTITLED must be recognised")
	}
	if !diags.HasError() {
		t.Fatal("NOT_ENTITLED must produce an error diagnostic")
	}
	for _, d := range diags {
		if _, ok := d.(diag.DiagnosticWithPath); ok {
			t.Errorf("NOT_ENTITLED must not be attached to an attribute path: %v", d)
		}
	}
}

// TestAppendWriteDiagnostics_MixedDedicatedIPs pins the member constraint whose
// server message names a wire field with no counterpart on this schema, so the
// translation is the only thing telling the operator what to change. It has to
// name the `ipsec` block, which is what actually sets a gateway's form, and the
// gateway data source, which is how the form of an unmanaged member is read —
// and it must not restate the wire field name.
func TestAppendWriteDiagnostics_MixedDedicatedIPs(t *testing.T) {
	var diags diag.Diagnostics

	if !appendWriteDiagnostics(&diags, apiError(422, codeMixedDedicatedIPs,
		"All member gateways must have the same dedicatedIps.enabled value.")) {
		t.Fatal("MIXED_DEDICATED_IPS_TYPES must be recognised")
	}

	found := false
	for _, d := range diags {
		withPath, ok := d.(diag.DiagnosticWithPath)
		if !ok {
			continue
		}
		if withPath.Path().Equal(path.Root("gateway_ids")) &&
			strings.Contains(d.Detail(), "`ipsec`") &&
			strings.Contains(d.Detail(), "jamfplatform_security_cloud_ztna_gateway") {
			found = true
		}
	}
	if !found {
		t.Errorf("want a gateway_ids diagnostic naming the ipsec block and the gateway data source; got %v", diags)
	}
	for _, d := range diags {
		authored, _, _ := strings.Cut(d.Detail(), "Reported by Jamf Security Cloud:")
		if strings.Contains(authored, "dedicatedIps") ||
			strings.Contains(authored, "dedicated_egress_ips_enabled") {
			t.Errorf("the diagnostic must not name a wire field or a nonexistent attribute: %v", d)
		}
	}
}

// TestAppendDeleteDiagnostics_NamesAccessPolicies pins the referrer the operator
// cannot resolve by reordering applies.
//
// The provider does not manage ZTNA access policies, so a grouped gateway referenced by one
// has no Terraform-visible dependency edge and no apply ordering releases it. The
// diagnostic used to list only zones and grouped gateways, sending the operator to
// check two things that were not the cause.
func TestAppendDeleteDiagnostics_NamesAccessPolicies(t *testing.T) {
	var diags diag.Diagnostics

	if !appendDeleteDiagnostics(&diags, apiError(http.StatusConflict, "", "")) {
		t.Fatal("a 409 must be recognised")
	}
	if !strings.Contains(diags[0].Detail(), "access polic") {
		t.Errorf("delete diagnostic must name access policies; got %q", diags[0].Detail())
	}
}

// TestAppendDeleteDiagnostics_SurfacesDetailWhenPresent pins that a structured
// detail is passed through rather than contradicted.
//
// The probed referrer cases answered with a bare 409, but the bundled spec
// documents per-referrer codes with remediation text. If the endpoint starts
// sending one, the operator must see it — the old wording asserted the body said
// nothing while never reading it.
func TestAppendDeleteDiagnostics_SurfacesDetailWhenPresent(t *testing.T) {
	var diags diag.Diagnostics

	if !appendDeleteDiagnostics(&diags, apiError(http.StatusConflict, "REFERENCED_BY_ACCESS_POLICIES",
		"Disconnect this from all policies, then try again.")) {
		t.Fatal("a 409 must be recognised")
	}
	if !strings.Contains(diags[0].Detail(), "Disconnect this from all policies") {
		t.Errorf("delete diagnostic must surface the server's own remedy; got %q", diags[0].Detail())
	}
}

// TestReportedDetails covers the shapes the delete conflict can carry, including
// the bare 409 the wire has actually been seen to send.
func TestReportedDetails(t *testing.T) {
	bare := jamfplatform.AsAPIError(apiError(http.StatusConflict, "", ""))
	if got := reportedDetails(bare); got != "" {
		t.Errorf("a detail with no description must add nothing, got %q", got)
	}

	withDetail := jamfplatform.AsAPIError(apiError(http.StatusConflict, "SOME_CODE", "Because reasons."))
	if got := reportedDetails(withDetail); !strings.Contains(got, "Because reasons.") {
		t.Errorf("reportedDetails = %q, want it to carry the description", got)
	}
}
