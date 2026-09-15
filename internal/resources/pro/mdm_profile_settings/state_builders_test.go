// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mdm_profile_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// fullResponse builds a MDMProfileSettings with every field populated.
func fullResponse(b1, b2, b3, b4 bool, i1, i2 int) *pro.DeviceCommunicationSettings {
	return &pro.DeviceCommunicationSettings{
		AutoRenewComputerMDMProfileWhenCaRenewed:                      new(b1),
		AutoRenewComputerMDMProfileWhenDeviceIdentityCertExpiring:     new(b2),
		MDMProfileComputerExpirationLimitInDays:                       new(i1),
		AutoRenewMobileDeviceMDMProfileWhenCaRenewed:                  new(b3),
		AutoRenewMobileDeviceMDMProfileWhenDeviceIdentityCertExpiring: new(b4),
		MDMProfileMobileDeviceExpirationLimitInDays:                   new(i2),
	}
}

func TestAssignMDMProfileSettingsResourceModel(t *testing.T) {
	tests := []struct {
		name     string
		response *pro.DeviceCommunicationSettings
		want     MDMProfileSettingsResourceModel
	}{
		{
			name:     "all true / limits set",
			response: fullResponse(true, true, true, true, 180, 90),
			want: MDMProfileSettingsResourceModel{
				AutoRenewComputerProfileWhenCaRenewed:     types.BoolValue(true),
				AutoRenewComputerProfileBeforeExpiry:      types.BoolValue(true),
				ComputerProfileExpirationLimitDays:        types.Int64Value(180),
				AutoRenewMobileDeviceProfileWhenCaRenewed: types.BoolValue(true),
				AutoRenewMobileDeviceProfileBeforeExpiry:  types.BoolValue(true),
				MobileDeviceProfileExpirationLimitDays:    types.Int64Value(90),
			},
		},
		{
			name:     "all false / limits set",
			response: fullResponse(false, false, false, false, 7, 365),
			want: MDMProfileSettingsResourceModel{
				AutoRenewComputerProfileWhenCaRenewed:     types.BoolValue(false),
				AutoRenewComputerProfileBeforeExpiry:      types.BoolValue(false),
				ComputerProfileExpirationLimitDays:        types.Int64Value(7),
				AutoRenewMobileDeviceProfileWhenCaRenewed: types.BoolValue(false),
				AutoRenewMobileDeviceProfileBeforeExpiry:  types.BoolValue(false),
				MobileDeviceProfileExpirationLimitDays:    types.Int64Value(365),
			},
		},
		{
			name:     "all nil -> defensive zero values",
			response: &pro.DeviceCommunicationSettings{},
			want: MDMProfileSettingsResourceModel{
				AutoRenewComputerProfileWhenCaRenewed:     types.BoolValue(false),
				AutoRenewComputerProfileBeforeExpiry:      types.BoolValue(false),
				ComputerProfileExpirationLimitDays:        types.Int64Value(0),
				AutoRenewMobileDeviceProfileWhenCaRenewed: types.BoolValue(false),
				AutoRenewMobileDeviceProfileBeforeExpiry:  types.BoolValue(false),
				MobileDeviceProfileExpirationLimitDays:    types.Int64Value(0),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var state MDMProfileSettingsResourceModel
			assignMDMProfileSettingsResourceModel(&state, tc.response)
			assertNoNullUnknown(t, state)
			if state.AutoRenewComputerProfileWhenCaRenewed != tc.want.AutoRenewComputerProfileWhenCaRenewed {
				t.Errorf("computer when_ca_renewed = %v, want %v", state.AutoRenewComputerProfileWhenCaRenewed, tc.want.AutoRenewComputerProfileWhenCaRenewed)
			}
			if state.AutoRenewComputerProfileBeforeExpiry != tc.want.AutoRenewComputerProfileBeforeExpiry {
				t.Errorf("computer before_expiry = %v, want %v", state.AutoRenewComputerProfileBeforeExpiry, tc.want.AutoRenewComputerProfileBeforeExpiry)
			}
			if state.ComputerProfileExpirationLimitDays != tc.want.ComputerProfileExpirationLimitDays {
				t.Errorf("computer limit = %v, want %v", state.ComputerProfileExpirationLimitDays, tc.want.ComputerProfileExpirationLimitDays)
			}
			if state.AutoRenewMobileDeviceProfileWhenCaRenewed != tc.want.AutoRenewMobileDeviceProfileWhenCaRenewed {
				t.Errorf("mobile when_ca_renewed = %v, want %v", state.AutoRenewMobileDeviceProfileWhenCaRenewed, tc.want.AutoRenewMobileDeviceProfileWhenCaRenewed)
			}
			if state.AutoRenewMobileDeviceProfileBeforeExpiry != tc.want.AutoRenewMobileDeviceProfileBeforeExpiry {
				t.Errorf("mobile before_expiry = %v, want %v", state.AutoRenewMobileDeviceProfileBeforeExpiry, tc.want.AutoRenewMobileDeviceProfileBeforeExpiry)
			}
			if state.MobileDeviceProfileExpirationLimitDays != tc.want.MobileDeviceProfileExpirationLimitDays {
				t.Errorf("mobile limit = %v, want %v", state.MobileDeviceProfileExpirationLimitDays, tc.want.MobileDeviceProfileExpirationLimitDays)
			}
		})
	}
}

func TestAssignMDMProfileSettingsDataSourceModel(t *testing.T) {
	var state MDMProfileSettingsDataSourceModel
	assignMDMProfileSettingsDataSourceModel(&state, fullResponse(true, false, true, false, 30, 60))

	if v := state.AutoRenewComputerProfileWhenCaRenewed; v.IsNull() || v.IsUnknown() || !v.ValueBool() {
		t.Errorf("computer when_ca_renewed = %v, want true", v)
	}
	if v := state.AutoRenewComputerProfileBeforeExpiry; v.IsNull() || v.IsUnknown() || v.ValueBool() {
		t.Errorf("computer before_expiry = %v, want false", v)
	}
	if v := state.ComputerProfileExpirationLimitDays; v.IsNull() || v.IsUnknown() || v.ValueInt64() != 30 {
		t.Errorf("computer limit = %v, want 30", v)
	}
	if v := state.AutoRenewMobileDeviceProfileWhenCaRenewed; v.IsNull() || v.IsUnknown() || !v.ValueBool() {
		t.Errorf("mobile when_ca_renewed = %v, want true", v)
	}
	if v := state.AutoRenewMobileDeviceProfileBeforeExpiry; v.IsNull() || v.IsUnknown() || v.ValueBool() {
		t.Errorf("mobile before_expiry = %v, want false", v)
	}
	if v := state.MobileDeviceProfileExpirationLimitDays; v.IsNull() || v.IsUnknown() || v.ValueInt64() != 60 {
		t.Errorf("mobile limit = %v, want 60", v)
	}
}

func assertNoNullUnknown(t *testing.T, state MDMProfileSettingsResourceModel) {
	t.Helper()
	bools := []types.Bool{
		state.AutoRenewComputerProfileWhenCaRenewed,
		state.AutoRenewComputerProfileBeforeExpiry,
		state.AutoRenewMobileDeviceProfileWhenCaRenewed,
		state.AutoRenewMobileDeviceProfileBeforeExpiry,
	}
	for i, b := range bools {
		if b.IsNull() || b.IsUnknown() {
			t.Fatalf("bool field %d is null/unknown; Optional+Computed state must be concrete", i)
		}
	}
	ints := []types.Int64{state.ComputerProfileExpirationLimitDays, state.MobileDeviceProfileExpirationLimitDays}
	for i, n := range ints {
		if n.IsNull() || n.IsUnknown() {
			t.Fatalf("int field %d is null/unknown; Optional+Computed state must be concrete", i)
		}
	}
}

// TestAssign_DoesNotClobberID verifies the assigners leave state.ID untouched. The
// canonical singleton pattern sets state.ID = helpers.SingletonID separately in the
// CRUD handlers; the assigners must not interfere.
func TestAssign_DoesNotClobberID(t *testing.T) {
	state := MDMProfileSettingsResourceModel{ID: types.StringValue("singleton")}
	assignMDMProfileSettingsResourceModel(&state, fullResponse(true, true, true, true, 1, 1))
	if state.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered: got %q, want %q", state.ID.ValueString(), "singleton")
	}

	dsState := MDMProfileSettingsDataSourceModel{ID: types.StringValue("singleton")}
	assignMDMProfileSettingsDataSourceModel(&dsState, fullResponse(false, false, false, false, 1, 1))
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
