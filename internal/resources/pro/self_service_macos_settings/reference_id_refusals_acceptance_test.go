// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package self_service_macos_settings_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProSelfServiceMacosSettings_ReferenceIDRefusals proves the reference IDs that
// Jamf Pro cannot clear to an empty value or 0 are refused at plan time
// (#573), before an apply could fail with an inconsistent result.
func TestAccResource_ProSelfServiceMacosSettings_ReferenceIDRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// ZeroHomeCategory
				Config: `
				resource "jamfplatform_pro_self_service_macos_settings" "test" {
				  default_landing_page     = "BROWSE"
				  default_home_category_id = 0
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must be none of`),
			},
		},
	})
}
