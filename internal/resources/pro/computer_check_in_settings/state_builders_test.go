// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_check_in_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// fullResponse returns a ClientCheckInV3 with every field populated, wiring distinct
// bool values per field so a swapped mapping is caught.
func fullResponse() *pro.ClientCheckInV3 {
	return &pro.ClientCheckInV3{
		CheckInFrequency:                 new(30),
		CreateStartupScript:              new(true),
		StartupLog:                       new(false),
		StartupPolicies:                  new(true),
		StartupSsh:                       new(false),
		CreateHooks:                      new(true),
		HookLog:                          new(false),
		HookPolicies:                     new(true),
		EnableLocalConfigurationProfiles: new(false),
	}
}

func TestAssignComputerCheckInSettingsResourceModel_AllFields(t *testing.T) {
	var state ComputerCheckInSettingsResourceModel
	assignComputerCheckInSettingsResourceModel(&state, fullResponse())

	if state.CheckInFrequency.ValueInt64() != 30 {
		t.Errorf("CheckInFrequency = %d, want 30", state.CheckInFrequency.ValueInt64())
	}
	checks := []struct {
		name string
		got  types.Bool
		want bool
	}{
		{"create_startup_script", state.CreateStartupScript, true},
		{"startup_log", state.StartupLog, false},
		{"startup_policies", state.StartupPolicies, true},
		{"startup_ssh", state.StartupSsh, false},
		{"create_login_hook", state.CreateLoginHook, true},
		{"login_hook_log", state.LoginHookLog, false},
		{"login_hook_policies", state.LoginHookPolicies, true},
		{"allow_network_state_change_triggers", state.AllowNetworkStateChangeTriggers, false},
	}
	for _, c := range checks {
		if c.got.IsNull() || c.got.IsUnknown() {
			t.Errorf("%s: expected concrete bool, got null/unknown", c.name)
			continue
		}
		if c.got.ValueBool() != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got.ValueBool(), c.want)
		}
	}
}

func TestAssignComputerCheckInSettingsDataSourceModel_AllFields(t *testing.T) {
	var state ComputerCheckInSettingsDataSourceModel
	assignComputerCheckInSettingsDataSourceModel(&state, fullResponse())

	if state.CheckInFrequency.ValueInt64() != 30 {
		t.Errorf("CheckInFrequency = %d, want 30", state.CheckInFrequency.ValueInt64())
	}
	checks := []struct {
		name string
		got  types.Bool
		want bool
	}{
		{"create_startup_script", state.CreateStartupScript, true},
		{"startup_log", state.StartupLog, false},
		{"startup_policies", state.StartupPolicies, true},
		{"startup_ssh", state.StartupSsh, false},
		{"create_login_hook", state.CreateLoginHook, true},
		{"login_hook_log", state.LoginHookLog, false},
		{"login_hook_policies", state.LoginHookPolicies, true},
		{"allow_network_state_change_triggers", state.AllowNetworkStateChangeTriggers, false},
	}
	for _, c := range checks {
		if c.got.ValueBool() != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got.ValueBool(), c.want)
		}
	}
}

// TestAssign_NilFieldsDefault verifies the defensive nil branches: nil bools collapse
// to false and a nil (impossible) check_in_frequency falls back to the OneOf-valid
// default rather than 0.
func TestAssign_NilFieldsDefault(t *testing.T) {
	var state ComputerCheckInSettingsResourceModel
	assignComputerCheckInSettingsResourceModel(&state, &pro.ClientCheckInV3{})

	if state.CheckInFrequency.ValueInt64() != defaultCheckInFrequency {
		t.Errorf("nil CheckInFrequency = %d, want %d", state.CheckInFrequency.ValueInt64(), defaultCheckInFrequency)
	}
	bools := []types.Bool{
		state.CreateStartupScript, state.StartupLog, state.StartupPolicies, state.StartupSsh,
		state.CreateLoginHook, state.LoginHookLog, state.LoginHookPolicies, state.AllowNetworkStateChangeTriggers,
	}
	for i, b := range bools {
		if b.IsNull() || b.IsUnknown() {
			t.Errorf("bool[%d]: expected concrete value, got null/unknown", i)
		}
		if b.ValueBool() != false {
			t.Errorf("bool[%d] = %v, want false", i, b.ValueBool())
		}
	}
}

// TestAssign_DoesNotClobberID verifies the assigners leave state.ID untouched.
func TestAssign_DoesNotClobberID(t *testing.T) {
	state := ComputerCheckInSettingsResourceModel{ID: types.StringValue("singleton")}
	assignComputerCheckInSettingsResourceModel(&state, fullResponse())
	if state.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered: got %q, want %q", state.ID.ValueString(), "singleton")
	}

	dsState := ComputerCheckInSettingsDataSourceModel{ID: types.StringValue("singleton")}
	assignComputerCheckInSettingsDataSourceModel(&dsState, fullResponse())
	if dsState.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered on data source: got %q, want %q", dsState.ID.ValueString(), "singleton")
	}
}

// TestSingletonIDConstant pins the import identifier.
func TestSingletonIDConstant(t *testing.T) {
	if helpers.SingletonID != "singleton" {
		t.Errorf("helpers.SingletonID drifted: got %q, want %q", helpers.SingletonID, "singleton")
	}
}
