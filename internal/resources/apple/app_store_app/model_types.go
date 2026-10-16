// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_store_app

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// AppStoreAppDataSourceModel is the Terraform model for jamfplatform_apple_app_store_app.
type AppStoreAppDataSourceModel struct {
	ID                      types.String   `tfsdk:"id"`
	BundleID                types.String   `tfsdk:"bundle_id"`
	StoreURL                types.String   `tfsdk:"store_url"`
	Target                  types.String   `tfsdk:"target"`
	CountryRegion           types.String   `tfsdk:"country_region"`
	Language                types.String   `tfsdk:"language"`
	Name                    types.String   `tfsdk:"name"`
	Version                 types.String   `tfsdk:"version"`
	URL                     types.String   `tfsdk:"url"`
	Description             types.String   `tfsdk:"description"`
	IconURL                 types.String   `tfsdk:"icon_url"`
	IsFree                  types.Bool     `tfsdk:"is_free"`
	Genres                  types.List     `tfsdk:"genres"`
	VPPDeviceBasedLicensing types.Bool     `tfsdk:"vpp_device_based_licensing"`
	Timeouts                timeouts.Value `tfsdk:"timeouts"`
}
