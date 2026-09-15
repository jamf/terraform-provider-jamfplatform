// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_macos

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// intPtrValueOrNull converts an SDK *int into a types.Int64, mapping nil to
// null. The macOS branding fields are individually-optional and nullable, so a
// nil pointer (server echoed null) must stay null in state to match config.
func intPtrValueOrNull(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

// assignSelfServiceBrandingMacosResourceModel copies the SDK config into the
// resource model. The assigner does NOT set ID — the CRUD handler stamps
// helpers.SingletonID.
func assignSelfServiceBrandingMacosResourceModel(state *SelfServiceBrandingMacosResourceModel, cfg *pro.MacOsBrandingConfiguration) {
	state.ApplicationHeader = helpers.StringPointerValueOrNull(cfg.ApplicationName)
	state.SidebarHeading = helpers.StringPointerValueOrNull(cfg.BrandingName)
	state.SidebarSubheading = helpers.StringPointerValueOrNull(cfg.BrandingNameSecondary)
	state.HomePageHeading = helpers.StringPointerValueOrNull(cfg.HomeHeading)
	state.HomePageSubheading = helpers.StringPointerValueOrNull(cfg.HomeSubheading)
	state.IconID = intPtrValueOrNull(cfg.IconID)
	state.BannerImageID = intPtrValueOrNull(cfg.BrandingHeaderImageID)
}

// assignSelfServiceBrandingMacosDataSourceModel copies the SDK config into the
// data source model.
func assignSelfServiceBrandingMacosDataSourceModel(data *SelfServiceBrandingMacosDataSourceModel, cfg *pro.MacOsBrandingConfiguration) {
	data.ApplicationHeader = helpers.StringPointerValueOrNull(cfg.ApplicationName)
	data.SidebarHeading = helpers.StringPointerValueOrNull(cfg.BrandingName)
	data.SidebarSubheading = helpers.StringPointerValueOrNull(cfg.BrandingNameSecondary)
	data.HomePageHeading = helpers.StringPointerValueOrNull(cfg.HomeHeading)
	data.HomePageSubheading = helpers.StringPointerValueOrNull(cfg.HomeSubheading)
	data.IconID = intPtrValueOrNull(cfg.IconID)
	data.BannerImageID = intPtrValueOrNull(cfg.BrandingHeaderImageID)
}
