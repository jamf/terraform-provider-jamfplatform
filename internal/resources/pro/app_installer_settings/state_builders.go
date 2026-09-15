// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignAppInstallerSettingsResourceModel populates a resource model from an SDK response.
func assignAppInstallerSettingsResourceModel(state *AppInstallerSettingsResourceModel, s *pro.AppInstallersGlobalSettings) {
	state.DeploymentSettings = assignDeploymentSettings(s.DeploymentProcessControls)
	state.EndUserExperience = assignEndUserExperience(s.EndUserExperienceSettings)
}

// assignAppInstallerSettingsDataSourceModel populates a data source model from an SDK response.
func assignAppInstallerSettingsDataSourceModel(state *AppInstallerSettingsDataSourceModel, s *pro.AppInstallersGlobalSettings) {
	state.DeploymentSettings = assignDeploymentSettings(s.DeploymentProcessControls)
	state.EndUserExperience = assignEndUserExperience(s.EndUserExperienceSettings)
}

// assignDeploymentSettings normalizes the SDK block using value-based logic:
// the block is ObjectNull iff ALL leaf fields are nil. This ensures import
// captures actual server values rather than returning null when prior state
// has no block.
func assignDeploymentSettings(s *pro.AppInstallersDeploymentProcessControls) types.Object {
	if s == nil {
		return types.ObjectNull(deploymentSettingsAttrTypes)
	}
	if s.CommandsBatchSize == nil && s.BatchFrequencyInMinutes == nil &&
		s.DaysOfWeek == nil && s.FromTimeOfDay == nil && s.ToTimeOfDay == nil {
		return types.ObjectNull(deploymentSettingsAttrTypes)
	}
	return types.ObjectValueMust(deploymentSettingsAttrTypes, map[string]attr.Value{
		"batch_size":       int64PtrValueOrNull(s.CommandsBatchSize),
		"batch_frequency":  int64PtrValueOrNull(s.BatchFrequencyInMinutes),
		"days":             assignDays(s.DaysOfWeek),
		"server_time_from": stringPtrValueOrNull(s.FromTimeOfDay),
		"server_time_to":   stringPtrValueOrNull(s.ToTimeOfDay),
	})
}

// assignEndUserExperience normalizes the SDK block using value-based logic.
func assignEndUserExperience(s pro.GlobalSettingsEndUserExperience) types.Object {
	if s.NotificationInterval == nil && s.NotificationMessage == nil &&
		s.Deadline == nil && s.DeadlineMessage == nil &&
		s.QuitDelay == nil && s.CompleteMessage == nil &&
		s.Relaunch == nil && s.Suppress == nil {
		return types.ObjectNull(endUserExperienceAttrTypes)
	}
	return types.ObjectValueMust(endUserExperienceAttrTypes, map[string]attr.Value{
		"notification_frequency":  int64PtrOrNull(s.NotificationInterval),
		"notification_message":    stringPtrValueOrNull(s.NotificationMessage),
		"update_deadline":         int64PtrOrNull(s.Deadline),
		"force_quit_message":      stringPtrValueOrNull(s.DeadlineMessage),
		"force_quit_grace_period": int64PtrOrNull(s.QuitDelay),
		"update_complete_message": stringPtrValueOrNull(s.CompleteMessage),
		"relaunch":                boolPtrValueOrNull(s.Relaunch),
		"suppress":                boolPtrValueOrNull(s.Suppress),
	})
}

// assignDays converts *[]string to types.Set, preserving the null vs empty distinction.
// nil pointer → SetNull (not configured); &[]string{} → empty Set; non-empty slice → populated Set.
func assignDays(days *[]string) types.Set {
	if days == nil {
		return types.SetNull(types.StringType)
	}
	elems := make([]attr.Value, len(*days))
	for i, d := range *days {
		elems[i] = types.StringValue(d)
	}
	return types.SetValueMust(types.StringType, elems)
}

func int64PtrValueOrNull(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

func int64PtrOrNull(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}

func stringPtrValueOrNull(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

func boolPtrValueOrNull(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}
