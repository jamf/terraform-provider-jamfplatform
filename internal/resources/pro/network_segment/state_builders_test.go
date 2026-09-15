// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package network_segment

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestAssignNetworkSegmentResourceModel_PopulatesFields(t *testing.T) {
	state := NetworkSegmentResourceModel{
		// Pre-populate Optional fields so Reconcile* sees configured values.
		Building:            types.StringValue("HQ"),
		Department:          types.StringValue("IT"),
		OverrideBuildings:   types.BoolValue(true),
		OverrideDepartments: types.BoolValue(false),
	}
	id := 42
	name := "HQ-Net"
	start := "10.0.0.0"
	end := "10.0.0.255"
	building := "HQ"
	department := "IT"
	overBuildings := true
	overDepartments := false
	dp := "Main DP"
	ds := "ds.example.com"
	swu := "swu.example.com"
	url := "https://example.com/netboot"

	api := &proclassic.NetworkSegment{
		ID:                  &id,
		Name:                &name,
		StartingAddress:     &start,
		EndingAddress:       &end,
		Building:            &building,
		Department:          &department,
		OverrideBuildings:   &overBuildings,
		OverrideDepartments: &overDepartments,
		DistributionPoint:   &dp,
		DistributionServer:  &ds,
		SwuServer:           &swu,
		URL:                 &url,
	}

	assignNetworkSegmentResourceModel(&state, api, false)

	if state.ID.ValueString() != "42" {
		t.Errorf("expected ID 42, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "HQ-Net" {
		t.Errorf("expected Name HQ-Net, got %q", state.Name.ValueString())
	}
	if state.StartingAddress.ValueString() != start {
		t.Errorf("expected StartingAddress %q, got %q", start, state.StartingAddress.ValueString())
	}
	if state.EndingAddress.ValueString() != end {
		t.Errorf("expected EndingAddress %q, got %q", end, state.EndingAddress.ValueString())
	}
	if state.Building.ValueString() != building {
		t.Errorf("expected Building %q, got %q", building, state.Building.ValueString())
	}
	if state.OverrideBuildings.ValueBool() != true {
		t.Errorf("expected OverrideBuildings true")
	}
	if state.OverrideDepartments.ValueBool() != false {
		t.Errorf("expected OverrideDepartments false")
	}
	if state.DistributionPoint.ValueString() != dp {
		t.Errorf("expected DistributionPoint %q, got %q", dp, state.DistributionPoint.ValueString())
	}
	if state.URL.ValueString() != url {
		t.Errorf("expected URL %q, got %q", url, state.URL.ValueString())
	}
}

func TestAssignNetworkSegmentResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := NetworkSegmentResourceModel{ID: types.StringValue("9")}
	api := &proclassic.NetworkSegment{ID: nil}

	assignNetworkSegmentResourceModel(&state, api, false)

	if state.ID.ValueString() != "9" {
		t.Errorf("expected state.ID preserved as %q, got %q", "9", state.ID.ValueString())
	}
}

func TestAssignNetworkSegmentResourceModel_NilAPIIsNoop(t *testing.T) {
	state := NetworkSegmentResourceModel{
		ID:   types.StringValue("7"),
		Name: types.StringValue("Keep"),
	}
	assignNetworkSegmentResourceModel(&state, nil, false)
	if state.ID.ValueString() != "7" || state.Name.ValueString() != "Keep" {
		t.Errorf("expected state unchanged, got id=%q name=%q", state.ID.ValueString(), state.Name.ValueString())
	}
}

func TestAssignNetworkSegmentResourceModel_OptionalReconcileKeepsNullWhenUnmanaged(t *testing.T) {
	// User did not configure building / override_buildings; API returns nothing.
	// Reconcile must leave state null rather than overwriting with empty value.
	state := NetworkSegmentResourceModel{
		Building:          types.StringNull(),
		OverrideBuildings: types.BoolNull(),
	}
	api := &proclassic.NetworkSegment{Building: nil, OverrideBuildings: nil}

	assignNetworkSegmentResourceModel(&state, api, false)

	if !state.Building.IsNull() {
		t.Errorf("expected Building to remain null, got %q", state.Building.ValueString())
	}
	if !state.OverrideBuildings.IsNull() {
		t.Errorf("expected OverrideBuildings to remain null, got %v", state.OverrideBuildings.ValueBool())
	}
}

func TestAssignNetworkSegmentDataSourceModel_PopulatesFields(t *testing.T) {
	state := NetworkSegmentDataSourceModel{}
	id := 11
	name := "Looked Up"
	start := "172.16.0.0"
	end := "172.16.0.255"
	api := &proclassic.NetworkSegment{
		ID:              &id,
		Name:            &name,
		StartingAddress: &start,
		EndingAddress:   &end,
	}

	assignNetworkSegmentDataSourceModel(&state, api)

	if state.ID.ValueString() != "11" {
		t.Errorf("expected ID %q, got %q", "11", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Looked Up" {
		t.Errorf("expected Name %q, got %q", "Looked Up", state.Name.ValueString())
	}
	if state.StartingAddress.ValueString() != start {
		t.Errorf("expected StartingAddress %q, got %q", start, state.StartingAddress.ValueString())
	}
}

func TestAssignNetworkSegmentDataSourceModel_PreservesSelectorOnNilAPIFields(t *testing.T) {
	state := NetworkSegmentDataSourceModel{
		ID:   types.StringNull(),
		Name: types.StringValue("HQ-Net"),
	}
	id := 7
	api := &proclassic.NetworkSegment{ID: &id, Name: nil}

	assignNetworkSegmentDataSourceModel(&state, api)

	if state.ID.ValueString() != "7" {
		t.Errorf("expected ID written, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "HQ-Net" {
		t.Errorf("expected Name preserved as %q, got %q", "HQ-Net", state.Name.ValueString())
	}
}

func TestAssignNetworkSegmentDataSourceModel_NilAPIIsNoop(t *testing.T) {
	state := NetworkSegmentDataSourceModel{
		ID:   types.StringValue("preset"),
		Name: types.StringValue("preset"),
	}
	assignNetworkSegmentDataSourceModel(&state, nil)
	if state.ID.ValueString() != "preset" || state.Name.ValueString() != "preset" {
		t.Errorf("expected state unchanged on nil API")
	}
}

// TestAssignNetworkSegmentResourceModel_AdoptsOverridesForConfigGeneration pins
// the adopt switch. A CRUD read must leave an Optional+Computed flag the
// practitioner never authored null, so a refresh does not snap it to the server
// default; config generation has no plan to reconcile against, so the server
// value has to survive. Reconciling on both paths is what left
// override_buildings and override_departments null on every generated network
// segment even though /networksegments returns them.
func TestAssignNetworkSegmentResourceModel_AdoptsOverridesForConfigGeneration(t *testing.T) {
	on := true
	name, building, department := "HQ", "HQ Building", "IT"
	wire := &proclassic.NetworkSegment{
		Name:                &name,
		Building:            &building,
		Department:          &department,
		OverrideBuildings:   &on,
		OverrideDepartments: &on,
	}

	// Config generation: a fresh model, so the wire value is authoritative.
	var generated NetworkSegmentResourceModel
	assignNetworkSegmentResourceModel(&generated, wire, true)
	if generated.OverrideBuildings.IsNull() || !generated.OverrideBuildings.ValueBool() {
		t.Errorf("override_buildings = %s, want the wire value adopted", generated.OverrideBuildings)
	}
	if generated.OverrideDepartments.IsNull() || !generated.OverrideDepartments.ValueBool() {
		t.Errorf("override_departments = %s, want the wire value adopted", generated.OverrideDepartments)
	}

	// A CRUD read against state that never authored them keeps them null.
	var refreshed NetworkSegmentResourceModel
	assignNetworkSegmentResourceModel(&refreshed, wire, false)
	if !refreshed.OverrideBuildings.IsNull() {
		t.Errorf("override_buildings = %s, want null on a refresh that never authored it", refreshed.OverrideBuildings)
	}
}
