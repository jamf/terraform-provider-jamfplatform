// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_computer_search

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

	empty := &proclassic.AdvancedComputerSearchDisplayFields{DisplayField: &[]proclassic.AdvancedComputerSearchDisplayFieldsDisplayFieldItem{}}
	got, _ = flattenDisplayFields(context.Background(), empty)
	if !got.IsNull() {
		t.Errorf("empty wrapper should flatten to null set")
	}
}

func TestFlattenDisplayFields_Populated(t *testing.T) {
	wrapper := &proclassic.AdvancedComputerSearchDisplayFields{
		DisplayField: &[]proclassic.AdvancedComputerSearchDisplayFieldsDisplayFieldItem{
			{Name: new("Computer Name")},
			{Name: new("Serial Number")},
		},
	}
	got, diags := flattenDisplayFields(context.Background(), wrapper)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if got.IsNull() {
		t.Fatal("expected non-null set")
	}
	if n := len(got.Elements()); n != 2 {
		t.Errorf("expected 2 display fields, got %d", n)
	}
}

func TestAssignResourceModel_OmitsMatchedRecords(t *testing.T) {
	search := &proclassic.AdvancedComputerSearch{
		ID:   new(461),
		Name: new("lab macs"),
		Site: &proclassic.SiteObject{ID: new(-1), Name: new("NONE")},
		Criteria: &proclassic.AdvancedComputerSearchCriteria{
			Criterion: &[]proclassic.Criterion{
				{Name: new("Computer Name"), SearchType: new("like"), Value: new("lab"), Priority: new(0)},
			},
		},
		DisplayFields: &proclassic.AdvancedComputerSearchDisplayFields{
			DisplayField: &[]proclassic.AdvancedComputerSearchDisplayFieldsDisplayFieldItem{{Name: new("Computer Name")}},
		},
		// Computers (matched records) is intentionally ignored by the assigner.
		Computers: &proclassic.AdvancedComputerSearchComputers{},
	}

	state := &AdvancedComputerSearchResourceModel{SiteID: types.StringValue("-1")}
	diags := assignAdvancedComputerSearchResourceModel(context.Background(), state, search)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.ID.ValueString() != "461" {
		t.Errorf("id mismatch: %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "lab macs" {
		t.Errorf("name mismatch: %q", state.Name.ValueString())
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
