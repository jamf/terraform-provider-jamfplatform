// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// apiError builds the wrapped error shape the SDK returns, so the test exercises
// the same unwrap path the CRUD callers do.
func apiError(status int, code, field, description string) error {
	return fmt.Errorf("CreateDnsZoneV1: %w", &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "POST",
		URL:        "/api/securitycloud/v1/dns/zones",
		Errors: []jamfplatform.ErrorDetail{
			{Code: code, Field: field, Description: description},
		},
	})
}

func TestAppendWriteDiagnostics_MapsCodesToAttributes(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantPath path.Path
		wantText string
	}{
		{
			name:     "domain conflict points at domains",
			err:      apiError(409, codeDomainConflict, "", "Domain already in use."),
			wantPath: path.Root("domains"),
			wantText: "only one custom DNS zone",
		},
		{
			name:     "gateway not found points at authoritative_name_servers, not the zone",
			err:      apiError(422, codeGatewayNotFound, "", "Referenced gateway not found."),
			wantPath: path.Root("authoritative_name_servers"),
			wantText: "gateway must exist before the zone can reference it",
		},
		{
			name:     "restricted ip points at authoritative_name_servers",
			err:      apiError(422, codeNameServerIPRestricted, "", "Name server IP is restricted."),
			wantPath: path.Root("authoritative_name_servers"),
			wantText: "Reserved ranges",
		},
		{
			name:     "out of range ip points at authoritative_name_servers",
			err:      apiError(422, codeNameServerIPOutOfRange, "", "Name server IP out of range."),
			wantPath: path.Root("authoritative_name_servers"),
			wantText: "refuses this name server address",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			if !appendWriteDiagnostics(&diags, tc.err) {
				t.Fatalf("expected the error to be recognised, diagnostics: %v", diags)
			}
			if !diags.HasError() {
				t.Fatal("expected an error diagnostic")
			}
			withPath, ok := diags[0].(diag.DiagnosticWithPath)
			if !ok {
				t.Fatalf("diagnostic %T carries no path; the message must point at the attribute that caused it", diags[0])
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

// TestAppendWriteDiagnostics_WholeResourceCodes covers the codes whose cause is
// not one attribute, so they attach to the resource rather than a path.
func TestAppendWriteDiagnostics_WholeResourceCodes(t *testing.T) {
	for _, code := range []string{codeListSizeExceeded, codeNotEntitled} {
		var diags diag.Diagnostics
		if !appendWriteDiagnostics(&diags, apiError(400, code, "domains", "size must be between 1 and 100")) {
			t.Fatalf("%s was not recognised", code)
		}
		if !diags.HasError() {
			t.Fatalf("%s produced no error diagnostic", code)
		}
		if _, ok := diags[0].(diag.DiagnosticWithPath); ok {
			t.Errorf("%s must not attach to a single attribute", code)
		}
	}
}

// TestAppendWriteDiagnostics_UnrecognisedErrorsFallThrough is the important
// negative case: an unmapped code must be reported false so the caller emits the
// raw API error instead of swallowing it.
func TestAppendWriteDiagnostics_UnrecognisedErrorsFallThrough(t *testing.T) {
	var diags diag.Diagnostics

	if appendWriteDiagnostics(&diags, apiError(500, "SOMETHING_NEW", "", "boom")) {
		t.Error("an unmapped error code must not be treated as handled")
	}
	if appendWriteDiagnostics(&diags, errors.New("connection reset")) {
		t.Error("a non-API error must not be treated as handled")
	}
	if diags.HasError() {
		t.Errorf("unhandled errors must add no diagnostics of their own, got %v", diags)
	}
}

// TestAppendWriteDiagnostics_ZoneNotFoundIsNotMapped keeps the read path's
// not-found handling the single owner of that case: a 404 during a write must
// fall through to the generic error rather than being reshaped here.
func TestAppendWriteDiagnostics_ZoneNotFoundIsNotMapped(t *testing.T) {
	var diags diag.Diagnostics
	if appendWriteDiagnostics(&diags, apiError(404, "ZONE_NOT_FOUND", "", "DNS zone not found.")) {
		t.Error("ZONE_NOT_FOUND must not be mapped on the write path")
	}
}
