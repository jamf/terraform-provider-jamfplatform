// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func sampleDeployment() *pro.AppTitleDeploymentRead {
	return &pro.AppTitleDeploymentRead{
		ID:                              "177",
		Name:                            "tf-acc-app-installer",
		Enabled:                         true,
		AppTitleID:                      "027",
		DeploymentType:                  "SELF_SERVICE",
		UpdateBehavior:                  "AUTOMATIC",
		SelectedVersion:                 "",
		LatestAvailableVersion:          "14.2",
		TitleAvailableInAis:             true,
		VersionRemoved:                  false,
		CategoryID:                      "-1",
		SiteID:                          "-1",
		SmartGroupID:                    "-1",
		InstallPredefinedConfigProfiles: false,
		TriggerAdminNotifications:       true,
	}
}

func TestAssignAppInstallerResourceModel_FlatScalars(t *testing.T) {
	state := AppInstallerResourceModel{}
	assignAppInstallerResourceModel(&state, sampleDeployment(), false)

	if state.ID.ValueString() != "177" {
		t.Errorf("id: got %q", state.ID.ValueString())
	}
	if state.LatestAvailableVersion.ValueString() != "14.2" {
		t.Errorf("latest_available_version: got %q", state.LatestAvailableVersion.ValueString())
	}
	if !state.TitleAvailableInAis.ValueBool() {
		t.Errorf("title_available_in_ais should be true")
	}
	if state.CategoryID.ValueString() != "-1" {
		t.Errorf("category_id should be -1 verbatim, got %q", state.CategoryID.ValueString())
	}
	if !state.TriggerAdminNotifications.ValueBool() {
		t.Errorf("trigger_admin_notifications should be true")
	}
}

func TestAssignAppInstallerResourceModel_NestedBlocksGated(t *testing.T) {
	// The server echoes both blocks populated; an unmanaged (nil) block in state
	// must stay nil so the framework consistency check passes.
	d := sampleDeployment()
	d.NotificationSettings = &pro.AppTitleDeploymentNotificationSettings{NotificationMessage: new("hi")}
	d.SelfServiceSettings = &pro.AppTitleDeploymentSelfServiceSettings{Description: new("desc")}

	state := AppInstallerResourceModel{} // both blocks nil (unmanaged)
	assignAppInstallerResourceModel(&state, d, false)
	if state.NotificationSettings != nil {
		t.Errorf("unmanaged notification_settings must stay nil")
	}
	if state.SelfServiceSettings != nil {
		t.Errorf("unmanaged self_service_settings must stay nil")
	}
}

func TestAssignAppInstallerResourceModel_NestedBlocksManaged(t *testing.T) {
	d := sampleDeployment()
	d.NotificationSettings = &pro.AppTitleDeploymentNotificationSettings{
		NotificationMessage: new("hi"),
		Deadline:            new(int64(48)),
		Suppress:            new(true),
	}
	d.SelfServiceSettings = &pro.AppTitleDeploymentSelfServiceSettings{
		Description:               new("desc"),
		IncludeInFeaturedCategory: new(true),
		Categories: &[]pro.AppTitleDeploymentSelfServiceSettingsCategoriesItem{
			{ID: "58", Featured: new(true)},
		},
	}

	state := AppInstallerResourceModel{
		NotificationSettings: &NotificationSettingsModel{}, // managed
		SelfServiceSettings: &SelfServiceSettingsModel{ // managed, categories supplied
			// featured Unknown mirrors the create-time plan (Optional+Computed);
			// the flatten resolves it from the server echo.
			Categories: []SelfServiceCategoryModel{
				{CategoryID: types.StringValue("58"), Featured: types.BoolUnknown()},
			},
		},
	}
	assignAppInstallerResourceModel(&state, d, false)

	if state.NotificationSettings == nil {
		t.Fatalf("managed notification_settings must be refreshed")
	}
	if state.NotificationSettings.NotificationMessage.ValueString() != "hi" {
		t.Errorf("notification_message: got %q", state.NotificationSettings.NotificationMessage.ValueString())
	}
	if state.NotificationSettings.Deadline.ValueInt64() != 48 {
		t.Errorf("deadline: got %d", state.NotificationSettings.Deadline.ValueInt64())
	}
	if !state.NotificationSettings.Suppress.ValueBool() {
		t.Errorf("suppress should be true")
	}

	if state.SelfServiceSettings == nil {
		t.Fatalf("managed self_service_settings must be refreshed")
	}
	if len(state.SelfServiceSettings.Categories) != 1 {
		t.Fatalf("expected 1 category, got %d", len(state.SelfServiceSettings.Categories))
	}
	c := state.SelfServiceSettings.Categories[0]
	if c.CategoryID.ValueString() != "58" || !c.Featured.ValueBool() {
		t.Errorf("category mismatch: %+v", c)
	}
}

func TestAssignAppInstallerResourceModel_CategoriesOmittedStaysNil(t *testing.T) {
	// categories is Optional-only: when the user omitted it (prior nil), the
	// post-apply value must stay nil/null even though the server echoes no
	// categories. Synthesising an empty slice would trip the framework's
	// "produced inconsistent result after apply" (plan null → apply empty set).
	d := sampleDeployment()
	d.SelfServiceSettings = &pro.AppTitleDeploymentSelfServiceSettings{Categories: nil}
	state := AppInstallerResourceModel{SelfServiceSettings: &SelfServiceSettingsModel{}} // Categories nil
	assignAppInstallerResourceModel(&state, d, false)
	if state.SelfServiceSettings.Categories != nil {
		t.Errorf("omitted categories must stay nil, got %v", state.SelfServiceSettings.Categories)
	}
}

func TestAssignAppInstallerResourceModel_CategoriesEmptyConfigStaysEmpty(t *testing.T) {
	// When the user supplied categories = [] (non-nil empty prior), the value
	// round-trips as an empty (non-nil) slice — clearing is honoured.
	d := sampleDeployment()
	d.SelfServiceSettings = &pro.AppTitleDeploymentSelfServiceSettings{Categories: nil}
	state := AppInstallerResourceModel{SelfServiceSettings: &SelfServiceSettingsModel{Categories: []SelfServiceCategoryModel{}}}
	assignAppInstallerResourceModel(&state, d, false)
	if state.SelfServiceSettings.Categories == nil {
		t.Errorf("config-supplied empty categories must stay non-nil empty")
	}
	if len(state.SelfServiceSettings.Categories) != 0 {
		t.Errorf("expected 0 categories, got %d", len(state.SelfServiceSettings.Categories))
	}
}

// TestAssignAppInstallerResourceModel_IncludeUnmanagedHydratesFromScratch pins
// the config-generation contract: with includeUnmanaged set and an empty
// starting model, every wire-present block is allocated and hydrated from the
// server even though no prior state manages them. (categories stays nil because
// a nil prior cannot disambiguate omitted-vs-empty membership; the block's
// scalar fields hydrate regardless.)
func TestAssignAppInstallerResourceModel_IncludeUnmanagedHydratesFromScratch(t *testing.T) {
	d := sampleDeployment()
	d.NotificationSettings = &pro.AppTitleDeploymentNotificationSettings{
		NotificationMessage: new("hi"),
		Deadline:            new(int64(48)),
	}
	d.SelfServiceSettings = &pro.AppTitleDeploymentSelfServiceSettings{
		Description:               new("desc"),
		IncludeInFeaturedCategory: new(true),
	}

	state := AppInstallerResourceModel{} // both blocks nil (unmanaged)
	assignAppInstallerResourceModel(&state, d, true)

	if state.NotificationSettings == nil {
		t.Fatal("expected notification_settings allocated when wire-present")
	}
	if state.NotificationSettings.NotificationMessage.ValueString() != "hi" || state.NotificationSettings.Deadline.ValueInt64() != 48 {
		t.Errorf("notification_settings not hydrated: %+v", state.NotificationSettings)
	}
	if state.SelfServiceSettings == nil {
		t.Fatal("expected self_service_settings allocated when wire-present")
	}
	if state.SelfServiceSettings.Description.ValueString() != "desc" || !state.SelfServiceSettings.IncludeInFeaturedCategory.ValueBool() {
		t.Errorf("self_service_settings not hydrated: %+v", state.SelfServiceSettings)
	}
}

func TestAssignAppInstallerResourceModel_NilSafe(t *testing.T) {
	state := AppInstallerResourceModel{ID: types.StringValue("keep")}
	assignAppInstallerResourceModel(&state, nil, false)
	if state.ID.ValueString() != "keep" {
		t.Errorf("nil response must not clobber state")
	}
}

func TestAssignAppInstallerDataSourceModel(t *testing.T) {
	data := AppInstallerDataSourceModel{}
	assignAppInstallerDataSourceModel(&data, sampleDeployment())
	if data.ID.ValueString() != "177" || data.Name.ValueString() != "tf-acc-app-installer" {
		t.Errorf("data source flat mapping mismatch: %+v", data)
	}
	if data.DeploymentType.ValueString() != "SELF_SERVICE" {
		t.Errorf("deployment_type: got %q", data.DeploymentType.ValueString())
	}
}

// TestFlattenSelfServiceSettings_HydratesCategoriesOnImport covers first-time
// import, where prior is nil because the parent block had not been written yet:
// categories must come from the wire rather than staying null under a block
// that did hydrate.
func TestFlattenSelfServiceSettings_HydratesCategoriesOnImport(t *testing.T) {
	cats := []pro.AppTitleDeploymentSelfServiceSettingsCategoriesItem{{ID: "7", Featured: new(true)}}
	out := flattenSelfServiceSettings(nil, &pro.AppTitleDeploymentSelfServiceSettings{Categories: &cats}, true)
	if out == nil {
		t.Fatal("block must be built")
	}
	if len(out.Categories) != 1 {
		t.Fatalf("categories expected 1 entry, got %d", len(out.Categories))
	}
	if out.Categories[0].CategoryID.ValueString() != "7" {
		t.Errorf("categories[0].category_id = %q, want 7", out.Categories[0].CategoryID.ValueString())
	}
}

// TestFlattenSelfServiceSettings_ImportWithNoCategoriesStaysNil asserts the
// non-empty rule: storing an empty set where a create that never declared the
// attribute stores null would break ImportStateVerify.
func TestFlattenSelfServiceSettings_ImportWithNoCategoriesStaysNil(t *testing.T) {
	empty := []pro.AppTitleDeploymentSelfServiceSettingsCategoriesItem{}
	out := flattenSelfServiceSettings(nil, &pro.AppTitleDeploymentSelfServiceSettings{Categories: &empty}, true)
	if out.Categories != nil {
		t.Errorf("categories must stay nil when the server carries none, got %+v", out.Categories)
	}
	out = flattenSelfServiceSettings(nil, &pro.AppTitleDeploymentSelfServiceSettings{}, true)
	if out.Categories != nil {
		t.Errorf("categories must stay nil when the server omits the list, got %+v", out.Categories)
	}
}
