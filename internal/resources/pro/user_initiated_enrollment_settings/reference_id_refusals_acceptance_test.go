// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package user_initiated_enrollment_settings_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProUserInitiatedEnrollmentSettings_ReferenceIDRefusals proves the reference IDs that
// Jamf Pro cannot clear to an empty value or 0 are refused at plan time
// (#573), before an apply could fail with an inconsistent result.
func TestAccResource_ProUserInitiatedEnrollmentSettings_ReferenceIDRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// EmptySite
				Config: `
				resource "jamfplatform_pro_user_initiated_enrollment_settings" "test" {
				  access_group = [{
				    name           = "tf-acc-siteclear"
				    ldap_server_id = "1"
				    site_id        = ""
				  }]
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid site ID`),
			},
			{
				// ZeroSite
				Config: `
				resource "jamfplatform_pro_user_initiated_enrollment_settings" "test" {
				  access_group = [{
				    name           = "tf-acc-siteclear"
				    ldap_server_id = "1"
				    site_id        = "0"
				  }]
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid site ID`),
			},
		},
	})
}
