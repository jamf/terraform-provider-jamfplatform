// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_macos_settings

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// fullResponse returns a SelfServiceSettings with distinct values per field so a swapped
// mapping is caught.
func fullResponse() *pro.SelfServiceSettings {
	return &pro.SelfServiceSettings{
		InstallSettings: &pro.SelfServiceInstallSettings{
			InstallAutomatically: new(true),
			InstallLocation:      "/Applications",
		},
		LoginSettings: &pro.SelfServiceLoginSettings{
			UserLoginLevel:  "Anonymous",
			AuthType:        "Basic",
			AllowRememberMe: new(true),
			UseFido2:        new(false),
		},
		ConfigurationSettings: &pro.SelfServiceInteractionSettings{
			NotificationsEnabled:  new(true),
			AlertUserApprovedMDM:  new(false),
			DefaultLandingPage:    new("BROWSE"),
			DefaultHomeCategoryID: new(42),
			BookmarksName:         "Bookmarks",
		},
	}
}

func TestAssignSelfServiceMacosSettingsResourceModel_AllFields(t *testing.T) {
	var state SelfServiceMacosSettingsResourceModel
	assignSelfServiceMacosSettingsResourceModel(&state, fullResponse())

	if !state.InstallAutomatically.ValueBool() {
		t.Errorf("install_automatically = %v, want true", state.InstallAutomatically)
	}
	if state.InstallLocation.ValueString() != "/Applications" {
		t.Errorf("install_location = %q, want /Applications", state.InstallLocation.ValueString())
	}
	if state.LoginMethod.ValueString() != "Anonymous" {
		t.Errorf("login_method = %q, want Anonymous", state.LoginMethod.ValueString())
	}
	if state.AuthenticationType.ValueString() != "Basic" {
		t.Errorf("authentication_type = %q, want Basic", state.AuthenticationType.ValueString())
	}
	if !state.KeychainCredentialStorageEnabled.ValueBool() {
		t.Errorf("keychain_credential_storage_enabled = %v, want true", state.KeychainCredentialStorageEnabled)
	}
	if state.Fido2Enabled.IsNull() || state.Fido2Enabled.ValueBool() {
		t.Errorf("fido2_enabled = %v, want false (non-null)", state.Fido2Enabled)
	}
	if !state.NotificationsEnabled.ValueBool() {
		t.Errorf("notifications_enabled = %v, want true", state.NotificationsEnabled)
	}
	if state.AlertUserApprovedMdm.IsNull() || state.AlertUserApprovedMdm.ValueBool() {
		t.Errorf("alert_user_approved_mdm = %v, want false (non-null)", state.AlertUserApprovedMdm)
	}
	if state.DefaultLandingPage.ValueString() != "BROWSE" {
		t.Errorf("default_landing_page = %q, want BROWSE", state.DefaultLandingPage.ValueString())
	}
	if state.DefaultHomeCategoryID.ValueInt64() != 42 {
		t.Errorf("default_home_category_id = %d, want 42", state.DefaultHomeCategoryID.ValueInt64())
	}
	if state.BookmarksDisplayName.ValueString() != "Bookmarks" {
		t.Errorf("bookmarks_display_name = %q, want Bookmarks", state.BookmarksDisplayName.ValueString())
	}
}

// TestAssign_NilGroups verifies a response missing nested groups (never observed on the
// wire) lands nulls for pointer fields rather than panicking or carrying stale state.
func TestAssign_NilGroups(t *testing.T) {
	state := SelfServiceMacosSettingsResourceModel{
		InstallAutomatically: types.BoolValue(true),
		LoginMethod:          types.StringValue("Required"),
	}
	assignSelfServiceMacosSettingsResourceModel(&state, &pro.SelfServiceSettings{})

	if !state.InstallAutomatically.IsNull() {
		t.Errorf("install_automatically = %v, want null for nil group", state.InstallAutomatically)
	}
	if state.LoginMethod.ValueString() != "" {
		t.Errorf("login_method = %q, want empty for nil group", state.LoginMethod.ValueString())
	}
	if !state.DefaultHomeCategoryID.IsNull() {
		t.Errorf("default_home_category_id = %v, want null for nil group", state.DefaultHomeCategoryID)
	}
	if !state.DefaultLandingPage.IsNull() {
		t.Errorf("default_landing_page = %v, want null for nil group", state.DefaultLandingPage)
	}
}

func TestAssignSelfServiceMacosSettingsDataSourceModel_AllFields(t *testing.T) {
	var state SelfServiceMacosSettingsDataSourceModel
	assignSelfServiceMacosSettingsDataSourceModel(&state, fullResponse())

	if !state.InstallAutomatically.ValueBool() {
		t.Errorf("install_automatically = %v, want true", state.InstallAutomatically)
	}
	if state.LoginMethod.ValueString() != "Anonymous" {
		t.Errorf("login_method = %q, want Anonymous", state.LoginMethod.ValueString())
	}
	if state.DefaultHomeCategoryID.ValueInt64() != 42 {
		t.Errorf("default_home_category_id = %d, want 42", state.DefaultHomeCategoryID.ValueInt64())
	}
	if state.BookmarksDisplayName.ValueString() != "Bookmarks" {
		t.Errorf("bookmarks_display_name = %q, want Bookmarks", state.BookmarksDisplayName.ValueString())
	}
}

// TestAssign_DoesNotClobberID verifies the assigners leave state.ID untouched.
func TestAssign_DoesNotClobberID(t *testing.T) {
	state := SelfServiceMacosSettingsResourceModel{ID: types.StringValue("singleton")}
	assignSelfServiceMacosSettingsResourceModel(&state, fullResponse())
	if state.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered: got %q, want %q", state.ID.ValueString(), "singleton")
	}

	dsState := SelfServiceMacosSettingsDataSourceModel{ID: types.StringValue("singleton")}
	assignSelfServiceMacosSettingsDataSourceModel(&dsState, fullResponse())
	if dsState.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered on data source: got %q, want %q", dsState.ID.ValueString(), "singleton")
	}
}

// TestSingletonIDConstant pins the import identifier.
func TestSingletonIDConstant(t *testing.T) {
	if helpers.SingletonID != "singleton" {
		t.Errorf("helpers.SingletonID drifted: got %q, want %q", helpers.SingletonID, "singleton")
	}
}
