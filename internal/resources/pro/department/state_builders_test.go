// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package department

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignDepartmentResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := DepartmentResourceModel{
		ID:   types.StringValue("42"),
		Name: types.StringValue("Engineering"),
	}
	api := &pro.Department{
		ID:   nil,
		Name: "Engineering refreshed",
	}

	assignDepartmentResourceModel(&state, api)

	if state.ID.ValueString() != "42" {
		t.Errorf("expected state.ID preserved as %q, got %q", "42", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Engineering refreshed" {
		t.Errorf("expected Name updated, got %q", state.Name.ValueString())
	}
}

func TestAssignDepartmentResourceModel_OverwritesIDWhenAPIPresent(t *testing.T) {
	state := DepartmentResourceModel{
		ID: types.StringValue("placeholder"),
	}
	id := "99"
	api := &pro.Department{
		ID:   &id,
		Name: "Operations",
	}

	assignDepartmentResourceModel(&state, api)

	if state.ID.ValueString() != "99" {
		t.Errorf("expected state.ID overwritten to %q, got %q", "99", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Operations" {
		t.Errorf("expected Name %q, got %q", "Operations", state.Name.ValueString())
	}
}

func TestAssignDepartmentDataSourceModel_PopulatedRoundTrip(t *testing.T) {
	state := DepartmentDataSourceModel{}
	id := "7"
	api := &pro.Department{
		ID:   &id,
		Name: "Sales",
	}

	assignDepartmentDataSourceModel(&state, api)

	if state.ID.ValueString() != "7" {
		t.Errorf("expected ID %q, got %q", "7", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Sales" {
		t.Errorf("expected Name %q, got %q", "Sales", state.Name.ValueString())
	}
}

func TestAssignDepartmentDataSourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := DepartmentDataSourceModel{
		ID: types.StringValue("1"),
	}
	api := &pro.Department{
		Name: "Finance",
	}

	assignDepartmentDataSourceModel(&state, api)

	if state.ID.ValueString() != "1" {
		t.Errorf("expected state.ID preserved, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Finance" {
		t.Errorf("expected Name %q, got %q", "Finance", state.Name.ValueString())
	}
}
