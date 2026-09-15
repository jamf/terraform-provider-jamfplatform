// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"
)

// apiError builds the wrapped error shape the SDK returns, so the test exercises
// the same unwrap path the CRUD callers do. Every body below is copied from the
// EU sandbox probe on 2026-08-29.
func apiError(status int, code, field, description string) error {
	return fmt.Errorf("CreateDeviceGroupV1: %w", &jamfplatform.APIResponseError{
		StatusCode: status,
		Method:     "POST",
		URL:        "/securitycloud/v1/groups",
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
			name:     "duplicate name points at name and says the match is exact",
			err:      apiError(409, codeGroupAlreadyExists, "name", "Group with name 'Executives' already exists for customer 9282."),
			wantPath: path.Root("name"),
			wantText: "comparison is",
		},
		{
			name:     "reserved name points at name and says the group is unmanageable",
			err:      apiError(400, codeReservedGroupName, "name", "Group name 'Default Group' is reserved and cannot be used"),
			wantPath: path.Root("name"),
			wantText: "cannot be managed by",
		},
		{
			name:     "blank name points at name and explains whitespace-only counts",
			err:      apiError(400, codeInvalidField, "name", "Group name must not be blank"),
			wantPath: path.Root("name"),
			wantText: "only of whitespace",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var diags diag.Diagnostics

			if !appendWriteDiagnostics(&diags, tc.err) {
				t.Fatalf("expected the code to be recognised")
			}
			if !diags.HasError() {
				t.Fatalf("expected an error diagnostic, got %v", diags)
			}

			found := false
			for _, d := range diags {
				withPath, ok := d.(diag.DiagnosticWithPath)
				if !ok {
					continue
				}
				if withPath.Path().Equal(tc.wantPath) && strings.Contains(d.Detail(), tc.wantText) {
					found = true
				}
			}
			if !found {
				t.Errorf("no diagnostic at %s containing %q; got %v", tc.wantPath, tc.wantText, diags)
			}
		})
	}
}

// TestAppendWriteDiagnostics_NotEntitledIsNotAttributeScoped pins that the
// entitlement failure is a whole-resource problem rather than a field one — no
// attribute the user could change would fix it.
func TestAppendWriteDiagnostics_NotEntitledIsNotAttributeScoped(t *testing.T) {
	var diags diag.Diagnostics

	if !appendWriteDiagnostics(&diags, apiError(403, codeNotEntitled, "", "Not entitled.")) {
		t.Fatal("NOT_ENTITLED must be recognised")
	}
	for _, d := range diags {
		if _, ok := d.(diag.DiagnosticWithPath); ok {
			t.Errorf("NOT_ENTITLED must not be attached to an attribute path: %v", d)
		}
	}
}

// TestAppendWriteDiagnostics_BadPermissionsIsNotTranslated pins the deliberate
// omission. The gateway returns 403 BAD_PERMISSIONS both for a genuine privilege
// gap and for a route it does not serve at all — a control probe on a bogus path
// returned the identical body — so any wording chosen here would be wrong half the
// time. The raw error is the honest surface.
func TestAppendWriteDiagnostics_BadPermissionsIsNotTranslated(t *testing.T) {
	var diags diag.Diagnostics

	if appendWriteDiagnostics(&diags, apiError(403, codeBadPermissions, "", "The given token was not authorized to access the requested resource.")) {
		t.Error("BAD_PERMISSIONS must fall through to the caller's generic error")
	}
	if diags.HasError() {
		t.Errorf("BAD_PERMISSIONS must add no diagnostics, got %v", diags)
	}
}

// TestAppendWriteDiagnostics_UnrecognisedCodeFallsThrough keeps the caller's
// generic error path reachable for anything new the server starts returning.
func TestAppendWriteDiagnostics_UnrecognisedCodeFallsThrough(t *testing.T) {
	var diags diag.Diagnostics

	if appendWriteDiagnostics(&diags, apiError(500, "SOMETHING_NEW", "", "Unexpected.")) {
		t.Error("an unrecognised code must not report a match")
	}
	if diags.HasError() {
		t.Errorf("an unrecognised code must add no diagnostics, got %v", diags)
	}
}

// TestAppendWriteDiagnostics_NonAPIError guards the nil-unwrap path: a transport
// failure carries no error details at all.
func TestAppendWriteDiagnostics_NonAPIError(t *testing.T) {
	var diags diag.Diagnostics

	if appendWriteDiagnostics(&diags, errors.New("connection reset by peer")) {
		t.Error("a non-API error must not report a match")
	}
	if diags.HasError() {
		t.Errorf("a non-API error must add no diagnostics, got %v", diags)
	}
}

// TestGroupsNamedExactly_BuiltInGroupIsReturnedNotSwallowed is the regression
// guard for the singular data source's built-in refusal.
//
// The refusal is only reachable if the lookup hands the caller an entry with an
// empty ID. The SDK's ResolveDeviceGroupV2ByName does the opposite — it fails with
// "matched element has no id field" and discards the element — which made the
// refusal dead code and its acceptance test red. Matching locally must keep the
// id-less entry visible.
func TestGroupsNamedExactly_BuiltInGroupIsReturnedNotSwallowed(t *testing.T) {
	items := []securitycloud.GroupListItem{
		{ID: "a", Name: "Alpha"},
		{ID: "", Name: defaultGroupName},
	}

	got := groupsNamedExactly(items, defaultGroupName)

	if len(got) != 1 {
		t.Fatalf("got %d matches, want 1", len(got))
	}
	if got[0].ID != "" {
		t.Errorf("ID = %q, want the empty ID to survive so the caller can refuse it", got[0].ID)
	}
	if got[0].Name != defaultGroupName {
		t.Errorf("Name = %q, want %q", got[0].Name, defaultGroupName)
	}
}

// TestGroupsNamedExactly pins the comparison. Jamf Security Cloud's own
// uniqueness check is case-sensitive, so folding case here could return a
// different group than the one the operator named.
func TestGroupsNamedExactly(t *testing.T) {
	items := []securitycloud.GroupListItem{
		{ID: "a", Name: "Executives"},
		{ID: "b", Name: "executives"},
		{ID: "c", Name: "Field Staff"},
	}

	tests := []struct {
		name    string
		lookup  string
		wantIDs []string
	}{
		{name: "exact match", lookup: "Executives", wantIDs: []string{"a"}},
		{name: "case differs — a different group", lookup: "EXECUTIVES", wantIDs: nil},
		{name: "internal whitespace is significant", lookup: "Field  Staff", wantIDs: nil},
		{name: "no match", lookup: "Contractors", wantIDs: nil},
		{name: "empty name matches nothing", lookup: "", wantIDs: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := groupsNamedExactly(items, tc.lookup)

			if len(got) != len(tc.wantIDs) {
				t.Fatalf("got %d matches, want %d (%v)", len(got), len(tc.wantIDs), got)
			}
			for i, id := range tc.wantIDs {
				if got[i].ID != id {
					t.Errorf("matches[%d].ID = %q, want %q", i, got[i].ID, id)
				}
			}
		})
	}
}

// TestGroupsNamedExactly_DuplicateNamesAreAllReturned keeps the caller's
// ambiguity branch reachable. The server refuses to store a duplicate name, so
// this should never happen in practice — which is exactly why the helper must not
// quietly return the first of two rather than letting the caller say so.
func TestGroupsNamedExactly_DuplicateNamesAreAllReturned(t *testing.T) {
	items := []securitycloud.GroupListItem{
		{ID: "a", Name: "Executives"},
		{ID: "b", Name: "Executives"},
	}

	if got := groupsNamedExactly(items, "Executives"); len(got) != 2 {
		t.Errorf("got %d matches, want both so the caller can refuse an ambiguous name", len(got))
	}
}

// TestGroupsNamedExactly_NilList guards the empty-tenant path.
func TestGroupsNamedExactly_NilList(t *testing.T) {
	if got := groupsNamedExactly(nil, "Executives"); len(got) != 0 {
		t.Errorf("got %d matches from a nil list, want 0", len(got))
	}
}

// TestSortGroupsByName pins the order the plural data source and the list resource
// document. The endpoint accepts no sort parameter, so this is the provider's
// guarantee, not the server's — an unsorted input must come back sorted.
func TestSortGroupsByName(t *testing.T) {
	items := []securitycloud.GroupListItem{
		{ID: "c", Name: "Field Staff"},
		{ID: "", Name: defaultGroupName},
		{ID: "a", Name: "Executives"},
		{ID: "b", Name: "Contractors"},
	}

	got := sortGroupsByName(items)

	want := []string{"Contractors", defaultGroupName, "Executives", "Field Staff"}
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("sorted[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

// TestSortGroupsByName_DoesNotMutateInput guards the callers: the plural data
// source and the list resource both read from the same response, and the list
// resource layers manageableGroups on top. A sort in place would make one
// construct's behaviour depend on whether the other ran first.
func TestSortGroupsByName_DoesNotMutateInput(t *testing.T) {
	items := []securitycloud.GroupListItem{
		{ID: "b", Name: "Bravo"},
		{ID: "a", Name: "Alpha"},
	}

	sortGroupsByName(items)

	if items[0].Name != "Bravo" {
		t.Errorf("input was reordered: items[0].Name = %q, want %q", items[0].Name, "Bravo")
	}
}

// TestSortGroupsByName_IsCaseSensitive pins that the comparison does not fold
// case, for the same reason groupsNamedExactly does not: uniqueness on this API is
// case-sensitive, so "example" and "Example" are different groups and must have a
// stable order relative to each other rather than an arbitrary one.
func TestSortGroupsByName_IsCaseSensitive(t *testing.T) {
	items := []securitycloud.GroupListItem{
		{ID: "a", Name: "example"},
		{ID: "b", Name: "Example"},
	}

	got := sortGroupsByName(items)

	if got[0].Name != "Example" {
		t.Errorf("sorted[0].Name = %q, want %q — uppercase sorts first bytewise", got[0].Name, "Example")
	}
}

// TestSortGroupsByName_Empty guards the empty and nil inputs the callers can hand it.
func TestSortGroupsByName_Empty(t *testing.T) {
	if got := sortGroupsByName(nil); len(got) != 0 {
		t.Errorf("nil input produced %d items, want 0", len(got))
	}
	if got := sortGroupsByName([]securitycloud.GroupListItem{}); len(got) != 0 {
		t.Errorf("empty input produced %d items, want 0", len(got))
	}
}
