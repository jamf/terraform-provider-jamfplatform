// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// TestAssignSmartComputerGroupResourceModel_NeverTouchesTheIdentifiers pins the
// fact behind this whole construct: the group read carries no identifier, so
// both are the caller's to set and a refresh must carry them through.
func TestAssignSmartComputerGroupResourceModel_NeverTouchesTheIdentifiers(t *testing.T) {
	state := SmartComputerGroupResourceModel{
		ID:         types.StringValue("41"),
		PlatformID: types.StringValue("11111111-2222-3333-4444-555555555555"),
	}
	description := "placeholder note"
	siteID := "7"

	assignSmartComputerGroupResourceModel(&state, &pro.SmartComputerGroupV3{
		Name:        "placeholder group",
		Description: &description,
		SiteID:      &siteID,
	})

	if state.ID.ValueString() != "41" {
		t.Errorf("id must survive the read, got %q", state.ID.ValueString())
	}
	if state.PlatformID.ValueString() != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("platform_id must survive the read, got %q", state.PlatformID.ValueString())
	}
	if state.Name.ValueString() != "placeholder group" {
		t.Errorf("name: got %q", state.Name.ValueString())
	}
	if state.Description.ValueString() != "placeholder note" {
		t.Errorf("description: got %q", state.Description.ValueString())
	}
	if state.SiteID.ValueString() != "7" {
		t.Errorf("site_id: got %q", state.SiteID.ValueString())
	}
	if state.Criteria != nil {
		t.Errorf("a group with no criteria must round-trip as a null list, got %v", state.Criteria)
	}
}

// TestAssignSmartComputerGroupResourceModel_NormalisesTheAbsentValues pins the
// two shapes Jamf Pro uses for "nothing here": a null description reads as the
// empty string the server stores, and a null site collapses to the single
// user-facing sentinel so a group with no site cannot flip between forms.
func TestAssignSmartComputerGroupResourceModel_NormalisesTheAbsentValues(t *testing.T) {
	var state SmartComputerGroupResourceModel
	assignSmartComputerGroupResourceModel(&state, &pro.SmartComputerGroupV3{Name: "placeholder group"})

	if state.Description.IsNull() || state.Description.ValueString() != "" {
		t.Errorf("an absent description must read as the empty string, got %#v", state.Description)
	}
	if state.SiteID.ValueString() != progroups.NoSiteID {
		t.Errorf("an absent site must read as %q, got %q", progroups.NoSiteID, state.SiteID.ValueString())
	}
}

// TestAssignSmartComputerGroupResourceModel_FlattensCriteria covers the read
// side of the criteria round-trip, including the parenthesis flags the wire
// carries as pointers.
func TestAssignSmartComputerGroupResourceModel_FlattensCriteria(t *testing.T) {
	opening := true
	wire := []pro.ComputerSmartGroupCriteriaV2{
		{Name: "Operating System Version", SearchType: "like", Value: "26.", AndOr: "and", Priority: 0, OpeningParen: &opening},
		{Name: "Model", SearchType: "like", Value: "MacBook", AndOr: "or", Priority: 1},
	}

	var state SmartComputerGroupResourceModel
	assignSmartComputerGroupResourceModel(&state, &pro.SmartComputerGroupV3{Name: "placeholder group", Criteria: &wire})

	if len(state.Criteria) != 2 {
		t.Fatalf("expected 2 criteria, got %d", len(state.Criteria))
	}
	if state.Criteria[0].Priority.ValueInt64() != 0 || state.Criteria[1].Priority.ValueInt64() != 1 {
		t.Errorf("priorities must come back in order, got %d and %d",
			state.Criteria[0].Priority.ValueInt64(), state.Criteria[1].Priority.ValueInt64())
	}
	if !state.Criteria[0].HasOpeningParenthesis.ValueBool() {
		t.Error("the opening parenthesis must survive the read")
	}
	if state.Criteria[1].HasClosingParenthesis.IsNull() || state.Criteria[1].HasClosingParenthesis.ValueBool() {
		t.Errorf("an absent parenthesis flag must read as false, got %#v", state.Criteria[1].HasClosingParenthesis)
	}
}

func TestAssignSmartComputerGroupDataSourceModel(t *testing.T) {
	state := SmartComputerGroupDataSourceModel{ID: types.StringValue("41")}
	assignSmartComputerGroupDataSourceModel(&state, &pro.SmartComputerGroupV3{Name: "placeholder group"})

	if state.ID.ValueString() != "41" {
		t.Errorf("id must survive the read, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "placeholder group" {
		t.Errorf("name: got %q", state.Name.ValueString())
	}
	if state.SiteID.ValueString() != progroups.NoSiteID {
		t.Errorf("an absent site must read as %q, got %q", progroups.NoSiteID, state.SiteID.ValueString())
	}
}

// TestSearchResultModel covers the plural mapping, including the rule that a
// group missing from the identifier bridge reports a null platform_id rather
// than failing the read.
func TestSearchResultModel(t *testing.T) {
	item := pro.SmartComputerGroupSearch{
		ID:              "41",
		Name:            "placeholder group",
		Description:     "placeholder note",
		SiteID:          "7",
		MembershipCount: 12,
	}

	got := searchResultModel(item, map[string]string{"41": "11111111-2222-3333-4444-555555555555"})
	if got.PlatformID.ValueString() != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("platform_id: got %q", got.PlatformID.ValueString())
	}
	if got.MemberCount.ValueInt64() != 12 {
		t.Errorf("member_count: got %d", got.MemberCount.ValueInt64())
	}
	if got.SiteID.ValueString() != "7" {
		t.Errorf("site_id: got %q", got.SiteID.ValueString())
	}

	unbridged := searchResultModel(item, nil)
	if !unbridged.PlatformID.IsNull() {
		t.Errorf("a group missing from the bridge must report a null platform_id, got %#v", unbridged.PlatformID)
	}
	if unbridged.ID.ValueString() != "41" {
		t.Errorf("the rest of the row must still be reported, got id %q", unbridged.ID.ValueString())
	}
}

// TestSearchResultModel_NormalisesTheNoSiteSentinel pins that an empty site on
// a search row collapses to the same sentinel the resource reports.
func TestSearchResultModel_NormalisesTheNoSiteSentinel(t *testing.T) {
	got := searchResultModel(pro.SmartComputerGroupSearch{ID: "41", Name: "placeholder group"}, nil)
	if got.SiteID.ValueString() != progroups.NoSiteID {
		t.Errorf("expected %q, got %q", progroups.NoSiteID, got.SiteID.ValueString())
	}
}

// TestWrittenSmartComputerGroupState_HoldsNoUnknownValue is the guard on the
// state a create records when the read that follows it fails. State cannot hold
// an unknown value, and the plan holds one for every Computed attribute nothing
// has filled in, so a copy of the plan would fail the apply a second time and
// leave the group unrecorded — the very thing writing state early exists to
// prevent.
func TestWrittenSmartComputerGroupState_HoldsNoUnknownValue(t *testing.T) {
	unset := criterion("Notes", "has", "urgent")
	unset.Priority = types.Int64Unknown()
	second := criterion("Model", "like", "MacBook")
	second.Priority = types.Int64Null()

	got := writtenSmartComputerGroupState(SmartComputerGroupResourceModel{
		ID:          types.StringValue("41"),
		PlatformID:  types.StringValue("11111111-2222-3333-4444-555555555555"),
		Name:        types.StringValue("placeholder group"),
		Description: types.StringUnknown(),
		SiteID:      types.StringValue(progroups.NoSiteID),
		Criteria:    []criteria.CriterionModel{unset, second},
	})

	if got.Description.IsUnknown() || got.Description.ValueString() != "" {
		t.Errorf("an unset description is the empty string Jamf Pro stores, got %v", got.Description)
	}
	for i, c := range got.Criteria {
		if c.Priority.IsUnknown() || c.Priority.IsNull() {
			t.Fatalf("criterion %d: priority must be resolved, got %v", i, c.Priority)
		}
		if c.Priority.ValueInt64() != int64(i) {
			t.Errorf("criterion %d: expected the position the builder emitted, got %d", i, c.Priority.ValueInt64())
		}
	}
}

// TestWrittenSmartComputerGroupState_KeepsTheAuthoredValues pins that resolving
// the unknowns changes nothing else. A configured description and an authored
// priority are what the write sent, and state disagreeing with the plan on
// either fails the apply.
func TestWrittenSmartComputerGroupState_KeepsTheAuthoredValues(t *testing.T) {
	authored := criterion("Notes", "has", "urgent")
	authored.Priority = types.Int64Value(0)
	plan := SmartComputerGroupResourceModel{
		ID:          types.StringValue("41"),
		Name:        types.StringValue("placeholder group"),
		Description: types.StringValue("placeholder note"),
		Criteria:    []criteria.CriterionModel{authored},
	}

	got := writtenSmartComputerGroupState(plan)

	if got.Description.ValueString() != "placeholder note" {
		t.Errorf("description: expected the configured value, got %v", got.Description)
	}
	if got.Criteria[0].Priority.ValueInt64() != 0 {
		t.Errorf("priority: expected the authored value, got %v", got.Criteria[0].Priority)
	}
	if plan.Criteria[0].Priority.IsNull() {
		t.Error("the caller's own criteria slice must not be written through")
	}
}

// TestWrittenSmartComputerGroupState_LeavesANullCriteriaListNull pins the one
// collection rule. A group configured with no criteria plans a null list, and
// recording an empty one instead disagrees with that plan.
func TestWrittenSmartComputerGroupState_LeavesANullCriteriaListNull(t *testing.T) {
	got := writtenSmartComputerGroupState(SmartComputerGroupResourceModel{
		ID:   types.StringValue("41"),
		Name: types.StringValue("placeholder group"),
	})
	if got.Criteria != nil {
		t.Errorf("expected no criteria collection at all, got %+v", got.Criteria)
	}
}
