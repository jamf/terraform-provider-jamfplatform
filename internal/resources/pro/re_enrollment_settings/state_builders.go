// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package re_enrollment_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignReEnrollmentSettingsResourceModel populates resource state from a
// Re-enrollment settings response. The five clear_* toggles are Optional+Computed
// and the server always returns a concrete value, so the state adopts it directly
// with BoolPointerValueOrNull — the Computed half of the attribute, so an omitted
// toggle shows (and preserves) the live value. NOT ReconcileOptionalBoolPointer:
// that helper returns null when prior state is unset, which would blank an omitted
// toggle instead of adopting the server value. The enum is Required and always
// present, taken directly.
func assignReEnrollmentSettingsResourceModel(state *ReEnrollmentSettingsResourceModel, s *pro.Reenrollment) {
	if s == nil {
		return
	}
	state.ClearPolicyLogs = helpers.BoolPointerValueOrNull(s.IsFlushPolicyHistoryEnabled)
	state.ClearLocationInformation = helpers.BoolPointerValueOrNull(s.IsFlushLocationInformationEnabled)
	state.ClearLocationInformationHistory = helpers.BoolPointerValueOrNull(s.IsFlushLocationInformationHistoryEnabled)
	state.ClearExtensionAttributes = helpers.BoolPointerValueOrNull(s.IsFlushExtensionAttributesEnabled)
	state.ClearSoftwareUpdatePlans = helpers.BoolPointerValueOrNull(s.IsFlushSoftwareUpdatePlansEnabled)
	state.ClearManagementHistory = stringValueOrNull(s.FlushMDMQueue)
}

// assignReEnrollmentSettingsDataSourceModel populates data source state from a
// Re-enrollment settings GET response.
func assignReEnrollmentSettingsDataSourceModel(state *ReEnrollmentSettingsDataSourceModel, s *pro.Reenrollment) {
	if s == nil {
		return
	}
	state.ClearPolicyLogs = helpers.BoolPointerValueOrNull(s.IsFlushPolicyHistoryEnabled)
	state.ClearLocationInformation = helpers.BoolPointerValueOrNull(s.IsFlushLocationInformationEnabled)
	state.ClearLocationInformationHistory = helpers.BoolPointerValueOrNull(s.IsFlushLocationInformationHistoryEnabled)
	state.ClearExtensionAttributes = helpers.BoolPointerValueOrNull(s.IsFlushExtensionAttributesEnabled)
	state.ClearSoftwareUpdatePlans = helpers.BoolPointerValueOrNull(s.IsFlushSoftwareUpdatePlansEnabled)
	state.ClearManagementHistory = stringValueOrNull(s.FlushMDMQueue)
}

// stringValueOrNull maps an empty wire string to a null state value.
func stringValueOrNull(v string) types.String {
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}
