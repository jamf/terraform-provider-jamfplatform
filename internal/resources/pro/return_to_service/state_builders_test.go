// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package return_to_service

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestAssignReturnToServiceResourceModel(t *testing.T) {
	var state ReturnToServiceResourceModel
	assignReturnToServiceResourceModel(&state, &pro.ReturnToServiceConfiguration{
		ID:            "26",
		DisplayName:   "Front Desk iPads",
		WifiProfileID: "70",
	})

	if state.ID.ValueString() != "26" {
		t.Errorf("id = %q, want %q", state.ID.ValueString(), "26")
	}
	if state.DisplayName.ValueString() != "Front Desk iPads" {
		t.Errorf("display_name = %q, want %q", state.DisplayName.ValueString(), "Front Desk iPads")
	}
	if state.WifiProfileID.ValueString() != "70" {
		t.Errorf("wifi_profile_id = %q, want %q", state.WifiProfileID.ValueString(), "70")
	}
}

func TestAssignReturnToServiceResourceModel_NilNoPanic(t *testing.T) {
	var state ReturnToServiceResourceModel
	assignReturnToServiceResourceModel(&state, nil)
	if !state.ID.IsNull() {
		t.Errorf("expected id to stay null on nil response")
	}
}

func TestAssignReturnToServiceDataSourceModel(t *testing.T) {
	var state ReturnToServiceDataSourceModel
	assignReturnToServiceDataSourceModel(&state, &pro.ReturnToServiceConfiguration{
		ID:            "27",
		DisplayName:   "Lab Macs",
		WifiProfileID: "70",
	})

	if state.ID.ValueString() != "27" {
		t.Errorf("id = %q, want %q", state.ID.ValueString(), "27")
	}
	if state.DisplayName.ValueString() != "Lab Macs" {
		t.Errorf("display_name = %q, want %q", state.DisplayName.ValueString(), "Lab Macs")
	}
	if state.WifiProfileID.ValueString() != "70" {
		t.Errorf("wifi_profile_id = %q, want %q", state.WifiProfileID.ValueString(), "70")
	}
}
