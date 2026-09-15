// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestFlattenMacApp_ReportsDrift pins the wire-authoritative read: an echoed
// value that differs from state must land in state so `terraform plan` reports
// the change. Every field asserted here round-trips through the classic
// /macapplications GET (Jamf Pro 11.31.1, wire-probed 2026-09-06). See issue
// #387.
func TestFlattenMacApp_ReportsDrift(t *testing.T) {
	t.Parallel()
	general := &MacAppGeneralModel{
		DeploymentType: types.StringValue("Make Available in Self Service"),
		CategoryID:     types.StringValue("11"),
		SiteID:         types.StringValue("-1"),
	}
	flattenMacAppGeneral(&proclassic.MacApplicationGeneral{
		Name:           new("app"),
		DeploymentType: new("Install Automatically/Prompt Users to Install"),
		Category:       &proclassic.CategoryObject{ID: new(653), Name: new("Operations")},
		Site:           &proclassic.SiteObject{ID: new(1), Name: new("AGATA")},
	}, general)
	for _, tc := range []struct{ name, want, got string }{
		{"deployment_type", "Install Automatically/Prompt Users to Install", general.DeploymentType.ValueString()},
		{"category_id", "653", general.CategoryID.ValueString()},
		{"site_id", "1", general.SiteID.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: wire value must win, want %q got %q", tc.name, tc.want, tc.got)
		}
	}

	ss := &MacAppSelfServiceModel{
		InstallButtonText: types.StringValue("state install"),
		FeatureOnMainPage: types.BoolValue(false),
	}
	flattenMacAppSelfService(&proclassic.MacApplicationSelfService{
		InstallButtonText: new("wire install"),
		FeatureOnMainPage: new(true),
	}, ss)
	if got := ss.InstallButtonText.ValueString(); got != "wire install" {
		t.Errorf("self_service.install_button_text: wire value must win, got %q", got)
	}
	if !ss.FeatureOnMainPage.ValueBool() {
		t.Error("self_service.feature_on_main_page: wire value must win, got false")
	}

	vpp := &MacAppVppModel{
		AssignVppDeviceBasedLicenses: types.BoolValue(false),
		VppAdminAccountID:            types.StringValue("-1"),
	}
	flattenMacAppVpp(&proclassic.MacApplicationVpp{
		AssignVppDeviceBasedLicenses: new(true),
		VppAdminAccountID:            new(4),
	}, vpp)
	if !vpp.AssignVppDeviceBasedLicenses.ValueBool() {
		t.Error("vpp.assign_vpp_device_based_licenses: wire value must win, got false")
	}
	if got := vpp.VppAdminAccountID.ValueString(); got != "4" {
		t.Errorf("vpp.vpp_admin_account_id: wire value must win, got %q", got)
	}
}

// TestFlattenMacApp_StickyFieldsIgnoreDrift pins the other half of the #387
// split: is_free keeps the value already in state, because Jamf Pro resolves it
// from the App Store listing and the write does not persist.
//
// The four self_service notification_* fields are asserted here too, but for a
// different rule: they are echoed only while the tenant-level Self Service
// notifications toggle is on, so with the toggle off — the empty
// MacApplicationSelfService passed in — state must be kept rather than nulled.
func TestFlattenMacApp_StickyFieldsIgnoreDrift(t *testing.T) {
	t.Parallel()
	general := &MacAppGeneralModel{IsFree: types.BoolValue(false)}
	flattenMacAppGeneral(&proclassic.MacApplicationGeneral{
		Name:   new("app"),
		IsFree: new(true),
	}, general)
	if general.IsFree.ValueBool() {
		t.Error("is_free: sticky read must keep false")
	}

	ss := &MacAppSelfServiceModel{
		NotificationEnabled: types.BoolValue(true),
		NotificationMethod:  types.StringValue("Self Service"),
		NotificationSubject: types.StringValue("state subject"),
		NotificationMessage: types.StringValue("state message"),
	}
	flattenMacAppSelfService(&proclassic.MacApplicationSelfService{}, ss)
	for _, tc := range []struct{ name, want, got string }{
		{"notification_method", "Self Service", ss.NotificationMethod.ValueString()},
		{"notification_subject", "state subject", ss.NotificationSubject.ValueString()},
		{"notification_message", "state message", ss.NotificationMessage.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: toggle off, so state must be kept as %q, got %q", tc.name, tc.want, tc.got)
		}
	}
	if !ss.NotificationEnabled.ValueBool() {
		t.Error("notification_enabled: toggle off, so state must be kept as true")
	}
}

// TestFlattenMacAppSelfService_NotificationDriftWhenEchoed pins the other side
// of the conditional echo: while the tenant-level Self Service notifications
// toggle is on the classic GET does return the <notification> family, and a
// value that differs from state must then win so drift is reported.
func TestFlattenMacAppSelfService_NotificationDriftWhenEchoed(t *testing.T) {
	t.Parallel()
	ss := &MacAppSelfServiceModel{
		NotificationEnabled: types.BoolValue(true),
		NotificationMethod:  types.StringValue("Self Service"),
		NotificationSubject: types.StringValue("state subject"),
		NotificationMessage: types.StringValue("state message"),
	}
	flattenMacAppSelfService(&proclassic.MacApplicationSelfService{
		Notification:        &proclassic.NotificationValue{Enabled: new(false), Method: new("Self Service and Notification Center")},
		NotificationSubject: new("wire subject"),
		NotificationMessage: new("wire message"),
	}, ss)
	for _, tc := range []struct{ name, want, got string }{
		{"notification_method", "Self Service and Notification Center", ss.NotificationMethod.ValueString()},
		{"notification_subject", "wire subject", ss.NotificationSubject.ValueString()},
		{"notification_message", "wire message", ss.NotificationMessage.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: wire value must win, want %q got %q", tc.name, tc.want, tc.got)
		}
	}
	if ss.NotificationEnabled.ValueBool() {
		t.Error("notification_enabled: wire false must win over state true")
	}
}
