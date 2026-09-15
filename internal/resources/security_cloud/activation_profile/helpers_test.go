// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_profile

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// writeAPIError builds the wrapped error shape the SDK returns, so the tests
// exercise the same unwrap path the CRUD callers do. Details are variadic
// because the multi-detail response is the case worth pinning.
func writeAPIError(status int, details ...jamfplatform.ErrorDetail) error {
	return fmt.Errorf("CreateActivationProfileV1: %w", &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "POST",
		URL:        "/securitycloud/v1/activation-profiles",
		Errors:     details,
	})
}

// TestAppendWriteDiagnostics_NotEntitledIsNamed covers the Security Cloud staple:
// valid credentials on a tenant that does not hold the surface, which a bare 403
// hides.
func TestAppendWriteDiagnostics_NotEntitledIsNamed(t *testing.T) {
	var diags diag.Diagnostics
	err := writeAPIError(http.StatusForbidden, jamfplatform.ErrorDetail{
		Code:        codeNotEntitled,
		Description: "Tenant is not entitled to this feature.",
	})

	if !appendWriteDiagnostics(&diags, err) {
		t.Fatalf("the entitlement refusal must be recognised, diagnostics: %v", diags)
	}
	if len(diags) != 1 {
		t.Fatalf("expected one diagnostic, got %d: %v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Summary(), "not available for this tenant") {
		t.Errorf("summary %q does not name the entitlement gap", diags[0].Summary())
	}
	if !strings.Contains(diags[0].Detail(), "Tenant is not entitled to this feature.") {
		t.Errorf("detail drops the server's own description:\n%s", diags[0].Detail())
	}
}

// TestAppendWriteDiagnostics_StateConflictExplainsTheSoftDelete pins the code the
// SDK has no constant for. The remedy matters as much as the cause: a
// soft-deleted profile reads back healthy, so the operator has to be told to
// remove it from state rather than wait for a refresh to notice.
func TestAppendWriteDiagnostics_StateConflictExplainsTheSoftDelete(t *testing.T) {
	var diags diag.Diagnostics
	err := writeAPIError(http.StatusConflict, jamfplatform.ErrorDetail{
		Code:        codeStateConflict,
		Description: "Activation profile is deleted.",
	})

	if !appendWriteDiagnostics(&diags, err) {
		t.Fatalf("the state conflict must be recognised, diagnostics: %v", diags)
	}
	if len(diags) != 1 {
		t.Fatalf("expected one diagnostic, got %d: %v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Summary(), "already been deleted") {
		t.Errorf("summary %q does not name the soft delete", diags[0].Summary())
	}
	for _, want := range []string{"terraform state rm", "Activation profile is deleted."} {
		if !strings.Contains(diags[0].Detail(), want) {
			t.Errorf("detail does not mention %q:\n%s", want, diags[0].Detail())
		}
	}
}

// TestAppendWriteDiagnostics_UnrecognisedErrorsFallThrough is the important
// negative case: an unmapped code and a transport error must both be reported
// false with no diagnostics of their own, so the caller emits the raw error
// rather than the provider swallowing it.
func TestAppendWriteDiagnostics_UnrecognisedErrorsFallThrough(t *testing.T) {
	var diags diag.Diagnostics

	unknown := writeAPIError(http.StatusInternalServerError, jamfplatform.ErrorDetail{
		Code:        "SOMETHING_NEW",
		Description: "boom",
	})
	if appendWriteDiagnostics(&diags, unknown) {
		t.Error("an unmapped error code must not be treated as handled")
	}
	if appendWriteDiagnostics(&diags, errors.New("boom")) {
		t.Error("a non-API error must not be treated as handled")
	}
	if len(diags) != 0 {
		t.Errorf("unhandled errors must add no diagnostics of their own, got %v", diags)
	}
}

// TestAppendWriteDiagnostics_MultiDetailSurfacesEveryDetail is the drop hazard.
//
// Recognising one detail takes the caller's generic fallback away, so any detail
// this function does not translate has to be surfaced here or it is reported
// nowhere at all — a second problem hidden behind the first.
func TestAppendWriteDiagnostics_MultiDetailSurfacesEveryDetail(t *testing.T) {
	var diags diag.Diagnostics
	err := writeAPIError(http.StatusForbidden,
		jamfplatform.ErrorDetail{Code: codeNotEntitled, Description: "Tenant is not entitled to this feature."},
		jamfplatform.ErrorDetail{Code: "SOMETHING_NEW", Description: "Quota exceeded."},
	)

	if !appendWriteDiagnostics(&diags, err) {
		t.Fatalf("a response carrying a recognised code must be handled, diagnostics: %v", diags)
	}
	if len(diags) != 2 {
		t.Fatalf("expected both details to surface, got %d: %v", len(diags), diags)
	}

	joined := diags[0].Summary() + diags[0].Detail() + diags[1].Summary() + diags[1].Detail()
	for _, want := range []string{"not available for this tenant", "SOMETHING_NEW", "Quota exceeded."} {
		if !strings.Contains(joined, want) {
			t.Errorf("the diagnostics drop %q:\n%v", want, diags)
		}
	}
}
