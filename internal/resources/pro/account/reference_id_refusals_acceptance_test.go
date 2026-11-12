// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package account_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProAccount_ReferenceIDRefusals proves the reference IDs that
// Jamf Pro cannot clear to an empty value or 0 are refused at plan time
// (#573), before an apply could fail with an inconsistent result.
func TestAccResource_ProAccount_ReferenceIDRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// ZeroSite
				Config: `
				resource "jamfplatform_pro_account" "test" {
				  username      = "tf-acc-refuse"
				  full_name     = "TF Acc Refuse"
				  email_address = "tf-acc-refuse@example.invalid"
				  access_level  = "Full Access"
				  privilege_set = "Administrator"
				  site_id = 0
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid site ID`),
			},
			{
				// ZeroLDAPServer
				Config: `
				resource "jamfplatform_pro_account" "test" {
				  username      = "tf-acc-refuse"
				  full_name     = "TF Acc Refuse"
				  email_address = "tf-acc-refuse@example.invalid"
				  access_level  = "Full Access"
				  privilege_set = "Administrator"
				  ldap_server_id = 0
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid LDAP server ID`),
			},
		},
	})
}
