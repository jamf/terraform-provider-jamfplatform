// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// apiError builds the shape the group endpoints return: a status plus decoded
// error details carrying a code, a field and a description.
func apiError(status int, code, field, description string) error {
	return &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     http.MethodPut,
		URL:        "https://example.invalid/pro/v3/computer-groups/static-groups/1",
		Errors: []jamfplatform.ErrorDetail{{
			Code:        code,
			Field:       field,
			Description: description,
		}},
	}
}

// TestCodeClassification pins that each code is recognised, that the four
// classifiers do not overlap, and that a plain error is never classified.
func TestCodeClassification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "dependencies", err: apiError(http.StatusUnprocessableEntity, codeHasDependencies, "", "dependent: a policy"), want: codeHasDependencies},
		{name: "invalid device", err: apiError(http.StatusBadRequest, codeInvalidDevice, "assignments", "Unable to process group due to invalid device IDs"), want: codeInvalidDevice},
		{name: "duplicate name", err: apiError(http.StatusUnprocessableEntity, codeDuplicateField, "name", "Group named x already exists"), want: codeDuplicateField},
		{name: "site refused", err: apiError(http.StatusForbidden, codeInvalidPrivilege, "siteId", "Access denied: insufficient privileges"), want: codeInvalidPrivilege},
		{name: "a plain error", err: errors.New("boom")},
		{name: "nil", err: nil},
		{name: "an API error with no details", err: &jamfplatform.APIResponseError{StatusCode: http.StatusInternalServerError}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]bool{
				codeHasDependencies:  IsDependencyConflict(tc.err),
				codeInvalidDevice:    IsInvalidDevice(tc.err),
				codeDuplicateField:   IsDuplicateName(tc.err),
				codeInvalidPrivilege: IsSiteRefused(tc.err),
			}
			for code, matched := range got {
				if want := code == tc.want; matched != want {
					t.Errorf("%s matched = %v, want %v", code, matched, want)
				}
			}
		})
	}
}

// TestHasCodeIgnoresFreeText pins that a code spelled inside a description does
// not classify the error. The delete refusal names its dependents in free text,
// so a group an operator happened to name after a code would otherwise be
// misread — which is the whole reason the match reads the decoded details.
func TestHasCodeIgnoresFreeText(t *testing.T) {
	err := apiError(http.StatusUnprocessableEntity, codeHasDependencies, "",
		"The following items are dependent on this group and need to be updated: INVALID_DEVICE; ")
	if IsInvalidDevice(err) {
		t.Fatal("a code appearing only in the description classified the error")
	}
	if !IsDependencyConflict(err) {
		t.Fatal("the real code was not recognised")
	}
}

// TestDeleteDiagnostics pins that the dependency refusal is reported with the
// server's own list of dependents and with the ordering advice, and that any
// other failure still surfaces.
func TestDeleteDiagnostics(t *testing.T) {
	err := apiError(http.StatusUnprocessableEntity, codeHasDependencies, "",
		"The following items are dependent on this group and need to be updated: Install Chrome; ")
	got := DeleteDiagnostics(err, "static computer group")
	if !got.HasError() {
		t.Fatal("expected an error")
	}
	detail := got.Errors()[0].Detail()
	for _, want := range []string{"Install Chrome", "apply that", "destroy the group"} {
		if !strings.Contains(strings.ToLower(detail), strings.ToLower(want)) {
			t.Errorf("detail does not mention %q:\n%s", want, detail)
		}
	}

	other := DeleteDiagnostics(errors.New("connection reset"), "static computer group")
	if !other.HasError() {
		t.Fatal("a non-dependency failure must still surface")
	}
	if strings.Contains(other.Errors()[0].Summary(), "scoped to it") {
		t.Errorf("a non-dependency failure was reported as a dependency: %s", other.Errors()[0].Summary())
	}
}

// TestWriteDiagnosticsAnchoring pins which attribute each write failure is
// reported against, since that is what puts the message beside the thing the
// operator has to change.
func TestWriteDiagnosticsAnchoring(t *testing.T) {
	sitePath := path.Root("site_id")
	membersPath := path.Root("assigned_computer_ids")

	device := WriteDiagnostics(apiError(http.StatusBadRequest, codeInvalidDevice, "assignments", "bad ids"), "static computer group", sitePath, membersPath)
	if !device.HasError() {
		t.Fatal("expected an error")
	}
	if at := device.Errors()[0]; !strings.Contains(at.Detail(), "site") {
		t.Errorf("the membership message should explain the site cause too:\n%s", at.Detail())
	}

	site := WriteDiagnostics(apiError(http.StatusForbidden, codeInvalidPrivilege, "siteId", "denied"), "smart computer group", sitePath, path.Empty())
	if !site.HasError() {
		t.Fatal("expected an error")
	}
	if detail := site.Errors()[0].Detail(); !strings.Contains(detail, "misleading") {
		t.Errorf("the site message should warn that Jamf Pro mislabels this:\n%s", detail)
	}

	smart := WriteDiagnostics(apiError(http.StatusBadRequest, codeInvalidDevice, "assignments", "bad ids"), "smart computer group", sitePath, path.Empty())
	if !smart.HasError() {
		t.Fatal("expected an error")
	}
}
