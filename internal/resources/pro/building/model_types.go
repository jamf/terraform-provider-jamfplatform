// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package building

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// BuildingResourceModel represents the Terraform resource model for a Jamf Pro building.
type BuildingResourceModel struct {
	ID             types.String           `tfsdk:"id"`
	Name           types.String           `tfsdk:"name"`
	City           types.String           `tfsdk:"city"`
	Country        types.String           `tfsdk:"country"`
	StateProvince  types.String           `tfsdk:"state_province"`
	StreetAddress1 types.String           `tfsdk:"street_address_1"`
	StreetAddress2 types.String           `tfsdk:"street_address_2"`
	ZipPostalCode  types.String           `tfsdk:"zip_postal_code"`
	Timeouts       resourceTimeouts.Value `tfsdk:"timeouts"`
}

// BuildingDataSourceModel represents the Terraform data source model for a Jamf Pro building.
type BuildingDataSourceModel struct {
	ID             types.String             `tfsdk:"id"`
	Name           types.String             `tfsdk:"name"`
	City           types.String             `tfsdk:"city"`
	Country        types.String             `tfsdk:"country"`
	StateProvince  types.String             `tfsdk:"state_province"`
	StreetAddress1 types.String             `tfsdk:"street_address_1"`
	StreetAddress2 types.String             `tfsdk:"street_address_2"`
	ZipPostalCode  types.String             `tfsdk:"zip_postal_code"`
	Timeouts       datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// buildingIdentityModel represents the identity object for building resources and list results.
type buildingIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// BuildingListResourceModel represents the config model for building list queries.
type BuildingListResourceModel struct {
	Filters  []filters.FilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value    `tfsdk:"timeouts"`
}

// BuildingsDataSourceModel represents the Terraform data source model for building searches.
type BuildingsDataSourceModel struct {
	ID        types.String                     `tfsdk:"id"`
	Buildings []BuildingsDataSourceResultModel `tfsdk:"buildings"`
	Filters   []filters.FilterModel            `tfsdk:"filter"`
	Timeouts  datasourceTimeouts.Value         `tfsdk:"timeouts"`
}

// BuildingsDataSourceResultModel represents a single building in the search results.
type BuildingsDataSourceResultModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	City           types.String `tfsdk:"city"`
	Country        types.String `tfsdk:"country"`
	StateProvince  types.String `tfsdk:"state_province"`
	StreetAddress1 types.String `tfsdk:"street_address_1"`
	StreetAddress2 types.String `tfsdk:"street_address_2"`
	ZipPostalCode  types.String `tfsdk:"zip_postal_code"`
}
