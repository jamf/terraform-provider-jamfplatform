// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// wireCriterion builds one criterion as the read reports it.
func wireCriterion(name, searchType, value string, priority int) pro.MobileDeviceSmartGroupCriteriaV2 {
	no := false
	return pro.MobileDeviceSmartGroupCriteriaV2{
		Name:         name,
		SearchType:   searchType,
		Value:        value,
		AndOr:        "and",
		Priority:     priority,
		OpeningParen: &no,
		ClosingParen: &no,
	}
}

func TestAssignResourceModel_MapsTheRead(t *testing.T) {
	state := SmartMobileDeviceGroupResourceModel{
		Description: types.StringUnknown(),
	}
	assignSmartMobileDeviceGroupResourceModel(&state, &pro.SmartGroupDetailV2{
		Count:            42,
		GroupID:          "19",
		GroupName:        "Supervised iPads",
		GroupDescription: "Loan pool",
		SiteID:           "7",
		Criteria: []pro.MobileDeviceSmartGroupCriteriaV2{
			wireCriterion("Model", "like", "iPad", 0),
		},
	})

	if state.ID.ValueString() != "19" {
		t.Errorf("id: got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Supervised iPads" {
		t.Errorf("name: got %q", state.Name.ValueString())
	}
	if state.Description.ValueString() != "Loan pool" {
		t.Errorf("description: got %q", state.Description.ValueString())
	}
	if state.SiteID.ValueString() != "7" {
		t.Errorf("site_id: got %q", state.SiteID.ValueString())
	}
	if len(state.Criteria) != 1 || state.Criteria[0].Name.ValueString() != "Model" {
		t.Errorf("criteria did not flatten: %+v", state.Criteria)
	}
	// The read carries a membership count and the resource model has nowhere to
	// put it, on purpose.
	if !state.PlatformID.IsNull() {
		t.Error("the state builder must leave platform_id to its caller")
	}
}

// TestAssignResourceModel_NoSiteCollapsesToTheSentinel keeps a group with no site
// from flipping between the two spellings the endpoints use for it, which would
// surface only on the no-site path and read as a flake.
func TestAssignResourceModel_NoSiteCollapsesToTheSentinel(t *testing.T) {
	for name, wire := range map[string]string{"sentinel": progroups.NoSiteID, "empty": ""} {
		state := SmartMobileDeviceGroupResourceModel{}
		assignSmartMobileDeviceGroupResourceModel(&state, &pro.SmartGroupDetailV2{GroupID: "1", SiteID: wire})
		if state.SiteID.ValueString() != progroups.NoSiteID {
			t.Errorf("%s site: got %q, want %q", name, state.SiteID.ValueString(), progroups.NoSiteID)
		}
	}
}

// TestAssignResourceModel_KeepsAnAuthoredEmptyDescription covers the one case a
// plain copy gets wrong: an author who set the description to an empty string
// means it, and the read cannot tell that from never having set it.
func TestAssignResourceModel_KeepsAnAuthoredEmptyDescription(t *testing.T) {
	state := SmartMobileDeviceGroupResourceModel{Description: types.StringValue("")}
	assignSmartMobileDeviceGroupResourceModel(&state, &pro.SmartGroupDetailV2{GroupID: "1", GroupDescription: ""})
	if state.Description.IsNull() {
		t.Error("an authored empty description must survive the read")
	}

	unset := SmartMobileDeviceGroupResourceModel{Description: types.StringUnknown()}
	assignSmartMobileDeviceGroupResourceModel(&unset, &pro.SmartGroupDetailV2{GroupID: "1", GroupDescription: ""})
	if !unset.Description.IsNull() {
		t.Errorf("an unset description should settle as null, got %q", unset.Description.ValueString())
	}
}

// TestAssignResourceModel_EmptyCriteriaKeepTheAuthoredShape is the difference
// between `criteria = []` settling and never settling. The read reports no
// criteria the same way either way, so the authored shape decides.
func TestAssignResourceModel_EmptyCriteriaKeepTheAuthoredShape(t *testing.T) {
	authoredEmpty := SmartMobileDeviceGroupResourceModel{Criteria: []criteria.CriterionModel{}}
	assignSmartMobileDeviceGroupResourceModel(&authoredEmpty, &pro.SmartGroupDetailV2{GroupID: "1"})
	if authoredEmpty.Criteria == nil {
		t.Error("an authored empty list must stay an empty list")
	}
	if len(authoredEmpty.Criteria) != 0 {
		t.Errorf("expected no criteria, got %d", len(authoredEmpty.Criteria))
	}

	absent := SmartMobileDeviceGroupResourceModel{}
	assignSmartMobileDeviceGroupResourceModel(&absent, &pro.SmartGroupDetailV2{GroupID: "1"})
	if absent.Criteria != nil {
		t.Error("an absent list must stay absent")
	}
}

// TestAssignResourceModel_KeepsTheIdentifierWhenTheReadOmitsIt guards the one
// path that would otherwise lose the resource id: a response without groupId.
func TestAssignResourceModel_KeepsTheIdentifierWhenTheReadOmitsIt(t *testing.T) {
	state := SmartMobileDeviceGroupResourceModel{ID: types.StringValue("19")}
	assignSmartMobileDeviceGroupResourceModel(&state, &pro.SmartGroupDetailV2{GroupName: "Group"})
	if state.ID.ValueString() != "19" {
		t.Errorf("id should be kept, got %q", state.ID.ValueString())
	}
}

func TestAssignResourceModel_NilReadIsANoOp(t *testing.T) {
	state := SmartMobileDeviceGroupResourceModel{ID: types.StringValue("19")}
	assignSmartMobileDeviceGroupResourceModel(&state, nil)
	if state.ID.ValueString() != "19" {
		t.Error("a nil read must change nothing")
	}
}

func TestAssignDataSourceModel_MapsTheRead(t *testing.T) {
	var data SmartMobileDeviceGroupDataSourceModel
	assignSmartMobileDeviceGroupDataSourceModel(&data, &pro.SmartGroupDetailV2{
		GroupID:          "19",
		GroupName:        "Supervised iPads",
		GroupDescription: "",
		SiteID:           "",
		Criteria: []pro.MobileDeviceSmartGroupCriteriaV2{
			wireCriterion("Model", "like", "iPad", 0),
			wireCriterion("Supervised", "is", "true", 1),
		},
	})

	if data.ID.ValueString() != "19" || data.Name.ValueString() != "Supervised iPads" {
		t.Errorf("identity did not map: %+v", data)
	}
	// A data source has no prior value, so the read is authoritative and an empty
	// description reports as an empty string rather than null.
	if data.Description.IsNull() {
		t.Error("the data source must report the read verbatim rather than reconciling")
	}
	if data.SiteID.ValueString() != progroups.NoSiteID {
		t.Errorf("no site should report the sentinel, got %q", data.SiteID.ValueString())
	}
	if len(data.Criteria) != 2 {
		t.Fatalf("expected 2 criteria, got %d", len(data.Criteria))
	}
	if data.Criteria[1].Priority.ValueInt64() != 1 {
		t.Errorf("criteria priorities should come from the read, got %d", data.Criteria[1].Priority.ValueInt64())
	}
}

func TestPluralResults_MapTheCollectionRead(t *testing.T) {
	got := smartMobileDeviceGroupsResults(
		[]pro.SmartGroup{
			{GroupID: "19", GroupName: "Supervised iPads", GroupDescription: "Loan pool", SiteID: "7", Count: 42},
			{GroupID: "20", GroupName: "Shared iPads", SiteID: progroups.NoSiteID, Count: 0},
		},
		map[string]string{"19": "11111111-2222-3333-4444-555555555555"},
	)

	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if got[0].MemberCount.ValueInt64() != 42 {
		t.Errorf("member_count: got %d", got[0].MemberCount.ValueInt64())
	}
	if got[0].PlatformID.ValueString() != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("platform_id: got %q", got[0].PlatformID.ValueString())
	}
	// A group the bridge could not place reports a null identifier rather than
	// failing the whole read.
	if !got[1].PlatformID.IsNull() {
		t.Errorf("an unresolved platform_id must be null, got %q", got[1].PlatformID.ValueString())
	}
	if got[1].SiteID.ValueString() != progroups.NoSiteID {
		t.Errorf("site_id: got %q", got[1].SiteID.ValueString())
	}
}

func TestPluralResults_EmptyCollectionYieldsAnEmptySlice(t *testing.T) {
	got := smartMobileDeviceGroupsResults(nil, nil)
	if got == nil {
		t.Fatal("an empty collection should yield an empty slice, not nil")
	}
	if len(got) != 0 {
		t.Errorf("expected no results, got %d", len(got))
	}
}
