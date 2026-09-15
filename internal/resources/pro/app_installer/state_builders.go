// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignAppInstallerResourceModel populates a resource model from the flat
// single-deployment GET response. The scalar fields are always refreshed; the
// nested notification_settings / self_service_settings blocks are only refreshed
// when the caller (plan or current state) already manages them. The server
// echoes both blocks populated even when the user never set them, so refreshing
// an unmanaged block would violate the framework's "produced inconsistent result
// after apply" check (plan said null, we'd return a populated object). See
// feedback_optional_computed_nested_object.
//
// includeUnmanaged inverts those block gates for the list resource's
// config-generation path (terraform query -generate-config-out): there is no
// plan to stay consistent with, so every wire-present block is refreshed,
// yielding a complete exported config rather than a scalar-only one. CRUD
// callers pass false.
func assignAppInstallerResourceModel(state *AppInstallerResourceModel, d *pro.AppTitleDeploymentRead, includeUnmanaged bool) {
	if d == nil {
		return
	}

	state.ID = types.StringValue(d.ID)
	state.Name = types.StringValue(d.Name)
	state.Enabled = types.BoolValue(d.Enabled)
	state.AppTitleID = types.StringValue(d.AppTitleID)
	state.DeploymentType = types.StringValue(d.DeploymentType)
	state.UpdateBehavior = types.StringValue(d.UpdateBehavior)
	state.SelectedVersion = types.StringValue(d.SelectedVersion)
	state.LatestAvailableVersion = types.StringValue(d.LatestAvailableVersion)
	state.TitleAvailableInAis = types.BoolValue(d.TitleAvailableInAis)
	state.VersionRemoved = types.BoolValue(d.VersionRemoved)
	state.CategoryID = types.StringValue(d.CategoryID)
	state.SiteID = types.StringValue(d.SiteID)
	state.SmartGroupID = types.StringValue(d.SmartGroupID)
	state.InstallPredefinedConfigProfiles = types.BoolValue(d.InstallPredefinedConfigProfiles)
	state.TriggerAdminNotifications = types.BoolValue(d.TriggerAdminNotifications)

	if (includeUnmanaged || state.NotificationSettings != nil) && d.NotificationSettings != nil {
		state.NotificationSettings = flattenNotificationSettings(d.NotificationSettings)
	}
	if (includeUnmanaged || state.SelfServiceSettings != nil) && d.SelfServiceSettings != nil {
		state.SelfServiceSettings = flattenSelfServiceSettings(state.SelfServiceSettings, d.SelfServiceSettings, includeUnmanaged)
	}
}

// flattenNotificationSettings maps the notification block. Every field is
// nullable on the wire (the server echoes null for an unset field, never a zero
// value), so null is preserved as a TF null — matching the Optional-only schema
// and the omit-when-unset input builder, which keeps state consistent with config.
func flattenNotificationSettings(n *pro.AppTitleDeploymentNotificationSettings) *NotificationSettingsModel {
	return &NotificationSettingsModel{
		NotificationMessage:  stringPtrValueOrNull(n.NotificationMessage),
		NotificationInterval: int64PtrValueOrNull(n.NotificationInterval),
		DeadlineMessage:      stringPtrValueOrNull(n.DeadlineMessage),
		Deadline:             int64PtrValueOrNull(n.Deadline),
		QuitDelay:            int64PtrValueOrNull(n.QuitDelay),
		CompleteMessage:      stringPtrValueOrNull(n.CompleteMessage),
		Relaunch:             boolPtrValueOrNull(n.Relaunch),
		Suppress:             boolPtrValueOrNull(n.Suppress),
	}
}

// flattenSelfServiceSettings maps the full Self Service block. The server echoes
// every field including per-category featured, so plain value assignment is safe
// for the scalars.
//
// categories is Optional-only (not Computed), so its post-apply value must match
// config exactly. The input builder always sends categories as a slice (empty =
// clear), so the server echoes the same membership we sent — but a GET cannot
// distinguish "user omitted categories" from "user sent []" (both wire as no
// categories). We therefore take the null-vs-empty membership decision from the
// prior model: a nil prior (omitted in config) stays nil/null; a non-nil prior
// (config-supplied, possibly empty) is refreshed from the server echo so the
// Optional+Computed per-entry featured resolves from its Unknown plan value.
// Synthesising an empty slice for a nil prior would turn an omitted (null) set
// into an empty set and trip the "produced inconsistent result after apply"
// check. See feedback_optional_computed_nested_object.
//
// includeUnmanaged releases that prior-model gate on first hydration. The
// caller's own gate on self_service_settings already released there, but prior
// is the pre-assignment value of state.SelfServiceSettings, which is nil on
// import — so the block hydrated while categories underneath it did not, on
// that Read or any later one (issue #391). Adoption is still conditional on the
// server actually reporting categories, because storing an empty set where a
// create that never declared the attribute stores null breaks
// ImportStateVerify.
func flattenSelfServiceSettings(prior *SelfServiceSettingsModel, s *pro.AppTitleDeploymentSelfServiceSettings, includeUnmanaged bool) *SelfServiceSettingsModel {
	out := &SelfServiceSettingsModel{
		Description:                 stringPtrValueOrNull(s.Description),
		ForceViewDescription:        boolPtrValueOrFalse(s.ForceViewDescription),
		IncludeInComplianceCategory: boolPtrValueOrFalse(s.IncludeInComplianceCategory),
		IncludeInFeaturedCategory:   boolPtrValueOrFalse(s.IncludeInFeaturedCategory),
	}

	declared := prior != nil && prior.Categories != nil
	hydrate := includeUnmanaged && s.Categories != nil && len(*s.Categories) > 0
	if !declared && !hydrate {
		return out
	}

	out.Categories = []SelfServiceCategoryModel{}
	if s.Categories != nil {
		out.Categories = make([]SelfServiceCategoryModel, 0, len(*s.Categories))
		for _, c := range *s.Categories {
			out.Categories = append(out.Categories, SelfServiceCategoryModel{
				CategoryID: types.StringValue(c.ID),
				Featured:   boolPtrValueOrFalse(c.Featured),
			})
		}
	}
	return out
}

// assignAppInstallerDataSourceModel projects the flat GET shape into the
// singular data source model (scalar fields only).
func assignAppInstallerDataSourceModel(state *AppInstallerDataSourceModel, d *pro.AppTitleDeploymentRead) {
	if d == nil {
		return
	}
	state.ID = types.StringValue(d.ID)
	state.Name = types.StringValue(d.Name)
	state.Enabled = types.BoolValue(d.Enabled)
	state.AppTitleID = types.StringValue(d.AppTitleID)
	state.DeploymentType = types.StringValue(d.DeploymentType)
	state.UpdateBehavior = types.StringValue(d.UpdateBehavior)
	state.SelectedVersion = types.StringValue(d.SelectedVersion)
	state.LatestAvailableVersion = types.StringValue(d.LatestAvailableVersion)
	state.TitleAvailableInAis = types.BoolValue(d.TitleAvailableInAis)
	state.VersionRemoved = types.BoolValue(d.VersionRemoved)
	state.CategoryID = types.StringValue(d.CategoryID)
	state.SiteID = types.StringValue(d.SiteID)
	state.SmartGroupID = types.StringValue(d.SmartGroupID)
	state.InstallPredefinedConfigProfiles = types.BoolValue(d.InstallPredefinedConfigProfiles)
	state.TriggerAdminNotifications = types.BoolValue(d.TriggerAdminNotifications)
}

// stringPtrValueOrNull maps an SDK *string to a TF String, nil → null. Used for
// nullable fields the server echoes as null when unset.
func stringPtrValueOrNull(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// int64PtrValueOrNull maps an SDK *int to a TF Int64, nil → null.
func int64PtrValueOrNull(p *int64) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*p)
}

// boolPtrValueOrNull maps an SDK *bool to a TF Bool, nil → null. Used for
// nullable notification booleans the server echoes as null when unset.
func boolPtrValueOrNull(p *bool) types.Bool {
	if p == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*p)
}

// boolPtrValueOrFalse maps an SDK *bool to a TF Bool, nil → false. Used for the
// Self Service booleans, which the server defaults to false.
func boolPtrValueOrFalse(p *bool) types.Bool {
	if p == nil {
		return types.BoolValue(false)
	}
	return types.BoolValue(*p)
}
