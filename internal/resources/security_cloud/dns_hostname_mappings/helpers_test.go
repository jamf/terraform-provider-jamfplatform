// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_hostname_mappings

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

// apiError builds the wrapped error shape the SDK returns, so the tests exercise the
// same unwrap path the CRUD callers do. Every body below is copied from the
// production EU probe on 2026-08-29.
func apiError(status int, code, field, description string) error {
	return fmt.Errorf("ReplaceDnsCustomHostnameMappingsV1: %w", &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "PUT",
		URL:        "/securitycloud/v1/dns/custom-hostname-mappings",
		Errors: []jamfplatform.ErrorDetail{
			{Code: code, Field: field, Description: description},
		},
	})
}

// apiErrorWithoutDetails builds the response the endpoint gives a duplicate host
// name: a status with an empty errors array and nothing named.
func apiErrorWithoutDetails(status int) error {
	return fmt.Errorf("ReplaceDnsCustomHostnameMappingsV1: %w", &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "PUT",
		URL:        "/securitycloud/v1/dns/custom-hostname-mappings",
	})
}

// TestAppendWriteDiagnostics_MapsCodesToAttributes covers the codes whose cause is a
// value inside `mappings`. The raw bodies name no field — a bare "Invalid field
// value." with a null field, and a size violation that sometimes names an indexed
// field and sometimes nothing — so the attribute the diagnostic points at is the
// provider's contribution, and the thing worth pinning.
func TestAppendWriteDiagnostics_MapsCodesToAttributes(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantPath path.Path
		wantText string
	}{
		{
			name:     "invalid field points at mappings and lists what to check",
			err:      apiError(http.StatusBadRequest, codeInvalidField, "", "Invalid field value."),
			wantPath: path.Root("mappings"),
			wantText: "at least one of the two",
		},
		{
			name:     "size violation points at mappings and quotes both bounds",
			err:      apiError(http.StatusBadRequest, codeListSizeExceeded, "[0].aRecords", "size must be between 1 and 10"),
			wantPath: path.Root("mappings"),
			wantText: "1 to 500",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics

			if !appendWriteDiagnostics(&diags, tc.err) {
				t.Fatalf("expected the code to be recognised, diagnostics: %v", diags)
			}
			if !diags.HasError() {
				t.Fatalf("expected an error diagnostic, got %v", diags)
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
			if !strings.Contains(diags[0].Detail(), "Reported by Jamf Security Cloud") {
				t.Errorf("detail %q drops the server's own words", diags[0].Detail())
			}
		})
	}
}

// TestAppendWriteDiagnostics_NotEntitledIsWholeResource pins the deliberate exception
// to the rule above. Entitlement is not a value in `mappings` — no edit to the
// configuration fixes it — so attaching this one to an attribute would send the
// operator to change something that is already correct.
func TestAppendWriteDiagnostics_NotEntitledIsWholeResource(t *testing.T) {
	var diags diag.Diagnostics

	if !appendWriteDiagnostics(&diags, apiError(http.StatusForbidden, codeNotEntitled, "", "Tenant is not entitled to this feature.")) {
		t.Fatalf("%s was not recognised", codeNotEntitled)
	}
	if !diags.HasError() {
		t.Fatalf("%s produced no error diagnostic", codeNotEntitled)
	}
	if _, ok := diags[0].(diag.DiagnosticWithPath); ok {
		t.Errorf("%s must not attach to a single attribute", codeNotEntitled)
	}
	if !strings.Contains(diags[0].Detail(), "Contact Jamf") {
		t.Errorf("detail %q must say who can fix it", diags[0].Detail())
	}
}

// TestAppendWriteDiagnostics_NonAPIErrorFallsThrough is the negative case that keeps
// the caller's own reporting reachable: a transport failure is not this function's to
// reshape, and swallowing it would leave an apply failing with no message at all.
func TestAppendWriteDiagnostics_NonAPIErrorFallsThrough(t *testing.T) {
	var diags diag.Diagnostics

	if appendWriteDiagnostics(&diags, errors.New("connection reset by peer")) {
		t.Error("a non-API error must not be treated as handled")
	}
	if diags.HasError() {
		t.Errorf("an unhandled error must add no diagnostics of its own, got %v", diags)
	}
}

// TestAppendWriteDiagnostics_UnrecognisedCodeIsReported pins that a code this package
// has no translation for is still surfaced by code.
//
// The endpoint's error vocabulary is the whole `securitycloud.ApiErrorItemCode` enum
// and this package translates three of it, so a code arriving here is either one the
// spec grew or one the DNS surface started sending. Either way the code itself is the
// only diagnosable thing in the body, and dropping it silently turns a named server
// refusal into an unexplained apply failure.
func TestAppendWriteDiagnostics_UnrecognisedCodeIsReported(t *testing.T) {
	const code = "SOMETHING_THE_SPEC_GREW"
	var diags diag.Diagnostics

	if !appendWriteDiagnostics(&diags, apiError(http.StatusBadRequest, code, "", "Unexpected.")) {
		t.Fatal("an unrecognised code must be reported as handled, so the caller does not add a second diagnostic for it")
	}
	if !diags.HasError() {
		t.Fatal("an unrecognised code must produce an error diagnostic rather than being dropped")
	}

	named := false
	for _, d := range diags {
		if strings.Contains(d.Summary()+d.Detail(), code) {
			named = true
		}
	}
	if !named {
		t.Errorf("the diagnostic must name the code the server sent, got %v", diags)
	}
}

// TestAppendDuplicateHostnameHint_Guard pins each of the three conditions
// independently, so inverting any one of them fails.
//
// The hint stands in for a message the server does not send: a duplicate host name
// across two mappings answers 500 with an empty errors array, naming nothing. That
// makes the guard narrow on purpose — a 500 that does carry details has something
// better to say, and a 400 with no details is a different failure this wording would
// misattribute.
func TestAppendDuplicateHostnameHint_Guard(t *testing.T) {
	cases := map[string]struct {
		err      error
		wantHint bool
	}{
		"500 with no details is the duplicate host name": {
			err:      apiErrorWithoutDetails(http.StatusInternalServerError),
			wantHint: true,
		},
		"500 carrying details is left to the code mapping": {
			err:      apiError(http.StatusInternalServerError, codeInvalidField, "", "Invalid field value."),
			wantHint: false,
		},
		"400 with no details is a different failure": {
			err:      apiErrorWithoutDetails(http.StatusBadRequest),
			wantHint: false,
		},
		"non-API error is not this function's business": {
			err:      errors.New("connection reset by peer"),
			wantHint: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics

			if got := appendDuplicateHostnameHint(&diags, tc.err); got != tc.wantHint {
				t.Fatalf("appendDuplicateHostnameHint = %t, want %t (diags: %v)", got, tc.wantHint, diags)
			}
			if !tc.wantHint {
				if diags.HasError() {
					t.Errorf("no hint means no diagnostic, got %v", diags)
				}
				return
			}
			if !diags.HasError() {
				t.Fatal("expected an error diagnostic")
			}
			withPath, ok := diags[0].(diag.DiagnosticWithPath)
			if !ok || !withPath.Path().Equal(path.Root("mappings")) {
				t.Errorf("the hint must point at mappings, got %T %v", diags[0], diags[0])
			}
			if !strings.Contains(diags[0].Detail(), "repeated `hostname`") {
				t.Errorf("detail %q must name the cause the operator can check", diags[0].Detail())
			}
		})
	}
}
