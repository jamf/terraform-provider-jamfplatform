// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// TestBuildSmartComputerGroupInput_EmitsEveryScalar pins the always-emit rule.
// The endpoint full-replaces a group's scalars, so a body omitting description
// clears it and one omitting siteId clears the site: there is no body that means
// "leave this alone" and nothing may rely on omission to retain.
func TestBuildSmartComputerGroupInput_EmitsEveryScalar(t *testing.T) {
	plan := SmartComputerGroupResourceModel{
		Name:        types.StringValue("placeholder group"),
		Description: types.StringNull(),
		SiteID:      types.StringNull(),
	}

	got := buildSmartComputerGroupInput(plan)

	if got.Name != "placeholder group" {
		t.Errorf("name: expected %q, got %q", "placeholder group", got.Name)
	}
	if got.Description == nil {
		t.Fatal("description must always be emitted, even when unset")
	}
	if *got.Description != "" {
		t.Errorf("an unset description must be emitted as the empty string, got %q", *got.Description)
	}
	if got.SiteID == nil {
		t.Fatal("siteId must always be emitted")
	}
	if *got.SiteID != progroups.NoSiteID {
		t.Errorf("an unset site must collapse to the no-site sentinel, got %q", *got.SiteID)
	}
	if got.Criteria == nil {
		t.Fatal("criteria must always be emitted: the empty slice is how every criterion is removed")
	}
	if len(*got.Criteria) != 0 {
		t.Errorf("expected no criteria, got %d", len(*got.Criteria))
	}
}

// TestBuildSmartComputerGroupInput_NumbersPrioritiesFromThePosition pins the
// rule the endpoint enforces: priorities run from zero upwards in list order,
// whatever an authored value says. criteriaPriorityValidator has already refused
// a disagreeing value by the time a write is built.
func TestBuildSmartComputerGroupInput_NumbersPrioritiesFromThePosition(t *testing.T) {
	first := criterion("Operating System Version", "like", "26.")
	second := criterion("Model", "like", "MacBook")
	second.AndOr = types.StringValue("or")
	second.HasClosingParenthesis = types.BoolValue(true)

	plan := SmartComputerGroupResourceModel{
		Name:     types.StringValue("placeholder group"),
		SiteID:   types.StringValue("7"),
		Criteria: []criteria.CriterionModel{first, second},
	}

	got := buildSmartComputerGroupInput(plan)
	if got.Criteria == nil {
		t.Fatal("criteria must be emitted")
	}
	built := *got.Criteria
	if len(built) != 2 {
		t.Fatalf("expected 2 criteria, got %d", len(built))
	}
	for i, criterion := range built {
		if criterion.Priority != i {
			t.Errorf("criterion %d: expected priority %d, got %d", i, i, criterion.Priority)
		}
	}
	if built[1].AndOr != "or" {
		t.Errorf("the authored join must survive, got %q", built[1].AndOr)
	}
	if built[1].ClosingParen == nil || !*built[1].ClosingParen {
		t.Error("the authored closing parenthesis must survive")
	}
	if *got.SiteID != "7" {
		t.Errorf("site: expected %q, got %q", "7", *got.SiteID)
	}
}

func TestDescriptionForWrite(t *testing.T) {
	cases := map[string]struct {
		planned types.String
		want    string
	}{
		"a configured note":  {types.StringValue("placeholder note"), "placeholder note"},
		"an explicit clear":  {types.StringValue(""), ""},
		"an omitted note":    {types.StringNull(), ""},
		"a first-time write": {types.StringUnknown(), ""},
	}
	for label, tc := range cases {
		if got := descriptionForWrite(tc.planned); got != tc.want {
			t.Errorf("%s: expected %q, got %q", label, tc.want, got)
		}
	}
}
