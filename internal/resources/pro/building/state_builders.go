// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package building

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignBuildingResourceModel populates a resource model from a Building response.
// Only overwrites state.ID when b.ID is non-nil so post-create GETs that omit the ID
// do not clobber the value captured from CreateBuildingV1's HrefResponse. Every
// Optional+Computed field uses ReconcileOptionalStringPointer so the explicit-null
// vs API-empty distinction the user set is preserved across refreshes.
func assignBuildingResourceModel(state *BuildingResourceModel, b *pro.Building) {
	if b.ID != nil {
		state.ID = types.StringValue(*b.ID)
	}
	state.Name = types.StringValue(b.Name)
	state.City = helpers.ReconcileOptionalStringPointer(b.City, state.City)
	state.Country = helpers.ReconcileOptionalStringPointer(b.Country, state.Country)
	state.StateProvince = helpers.ReconcileOptionalStringPointer(b.StateProvince, state.StateProvince)
	state.StreetAddress1 = helpers.ReconcileOptionalStringPointer(b.StreetAddress1, state.StreetAddress1)
	state.StreetAddress2 = helpers.ReconcileOptionalStringPointer(b.StreetAddress2, state.StreetAddress2)
	state.ZipPostalCode = helpers.ReconcileOptionalStringPointer(b.ZipPostalCode, state.ZipPostalCode)
}

// assignBuildingDataSourceModel populates a data source model from a Building response.
func assignBuildingDataSourceModel(state *BuildingDataSourceModel, b *pro.Building) {
	if b.ID != nil {
		state.ID = types.StringValue(*b.ID)
	}
	state.Name = types.StringValue(b.Name)
	state.City = helpers.StringPointerValueOrNull(b.City)
	state.Country = helpers.StringPointerValueOrNull(b.Country)
	state.StateProvince = helpers.StringPointerValueOrNull(b.StateProvince)
	state.StreetAddress1 = helpers.StringPointerValueOrNull(b.StreetAddress1)
	state.StreetAddress2 = helpers.StringPointerValueOrNull(b.StreetAddress2)
	state.ZipPostalCode = helpers.StringPointerValueOrNull(b.ZipPostalCode)
}
