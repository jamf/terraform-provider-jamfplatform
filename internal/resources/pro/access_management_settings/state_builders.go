// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package access_management_settings

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignAccessManagementSettingsResourceModel populates resource state from an Access
// Management settings response.
//
// The field is Optional+Computed. It is reconciled with ReconcileOptionalStringPointer
// (NOT StringPointerValueOrNull): a fresh tenant echoes "" for "no ADE server
// configured", which maps to a null state value — but if the user explicitly declared
// `automated_device_enrollment_server_uuid = ""` (to clear the setting), Reconcile keeps
// that explicit "" so the planned value equals the post-apply state value and the
// framework consistency check passes. The `current` argument is the prior model field,
// which carries that explicit-"" distinction.
func assignAccessManagementSettingsResourceModel(state *AccessManagementSettingsResourceModel, s *pro.AccessManagementSetting) {
	if s == nil {
		return
	}
	state.AutomatedDeviceEnrollmentServerUUID = helpers.ReconcileOptionalStringPointer(
		s.AutomatedDeviceEnrollmentServerUUID,
		state.AutomatedDeviceEnrollmentServerUUID,
	)
}

// assignAccessManagementSettingsDataSourceModel populates data source state from an
// Access Management settings GET response. The data source has no prior user config, so
// an empty wire value maps straight to null.
func assignAccessManagementSettingsDataSourceModel(state *AccessManagementSettingsDataSourceModel, s *pro.AccessManagementSetting) {
	if s == nil {
		return
	}
	state.AutomatedDeviceEnrollmentServerUUID = helpers.StringPointerValueOrNull(s.AutomatedDeviceEnrollmentServerUUID)
}
