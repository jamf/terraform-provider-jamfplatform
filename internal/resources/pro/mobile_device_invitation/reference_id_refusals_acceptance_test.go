// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mobile_device_invitation_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProMobileDeviceInvitation_ReferenceIDRefusals proves the reference IDs that
// Jamf Pro cannot clear to an empty value or 0 are refused at plan time
// (#573), before an apply could fail with an inconsistent result.
func TestAccResource_ProMobileDeviceInvitation_ReferenceIDRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// EmptySite
				Config: `
				resource "jamfplatform_pro_mobile_device_invitation" "test" {
				  invitation_type     = "USER_INITIATED_URL"
				  expiration_date     = "2030-12-31 23:59:00"
  
				  enroll_into_site_id = ""
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid site ID`),
			},
			{
				// ZeroSite
				Config: `
				resource "jamfplatform_pro_mobile_device_invitation" "test" {
				  invitation_type     = "USER_INITIATED_URL"
				  expiration_date     = "2030-12-31 23:59:00"
  
				  enroll_into_site_id = "0"
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid site ID`),
			},
		},
	})
}
