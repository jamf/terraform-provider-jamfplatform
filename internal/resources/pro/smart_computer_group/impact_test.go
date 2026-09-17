// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
)

func criterion(name, searchType, value string) criteria.CriterionModel {
	return criteria.CriterionModel{
		Priority:              types.Int64Value(0),
		Name:                  types.StringValue(name),
		SearchType:            types.StringValue(searchType),
		Value:                 types.StringValue(value),
		AndOr:                 types.StringValue("and"),
		HasOpeningParenthesis: types.BoolValue(false),
		HasClosingParenthesis: types.BoolValue(false),
	}
}

func groupModel(name, siteID string, criteriaModels ...criteria.CriterionModel) SmartComputerGroupResourceModel {
	return SmartComputerGroupResourceModel{
		ID:          types.StringValue("41"),
		PlatformID:  types.StringValue("11111111-2222-3333-4444-555555555555"),
		Name:        types.StringValue(name),
		Description: types.StringValue("placeholder note"),
		SiteID:      types.StringValue(siteID),
		Criteria:    criteriaModels,
	}
}

// TestMembershipFor_AlwaysUndetermined pins the fact that a smart group's
// post-change membership is Jamf Pro's to decide, so the alert never reports a
// figure for it.
func TestMembershipFor_AlwaysUndetermined(t *testing.T) {
	base := groupModel("placeholder group", "-1", criterion("Operating System Version", "like", "26."))
	got := membershipFor(base, base)

	if got.Undetermined != impact.CriteriaUndetermined {
		t.Errorf("expected the shared criteria reason, got %q", got.Undetermined)
	}
	if got.NextKnown || got.CurrentKnown {
		t.Errorf("neither figure is knowable here, got currentKnown=%v nextKnown=%v", got.CurrentKnown, got.NextKnown)
	}
	if got.Noun != "computers" {
		t.Errorf("expected the alert to count computers, got %q", got.Noun)
	}
}

// TestMembershipChanging_RenameIsNotAMembershipChange pins the rule the alert
// exists to respect: Jamf Pro raises its criteria alert on a membership edit,
// not on a rename or a note.
func TestMembershipChanging_RenameIsNotAMembershipChange(t *testing.T) {
	sameCriteria := criterion("Operating System Version", "like", "26.")
	state := groupModel("placeholder group", "-1", sameCriteria)
	plan := groupModel("placeholder group renamed", "-1", sameCriteria)

	if membershipChanging(plan, state) {
		t.Error("a rename must not read as a membership change")
	}

	plan = state
	plan.Description = types.StringValue("a different note")
	if membershipChanging(plan, state) {
		t.Error("a description edit must not read as a membership change")
	}
}

// TestMembershipChanging_SiteMoveIsAMembershipChange pins the less obvious half:
// a group in a site only contains computers in that site, so moving it changes
// who is in it even when the criteria are untouched.
func TestMembershipChanging_SiteMoveIsAMembershipChange(t *testing.T) {
	sameCriteria := criterion("Operating System Version", "like", "26.")
	state := groupModel("placeholder group", "-1", sameCriteria)
	plan := groupModel("placeholder group", "7", sameCriteria)

	if !membershipChanging(plan, state) {
		t.Error("moving the group to a site must read as a membership change")
	}
}

func TestCriteriaDiffer(t *testing.T) {
	base := []criteria.CriterionModel{criterion("Operating System Version", "like", "26.")}

	if criteriaDiffer(base, base) {
		t.Error("identical criteria must not read as changed")
	}
	if !criteriaDiffer(base, nil) {
		t.Error("adding a criterion must read as changed")
	}

	cases := map[string][]criteria.CriterionModel{
		"an edited value":     {criterion("Operating System Version", "like", "15.")},
		"an edited operator":  {criterion("Operating System Version", "is", "26.")},
		"an edited attribute": {criterion("Model", "like", "26.")},
	}
	for label, other := range cases {
		if !criteriaDiffer(base, other) {
			t.Errorf("%s must read as changed", label)
		}
	}

	joined := []criteria.CriterionModel{criterion("Operating System Version", "like", "26.")}
	joined[0].AndOr = types.StringValue("or")
	if !criteriaDiffer(base, joined) {
		t.Error("an edited join must read as changed")
	}

	parenthesised := []criteria.CriterionModel{criterion("Operating System Version", "like", "26.")}
	parenthesised[0].HasOpeningParenthesis = types.BoolValue(true)
	if !criteriaDiffer(base, parenthesised) {
		t.Error("an added parenthesis must read as changed")
	}
}

// TestCriteriaDiffer_IgnoresPriority pins that priority is the element's own
// position, so it cannot differ on its own.
func TestCriteriaDiffer_IgnoresPriority(t *testing.T) {
	a := []criteria.CriterionModel{criterion("Model", "like", "MacBook")}
	b := []criteria.CriterionModel{criterion("Model", "like", "MacBook")}
	a[0].Priority = types.Int64Value(0)
	b[0].Priority = types.Int64Value(1)

	if criteriaDiffer(a, b) {
		t.Error("a differing priority alone must not read as a membership change")
	}
}
