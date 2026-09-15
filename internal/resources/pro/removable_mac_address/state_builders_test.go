// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package removable_mac_address

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestAssignRemovableMacAddressResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := RemovableMacAddressResourceModel{
		ID:         types.StringValue("42"),
		MacAddress: types.StringValue("00:A0:C9:14:C8:20"),
	}
	name := "00:A0:C9:14:C8:21"
	api := &proclassic.RemovableMacAddress{ID: nil, Name: &name}

	assignRemovableMacAddressResourceModel(&state, api)

	if state.ID.ValueString() != "42" {
		t.Errorf("expected state.ID preserved as %q, got %q", "42", state.ID.ValueString())
	}
	if state.MacAddress.ValueString() != "00:A0:C9:14:C8:21" {
		t.Errorf("expected MacAddress refreshed, got %q", state.MacAddress.ValueString())
	}
}

func TestAssignRemovableMacAddressResourceModel_OverwritesIDWhenAPIPresent(t *testing.T) {
	state := RemovableMacAddressResourceModel{ID: types.StringValue("placeholder")}
	id := 99
	name := "aa:bb:cc:dd:ee:ff"
	api := &proclassic.RemovableMacAddress{ID: &id, Name: &name}

	assignRemovableMacAddressResourceModel(&state, api)

	if state.ID.ValueString() != "99" {
		t.Errorf("expected state.ID overwritten to %q, got %q", "99", state.ID.ValueString())
	}
	if state.MacAddress.ValueString() != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("expected MacAddress %q, got %q", "aa:bb:cc:dd:ee:ff", state.MacAddress.ValueString())
	}
}

func TestAssignRemovableMacAddressResourceModel_NilAPIIsNoop(t *testing.T) {
	state := RemovableMacAddressResourceModel{
		ID:         types.StringValue("7"),
		MacAddress: types.StringValue("keep:me"),
	}
	assignRemovableMacAddressResourceModel(&state, nil)
	if state.ID.ValueString() != "7" || state.MacAddress.ValueString() != "keep:me" {
		t.Errorf("expected state unchanged, got id=%q mac=%q", state.ID.ValueString(), state.MacAddress.ValueString())
	}
}

func TestAssignRemovableMacAddressDataSourceModel_PopulatesBoth(t *testing.T) {
	state := RemovableMacAddressDataSourceModel{}
	id := 11
	name := "00:A0:C9:14:C8:20"
	api := &proclassic.RemovableMacAddress{ID: &id, Name: &name}

	assignRemovableMacAddressDataSourceModel(&state, api)

	if state.ID.ValueString() != "11" {
		t.Errorf("expected ID %q, got %q", "11", state.ID.ValueString())
	}
	if state.MacAddress.ValueString() != "00:A0:C9:14:C8:20" {
		t.Errorf("expected MacAddress %q, got %q", "00:A0:C9:14:C8:20", state.MacAddress.ValueString())
	}
}

func TestAssignRemovableMacAddressDataSourceModel_PreservesSelectorOnNilAPIFields(t *testing.T) {
	// Caller looked the record up by mac_address; the SDK responded with a nil Name
	// field. The DS must NOT overwrite the caller-supplied value with null. Same
	// contract as the resource model — symmetric guard.
	state := RemovableMacAddressDataSourceModel{
		ID:         types.StringNull(),
		MacAddress: types.StringValue("00:A0:C9:14:C8:20"),
	}
	id := 7
	api := &proclassic.RemovableMacAddress{ID: &id, Name: nil}

	assignRemovableMacAddressDataSourceModel(&state, api)

	if state.ID.ValueString() != "7" {
		t.Errorf("expected ID written, got %q", state.ID.ValueString())
	}
	if state.MacAddress.ValueString() != "00:A0:C9:14:C8:20" {
		t.Errorf("expected MacAddress preserved as %q, got %q", "00:A0:C9:14:C8:20", state.MacAddress.ValueString())
	}
}

func TestAssignRemovableMacAddressDataSourceModel_NilAPIIsNoop(t *testing.T) {
	state := RemovableMacAddressDataSourceModel{
		ID:         types.StringValue("preset"),
		MacAddress: types.StringValue("preset"),
	}
	assignRemovableMacAddressDataSourceModel(&state, nil)
	if state.ID.ValueString() != "preset" || state.MacAddress.ValueString() != "preset" {
		t.Errorf("expected state unchanged on nil API")
	}
}
