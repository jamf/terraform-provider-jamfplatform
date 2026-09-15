// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package building

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignBuildingResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := BuildingResourceModel{
		ID:   types.StringValue("42"),
		Name: types.StringValue("HQ"),
	}
	api := &pro.Building{
		ID:   nil,
		Name: "HQ refreshed",
	}

	assignBuildingResourceModel(&state, api)

	if state.ID.ValueString() != "42" {
		t.Errorf("expected state.ID preserved as %q, got %q", "42", state.ID.ValueString())
	}
	if state.Name.ValueString() != "HQ refreshed" {
		t.Errorf("expected Name updated, got %q", state.Name.ValueString())
	}
}

func TestAssignBuildingResourceModel_OverwritesIDWhenAPIPresent(t *testing.T) {
	state := BuildingResourceModel{
		ID: types.StringValue("placeholder"),
	}
	id := "99"
	api := &pro.Building{
		ID:   &id,
		Name: "Campus",
	}

	assignBuildingResourceModel(&state, api)

	if state.ID.ValueString() != "99" {
		t.Errorf("expected state.ID overwritten to %q, got %q", "99", state.ID.ValueString())
	}
}

func TestAssignBuildingResourceModel_NilOptionalPointersBecomeNull(t *testing.T) {
	state := BuildingResourceModel{
		ID:   types.StringValue("1"),
		Name: types.StringValue("HQ"),
	}
	id := "1"
	api := &pro.Building{
		ID:   &id,
		Name: "HQ",
	}

	assignBuildingResourceModel(&state, api)

	for _, f := range []struct {
		name string
		got  types.String
	}{
		{"City", state.City},
		{"Country", state.Country},
		{"StateProvince", state.StateProvince},
		{"StreetAddress1", state.StreetAddress1},
		{"StreetAddress2", state.StreetAddress2},
		{"ZipPostalCode", state.ZipPostalCode},
	} {
		if !f.got.IsNull() {
			t.Errorf("%s: expected null for nil API pointer, got %q", f.name, f.got.ValueString())
		}
	}
}

func TestAssignBuildingResourceModel_PopulatedOptionalsRoundTrip(t *testing.T) {
	state := BuildingResourceModel{
		ID:   types.StringValue("1"),
		Name: types.StringValue("HQ"),
	}
	id := "1"
	city := "Minneapolis"
	country := "USA"
	stateProv := "MN"
	street1 := "100 Washington Ave S"
	street2 := "Suite 1100"
	zip := "55401"
	api := &pro.Building{
		ID:             &id,
		Name:           "HQ",
		City:           &city,
		Country:        &country,
		StateProvince:  &stateProv,
		StreetAddress1: &street1,
		StreetAddress2: &street2,
		ZipPostalCode:  &zip,
	}

	assignBuildingResourceModel(&state, api)

	cases := []struct {
		name     string
		got      types.String
		expected string
	}{
		{"City", state.City, "Minneapolis"},
		{"Country", state.Country, "USA"},
		{"StateProvince", state.StateProvince, "MN"},
		{"StreetAddress1", state.StreetAddress1, "100 Washington Ave S"},
		{"StreetAddress2", state.StreetAddress2, "Suite 1100"},
		{"ZipPostalCode", state.ZipPostalCode, "55401"},
	}
	for _, c := range cases {
		if c.got.IsNull() {
			t.Errorf("%s: expected %q, got null", c.name, c.expected)
			continue
		}
		if c.got.ValueString() != c.expected {
			t.Errorf("%s: expected %q, got %q", c.name, c.expected, c.got.ValueString())
		}
	}
}

func TestAssignBuildingDataSourceModel_NilPointersBecomeNull(t *testing.T) {
	state := BuildingDataSourceModel{
		ID: types.StringValue("1"),
	}
	api := &pro.Building{
		Name: "HQ",
	}

	assignBuildingDataSourceModel(&state, api)

	if state.ID.ValueString() != "1" {
		t.Errorf("expected state.ID preserved, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "HQ" {
		t.Errorf("expected Name %q, got %q", "HQ", state.Name.ValueString())
	}
	for _, f := range []struct {
		name string
		got  types.String
	}{
		{"City", state.City},
		{"Country", state.Country},
		{"StateProvince", state.StateProvince},
		{"StreetAddress1", state.StreetAddress1},
		{"StreetAddress2", state.StreetAddress2},
		{"ZipPostalCode", state.ZipPostalCode},
	} {
		if !f.got.IsNull() {
			t.Errorf("%s: expected null for nil API pointer, got %q", f.name, f.got.ValueString())
		}
	}
}

func TestAssignBuildingDataSourceModel_PopulatedRoundTrip(t *testing.T) {
	state := BuildingDataSourceModel{}
	id := "7"
	city := "Eau Claire"
	country := "USA"
	stateProv := "WI"
	street1 := "1 Main St"
	street2 := "Floor 2"
	zip := "54701"
	api := &pro.Building{
		ID:             &id,
		Name:           "Branch",
		City:           &city,
		Country:        &country,
		StateProvince:  &stateProv,
		StreetAddress1: &street1,
		StreetAddress2: &street2,
		ZipPostalCode:  &zip,
	}

	assignBuildingDataSourceModel(&state, api)

	if state.ID.ValueString() != "7" {
		t.Errorf("expected ID %q, got %q", "7", state.ID.ValueString())
	}
	if state.City.ValueString() != "Eau Claire" {
		t.Errorf("expected City %q, got %q", "Eau Claire", state.City.ValueString())
	}
	if state.ZipPostalCode.ValueString() != "54701" {
		t.Errorf("expected ZipPostalCode %q, got %q", "54701", state.ZipPostalCode.ValueString())
	}
}
