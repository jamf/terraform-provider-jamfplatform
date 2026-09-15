// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package impact_alert_notification_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignImpactAlertNotificationSettingsResourceModel populates a resource model from an SDK
// response. All four fields are non-pointer bools that the API echoes on every successful
// GET (full-replace PUT), so this is a pure copy — no nil guard, no prior-model carry
// (nothing read from prior state, so the smtp_server state/prior aliasing trap does not
// apply here).
func assignImpactAlertNotificationSettingsResourceModel(state *ImpactAlertNotificationSettingsResourceModel, s *pro.ImpactAlertNotificationSettingsV1) {
	state.DeployableObjectsAlertEnabled = types.BoolValue(s.DeployableObjectsAlertEnabled)
	state.DeployableObjectsConfirmationCodeEnabled = types.BoolValue(s.DeployableObjectsConfirmationCodeEnabled)
	state.ScopeableObjectsAlertEnabled = types.BoolValue(s.ScopeableObjectsAlertEnabled)
	state.ScopeableObjectsConfirmationCodeEnabled = types.BoolValue(s.ScopeableObjectsConfirmationCodeEnabled)
}

// assignImpactAlertNotificationSettingsDataSourceModel populates a data source model from an
// SDK response. Same pure-copy semantics as the resource assigner.
func assignImpactAlertNotificationSettingsDataSourceModel(state *ImpactAlertNotificationSettingsDataSourceModel, s *pro.ImpactAlertNotificationSettingsV1) {
	state.DeployableObjectsAlertEnabled = types.BoolValue(s.DeployableObjectsAlertEnabled)
	state.DeployableObjectsConfirmationCodeEnabled = types.BoolValue(s.DeployableObjectsConfirmationCodeEnabled)
	state.ScopeableObjectsAlertEnabled = types.BoolValue(s.ScopeableObjectsAlertEnabled)
	state.ScopeableObjectsConfirmationCodeEnabled = types.BoolValue(s.ScopeableObjectsConfirmationCodeEnabled)
}
