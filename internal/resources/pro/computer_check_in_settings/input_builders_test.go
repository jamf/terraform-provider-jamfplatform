// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_check_in_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestBuildComputerCheckInSettingsInput verifies every plan field maps to the correct SDK
// pointer. Distinct bool values per field catch a swapped mapping.
func TestBuildComputerCheckInSettingsInput(t *testing.T) {
	plan := ComputerCheckInSettingsResourceModel{
		CheckInFrequency:                types.Int64Value(60),
		CreateStartupScript:             types.BoolValue(true),
		StartupLog:                      types.BoolValue(false),
		StartupPolicies:                 types.BoolValue(true),
		StartupSsh:                      types.BoolValue(false),
		CreateLoginHook:                 types.BoolValue(true),
		LoginHookLog:                    types.BoolValue(false),
		LoginHookPolicies:               types.BoolValue(true),
		AllowNetworkStateChangeTriggers: types.BoolValue(false),
	}

	out := buildComputerCheckInSettingsInput(plan, nil)

	if out.CheckInFrequency == nil || *out.CheckInFrequency != 60 {
		t.Errorf("CheckInFrequency = %v, want 60", out.CheckInFrequency)
	}

	checks := []struct {
		name string
		got  *bool
		want bool
	}{
		{"CreateStartupScript", out.CreateStartupScript, true},
		{"StartupLog", out.StartupLog, false},
		{"StartupPolicies", out.StartupPolicies, true},
		{"StartupSsh", out.StartupSsh, false},
		{"CreateHooks (create_login_hook)", out.CreateHooks, true},
		{"HookLog (login_hook_log)", out.HookLog, false},
		{"HookPolicies (login_hook_policies)", out.HookPolicies, true},
		{"EnableLocalConfigurationProfiles (allow_network_state_change_triggers)", out.EnableLocalConfigurationProfiles, false},
	}
	for _, c := range checks {
		if c.got == nil {
			t.Errorf("%s: nil pointer, want %v", c.name, c.want)
			continue
		}
		if *c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, *c.got, c.want)
		}
	}
}

// TestBuildComputerCheckInSettingsInput_OmittedAdoptsCurrent verifies the
// GET-on-create merge: a toggle omitted from the plan (Unknown, as on first create)
// takes the value from the live settings `current` rather than defaulting to false,
// so the singleton is adopted, not reset. A declared toggle still wins over current.
func TestBuildComputerCheckInSettingsInput_OmittedAdoptsCurrent(t *testing.T) {
	tr, fa := true, false
	current := &pro.ClientCheckInV3{
		CreateStartupScript:              &tr, // omitted in plan -> adopt true
		StartupLog:                       &tr, // declared false in plan -> plan wins
		StartupPolicies:                  &tr, // omitted -> adopt true
		StartupSsh:                       &fa, // omitted -> adopt false
		CreateHooks:                      &tr, // omitted -> adopt true
		HookLog:                          &tr, // omitted -> adopt true
		HookPolicies:                     &tr, // omitted -> adopt true
		EnableLocalConfigurationProfiles: &tr, // omitted -> adopt true
	}
	plan := ComputerCheckInSettingsResourceModel{
		CheckInFrequency:                types.Int64Value(15),
		CreateStartupScript:             types.BoolUnknown(),
		StartupLog:                      types.BoolValue(false), // declared
		StartupPolicies:                 types.BoolUnknown(),
		StartupSsh:                      types.BoolNull(),
		CreateLoginHook:                 types.BoolUnknown(),
		LoginHookLog:                    types.BoolUnknown(),
		LoginHookPolicies:               types.BoolUnknown(),
		AllowNetworkStateChangeTriggers: types.BoolUnknown(),
	}

	out := buildComputerCheckInSettingsInput(plan, current)

	want := map[string]struct {
		got  *bool
		want bool
	}{
		"CreateStartupScript":              {out.CreateStartupScript, true},
		"StartupLog":                       {out.StartupLog, false}, // plan wins
		"StartupPolicies":                  {out.StartupPolicies, true},
		"StartupSsh":                       {out.StartupSsh, false},
		"CreateHooks":                      {out.CreateHooks, true},
		"HookLog":                          {out.HookLog, true},
		"HookPolicies":                     {out.HookPolicies, true},
		"EnableLocalConfigurationProfiles": {out.EnableLocalConfigurationProfiles, true},
	}
	for name, c := range want {
		if c.got == nil {
			t.Errorf("%s = nil, want %v", name, c.want)
			continue
		}
		if *c.got != c.want {
			t.Errorf("%s = %v, want %v", name, *c.got, c.want)
		}
	}
}
