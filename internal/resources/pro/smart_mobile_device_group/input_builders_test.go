// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// criterion builds one plan criterion with the fields a practitioner writes and
// the rest left to the schema.
func criterion(name, searchType, value string) criteria.CriterionModel {
	return criteria.CriterionModel{
		Priority:              types.Int64Null(),
		Name:                  types.StringValue(name),
		SearchType:            types.StringValue(searchType),
		Value:                 types.StringValue(value),
		AndOr:                 types.StringValue("and"),
		HasOpeningParenthesis: types.BoolValue(false),
		HasClosingParenthesis: types.BoolValue(false),
	}
}

func TestBuildInput_MapsTheGroupScalars(t *testing.T) {
	got := buildSmartMobileDeviceGroupInput(SmartMobileDeviceGroupResourceModel{
		Name:        types.StringValue("Supervised iPads"),
		Description: types.StringValue("Loan pool"),
		SiteID:      types.StringValue("7"),
	})

	if got.GroupName != "Supervised iPads" {
		t.Errorf("name: got %q", got.GroupName)
	}
	if got.GroupDescription == nil || *got.GroupDescription != "Loan pool" {
		t.Errorf("description: got %v", got.GroupDescription)
	}
	if got.SiteID == nil || *got.SiteID != "7" {
		t.Errorf("site: got %v", got.SiteID)
	}
	if got.GroupID != nil {
		t.Error("the body must not carry an identifier: the create allocates one and the update takes it in the path")
	}
}

// TestBuildInput_AlwaysEmitsSite is the guard for this endpoint's one divergence
// from its computer counterpart: a write that omits the site is refused outright
// here, and the refusal reads as a permission fault.
func TestBuildInput_AlwaysEmitsSite(t *testing.T) {
	for name, siteID := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
		"empty":   types.StringValue(""),
	} {
		got := buildSmartMobileDeviceGroupInput(SmartMobileDeviceGroupResourceModel{
			Name:   types.StringValue("Group"),
			SiteID: siteID,
		})
		if got.SiteID == nil {
			t.Fatalf("%s site_id produced no site on the wire", name)
		}
		if *got.SiteID != progroups.NoSiteID {
			t.Errorf("%s site_id should collapse to the no-site sentinel, got %q", name, *got.SiteID)
		}
	}
}

// TestBuildInput_AlwaysEmitsDescription covers the full-replace rule: an omitted
// description clears the stored one, so a body that skipped the key on a null
// plan value would silently disagree with state.
func TestBuildInput_AlwaysEmitsDescription(t *testing.T) {
	got := buildSmartMobileDeviceGroupInput(SmartMobileDeviceGroupResourceModel{
		Name:        types.StringValue("Group"),
		Description: types.StringNull(),
	})
	if got.GroupDescription == nil {
		t.Fatal("description must always be emitted")
	}
	if *got.GroupDescription != "" {
		t.Errorf("a null description should clear, got %q", *got.GroupDescription)
	}
}

// TestBuildInput_EmptyCriteriaAreEmittedAsAnEmptyArray is what lets a
// configuration remove every criterion: the endpoint full-replaces the array, so
// an empty one is the clear and a missing one is indistinguishable from it only
// because the builder never omits it.
func TestBuildInput_EmptyCriteriaAreEmittedAsAnEmptyArray(t *testing.T) {
	got := buildSmartMobileDeviceGroupInput(SmartMobileDeviceGroupResourceModel{
		Name:     types.StringValue("Group"),
		Criteria: nil,
	})
	if got.Criteria == nil {
		t.Fatal("criteria must be emitted even when empty")
	}
	if len(*got.Criteria) != 0 {
		t.Errorf("expected an empty criteria array, got %d entries", len(*got.Criteria))
	}
}

// TestBuildInput_CriteriaPriorityIsThePosition pins the numbering the endpoint
// requires: priorities run from zero upwards with no gaps, and the list index is
// the only numbering that satisfies it.
func TestBuildInput_CriteriaPriorityIsThePosition(t *testing.T) {
	got := buildSmartMobileDeviceGroupInput(SmartMobileDeviceGroupResourceModel{
		Name: types.StringValue("Group"),
		Criteria: []criteria.CriterionModel{
			criterion("Model", "like", "iPad"),
			criterion("OS Version", "greater than", "18"),
			criterion("Supervised", "is", "true"),
		},
	})
	if got.Criteria == nil {
		t.Fatal("criteria must be emitted")
	}
	built := *got.Criteria
	if len(built) != 3 {
		t.Fatalf("expected 3 criteria, got %d", len(built))
	}
	for i, c := range built {
		if c.Priority != i {
			t.Errorf("criterion %d: priority %d, want %d", i, c.Priority, i)
		}
	}
	if built[0].Name != "Model" || built[0].SearchType != "like" || built[0].Value != "iPad" {
		t.Errorf("first criterion did not round-trip: %+v", built[0])
	}
	if built[1].OpeningParen == nil || *built[1].OpeningParen {
		t.Error("an unparenthesised criterion must send false rather than nothing")
	}
}

// TestBuildClassicInput_MapsTheGroupScalars covers the body the fallback write
// sends. The older interface names the group's fields without the prefixes the
// group endpoint uses, so every one of them is a mapping that can be got wrong
// in a way nothing else in the suite would notice.
func TestBuildClassicInput_MapsTheGroupScalars(t *testing.T) {
	got := buildClassicSmartMobileDeviceGroupInput(SmartMobileDeviceGroupResourceModel{
		Name:        types.StringValue("Supervised iPads"),
		Description: types.StringValue("Loan pool"),
		SiteID:      types.StringValue("7"),
	})

	if got.Name == nil || *got.Name != "Supervised iPads" {
		t.Errorf("name: got %v", got.Name)
	}
	if got.IsSmart == nil || !*got.IsSmart {
		t.Errorf("a smart group must declare itself smart: got %v", got.IsSmart)
	}
	if got.Site == nil || got.Site.ID == nil || *got.Site.ID != 7 {
		t.Errorf("site: got %+v", got.Site)
	}
	if got.ID != nil {
		t.Error("the body must not carry an identifier: the create allocates one and the update takes it in the path")
	}
	if got.MobileDevices != nil {
		t.Error("a smart group has no membership collection; Jamf Pro computes its members from the criteria")
	}
}

// TestBuildClassicInput_AlwaysEmitsSite is the always-emit guard on the fallback
// path. The older interface full-replaces the scalars it is given, so a body
// that dropped the site on an unset plan value would move the group out of its
// site on any edit that took this path.
func TestBuildClassicInput_AlwaysEmitsSite(t *testing.T) {
	for name, siteID := range map[string]types.String{
		"null":        types.StringNull(),
		"unknown":     types.StringUnknown(),
		"empty":       types.StringValue(""),
		"non-numeric": types.StringValue("Loan Pool"),
	} {
		got := buildClassicSmartMobileDeviceGroupInput(SmartMobileDeviceGroupResourceModel{
			Name:   types.StringValue("Group"),
			SiteID: siteID,
		})
		if got.Site == nil || got.Site.ID == nil {
			t.Fatalf("%s site_id produced no site on the wire", name)
		}
		if *got.Site.ID != -1 {
			t.Errorf("%s site_id should collapse to the no-site sentinel, got %d", name, *got.Site.ID)
		}
	}
}

// TestBuildClassicInput_AlwaysEmitsName pins the other half of the full-replace
// rule. A pointer field is emitted by being set rather than by being non-empty,
// so an empty name has to travel as an empty element and not as an absent one.
func TestBuildClassicInput_AlwaysEmitsName(t *testing.T) {
	got := buildClassicSmartMobileDeviceGroupInput(SmartMobileDeviceGroupResourceModel{
		Name: types.StringNull(),
	})
	if got.Name == nil {
		t.Fatal("name must always be emitted")
	}
	if *got.Name != "" {
		t.Errorf("a null name should render as empty, got %q", *got.Name)
	}
}

// TestBuildClassicInput_CriteriaGoThroughTheSharedClassicBuilder covers the
// mapping the fallback exists for. The criteria are the reason the write moved
// interfaces, so the one thing this body must get right is every criterion, in
// order, with the positional priority the endpoint requires.
func TestBuildClassicInput_CriteriaGoThroughTheSharedClassicBuilder(t *testing.T) {
	got := buildClassicSmartMobileDeviceGroupInput(SmartMobileDeviceGroupResourceModel{
		Name: types.StringValue("Group"),
		Criteria: []criteria.CriterionModel{
			criterion("Model", "like", "iPad"),
			criterion("Battery Level", "has", "40"),
		},
	})
	if got.Criteria == nil || got.Criteria.Criterion == nil {
		t.Fatal("criteria must be emitted")
	}
	built := *got.Criteria.Criterion
	if len(built) != 2 {
		t.Fatalf("expected 2 criteria, got %d", len(built))
	}
	for i, c := range built {
		if c.Priority == nil || *c.Priority != i {
			t.Errorf("criterion %d: priority %v, want %d", i, c.Priority, i)
		}
	}
	if built[1].Name == nil || *built[1].Name != "Battery Level" || built[1].SearchType == nil || *built[1].SearchType != "has" {
		t.Errorf("second criterion did not round-trip: %+v", built[1])
	}
	if built[0].OpeningParen == nil || *built[0].OpeningParen {
		t.Error("an unparenthesised criterion must send false rather than nothing")
	}
}

// TestBuildClassicInput_EmptyCriteriaAreEmittedAsAnEmptyElement is what lets the
// fallback path remove every criterion. An omitted criteria element leaves the
// stored criteria alone there, and an empty one clears them, so the wrapper is
// always present.
func TestBuildClassicInput_EmptyCriteriaAreEmittedAsAnEmptyElement(t *testing.T) {
	got := buildClassicSmartMobileDeviceGroupInput(SmartMobileDeviceGroupResourceModel{
		Name:     types.StringValue("Group"),
		Criteria: nil,
	})
	if got.Criteria == nil || got.Criteria.Criterion == nil {
		t.Fatal("the criteria element must be emitted even when empty")
	}
	if len(*got.Criteria.Criterion) != 0 {
		t.Errorf("expected an empty criteria element, got %d entries", len(*got.Criteria.Criterion))
	}
}

// TestBuildDescriptionPatch_CarriesNothingButTheDescription is the guard on the
// most damaging regression this change could grow.
//
// The description write is a merge patch on a request that also accepts criteria
// and a membership list. A body that carried the criteria would put the very
// criteria the group endpoint refused back through that endpoint, undoing the
// fallback write it is there to complete — and a body that carried assignments
// would say something about a smart group's membership, which is Jamf Pro's.
// Marshalling rather than checking fields one by one is deliberate: it fails for
// a field added later that no assertion here was written to look at.
func TestBuildDescriptionPatch_CarriesNothingButTheDescription(t *testing.T) {
	body, err := json.Marshal(buildDescriptionPatch(types.StringValue("Loan pool")))
	if err != nil {
		t.Fatalf("marshalling the description patch: %v", err)
	}
	if want := `{"groupDescription":"Loan pool"}`; string(body) != want {
		t.Errorf("the description patch must carry the description alone.\n got: %s\nwant: %s", body, want)
	}

	patch := buildDescriptionPatch(types.StringValue("Loan pool"))
	if patch.Criteria != nil {
		t.Error("the description patch must never carry criteria")
	}
	if patch.Assignments != nil {
		t.Error("the description patch must never carry a membership list")
	}
	if patch.GroupName != nil {
		t.Error("the description patch must never carry the name")
	}
}

// TestBuildDescriptionPatch_ClearsOnAnUnsetDescription pins the one write that
// can clear the field. An unset description collapses to the empty string, which
// is what Jamf Pro stores for a group without one.
func TestBuildDescriptionPatch_ClearsOnAnUnsetDescription(t *testing.T) {
	for name, planned := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
	} {
		patch := buildDescriptionPatch(planned)
		if patch.GroupDescription == nil {
			t.Fatalf("%s description produced no field", name)
		}
		if *patch.GroupDescription != "" {
			t.Errorf("%s description should render as empty, got %q", name, *patch.GroupDescription)
		}
	}
}

// TestDescriptionNeedsWrite_UnchangedDescriptionIssuesNoWrite is the rule that
// keeps an ordinary edit on the fallback path to a single request.
//
// The older interface preserves a description it cannot express, so an update
// that leaves the description alone has nothing to do. Issuing the merge patch
// anyway would spend a second write on a group whose membership Jamf Pro
// recalculates synchronously, for no change at all.
func TestDescriptionNeedsWrite_UnchangedDescriptionIssuesNoWrite(t *testing.T) {
	tests := map[string]struct {
		planned, prior types.String
		want           bool
	}{
		"identical":                {planned: types.StringValue("Loan pool"), prior: types.StringValue("Loan pool")},
		"both unset":               {planned: types.StringNull(), prior: types.StringNull()},
		"unset against empty":      {planned: types.StringNull(), prior: types.StringValue("")},
		"unknown against empty":    {planned: types.StringUnknown(), prior: types.StringValue("")},
		"changed":                  {planned: types.StringValue("Loan pool"), prior: types.StringValue("Kiosks"), want: true},
		"set against unset":        {planned: types.StringValue("Loan pool"), prior: types.StringNull(), want: true},
		"cleared against a value":  {planned: types.StringValue(""), prior: types.StringValue("Loan pool"), want: true},
		"created with description": {planned: types.StringValue("Loan pool"), prior: types.StringNull(), want: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := descriptionNeedsWrite(tc.planned, tc.prior); got != tc.want {
				t.Errorf("descriptionNeedsWrite = %v, want %v", got, tc.want)
			}
		})
	}
}
