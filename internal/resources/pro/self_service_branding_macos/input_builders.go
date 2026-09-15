// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_macos

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildSelfServiceBrandingMacosInput maps the Terraform plan into the SDK
// request struct. Every field is individually-optional and nullable: a null
// attribute is omitted from the body (the `*` pointer stays nil so omitempty
// drops it). The macOS branding PUT is full-replace, so an omitted (removed)
// field is cleared to null server-side — which is exactly the desired
// "user removed the attribute ⇒ cleared" behaviour. No merge base is needed.
func buildSelfServiceBrandingMacosInput(plan SelfServiceBrandingMacosResourceModel) *pro.MacOsBrandingConfiguration {
	return &pro.MacOsBrandingConfiguration{
		ApplicationName:       helpers.OptionalStringPointer(plan.ApplicationHeader),
		BrandingName:          helpers.OptionalStringPointer(plan.SidebarHeading),
		BrandingNameSecondary: helpers.OptionalStringPointer(plan.SidebarSubheading),
		HomeHeading:           helpers.OptionalStringPointer(plan.HomePageHeading),
		HomeSubheading:        helpers.OptionalStringPointer(plan.HomePageSubheading),
		IconID:                helpers.OptionalInt64Pointer(plan.IconID),
		BrandingHeaderImageID: helpers.OptionalInt64Pointer(plan.BannerImageID),
	}
}
