// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_plus_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

func TestAssignSelfServicePlusSettingsResourceModel(t *testing.T) {
	enabledTrue := true
	enabledFalse := false
	tests := []struct {
		name        string
		response    *pro.SelfServicePlusSettings
		wantEnabled bool
	}{
		{
			name:        "enabled true preserved",
			response:    &pro.SelfServicePlusSettings{Enabled: &enabledTrue},
			wantEnabled: true,
		},
		{
			name:        "enabled false preserved",
			response:    &pro.SelfServicePlusSettings{Enabled: &enabledFalse},
			wantEnabled: false,
		},
		{
			name:        "nil enabled defaults to false",
			response:    &pro.SelfServicePlusSettings{Enabled: nil},
			wantEnabled: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var state SelfServicePlusSettingsResourceModel
			assignSelfServicePlusSettingsResourceModel(&state, tc.response)
			if state.Enabled.IsNull() || state.Enabled.IsUnknown() {
				t.Fatalf("expected concrete bool, got null/unknown")
			}
			if got := state.Enabled.ValueBool(); got != tc.wantEnabled {
				t.Errorf("Enabled = %v, want %v", got, tc.wantEnabled)
			}
		})
	}
}

func TestAssignSelfServicePlusSettingsDataSourceModel(t *testing.T) {
	enabledTrue := true
	enabledFalse := false
	tests := []struct {
		name        string
		response    *pro.SelfServicePlusSettings
		wantEnabled bool
	}{
		{
			name:        "enabled true preserved",
			response:    &pro.SelfServicePlusSettings{Enabled: &enabledTrue},
			wantEnabled: true,
		},
		{
			name:        "enabled false preserved",
			response:    &pro.SelfServicePlusSettings{Enabled: &enabledFalse},
			wantEnabled: false,
		},
		{
			name:        "nil enabled defaults to false",
			response:    &pro.SelfServicePlusSettings{Enabled: nil},
			wantEnabled: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var state SelfServicePlusSettingsDataSourceModel
			assignSelfServicePlusSettingsDataSourceModel(&state, tc.response)
			if state.Enabled.IsNull() || state.Enabled.IsUnknown() {
				t.Fatalf("expected concrete bool, got null/unknown")
			}
			if got := state.Enabled.ValueBool(); got != tc.wantEnabled {
				t.Errorf("Enabled = %v, want %v", got, tc.wantEnabled)
			}
		})
	}
}

// TestAssign_DoesNotClobberID verifies the assigners leave state.ID untouched. The
// canonical singleton pattern sets state.ID = helpers.SingletonID separately in the
// CRUD handlers; the assigners must not interfere.
func TestAssign_DoesNotClobberID(t *testing.T) {
	enabledTrue := true
	enabledFalse := false

	state := SelfServicePlusSettingsResourceModel{
		ID: types.StringValue("singleton"),
	}
	assignSelfServicePlusSettingsResourceModel(&state, &pro.SelfServicePlusSettings{Enabled: &enabledTrue})
	if state.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered: got %q, want %q", state.ID.ValueString(), "singleton")
	}

	dsState := SelfServicePlusSettingsDataSourceModel{
		ID: types.StringValue("singleton"),
	}
	assignSelfServicePlusSettingsDataSourceModel(&dsState, &pro.SelfServicePlusSettings{Enabled: &enabledFalse})
	if dsState.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered on data source: got %q, want %q", dsState.ID.ValueString(), "singleton")
	}
}

// TestSingletonIDConstant pins the import identifier so a downstream rename to
// helpers.SingletonID would be caught by this package's tests (it is load-bearing
// for ImportState validation and the import.sh example).
func TestSingletonIDConstant(t *testing.T) {
	if helpers.SingletonID != "singleton" {
		t.Errorf("helpers.SingletonID drifted: got %q, want %q", helpers.SingletonID, "singleton")
	}
}
