// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_macos_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildSelfServiceMacosSettingsInput converts the Terraform plan model into a Self Service
// settings full-replace PUT payload.
//
// Wire-probed 2026-06-10: the PUT is full-replace (an omitted field resets to its server
// default) and ALL THREE nested setting groups are required on every write — omitting one
// returns HTTP 500. So the payload always carries all three groups with every field set.
//
// Every attribute is Optional+Computed with UseStateForUnknown. For each, a known plan value
// (declared, or USFU-carried on update) is sent; an unknown/null plan value falls back to
// the value read from the live settings (current) — adopting the existing singleton rather
// than resetting it. Both Create and Update supply a live merge base: on create every
// undeclared field is null (no prior state); on update the only unknown is the one ModifyPlan
// introduces (authentication_type on a login_method → NotRequired transition), which must
// round-trip the live server value.
func buildSelfServiceMacosSettingsInput(plan SelfServiceMacosSettingsResourceModel, current *pro.SelfServiceSettings) *pro.SelfServiceSettings {
	var curInstall pro.SelfServiceInstallSettings
	var curLogin pro.SelfServiceLoginSettings
	var curInteraction pro.SelfServiceInteractionSettings
	if current != nil {
		if current.InstallSettings != nil {
			curInstall = *current.InstallSettings
		}
		if current.LoginSettings != nil {
			curLogin = *current.LoginSettings
		}
		if current.ConfigurationSettings != nil {
			curInteraction = *current.ConfigurationSettings
		}
	}

	return &pro.SelfServiceSettings{
		InstallSettings: &pro.SelfServiceInstallSettings{
			InstallAutomatically: boolOrCurrent(plan.InstallAutomatically, curInstall.InstallAutomatically),
			InstallLocation:      stringOrCurrent(plan.InstallLocation, curInstall.InstallLocation),
		},
		LoginSettings: &pro.SelfServiceLoginSettings{
			UserLoginLevel:  stringOrCurrent(plan.LoginMethod, curLogin.UserLoginLevel),
			AuthType:        stringOrCurrent(plan.AuthenticationType, curLogin.AuthType),
			AllowRememberMe: boolOrCurrent(plan.KeychainCredentialStorageEnabled, curLogin.AllowRememberMe),
			UseFido2:        boolOrCurrent(plan.Fido2Enabled, curLogin.UseFido2),
		},
		ConfigurationSettings: &pro.SelfServiceInteractionSettings{
			NotificationsEnabled:  boolOrCurrent(plan.NotificationsEnabled, curInteraction.NotificationsEnabled),
			AlertUserApprovedMDM:  boolOrCurrent(plan.AlertUserApprovedMdm, curInteraction.AlertUserApprovedMDM),
			DefaultLandingPage:    stringPointerOrCurrent(plan.DefaultLandingPage, curInteraction.DefaultLandingPage),
			DefaultHomeCategoryID: intOrCurrent(plan.DefaultHomeCategoryID, curInteraction.DefaultHomeCategoryID),
			BookmarksName:         stringOrCurrent(plan.BookmarksDisplayName, curInteraction.BookmarksName),
		},
	}
}

// boolOrCurrent emits the plan value when known (declared, or USFU-carried on update), else
// falls back to the live value read from the server (adopt undeclared toggle on create).
// A nil fallback (no merge base and no plan value) stays nil so omitempty drops the field
// and the server default applies — unreachable in practice: Create always supplies a live
// merge base and on update every plan value is known via UseStateForUnknown.
func boolOrCurrent(v types.Bool, current *bool) *bool {
	if !v.IsNull() && !v.IsUnknown() {
		b := v.ValueBool()
		return &b
	}
	return current
}

// intOrCurrent mirrors boolOrCurrent for the *int wire fields, narrowing Terraform's Int64
// at the boundary.
func intOrCurrent(v types.Int64, current *int) *int {
	if !v.IsNull() && !v.IsUnknown() {
		i := int(v.ValueInt64())
		return &i
	}
	return current
}

// stringPointerOrCurrent mirrors boolOrCurrent for the *string wire fields.
func stringPointerOrCurrent(v types.String, current *string) *string {
	if !v.IsNull() && !v.IsUnknown() {
		s := v.ValueString()
		return &s
	}
	return current
}

// stringOrCurrent emits the plan value when known, else falls back to the live value read
// from the server. The wire field is a non-pointer string (always present on the wire), so
// the return is a plain string.
func stringOrCurrent(v types.String, current string) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	return current
}
