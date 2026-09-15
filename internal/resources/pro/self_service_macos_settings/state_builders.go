// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_macos_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignSelfServiceMacosSettingsResourceModel populates a resource model from a GET
// response. Pure copy — the server always echoes every field (wire-probed 2026-06-10), so
// pointer fields adopt the server value directly (per STYLE_GUIDE §Full-replace endpoints:
// Optional+Computed bools/ints adopt, they do not reconcile). A nil nested group has never
// been observed on the wire; the guard substitutes an empty group so its fields land null
// rather than carrying stale prior state.
func assignSelfServiceMacosSettingsResourceModel(state *SelfServiceMacosSettingsResourceModel, s *pro.SelfServiceSettings) {
	install, login, interaction := settingsGroups(s)

	state.InstallAutomatically = helpers.BoolPointerValueOrNull(install.InstallAutomatically)
	state.InstallLocation = types.StringValue(install.InstallLocation)

	state.LoginMethod = types.StringValue(login.UserLoginLevel)
	state.AuthenticationType = types.StringValue(login.AuthType)
	state.KeychainCredentialStorageEnabled = helpers.BoolPointerValueOrNull(login.AllowRememberMe)
	state.Fido2Enabled = helpers.BoolPointerValueOrNull(login.UseFido2)

	state.NotificationsEnabled = helpers.BoolPointerValueOrNull(interaction.NotificationsEnabled)
	state.AlertUserApprovedMdm = helpers.BoolPointerValueOrNull(interaction.AlertUserApprovedMDM)
	state.DefaultLandingPage = helpers.StringPointerValueOrNull(interaction.DefaultLandingPage)
	state.DefaultHomeCategoryID = intPointerValueOrNull(interaction.DefaultHomeCategoryID)
	state.BookmarksDisplayName = types.StringValue(interaction.BookmarksName)
}

// assignSelfServiceMacosSettingsDataSourceModel populates a data source model from a GET
// response. Same pure-copy semantics as the resource assigner.
func assignSelfServiceMacosSettingsDataSourceModel(state *SelfServiceMacosSettingsDataSourceModel, s *pro.SelfServiceSettings) {
	install, login, interaction := settingsGroups(s)

	state.InstallAutomatically = helpers.BoolPointerValueOrNull(install.InstallAutomatically)
	state.InstallLocation = types.StringValue(install.InstallLocation)

	state.LoginMethod = types.StringValue(login.UserLoginLevel)
	state.AuthenticationType = types.StringValue(login.AuthType)
	state.KeychainCredentialStorageEnabled = helpers.BoolPointerValueOrNull(login.AllowRememberMe)
	state.Fido2Enabled = helpers.BoolPointerValueOrNull(login.UseFido2)

	state.NotificationsEnabled = helpers.BoolPointerValueOrNull(interaction.NotificationsEnabled)
	state.AlertUserApprovedMdm = helpers.BoolPointerValueOrNull(interaction.AlertUserApprovedMDM)
	state.DefaultLandingPage = helpers.StringPointerValueOrNull(interaction.DefaultLandingPage)
	state.DefaultHomeCategoryID = intPointerValueOrNull(interaction.DefaultHomeCategoryID)
	state.BookmarksDisplayName = types.StringValue(interaction.BookmarksName)
}

// settingsGroups unwraps the three nested setting groups nil-safely, substituting empty
// groups for any the response did not carry.
func settingsGroups(s *pro.SelfServiceSettings) (pro.SelfServiceInstallSettings, pro.SelfServiceLoginSettings, pro.SelfServiceInteractionSettings) {
	var install pro.SelfServiceInstallSettings
	var login pro.SelfServiceLoginSettings
	var interaction pro.SelfServiceInteractionSettings
	if s != nil {
		if s.InstallSettings != nil {
			install = *s.InstallSettings
		}
		if s.LoginSettings != nil {
			login = *s.LoginSettings
		}
		if s.ConfigurationSettings != nil {
			interaction = *s.ConfigurationSettings
		}
	}
	return install, login, interaction
}

// intPointerValueOrNull safely unwraps a *int into a Terraform Int64. No helpers equivalent
// exists for the *int → Int64 direction; siblings inline the conversion, this package has
// two call sites so a local helper keeps them aligned.
func intPointerValueOrNull(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}
