// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package jamf_connect

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignJamfConnectResourceModel populates the resource model from a matched
// or echoed LinkedConnectProfile. The same shape serves the list-and-match
// Read and the PUT echo (both Create and Update), so all three feed this.
//
// version is normalised to null when empty: Jamf Connect returns "" whenever
// auto_deployment_type is NONE (and the user is forbidden from setting one),
// so an empty wire value maps to a null attribute rather than "".
func assignJamfConnectResourceModel(state *JamfConnectResourceModel, p *pro.LinkedConnectProfile) {
	profileID := derefInt(p.ProfileID)
	state.ProfileID = types.Int64Value(int64(profileID))
	state.ID = types.StringValue(strconv.Itoa(profileID))
	state.ConfigProfileUUID = helpers.StringPointerValueOrNull(p.UUID)

	state.AutoDeploymentType = types.StringValue(helpers.DerefString(p.AutoDeploymentType))
	state.Version = helpers.StringPointerValueOrNull(p.Version)

	state.ProfileName = helpers.StringPointerValueOrNull(p.ProfileName)
	state.ScopeDescription = helpers.StringPointerValueOrNull(p.ScopeDescription)
	state.SiteID = helpers.StringPointerValueOrNull(p.SiteID)
}

// assignJamfConnectDataSourceModel populates the data source model from a
// matched row. The lookup keys (profile_id / profile_name / config_profile_uuid)
// are all set from the wire so the unsupplied ones are filled in.
func assignJamfConnectDataSourceModel(data *JamfConnectDataSourceModel, p *pro.LinkedConnectProfile) {
	data.ProfileID = types.Int64Value(int64(derefInt(p.ProfileID)))
	data.ConfigProfileUUID = helpers.StringPointerValueOrNull(p.UUID)
	data.ProfileName = helpers.StringPointerValueOrNull(p.ProfileName)
	data.AutoDeploymentType = types.StringValue(helpers.DerefString(p.AutoDeploymentType))
	data.Version = helpers.StringPointerValueOrNull(p.Version)
	data.ScopeDescription = helpers.StringPointerValueOrNull(p.ScopeDescription)
	data.SiteID = helpers.StringPointerValueOrNull(p.SiteID)
}

// derefInt returns the pointed-to int, or 0 when nil.
func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}
