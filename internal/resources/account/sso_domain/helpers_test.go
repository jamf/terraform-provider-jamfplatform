// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// apiError builds the wrapped error shape the SDK returns, so the test exercises
// the same unwrap path the CRUD callers do.
func apiError(status int, code, field, description string) error {
	return fmt.Errorf("CreateDomain: %w", &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "POST",
		URL:        "/sso/v1/domains",
		Errors: []jamfplatform.ErrorDetail{
			{Code: code, Field: field, Description: description},
		},
	})
}

// TestAppendClaimDiagnostics_MapsCodesToTheDomain checks each translated code
// lands on `domain` with a message naming the fix.
//
// The blank-name case sets namesValue false deliberately: that code fires when no
// domain name reached Jamf at all, so echoing the configured value back would
// contradict the diagnostic.
func TestAppendClaimDiagnostics_MapsCodesToTheDomain(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantText   string
		namesValue bool
	}{
		{
			name:       "duplicate claim names both causes",
			err:        apiError(409, codeConflict, "", "Domain is already added to your organization"),
			wantText:   "only one Jamf Account",
			namesValue: true,
		},
		{
			name:       "malformed name explains the accepted form",
			err:        apiError(400, codeBadRequest, "", "Invalid domain provided"),
			wantText:   "bare domain",
			namesValue: true,
		},
		{
			name:     "blank name points at the likely cause",
			err:      apiError(400, codeFieldValidation, "domain", "must not be blank"),
			wantText: "empty string",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			if !appendClaimDiagnostics(&diags, "corp.example", tc.err) {
				t.Fatalf("expected the error to be recognised, diagnostics: %v", diags)
			}
			if !diags.HasError() {
				t.Fatal("expected an error diagnostic")
			}
			withPath, ok := diags[0].(diag.DiagnosticWithPath)
			if !ok {
				t.Fatalf("diagnostic %T carries no path; the message must point at the attribute that caused it", diags[0])
			}
			if !withPath.Path().Equal(path.Root("domain")) {
				t.Errorf("path = %s, want %s", withPath.Path(), path.Root("domain"))
			}
			if !strings.Contains(diags[0].Detail(), tc.wantText) {
				t.Errorf("detail %q does not explain the fix (looking for %q)", diags[0].Detail(), tc.wantText)
			}
			if tc.namesValue && !strings.Contains(diags[0].Detail(), "corp.example") {
				t.Errorf("detail %q does not name the domain that failed", diags[0].Detail())
			}
		})
	}
}

// TestAppendClaimDiagnostics_UnrecognisedErrorsFallThrough is the important
// negative case: an unmapped code must be reported false so the caller emits the
// raw error instead of swallowing it.
func TestAppendClaimDiagnostics_UnrecognisedErrorsFallThrough(t *testing.T) {
	var diags diag.Diagnostics

	if appendClaimDiagnostics(&diags, "corp.example", apiError(500, "UPSTREAM_ERROR", "", "The request could not be completed")) {
		t.Error("an unmapped error code must not be treated as handled")
	}
	if appendClaimDiagnostics(&diags, "corp.example", errors.New("connection reset")) {
		t.Error("a non-API error must not be treated as handled")
	}
	if diags.HasError() {
		t.Errorf("unhandled errors must add no diagnostics of their own, got %v", diags)
	}
}

// TestAppendClaimDiagnostics_NotFoundIsNotMapped keeps the delete path the single
// owner of not-found handling: a 404 must fall through to the generic error
// rather than being reshaped into a claim diagnostic.
func TestAppendClaimDiagnostics_NotFoundIsNotMapped(t *testing.T) {
	var diags diag.Diagnostics
	if appendClaimDiagnostics(&diags, "corp.example", apiError(404, codeNotFound, "", "Unable to find domain by id: 99999999")) {
		t.Error("NOT_FOUND must not be mapped on the claim path")
	}
}

func TestFindDomain(t *testing.T) {
	domains := []account.Domain{
		{Domain: "one.example"},
		{Domain: "two.example", VerificationKey: "key-two"},
	}

	found := findDomain(domains, "two.example")
	if found == nil {
		t.Fatal("findDomain returned nil for a domain in the collection")
	}
	if found.VerificationKey != "key-two" {
		t.Errorf("findDomain returned the wrong element: %+v", found)
	}
	if findDomain(domains, "three.example") != nil {
		t.Error("findDomain must return nil for a domain the organization does not hold")
	}
	if findDomain(nil, "one.example") != nil {
		t.Error("findDomain must return nil for an empty collection")
	}
}

// TestFindDomain_IgnoresCase pins the import path. Jamf lower-cases a domain when
// it stores it, so `terraform import … Corp.Example` has to find the claim rather
// than being told the organization does not hold it.
func TestFindDomain_IgnoresCase(t *testing.T) {
	domains := []account.Domain{{Domain: "corp.example"}}

	if findDomain(domains, "Corp.Example") == nil {
		t.Error("findDomain must match a stored domain against a differently-cased request")
	}
}

func TestNumberValueOrNull(t *testing.T) {
	cases := map[string]struct {
		in       *json.Number
		wantNull bool
		want     string
	}{
		"nil is null":            {nil, true, ""},
		"empty is null":          {numberPtr(""), true, ""},
		"decimal passes through": {numberPtr("26917"), false, "26917"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := numberValueOrNull(tc.in)
			if got.IsNull() != tc.wantNull {
				t.Fatalf("IsNull() = %v, want %v", got.IsNull(), tc.wantNull)
			}
			if !tc.wantNull && got.ValueString() != tc.want {
				t.Errorf("value = %q, want %q", got.ValueString(), tc.want)
			}
		})
	}
}

// TestNumberValueOrNull_PreservesAnIdentifierTooLargeForAnInt pins the reason the
// identifier is carried as text. Jamf's schema calls it a string while the value
// it sends is a bare number, and parsing it into an integer would be a lossy
// round trip for no gain — the value is never sent back.
func TestNumberValueOrNull_PreservesAnIdentifierTooLargeForAnInt(t *testing.T) {
	const huge = "170141183460469231731687303715884105727"

	if got := numberValueOrNull(numberPtr(huge)).ValueString(); got != huge {
		t.Errorf("value = %q, want %q", got, huge)
	}
}

func TestTimeValueOrNull_NilIsNull(t *testing.T) {
	if got := timeValueOrNull(nil); !got.IsNull() {
		t.Errorf("timeValueOrNull(nil) = %q, want null", got.ValueString())
	}
}

func TestVerificationTXTRecord(t *testing.T) {
	got := verificationTXTRecord("verification-key-example")
	if want := "jamf-site-verification=verification-key-example"; got.ValueString() != want {
		t.Errorf("record = %q, want %q", got.ValueString(), want)
	}
	if !verificationTXTRecord("").IsNull() {
		t.Error("an absent verification key must produce a null record, not a bare prefix")
	}
}

func TestVerificationStatusDocs_NamesEveryStatus(t *testing.T) {
	assertDocumentsEveryStatus(t, verificationStatusDocs())
}
