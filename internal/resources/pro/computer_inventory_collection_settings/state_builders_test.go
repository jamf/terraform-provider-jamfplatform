// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_inventory_collection_settings

import (
	"context"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

func sampleResponse() *pro.ComputerInventoryCollectionSettingsV2 {
	return &pro.ComputerInventoryCollectionSettingsV2{
		ComputerInventoryCollectionPreferences: &pro.ComputerInventoryCollectionPreferencesV2{
			IncludeAccounts:               new(true),
			CalculateSizes:                new(false),
			IncludeHiddenAccounts:         new(true),
			IncludePrinters:               new(false),
			IncludeServices:               new(true),
			CollectSyncedMobileDeviceInfo: new(false),
			UpdateLdapInfoOnComputerInventorySubmissions: new(true),
			IncludePackages:              new(false),
			IncludeSoftwareUpdates:       new(true),
			CollectUnmanagedCertificates: new(false),
			MonitorBeacons:               new(true),
			AllowChangingUserAndLocation: new(false),
			MonitorApplicationUsage:      new(true),
			UseUnixUserPaths:             new(false),
			IncludeSoftwareID:            new(true),
		},
		ApplicationPaths: &[]pro.AppPath{
			{ID: "-1", Path: "/Applications/"},
			{ID: "-1", Path: "/System/Applications/"},
			{ID: "197", Path: "/Custom/App/"},
			{ID: "198", Path: "/Another/"},
		},
	}
}

func TestAssignResourceModel_PreferencesAndPaths(t *testing.T) {
	var state ComputerInventoryCollectionSettingsResourceModel
	if diags := assignComputerInventoryCollectionSettingsResourceModel(context.Background(), &state, sampleResponse()); diags.HasError() {
		t.Fatalf("assign diagnostics: %v", diags)
	}

	checks := map[string]struct {
		got  types.Bool
		want bool
	}{
		"collect_local_user_accounts":                      {state.CollectLocalUserAccounts, true},
		"include_home_directory_sizes":                     {state.IncludeHomeDirectorySizes, false},
		"include_hidden_accounts":                          {state.IncludeHiddenAccounts, true},
		"collect_printers":                                 {state.CollectPrinters, false},
		"collect_active_services":                          {state.CollectActiveServices, true},
		"collect_synced_mobile_device_backup_dates":        {state.CollectSyncedMobileDeviceBackupDates, false},
		"collect_user_and_location_from_directory_service": {state.CollectUserAndLocationFromDirectoryService, true},
		"collect_package_receipts":                         {state.CollectPackageReceipts, false},
		"collect_available_software_updates":               {state.CollectAvailableSoftwareUpdates, true},
		"collect_unmanaged_certificates":                   {state.CollectUnmanagedCertificates, false},
		"monitor_beacon_regions":                           {state.MonitorBeaconRegions, true},
		"allow_jamf_binary_user_and_location_changes":      {state.AllowJamfBinaryUserAndLocationChanges, false},
		"collect_application_usage_information":            {state.CollectApplicationUsageInformation, true},
		"use_unix_user_paths":                              {state.UseUnixUserPaths, false},
		"include_software_id":                              {state.IncludeSoftwareID, true},
	}
	for name, c := range checks {
		if c.got.IsNull() || c.got.IsUnknown() {
			t.Errorf("%s: expected concrete bool, got null/unknown", name)
			continue
		}
		if c.got.ValueBool() != c.want {
			t.Errorf("%s = %v, want %v", name, c.got.ValueBool(), c.want)
		}
	}

	// Built-in (id == "-1") paths must be excluded; only user paths survive.
	got, diags := helpers.SetToStringSlice(context.Background(), state.ApplicationSearchPaths)
	if diags.HasError() {
		t.Fatalf("set conversion diagnostics: %v", diags)
	}
	sort.Strings(got)
	want := []string{"/Another/", "/Custom/App/"}
	if len(got) != len(want) {
		t.Fatalf("application_search_paths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("application_search_paths[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAssignResourceModel_NilPaths(t *testing.T) {
	resp := &pro.ComputerInventoryCollectionSettingsV2{
		ComputerInventoryCollectionPreferences: &pro.ComputerInventoryCollectionPreferencesV2{IncludeAccounts: new(true)},
	}
	var state ComputerInventoryCollectionSettingsResourceModel
	if diags := assignComputerInventoryCollectionSettingsResourceModel(context.Background(), &state, resp); diags.HasError() {
		t.Fatalf("assign diagnostics: %v", diags)
	}
	if state.ApplicationSearchPaths.IsNull() || state.ApplicationSearchPaths.IsUnknown() {
		t.Errorf("nil ApplicationPaths must yield an empty (non-null) set, got %v", state.ApplicationSearchPaths)
	}
	got, _ := helpers.SetToStringSlice(context.Background(), state.ApplicationSearchPaths)
	if len(got) != 0 {
		t.Errorf("expected empty path set, got %v", got)
	}
}

func TestAssignResourceModel_NilPointerDefaultsFalse(t *testing.T) {
	resp := &pro.ComputerInventoryCollectionSettingsV2{
		ComputerInventoryCollectionPreferences: &pro.ComputerInventoryCollectionPreferencesV2{}, // all nil
	}
	var state ComputerInventoryCollectionSettingsResourceModel
	if diags := assignComputerInventoryCollectionSettingsResourceModel(context.Background(), &state, resp); diags.HasError() {
		t.Fatalf("assign diagnostics: %v", diags)
	}
	if state.CollectLocalUserAccounts.IsNull() || state.CollectLocalUserAccounts.ValueBool() != false {
		t.Errorf("nil preference pointer must default to concrete false, got %v", state.CollectLocalUserAccounts)
	}
}

func TestAssignDataSourceModel_Paths(t *testing.T) {
	var state ComputerInventoryCollectionSettingsDataSourceModel
	if diags := assignComputerInventoryCollectionSettingsDataSourceModel(context.Background(), &state, sampleResponse()); diags.HasError() {
		t.Fatalf("assign diagnostics: %v", diags)
	}
	got, _ := helpers.SetToStringSlice(context.Background(), state.ApplicationSearchPaths)
	if len(got) != 2 {
		t.Errorf("data source path set = %v, want 2 user paths", got)
	}
}

func TestAssign_DoesNotClobberID(t *testing.T) {
	state := ComputerInventoryCollectionSettingsResourceModel{ID: types.StringValue("singleton")}
	_ = assignComputerInventoryCollectionSettingsResourceModel(context.Background(), &state, sampleResponse())
	if state.ID.ValueString() != "singleton" {
		t.Errorf("ID clobbered: got %q, want %q", state.ID.ValueString(), "singleton")
	}
}

func TestSingletonIDConstant(t *testing.T) {
	if helpers.SingletonID != "singleton" {
		t.Errorf("helpers.SingletonID drifted: got %q, want %q", helpers.SingletonID, "singleton")
	}
}

func TestBuiltInPathIDConstant(t *testing.T) {
	if builtInPathID != "-1" {
		t.Errorf("builtInPathID drifted: got %q, want %q", builtInPathID, "-1")
	}
}
