// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_user_search

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestFlattenDisplayFields_NilToNullSet(t *testing.T) {
	got, diags := flattenDisplayFields(context.Background(), nil)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !got.IsNull() {
		t.Errorf("nil wrapper should flatten to null set")
	}
}

func TestFlattenDisplayFields_Populated(t *testing.T) {
	wrapper := &proclassic.AdvancedUserSearchDisplayFields{
		DisplayField: &[]proclassic.AdvancedUserSearchDisplayFieldsDisplayFieldItem{
			{Name: new("Full Name")},
			{Name: new("Email Address")},
		},
	}
	got, diags := flattenDisplayFields(context.Background(), wrapper)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if got.IsNull() || len(got.Elements()) != 2 {
		t.Errorf("expected 2 display fields, got %v", got)
	}
}

func TestAssignResourceModel_OmitsMatchedRecords(t *testing.T) {
	search := &proclassic.AdvancedUserSearch{
		ID:   new(463),
		Name: new("vips"),
		Site: &proclassic.SiteObject{ID: new(-1), Name: new("NONE")},
		Criteria: &proclassic.AdvancedUserSearchCriteria{
			Criterion: &[]proclassic.Criterion{
				{Name: new("Full Name"), SearchType: new("like"), Value: new("a"), Priority: new(0)},
			},
		},
		DisplayFields: &proclassic.AdvancedUserSearchDisplayFields{
			DisplayField: &[]proclassic.AdvancedUserSearchDisplayFieldsDisplayFieldItem{{Name: new("Full Name")}},
		},
		// Users (matched records) is intentionally ignored by the assigner.
		Users: &proclassic.AdvancedUserSearchUsers{},
	}

	state := &AdvancedUserSearchResourceModel{SiteID: types.StringValue("-1")}
	diags := assignAdvancedUserSearchResourceModel(context.Background(), state, search)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.ID.ValueString() != "463" {
		t.Errorf("id mismatch: %q", state.ID.ValueString())
	}
	// Sentinel site (id -1): derived name nulls, not the flaky server echo.
	if !state.SiteName.IsNull() {
		t.Errorf("site_name should be null on the sentinel, got %q", state.SiteName.ValueString())
	}
	if len(state.Criteria) != 1 {
		t.Errorf("expected 1 criterion, got %d", len(state.Criteria))
	}
	if state.DisplayFields.IsNull() || len(state.DisplayFields.Elements()) != 1 {
		t.Errorf("expected 1 display field, got %v", state.DisplayFields)
	}
}
