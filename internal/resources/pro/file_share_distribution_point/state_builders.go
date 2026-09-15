// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package file_share_distribution_point

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignFileShareDistributionPointResourceModel populates a resource model
// from a DistributionPoint response.
//
//   - The three plaintext passwords are WriteOnly — the framework excludes
//     them from state and Jamf Pro never echoes them, so they are not touched
//     here. Their `*_wo_version` companions are regular Optional Int64 values
//     the framework preserves verbatim.
//   - Server-defaulted toggles (principal, enable_load_balancing,
//     https_enabled) and ports are adopted directly: Jamf Pro always echoes a
//     value, so the live value is authoritative (UseStateForUnknown carries an
//     omitted field's prior value into the plan, then this read confirms it).
//   - User-authored strings reconcile so an explicit empty string the user set
//     to clear a field round-trips, while a never-set field stays null.
//   - state.ID is only overwritten when the API ID is non-nil so a transient
//     GET that drops the ID does not clobber the value persisted from Create.
func assignFileShareDistributionPointResourceModel(state *FileShareDistributionPointResourceModel, c *pro.DistributionPoint) {
	if c == nil {
		return
	}
	if c.ID != nil {
		state.ID = types.StringValue(*c.ID)
	}
	state.Name = types.StringValue(c.Name)
	state.ServerName = types.StringValue(c.ServerName)
	state.FileSharingConnectionType = types.StringValue(c.FileSharingConnectionType)

	state.Principal = helpers.BoolPointerValueOrNull(c.Principal)
	state.BackupDistributionPointID = helpers.ReconcileOptionalStringPointer(c.BackupDistributionPointID, state.BackupDistributionPointID)
	state.EnableLoadBalancing = helpers.BoolPointerValueOrNull(c.EnableLoadBalancing)

	state.ShareName = helpers.ReconcileOptionalStringPointer(c.ShareName, state.ShareName)
	state.Port = helpers.Int64FromIntPtr(c.Port)
	state.Workgroup = helpers.ReconcileOptionalStringPointer(c.Workgroup, state.Workgroup)

	state.ReadWriteUsername = helpers.ReconcileOptionalStringPointer(c.ReadWriteUsername, state.ReadWriteUsername)
	state.ReadOnlyUsername = helpers.ReconcileOptionalStringPointer(c.ReadOnlyUsername, state.ReadOnlyUsername)

	state.HTTPSEnabled = helpers.BoolPointerValueOrNull(c.HttpsEnabled)
	state.HTTPSPort = helpers.Int64FromIntPtr(c.HttpsPort)
	state.HTTPSContext = helpers.ReconcileOptionalStringPointer(c.HttpsContext, state.HTTPSContext)
	state.HTTPSSecurityType = helpers.ReconcileOptionalStringPointer(c.HttpsSecurityType, state.HTTPSSecurityType)
	state.HTTPSUsername = helpers.ReconcileOptionalStringPointer(c.HttpsUsername, state.HTTPSUsername)
}

// assignFileShareDistributionPointDataSourceModel populates a data source
// model from a DistributionPoint response. Read-only; the plaintext passwords
// and `*_wo_version` triggers are absent from the data source schema.
func assignFileShareDistributionPointDataSourceModel(state *FileShareDistributionPointDataSourceModel, c *pro.DistributionPoint) {
	if c == nil {
		return
	}
	if c.ID != nil {
		state.ID = types.StringValue(*c.ID)
	}
	state.Name = types.StringValue(c.Name)
	state.ServerName = types.StringValue(c.ServerName)
	state.FileSharingConnectionType = types.StringValue(c.FileSharingConnectionType)

	state.Principal = helpers.BoolPointerValueOrNull(c.Principal)
	state.BackupDistributionPointID = helpers.StringPointerValueOrNull(c.BackupDistributionPointID)
	state.EnableLoadBalancing = helpers.BoolPointerValueOrNull(c.EnableLoadBalancing)

	state.ShareName = helpers.StringPointerValueOrNull(c.ShareName)
	state.Port = helpers.Int64FromIntPtr(c.Port)
	state.Workgroup = helpers.StringPointerValueOrNull(c.Workgroup)

	state.ReadWriteUsername = helpers.StringPointerValueOrNull(c.ReadWriteUsername)
	state.ReadOnlyUsername = helpers.StringPointerValueOrNull(c.ReadOnlyUsername)

	state.HTTPSEnabled = helpers.BoolPointerValueOrNull(c.HttpsEnabled)
	state.HTTPSPort = helpers.Int64FromIntPtr(c.HttpsPort)
	state.HTTPSContext = helpers.StringPointerValueOrNull(c.HttpsContext)
	state.HTTPSSecurityType = helpers.StringPointerValueOrNull(c.HttpsSecurityType)
	state.HTTPSUsername = helpers.StringPointerValueOrNull(c.HttpsUsername)
}
