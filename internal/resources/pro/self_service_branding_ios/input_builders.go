// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_ios

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildSelfServiceBrandingIosInput maps the Terraform plan into the SDK request
// struct. main_header and the four colour fields are Required (the API rejects
// a create without them — wire-probed), so they are always sent. icon_id is
// individually-optional and nullable: null ⇒ nil pointer ⇒ omitted. The iOS
// branding PUT is full-replace, so a removed icon_id is cleared server-side.
func buildSelfServiceBrandingIosInput(plan SelfServiceBrandingIosResourceModel) *pro.IosBrandingConfiguration {
	return &pro.IosBrandingConfiguration{
		BrandingName:              plan.MainHeader.ValueString(),
		BrandingNameColorCode:     plan.BrandingNameColorCode.ValueString(),
		HeaderBackgroundColorCode: plan.HeaderBackgroundColorCode.ValueString(),
		MenuIconColorCode:         plan.MenuIconColorCode.ValueString(),
		StatusBarTextColor:        plan.StatusBarTextColor.ValueString(),
		IconID:                    helpers.OptionalInt64Pointer(plan.IconID),
	}
}
