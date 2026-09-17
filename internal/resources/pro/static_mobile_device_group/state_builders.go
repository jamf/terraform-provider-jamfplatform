// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// assignResourceModel writes a group read back from Jamf Pro over the resource
// model's scalars. Membership is not among them: the read that carries it is a
// separate request the caller makes only when membership is managed.
//
// The read reports the Jamf Pro identifier whichever identifier the request used,
// which is what lets Create address the group by its platform identifier and
// still learn the numeric one.
func assignResourceModel(state *StaticMobileDeviceGroupResourceModel, got *pro.StaticGroup) {
	if got == nil {
		return
	}
	if got.GroupID != "" {
		state.ID = types.StringValue(got.GroupID)
	}
	state.Name = types.StringValue(got.GroupName)
	state.Description = helpers.ReconcileOptionalString(got.GroupDescription, state.Description)
	state.SiteID = progroups.SiteIDForState(&got.SiteID)
	state.MemberCount = types.Int64Value(int64(got.Count))
}

// assignMembership writes the group's membership into the resource model.
//
// manage is whether the configuration declared the attribute, and it is what
// makes the three states distinguishable: a managed group's set is written
// verbatim, including empty, while an unmanaged group's stays null so a later
// plan cannot read "Jamf Pro has these members" as "the practitioner asked for
// these members".
//
// hydrating releases that gate for a first-time import, where the attribute
// arrives null however many devices the group holds. It adopts the list only
// when Jamf Pro reports members: adopting an empty one would store [] where a
// create that never declared the attribute stores null, which breaks
// ImportStateVerify.
func assignMembership(ctx context.Context, state *StaticMobileDeviceGroupResourceModel, ids []string, manage, hydrating bool) diag.Diagnostics {
	var diags diag.Diagnostics
	adopting := hydrating && len(ids) > 0
	if !manage && !adopting {
		state.AssignedMobileDeviceIDs = types.SetNull(types.StringType)
		return diags
	}
	if ids == nil {
		ids = []string{}
	}
	set, setDiags := types.SetValueFrom(ctx, types.StringType, ids)
	diags.Append(setDiags...)
	if !diags.HasError() {
		state.AssignedMobileDeviceIDs = set
	}
	return diags
}

// membershipIDs pulls the Jamf Pro mobile device identifiers out of a membership
// read, dropping any entry that carries none.
func membershipIDs(devices []pro.InventoryListMobileDevice) []string {
	out := make([]string, 0, len(devices))
	for _, d := range devices {
		if d.MobileDeviceID == "" {
			continue
		}
		out = append(out, d.MobileDeviceID)
	}
	return out
}

// assignDataSourceModel writes a group read back from Jamf Pro over the singular
// data source model's scalars.
func assignDataSourceModel(state *StaticMobileDeviceGroupDataSourceModel, got *pro.StaticGroup) {
	if got == nil {
		return
	}
	if got.GroupID != "" {
		state.ID = types.StringValue(got.GroupID)
	}
	state.Name = types.StringValue(got.GroupName)
	state.Description = descriptionValue(got.GroupDescription)
	state.SiteID = progroups.SiteIDForState(&got.SiteID)
	state.MemberCount = types.Int64Value(int64(got.Count))
}

// flattenMembershipIDs renders membership identifiers for a data source, in the
// order Jamf Pro returned them.
func flattenMembershipIDs(ids []string) []types.String {
	out := make([]types.String, 0, len(ids))
	for _, id := range ids {
		out = append(out, types.StringValue(id))
	}
	return out
}

// pluralResultModel renders one group for the plural data source, taking its
// platform identifier from the page-wide lookup rather than a request of its own.
func pluralResultModel(got pro.StaticGroup, platformIDs map[string]string) StaticMobileDeviceGroupsDataSourceResultModel {
	return StaticMobileDeviceGroupsDataSourceResultModel{
		ID:          types.StringValue(got.GroupID),
		PlatformID:  progroups.PlatformIDValue(platformIDs, got.GroupID),
		Name:        types.StringValue(got.GroupName),
		Description: descriptionValue(got.GroupDescription),
		SiteID:      progroups.SiteIDForState(&got.SiteID),
		MemberCount: types.Int64Value(int64(got.Count)),
	}
}

// listResultModel renders one group as the resource body a list result carries.
//
// Membership is null rather than empty: the list reads one page and never fans
// out a membership request per group, so a generated configuration leaves the
// attribute undeclared and the group's members alone.
func listResultModel(got pro.StaticGroup, platformIDs map[string]string) StaticMobileDeviceGroupResourceModel {
	return StaticMobileDeviceGroupResourceModel{
		ID:                      types.StringValue(got.GroupID),
		PlatformID:              progroups.PlatformIDValue(platformIDs, got.GroupID),
		Name:                    types.StringValue(got.GroupName),
		Description:             descriptionValue(got.GroupDescription),
		SiteID:                  progroups.SiteIDForState(&got.SiteID),
		AssignedMobileDeviceIDs: types.SetNull(types.StringType),
		MemberCount:             types.Int64Value(int64(got.Count)),
		Timeouts:                helpers.NewResourceTimeoutsNullValue(staticMobileDeviceGroupTimeoutAttributeTypes),
	}
}

// descriptionValue renders a read-only description, reporting an empty one as
// null so a group without a description reads the same way everywhere.
func descriptionValue(wire string) types.String {
	if wire == "" {
		return types.StringNull()
	}
	return types.StringValue(wire)
}
