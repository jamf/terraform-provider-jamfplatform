// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildMergedInput merges the plan over the current server state. This preserves the
// "omit = don't manage" contract: if the user omits a top-level block (null or
// unknown in plan) the current server values for that block are passed through
// unchanged. Within a managed block, null leaf values clear the corresponding
// server field (field-level full-replace).
// Pass current=nil on unit tests that exercise the build logic in isolation.
func buildMergedInput(current *pro.AppInstallersGlobalSettings, plan AppInstallerSettingsResourceModel) *pro.AppInstallersGlobalSettings {
	out := &pro.AppInstallersGlobalSettings{}

	if plan.DeploymentSettings.IsNull() || plan.DeploymentSettings.IsUnknown() {
		if current != nil {
			out.DeploymentProcessControls = current.DeploymentProcessControls
		}
	} else {
		var m DeploymentSettingsModel
		_ = plan.DeploymentSettings.As(context.Background(), &m, basetypes.ObjectAsOptions{})
		out.DeploymentProcessControls = buildDeploymentSettings(&m)
	}

	if plan.EndUserExperience.IsNull() || plan.EndUserExperience.IsUnknown() {
		if current != nil {
			out.EndUserExperienceSettings = current.EndUserExperienceSettings
		}
	} else {
		var m EndUserExperienceModel
		_ = plan.EndUserExperience.As(context.Background(), &m, basetypes.ObjectAsOptions{})
		out.EndUserExperienceSettings = buildEndUserExperience(&m)
	}

	return out
}

// buildAppInstallerSettingsInput converts the Terraform plan model into an SDK payload
// without a merge base. Used only in unit tests that verify the build helpers in isolation.
func buildAppInstallerSettingsInput(plan AppInstallerSettingsResourceModel) *pro.AppInstallersGlobalSettings {
	return buildMergedInput(nil, plan)
}

func buildDeploymentSettings(m *DeploymentSettingsModel) *pro.AppInstallersDeploymentProcessControls {
	if m == nil {
		return nil
	}
	return &pro.AppInstallersDeploymentProcessControls{
		CommandsBatchSize:       int64ValueToIntPtr(m.BatchSize),
		BatchFrequencyInMinutes: int64ValueToIntPtr(m.BatchFrequency),
		DaysOfWeek:              buildDays(m.Days),
		FromTimeOfDay:           stringValueToPtr(m.ServerTimeFrom),
		ToTimeOfDay:             stringValueToPtr(m.ServerTimeTo),
	}
}

// buildEndUserExperience returns the block by value: the Jamf API declares
// endUserExperienceSettings as a required member of the global-settings body, so
// there is no omitted form to express — a nil model yields the all-null block,
// which the server reads as "clear every field".
func buildEndUserExperience(m *EndUserExperienceModel) pro.GlobalSettingsEndUserExperience {
	if m == nil {
		return pro.GlobalSettingsEndUserExperience{}
	}
	return pro.GlobalSettingsEndUserExperience{
		NotificationInterval: int64ValueToPtr(m.NotificationFrequency),
		NotificationMessage:  stringValueToPtr(m.NotificationMessage),
		Deadline:             int64ValueToPtr(m.UpdateDeadline),
		DeadlineMessage:      stringValueToPtr(m.ForceQuitMessage),
		QuitDelay:            int64ValueToPtr(m.ForceQuitGracePeriod),
		CompleteMessage:      stringValueToPtr(m.UpdateCompleteMessage),
		Relaunch:             boolValueToPtr(m.Relaunch),
		Suppress:             boolValueToPtr(m.Suppress),
	}
}

// buildDays converts types.Set to *[]string.
// null/unknown → nil (omit from payload = clear); empty set → &[]string{} (explicit empty).
func buildDays(s types.Set) *[]string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	days := make([]string, 0, len(s.Elements()))
	for _, elem := range s.Elements() {
		days = append(days, elem.(types.String).ValueString())
	}
	return &days
}

func int64ValueToIntPtr(v types.Int64) *int {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	n := int(v.ValueInt64())
	return &n
}

func int64ValueToPtr(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return new(v.ValueInt64())
}

func stringValueToPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func boolValueToPtr(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}
