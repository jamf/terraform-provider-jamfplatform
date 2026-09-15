// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_check_in_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// defaultCheckInFrequency is the value used for the impossible-nil defensive branch in
// the assigners. The Jamf Pro API echoes checkInFrequency on every successful GET, so
// this branch should never fire; 15 is the UI default and a member of the OneOf set,
// so even the dead branch cannot emit an out-of-set value.
const defaultCheckInFrequency int64 = 15

// assignComputerCheckInSettingsResourceModel populates a resource model from an SDK response.
//
// check_in_frequency is Required in the schema, so state.CheckInFrequency must always
// end up populated with a concrete value. The bools are Optional+Computed; the API
// echoes every field on every successful GET (full-replace PUT), so the nil branches
// below are defensive fallbacks only — `false` is the safest default for a missing
// feature-toggle bool.
func assignComputerCheckInSettingsResourceModel(state *ComputerCheckInSettingsResourceModel, s *pro.ClientCheckInV3) {
	if s.CheckInFrequency != nil {
		state.CheckInFrequency = types.Int64Value(int64(*s.CheckInFrequency))
	} else {
		state.CheckInFrequency = types.Int64Value(defaultCheckInFrequency)
	}
	state.CreateStartupScript = boolOrFalse(s.CreateStartupScript)
	state.StartupLog = boolOrFalse(s.StartupLog)
	state.StartupPolicies = boolOrFalse(s.StartupPolicies)
	state.StartupSsh = boolOrFalse(s.StartupSsh)
	state.CreateLoginHook = boolOrFalse(s.CreateHooks)
	state.LoginHookLog = boolOrFalse(s.HookLog)
	state.LoginHookPolicies = boolOrFalse(s.HookPolicies)
	state.AllowNetworkStateChangeTriggers = boolOrFalse(s.EnableLocalConfigurationProfiles)
}

// assignComputerCheckInSettingsDataSourceModel populates a data source model from an SDK
// response. Same nil-fallback semantics as the resource assigner.
func assignComputerCheckInSettingsDataSourceModel(state *ComputerCheckInSettingsDataSourceModel, s *pro.ClientCheckInV3) {
	if s.CheckInFrequency != nil {
		state.CheckInFrequency = types.Int64Value(int64(*s.CheckInFrequency))
	} else {
		state.CheckInFrequency = types.Int64Value(defaultCheckInFrequency)
	}
	state.CreateStartupScript = boolOrFalse(s.CreateStartupScript)
	state.StartupLog = boolOrFalse(s.StartupLog)
	state.StartupPolicies = boolOrFalse(s.StartupPolicies)
	state.StartupSsh = boolOrFalse(s.StartupSsh)
	state.CreateLoginHook = boolOrFalse(s.CreateHooks)
	state.LoginHookLog = boolOrFalse(s.HookLog)
	state.LoginHookPolicies = boolOrFalse(s.HookPolicies)
	state.AllowNetworkStateChangeTriggers = boolOrFalse(s.EnableLocalConfigurationProfiles)
}

// boolOrFalse maps an SDK *bool to a concrete types.Bool, defaulting a nil pointer to
// false (assumes "off" rather than "on" for a missing feature toggle).
func boolOrFalse(b *bool) types.Bool {
	if b != nil {
		return types.BoolValue(*b)
	}
	return types.BoolValue(false)
}
