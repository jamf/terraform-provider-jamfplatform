// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

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

// apiError builds the wrapped error shape the SDK returns, so the test exercises the
// same unwrap path the CRUD callers do. The details are passed through verbatim
// because the error-code spellings below are wire-confirmed against production EU on
// 2026-08-30 and a paraphrase would stop pinning them.
func apiError(status int, details ...jamfplatform.ErrorDetail) error {
	return fmt.Errorf("CreateZtnaAppV1: %w", &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "POST",
		URL:        "/securitycloud/v1/ztna/apps",
		Errors:     details,
	})
}

// detail is shorthand for one element of the `errors` array.
func detail(code, field, description string) jamfplatform.ErrorDetail {
	return jamfplatform.ErrorDetail{Code: code, Field: field, Description: description}
}

// TestAppendWriteDiagnostics_MapsCodes covers every code the switch translates,
// pinning both the attribute the diagnostic attaches to and the part of the wording
// that tells the operator what to do about it. NOT_ENTITLED is the one entry with no
// attribute path: no attribute caused it and no edit fixes it.
func TestAppendWriteDiagnostics_MapsCodes(t *testing.T) {
	cases := []struct {
		name               string
		err                error
		hasPredefinedAppID bool
		wantPath           *path.Path
		wantText           string
	}{
		{
			name:     "host name conflict points at hostnames",
			err:      apiError(http.StatusConflict, detail(codeHostnameConflict, "hostnames", "Hostname 'x.example.com' is already assigned to another App.")),
			wantPath: new(path.Root("hostnames")),
			wantText: "only one application across the whole tenant",
		},
		{
			name:     "bare IP conflict points at the address list",
			err:      apiError(http.StatusConflict, detail(codeBareIPsConflict, "", "Bare IPs conflict with another App.")),
			wantPath: new(path.Root("direct_ips_and_subnets")),
			wantText: "It does not say which range",
		},
		{
			name:     "unknown category names the display_name vocabulary",
			err:      apiError(http.StatusConflict, detail(codeMissingCategoryName, "", "Category [Nope] does not exist.")),
			wantPath: new(path.Root("category")),
			wantText: "`display_name`",
		},
		{
			name:     "missing user groups points at device_group_ids",
			err:      apiError(http.StatusConflict, detail(codeMissingUserGroups, "", "User groups [some-id] do not exist.")),
			wantPath: new(path.Root("device_group_ids")),
			wantText: "must exist before the application can reference it",
		},
		{
			name:     "unknown predefined app points at predefined_app_id",
			err:      apiError(http.StatusConflict, detail(codePredefinedAppNotFound, "", "Predefined app [zoom] does not exist.")),
			wantPath: new(path.Root("predefined_app_id")),
			wantText: "data source to look the ID up",
		},
		{
			name:               "conflict on a predefined app explains the one-per-template rule",
			err:                apiError(http.StatusConflict, detail(codeConflict, "", "Resource already exists.")),
			hasPredefinedAppID: true,
			wantPath:           new(path.Root("predefined_app_id")),
			wantText:           "only one access policy application per predefined application",
		},
		{
			name:     "not entitled carries no attribute path",
			err:      apiError(http.StatusForbidden, detail(codeNotEntitled, "", "Not entitled.")),
			wantText: "does not have the ZTNA surface",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics
			if !appendWriteDiagnostics(&diags, tc.err, tc.hasPredefinedAppID) {
				t.Fatalf("expected the error to be recognised, diagnostics: %v", diags)
			}
			if len(diags) != 1 {
				t.Fatalf("expected exactly one diagnostic, got %d: %v", len(diags), diags)
			}
			if !strings.Contains(diags[0].Detail(), tc.wantText) {
				t.Errorf("detail %q does not explain the fix (looking for %q)", diags[0].Detail(), tc.wantText)
			}
			withPath, hasPath := diags[0].(diag.DiagnosticWithPath)
			if tc.wantPath == nil {
				if hasPath {
					t.Errorf("diagnostic should not attach to a single attribute, got %s", withPath.Path())
				}
				return
			}
			if !hasPath {
				t.Fatalf("diagnostic %T carries no path; it must point at the attribute that caused it", diags[0])
			}
			if !withPath.Path().Equal(*tc.wantPath) {
				t.Errorf("path = %s, want %s", withPath.Path(), *tc.wantPath)
			}
		})
	}
}

// TestAppendWriteDiagnostics_UnrecognisedErrorsFallThrough is the important negative
// case: nothing recognised must report false so the caller emits the raw API error,
// which carries the HTTP status, the trace ID and the request URL as well as the
// detail. INVALID_FIELD is here on purpose — it is the code the live API returns for
// the host name mutual-exclusivity refusal and it has no case of its own, so this
// pins that it reaches the operator through the caller rather than being swallowed.
func TestAppendWriteDiagnostics_UnrecognisedErrorsFallThrough(t *testing.T) {
	var diags diag.Diagnostics

	if appendWriteDiagnostics(&diags, apiError(http.StatusInternalServerError, detail("SOMETHING_NEW", "", "boom")), false) {
		t.Error("an unmapped error code must not be treated as handled")
	}
	if appendWriteDiagnostics(&diags, apiError(http.StatusBadRequest, detail("INVALID_FIELD", "hostnames", "Hostnames have to be mutually exclusive.")), false) {
		t.Error("INVALID_FIELD has no case, so it must not be treated as handled")
	}
	if appendWriteDiagnostics(&diags, errors.New("connection reset"), false) {
		t.Error("a non-API error must not be treated as handled")
	}
	if diags.HasError() {
		t.Errorf("unhandled errors must add no diagnostics of their own, got %v", diags)
	}
}

// TestAppendWriteDiagnostics_ConflictOnlyTranslatedForPredefinedApps pins the gate on
// the generic CONFLICT code. A custom app cannot hit the one-app-per-template rule,
// so labelling its CONFLICT "Predefined application already in use" would send the
// operator after an attribute the configuration does not even set.
func TestAppendWriteDiagnostics_ConflictOnlyTranslatedForPredefinedApps(t *testing.T) {
	conflict := apiError(http.StatusConflict, detail(codeConflict, "", "Resource already exists."))

	var custom diag.Diagnostics
	if appendWriteDiagnostics(&custom, conflict, false) {
		t.Error("a CONFLICT on a custom app must fall through to the caller's generic error")
	}
	if custom.HasError() {
		t.Errorf("the skipped CONFLICT must add no diagnostics, got %v", custom)
	}

	var predefined diag.Diagnostics
	if !appendWriteDiagnostics(&predefined, conflict, true) {
		t.Fatal("a CONFLICT on a predefined app must be recognised")
	}
	if !strings.Contains(predefined[0].Summary(), "Predefined application already in use") {
		t.Errorf("summary = %q, want the predefined-app conflict", predefined[0].Summary())
	}
}

// TestAppendWriteDiagnostics_ReportsUntranslatedSiblings covers the mixed-detail body:
// every refusal probed on 2026-08-30 carried a single-element `errors` array, so this
// is defensive, but the callers only emit the raw error when nothing matched and a
// detail dropped because a sibling was recognised would be invisible.
func TestAppendWriteDiagnostics_ReportsUntranslatedSiblings(t *testing.T) {
	var diags diag.Diagnostics
	err := apiError(http.StatusConflict,
		detail(codeHostnameConflict, "hostnames", "Hostname 'x.example.com' is already assigned to another App."),
		detail("SOMETHING_NEW", "widgets", "Widgets are wrong."),
		detail(codeConflict, "", "Resource already exists."),
	)

	if !appendWriteDiagnostics(&diags, err, false) {
		t.Fatal("a body carrying a known code must be recognised")
	}
	if len(diags) != 3 {
		t.Fatalf("expected the translated detail plus both untranslated ones, got %d: %v", len(diags), diags)
	}
	joined := fmt.Sprintf("%v", diags)
	for _, want := range []string{"Widgets are wrong.", "SOMETHING_NEW", "widgets", "Resource already exists."} {
		if !strings.Contains(joined, want) {
			t.Errorf("diagnostics drop %q:\n%v", want, diags)
		}
	}
}

// TestReportedDetail covers the shapes an untranslated detail can carry, including
// the bare body with neither a code nor a field.
func TestReportedDetail(t *testing.T) {
	cases := []struct {
		name string
		in   jamfplatform.ErrorDetail
		want string
	}{
		{
			name: "code and field",
			in:   detail("INVALID_FIELD", "hostnames", "Hostnames have to be mutually exclusive."),
			want: "[INVALID_FIELD] hostnames: Hostnames have to be mutually exclusive.",
		},
		{
			name: "code only",
			in:   detail("CONFLICT", "", "Resource already exists."),
			want: "[CONFLICT] Resource already exists.",
		},
		{
			name: "neither",
			in:   detail("", "", "Something went wrong."),
			want: "Something went wrong.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reportedDetail(tc.in); got != tc.want {
				t.Errorf("reportedDetail = %q, want %q", got, tc.want)
			}
		})
	}
}
