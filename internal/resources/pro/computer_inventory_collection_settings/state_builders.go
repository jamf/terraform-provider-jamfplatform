// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_inventory_collection_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// builtInPathID is the id Jamf Pro assigns to its server-managed built-in application
// search paths (e.g. /Applications/, /System/Applications/). These are never
// user-created, cannot be deleted, and must be filtered out of Terraform state so they
// do not appear as drift against a user's declared custom paths.
const builtInPathID = "-1"

// boolOr returns the dereferenced bool, or false when the pointer is nil. The V2 GET
// echoes every preference on a healthy tenant; the nil branch is a defensive fallback
// (a missing feature-toggle is treated as "off"). Every preference attribute is
// Optional+Computed or Computed, so committed state must always hold a concrete bool.
func boolOr(p *bool) types.Bool {
	if p == nil {
		return types.BoolValue(false)
	}
	return types.BoolValue(*p)
}

// userApplicationPaths returns the path strings of user-created custom application search
// paths from a V2 settings response, excluding Jamf Pro's built-in (id == "-1") entries.
func userApplicationPaths(s *pro.ComputerInventoryCollectionSettingsV2) []string {
	if s == nil || s.ApplicationPaths == nil {
		return []string{}
	}
	paths := make([]string, 0, len(*s.ApplicationPaths))
	for _, p := range *s.ApplicationPaths {
		if p.ID == builtInPathID {
			continue
		}
		paths = append(paths, p.Path)
	}
	return paths
}

// assignComputerInventoryCollectionSettingsResourceModel populates a resource model from a
// V2 settings response.
func assignComputerInventoryCollectionSettingsResourceModel(ctx context.Context, state *ComputerInventoryCollectionSettingsResourceModel, s *pro.ComputerInventoryCollectionSettingsV2) diag.Diagnostics {
	var diags diag.Diagnostics

	if p := s.ComputerInventoryCollectionPreferences; p != nil {
		state.CollectLocalUserAccounts = boolOr(p.IncludeAccounts)
		state.IncludeHomeDirectorySizes = boolOr(p.CalculateSizes)
		state.IncludeHiddenAccounts = boolOr(p.IncludeHiddenAccounts)
		state.CollectPrinters = boolOr(p.IncludePrinters)
		state.CollectActiveServices = boolOr(p.IncludeServices)
		state.CollectSyncedMobileDeviceBackupDates = boolOr(p.CollectSyncedMobileDeviceInfo)
		state.CollectUserAndLocationFromDirectoryService = boolOr(p.UpdateLdapInfoOnComputerInventorySubmissions)
		state.CollectPackageReceipts = boolOr(p.IncludePackages)
		state.CollectAvailableSoftwareUpdates = boolOr(p.IncludeSoftwareUpdates)
		state.CollectUnmanagedCertificates = boolOr(p.CollectUnmanagedCertificates)
		state.MonitorBeaconRegions = boolOr(p.MonitorBeacons)
		state.AllowJamfBinaryUserAndLocationChanges = boolOr(p.AllowChangingUserAndLocation)
		state.CollectApplicationUsageInformation = boolOr(p.MonitorApplicationUsage)
		state.UseUnixUserPaths = boolOr(p.UseUnixUserPaths)
		state.IncludeSoftwareID = boolOr(p.IncludeSoftwareID)
	}

	set, setDiags := types.SetValueFrom(ctx, types.StringType, userApplicationPaths(s))
	diags.Append(setDiags...)
	state.ApplicationSearchPaths = set

	return diags
}

// assignComputerInventoryCollectionSettingsDataSourceModel populates a data source model
// from a V2 settings response. Same semantics as the resource assigner.
func assignComputerInventoryCollectionSettingsDataSourceModel(ctx context.Context, state *ComputerInventoryCollectionSettingsDataSourceModel, s *pro.ComputerInventoryCollectionSettingsV2) diag.Diagnostics {
	var diags diag.Diagnostics

	if p := s.ComputerInventoryCollectionPreferences; p != nil {
		state.CollectLocalUserAccounts = boolOr(p.IncludeAccounts)
		state.IncludeHomeDirectorySizes = boolOr(p.CalculateSizes)
		state.IncludeHiddenAccounts = boolOr(p.IncludeHiddenAccounts)
		state.CollectPrinters = boolOr(p.IncludePrinters)
		state.CollectActiveServices = boolOr(p.IncludeServices)
		state.CollectSyncedMobileDeviceBackupDates = boolOr(p.CollectSyncedMobileDeviceInfo)
		state.CollectUserAndLocationFromDirectoryService = boolOr(p.UpdateLdapInfoOnComputerInventorySubmissions)
		state.CollectPackageReceipts = boolOr(p.IncludePackages)
		state.CollectAvailableSoftwareUpdates = boolOr(p.IncludeSoftwareUpdates)
		state.CollectUnmanagedCertificates = boolOr(p.CollectUnmanagedCertificates)
		state.MonitorBeaconRegions = boolOr(p.MonitorBeacons)
		state.AllowJamfBinaryUserAndLocationChanges = boolOr(p.AllowChangingUserAndLocation)
		state.CollectApplicationUsageInformation = boolOr(p.MonitorApplicationUsage)
		state.UseUnixUserPaths = boolOr(p.UseUnixUserPaths)
		state.IncludeSoftwareID = boolOr(p.IncludeSoftwareID)
	}

	set, setDiags := types.SetValueFrom(ctx, types.StringType, userApplicationPaths(s))
	diags.Append(setDiags...)
	state.ApplicationSearchPaths = set

	return diags
}
