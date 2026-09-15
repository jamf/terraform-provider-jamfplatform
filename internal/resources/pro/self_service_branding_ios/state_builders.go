// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_ios

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// intPtrValueOrNull converts an SDK *int into a types.Int64, mapping nil to
// null (icon_id is optional and nullable).
func intPtrValueOrNull(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

// assignSelfServiceBrandingIosResourceModel copies the SDK config into the
// resource model. The Required colour/header fields are non-pointer strings the
// server always returns. The assigner does NOT set ID — the CRUD handler stamps
// helpers.SingletonID.
func assignSelfServiceBrandingIosResourceModel(state *SelfServiceBrandingIosResourceModel, cfg *pro.IosBrandingConfiguration) {
	state.MainHeader = types.StringValue(cfg.BrandingName)
	state.BrandingNameColorCode = types.StringValue(cfg.BrandingNameColorCode)
	state.HeaderBackgroundColorCode = types.StringValue(cfg.HeaderBackgroundColorCode)
	state.MenuIconColorCode = types.StringValue(cfg.MenuIconColorCode)
	state.StatusBarTextColor = types.StringValue(cfg.StatusBarTextColor)
	state.IconID = intPtrValueOrNull(cfg.IconID)
}

// assignSelfServiceBrandingIosDataSourceModel copies the SDK config into the
// data source model.
func assignSelfServiceBrandingIosDataSourceModel(data *SelfServiceBrandingIosDataSourceModel, cfg *pro.IosBrandingConfiguration) {
	data.MainHeader = types.StringValue(cfg.BrandingName)
	data.BrandingNameColorCode = types.StringValue(cfg.BrandingNameColorCode)
	data.HeaderBackgroundColorCode = types.StringValue(cfg.HeaderBackgroundColorCode)
	data.MenuIconColorCode = types.StringValue(cfg.MenuIconColorCode)
	data.StatusBarTextColor = types.StringValue(cfg.StatusBarTextColor)
	data.IconID = intPtrValueOrNull(cfg.IconID)
}
