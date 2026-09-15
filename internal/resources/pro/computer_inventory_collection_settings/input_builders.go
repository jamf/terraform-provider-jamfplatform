// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_inventory_collection_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// optBool returns a *bool only when the planned value is known and non-null. The V2
// Update is a JSON merge-patch (Content-Type application/merge-patch+json): omitting a
// field leaves the server's current value untouched. Because every preference toggle
// is Optional+Computed, an attribute the user did not set arrives null/unknown — we
// must omit it (nil) so the merge-patch preserves the server value rather than
// clobbering it to false. Fields the user did set round-trip as concrete values.
func optBool(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// buildComputerInventoryCollectionSettingsInput converts the Terraform plan into the V2
// settings merge-patch payload. It carries ONLY the collection preferences — never
// applicationPaths. Custom paths cannot be created through the settings PATCH (the wire
// rejects entries without a server-assigned id), so they are managed exclusively via the
// dedicated custom-path create/delete endpoints. include_software_id is Computed-only and
// is therefore never sent.
func buildComputerInventoryCollectionSettingsInput(plan ComputerInventoryCollectionSettingsResourceModel) *pro.ComputerInventoryCollectionSettingsV2 {
	return &pro.ComputerInventoryCollectionSettingsV2{
		ComputerInventoryCollectionPreferences: &pro.ComputerInventoryCollectionPreferencesV2{
			IncludeAccounts:               optBool(plan.CollectLocalUserAccounts),
			CalculateSizes:                optBool(plan.IncludeHomeDirectorySizes),
			IncludeHiddenAccounts:         optBool(plan.IncludeHiddenAccounts),
			IncludePrinters:               optBool(plan.CollectPrinters),
			IncludeServices:               optBool(plan.CollectActiveServices),
			CollectSyncedMobileDeviceInfo: optBool(plan.CollectSyncedMobileDeviceBackupDates),
			UpdateLdapInfoOnComputerInventorySubmissions: optBool(plan.CollectUserAndLocationFromDirectoryService),
			IncludePackages:              optBool(plan.CollectPackageReceipts),
			IncludeSoftwareUpdates:       optBool(plan.CollectAvailableSoftwareUpdates),
			CollectUnmanagedCertificates: optBool(plan.CollectUnmanagedCertificates),
			MonitorBeacons:               optBool(plan.MonitorBeaconRegions),
			AllowChangingUserAndLocation: optBool(plan.AllowJamfBinaryUserAndLocationChanges),
			MonitorApplicationUsage:      optBool(plan.CollectApplicationUsageInformation),
			UseUnixUserPaths:             optBool(plan.UseUnixUserPaths),
		},
	}
}
