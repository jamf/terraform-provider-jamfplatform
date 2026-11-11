// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mac_app_store_app_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProMacAppStoreApp_ReferenceIDRefusals proves the reference IDs that
// Jamf Pro cannot clear to an empty value or 0 are refused at plan time
// (#573), before an apply could fail with an inconsistent result.
func TestAccResource_ProMacAppStoreApp_ReferenceIDRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// EmptyIcon
				Config: `
				resource "jamfplatform_pro_mac_app_store_app" "test" {
				  general = {
				    name            = "tf-acc-macapp-refuse"
				    bundle_id       = "com.example.refuse"
				    deployment_type = "Make Available in Self Service"
				    url = "https://apps.apple.com/us/app/example/id1"
				    version         = "1.0"
				  }
				  self_service = {
				    self_service_icon = {
				      id = ""
				    }
				  }
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid icon ID`),
			},
			{
				// MinusOneIcon
				Config: `
				resource "jamfplatform_pro_mac_app_store_app" "test" {
				  general = {
				    name            = "tf-acc-macapp-refuse"
				    bundle_id       = "com.example.refuse"
				    deployment_type = "Make Available in Self Service"
				    url = "https://apps.apple.com/us/app/example/id1"
				    version         = "1.0"
				  }
				  self_service = {
				    self_service_icon = {
				      id = "-1"
				    }
				  }
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid icon ID`),
			},
			{
				// ZeroIcon
				Config: `
				resource "jamfplatform_pro_mac_app_store_app" "test" {
				  general = {
				    name            = "tf-acc-macapp-refuse"
				    bundle_id       = "com.example.refuse"
				    deployment_type = "Make Available in Self Service"
				    url = "https://apps.apple.com/us/app/example/id1"
				    version         = "1.0"
				  }
				  self_service = {
				    self_service_icon = {
				      id = "0"
				    }
				  }
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid icon ID`),
			},
		},
	})
}
