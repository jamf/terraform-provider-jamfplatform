// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package building

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildBuildingInput converts the Terraform plan model into an SDK Building payload.
// Null or Unknown plan values become omitted (nil) fields — Pro PUT semantics treat
// a missing field as a clear-on-omit, so dropping a previously-set Optional value
// from config causes the server to clear the field. helpers.OptionalStringPointer
// nils both Null *and* Unknown so Optional+Computed attributes do not send empty
// strings; see STYLE_GUIDE.md §Server-derived computed fields & Optional+Computed
// attributes for the broader pattern.
func buildBuildingInput(plan BuildingResourceModel) *pro.Building {
	return &pro.Building{
		Name:           plan.Name.ValueString(),
		City:           helpers.OptionalStringPointer(plan.City),
		Country:        helpers.OptionalStringPointer(plan.Country),
		StateProvince:  helpers.OptionalStringPointer(plan.StateProvince),
		StreetAddress1: helpers.OptionalStringPointer(plan.StreetAddress1),
		StreetAddress2: helpers.OptionalStringPointer(plan.StreetAddress2),
		ZipPostalCode:  helpers.OptionalStringPointer(plan.ZipPostalCode),
	}
}
