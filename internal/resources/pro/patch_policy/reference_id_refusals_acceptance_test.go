// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package patch_policy_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProPatchPolicy_ReferenceIDRefusals proves the reference IDs that
// Jamf Pro cannot clear to an empty value or 0 are refused at plan time
// (#573), before an apply could fail with an inconsistent result.
func TestAccResource_ProPatchPolicy_ReferenceIDRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// EmptyIcon
				Config: `
				resource "jamfplatform_pro_patch_policy" "test" {
				  software_title_configuration_id = "1"
				  name                            = "tf-acc-pp-refuse"
				  target_version                  = "1.0"
				  distribution_method             = "selfservice"
				  user_interaction = {
				    self_service_icon_id = ""
				  }
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid icon ID`),
			},
			{
				// MinusOneIcon
				Config: `
				resource "jamfplatform_pro_patch_policy" "test" {
				  software_title_configuration_id = "1"
				  name                            = "tf-acc-pp-refuse"
				  target_version                  = "1.0"
				  distribution_method             = "selfservice"
				  user_interaction = {
				    self_service_icon_id = "-1"
				  }
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid icon ID`),
			},
			{
				// ZeroIcon
				Config: `
				resource "jamfplatform_pro_patch_policy" "test" {
				  software_title_configuration_id = "1"
				  name                            = "tf-acc-pp-refuse"
				  target_version                  = "1.0"
				  distribution_method             = "selfservice"
				  user_interaction = {
				    self_service_icon_id = "0"
				  }
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid icon ID`),
			},
		},
	})
}
