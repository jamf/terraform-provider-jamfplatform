// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// assignSmartMobileDeviceGroupResourceModel populates a resource model from a
// group read.
//
// target doubles as the source for two decisions, which is why it is read as
// well as written. Its Description supplies the reconcile that keeps an
// explicitly authored empty string from collapsing to null. Its Criteria supply
// the empty-versus-absent distinction: the read reports no criteria the same way
// whether the author wrote `criteria = []` or left the attribute out, and
// flattening both to null would make the first of those never settle. So an
// authored empty list stays an empty list, and an absent one stays absent.
//
// platform_id is not touched here. It is resolved once, by the caller, and only
// on the paths that have no value for it yet.
func assignSmartMobileDeviceGroupResourceModel(target *SmartMobileDeviceGroupResourceModel, got *pro.SmartGroupDetailV2) {
	if got == nil {
		return
	}
	if got.GroupID != "" {
		target.ID = types.StringValue(got.GroupID)
	}
	target.Name = types.StringValue(got.GroupName)
	target.Description = helpers.ReconcileOptionalString(got.GroupDescription, target.Description)
	target.SiteID = progroups.SiteIDForState(&got.SiteID)

	flattened := criteria.FlattenMobileDeviceSmartGroupCriteria(&got.Criteria)
	if flattened == nil && target.Criteria != nil {
		flattened = []criteria.CriterionModel{}
	}
	target.Criteria = flattened
}

// assignSmartMobileDeviceGroupDataSourceModel populates the singular data source
// model. Nothing is reconciled: a data source has no prior value to preserve, so
// the read is authoritative for every field.
func assignSmartMobileDeviceGroupDataSourceModel(target *SmartMobileDeviceGroupDataSourceModel, got *pro.SmartGroupDetailV2) {
	if got == nil {
		return
	}
	if got.GroupID != "" {
		target.ID = types.StringValue(got.GroupID)
	}
	target.Name = types.StringValue(got.GroupName)
	target.Description = types.StringValue(got.GroupDescription)
	target.SiteID = progroups.SiteIDForState(&got.SiteID)
	target.Criteria = criteria.FlattenMobileDeviceSmartGroupCriteria(&got.Criteria)
}

// smartMobileDeviceGroupsResults maps a page of the collection read to the
// plural data source's results.
//
// byPlatformID is one lookup for the whole page, built by a single bridge
// request, and a group missing from it reports a null platform_id rather than
// failing the read.
//
// member_count appears here and on no other construct in this package. On the
// collection read it is reporting, which is what a data source is for; in
// resource state it would be drift, because Jamf Pro recomputes it from
// inventory with no configuration change in sight.
func smartMobileDeviceGroupsResults(groups []pro.SmartGroup, byPlatformID map[string]string) []SmartMobileDeviceGroupsDataSourceResultModel {
	out := make([]SmartMobileDeviceGroupsDataSourceResultModel, 0, len(groups))
	for _, g := range groups {
		out = append(out, SmartMobileDeviceGroupsDataSourceResultModel{
			ID:          types.StringValue(g.GroupID),
			PlatformID:  progroups.PlatformIDValue(byPlatformID, g.GroupID),
			Name:        types.StringValue(g.GroupName),
			Description: types.StringValue(g.GroupDescription),
			SiteID:      progroups.SiteIDForState(&g.SiteID),
			MemberCount: types.Int64Value(int64(g.Count)),
		})
	}
	return out
}
