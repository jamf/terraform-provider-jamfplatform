// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_search_domain

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
	return fmt.Errorf("SetDnsSearchDomainV1: %w", &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "PUT",
		URL:        "/securitycloud/v1/dns/search-domains",
		Errors: []jamfplatform.ErrorDetail{
			{Code: code, Field: field, Description: description},
		},
	})
}

// bareError builds an API failure carrying no structured detail at all — the shape
// the gateway returns for a route it does not serve.
func bareError(status int) error {
	return fmt.Errorf("ClearDnsSearchDomainV1: %w", &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "DELETE",
		URL:        "/securitycloud/v1/dns/search-domains",
		Body:       "404 page not found",
	})
}

// TestAppendWriteDiagnostics_MapsCodesToAttributes pins that a translated code
// points at the attribute an operator has to edit. The endpoint names no field on
// this code, so the path is the provider's contribution and the only thing saying
// where to look.
func TestAppendWriteDiagnostics_MapsCodesToAttributes(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantPath path.Path
		wantText string
	}{
		{
			name:     "invalid field points at domain_name",
			err:      apiError(400, codeInvalidField, "", "Invalid field value."),
			wantPath: path.Root("domain_name"),
			wantText: "1 to 253 characters",
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

// TestAppendWriteDiagnostics_WholeResourceCodes covers the code whose cause is not
// one attribute. An unentitled tenant is nothing an operator fixes by editing
// domain_name, so attaching the message to that attribute would send them to the
// wrong place.
func TestAppendWriteDiagnostics_WholeResourceCodes(t *testing.T) {
	for _, code := range []string{codeNotEntitled} {
		var diags diag.Diagnostics
		if !appendWriteDiagnostics(&diags, apiError(403, code, "", "Tenant is not entitled.")) {
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
	if appendWriteDiagnostics(&diags, bareError(404)) {
		t.Error("an API error carrying no structured detail must not be treated as handled")
	}
	if diags.HasError() {
		t.Errorf("unhandled errors must add no diagnostics of their own, got %v", diags)
	}
}

// TestAppendWriteDiagnostics_SearchDomainNotSetIsNotMapped keeps the read path's
// not-found handling the single owner of that case: an absence code reaching a
// write must fall through to the generic error rather than being reshaped here,
// which would turn an ordinary empty tenant into a validation message.
func TestAppendWriteDiagnostics_SearchDomainNotSetIsNotMapped(t *testing.T) {
	var diags diag.Diagnostics
	if appendWriteDiagnostics(&diags, apiError(404, codeSearchDomainNotSet, "", "Search domain not set.")) {
		t.Error("SEARCH_DOMAIN_NOT_SET must not be mapped on the write path")
	}
	if diags.HasError() {
		t.Errorf("expected no diagnostics, got %v", diags)
	}
}

// TestIsSearchDomainNotSet is the guard on Delete's one tolerated failure. Clearing
// answers 204 whether or not anything was set, so a 404 recognised by status alone
// would swallow the gateway's bare "page not found" for an unrouted path and report
// a destroy that cleared nothing.
func TestIsSearchDomainNotSet(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"documented absence code":         {err: apiError(404, codeSearchDomainNotSet, "", "Search domain not set."), want: true},
		"unrouted path, no code at all":   {err: bareError(404), want: false},
		"another code on the same status": {err: apiError(404, codeNotEntitled, "", "Tenant is not entitled."), want: false},
		"non-API error":                   {err: errors.New("connection reset"), want: false},
		"nil":                             {err: nil, want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isSearchDomainNotSet(tc.err); got != tc.want {
				t.Errorf("isSearchDomainNotSet = %v, want %v", got, tc.want)
			}
		})
	}
}
