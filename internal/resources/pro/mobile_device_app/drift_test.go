// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// TestFlattenMobileApp_ReportsDrift pins the wire-authoritative read: an echoed
// value that differs from state must land in state so `terraform plan` reports
// the change. Every field asserted here round-trips through the classic
// /mobiledeviceapplications GET on both the POST and the PUT path (Jamf Pro
// 11.31.1, wire-probed 2026-09-06). See issue #387.
func TestFlattenMobileApp_ReportsDrift(t *testing.T) {
	t.Parallel()
	general := &MobileAppGeneralModel{
		DeploymentType:                   types.StringValue("Make Available in Self Service"),
		ExternalURL:                      types.StringValue("https://state.invalid/"),
		ItunesStoreURL:                   types.StringValue("https://apps.apple.com/app/id1"),
		ItunesCountryRegion:              types.StringValue("GB"),
		ItunesSyncTime:                   types.Int64Value(9),
		IsFree:                           types.BoolValue(false),
		KeepAppUpdatedOnDevices:          types.BoolValue(false),
		RemoveAppWhenMDMProfileIsRemoved: types.BoolValue(false),
		CategoryID:                       types.StringValue("11"),
		SiteID:                           types.StringValue("-1"),
	}
	flattenMobileAppGeneral(&proclassic.MobileDeviceApplicationGeneral{
		Name:                             new("app"),
		DeploymentType:                   new("Install Automatically/Prompt Users to Install"),
		ExternalURL:                      new("https://wire.invalid/"),
		ItunesStoreURL:                   new("https://apps.apple.com/app/id2"),
		ItunesCountryRegion:              new("US"),
		ItunesSyncTime:                   new(5),
		Free:                             new(true),
		KeepAppUpdatedOnDevices:          new(true),
		RemoveAppWhenMDMProfileIsRemoved: new(true),
		Category:                         &proclassic.CategoryObject{ID: new(653), Name: new("Operations")},
		Site:                             &proclassic.SiteObject{ID: new(1), Name: new("AGATA")},
	}, general)

	for _, tc := range []struct{ name, want, got string }{
		{"deployment_type", "Install Automatically/Prompt Users to Install", general.DeploymentType.ValueString()},
		{"external_url", "https://wire.invalid/", general.ExternalURL.ValueString()},
		{"itunes_store_url", "https://apps.apple.com/app/id2", general.ItunesStoreURL.ValueString()},
		{"itunes_country_region", "US", general.ItunesCountryRegion.ValueString()},
		{"category_id", "653", general.CategoryID.ValueString()},
		{"site_id", "1", general.SiteID.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: wire value must win, want %q got %q", tc.name, tc.want, tc.got)
		}
	}
	if general.ItunesSyncTime.ValueInt64() != 5 {
		t.Errorf("itunes_sync_time: wire value must win, got %d", general.ItunesSyncTime.ValueInt64())
	}
	for _, tc := range []struct {
		name string
		got  types.Bool
	}{
		{"is_free", general.IsFree},
		{"keep_app_updated_on_devices", general.KeepAppUpdatedOnDevices},
		{"remove_app_when_mdm_profile_is_removed", general.RemoveAppWhenMDMProfileIsRemoved},
	} {
		if !tc.got.ValueBool() {
			t.Errorf("%s: wire value must win, got false", tc.name)
		}
	}

	vpp := &MobileAppVppModel{
		AssignVppDeviceBasedLicenses: types.BoolValue(false),
		VppAdminAccountID:            types.StringValue("-1"),
	}
	flattenMobileAppVpp(&proclassic.MobileDeviceApplicationVpp{
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

// TestFlattenMobileApp_StickyFieldsIgnoreDrift pins the other half of the #387
// split: host_externally keeps the value already in state, because the write
// does not persist while external_url is set.
//
// after_install_button_text and the three self_service notification_* fields
// are asserted here too, but for a different rule: each is echoed only while
// its gate is on — general.make_available_after_install for the first, the
// tenant-level Self Service notifications toggle for the rest. With the gates
// off, which is what the empty MobileDeviceApplicationSelfService stands for,
// state must be kept rather than nulled.
func TestFlattenMobileApp_StickyFieldsIgnoreDrift(t *testing.T) {
	t.Parallel()
	general := &MobileAppGeneralModel{HostExternally: types.BoolValue(false)}
	flattenMobileAppGeneral(&proclassic.MobileDeviceApplicationGeneral{
		Name:           new("app"),
		HostExternally: new(true),
	}, general)
	if general.HostExternally.ValueBool() {
		t.Error("host_externally: sticky read must keep false")
	}

	ss := &MobileAppSelfServiceModel{
		AfterInstallButtonText: types.StringValue("state after"),
		NotificationEnabled:    types.BoolValue(true),
		NotificationSubject:    types.StringValue("state subject"),
		NotificationMessage:    types.StringValue("state message"),
	}
	flattenMobileAppSelfService(&proclassic.MobileDeviceApplicationSelfService{}, ss, false)
	for _, tc := range []struct{ name, want, got string }{
		{"after_install_button_text", "state after", ss.AfterInstallButtonText.ValueString()},
		{"notification_subject", "state subject", ss.NotificationSubject.ValueString()},
		{"notification_message", "state message", ss.NotificationMessage.ValueString()},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: gate off, so state must be kept as %q, got %q", tc.name, tc.want, tc.got)
		}
	}
	if !ss.NotificationEnabled.ValueBool() {
		t.Error("notification_enabled: gate off, so state must be kept as true")
	}
}

// TestFlattenMobileAppSelfService_GatedFieldsDriftWhenEchoed pins the other
// side of both conditional echoes: with the gates on the classic GET does
// return these fields, and a value that differs from state must then win so
// drift is reported.
func TestFlattenMobileAppSelfService_GatedFieldsDriftWhenEchoed(t *testing.T) {
	t.Parallel()
	ss := &MobileAppSelfServiceModel{
		AfterInstallButtonText: types.StringValue("state after"),
		NotificationEnabled:    types.BoolValue(true),
		NotificationSubject:    types.StringValue("state subject"),
		NotificationMessage:    types.StringValue("state message"),
	}
	flattenMobileAppSelfService(&proclassic.MobileDeviceApplicationSelfService{
		SelfServiceAfterInstallButtonText: new("wire after"),
		Notification:                      &proclassic.NotificationValue{Enabled: new(false)},
		NotificationSubject:               new("wire subject"),
		NotificationMessage:               new("wire message"),
	}, ss, false)
	for _, tc := range []struct{ name, want, got string }{
		{"after_install_button_text", "wire after", ss.AfterInstallButtonText.ValueString()},
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
