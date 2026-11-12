// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package file_share_distribution_point_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProFileShareDistributionPoint_ReferenceIDRefusals proves the reference IDs that
// Jamf Pro cannot clear to an empty value or 0 are refused at plan time
// (#573), before an apply could fail with an inconsistent result.
func TestAccResource_ProFileShareDistributionPoint_ReferenceIDRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// EmptyBackup
				Config: `
				resource "jamfplatform_pro_file_share_distribution_point" "test" {
				  name                         = "tf-acc-fsdp-refuse"
				  server_name                  = "smb.acc.example.com"
				  file_sharing_connection_type = "SMB"
				  share_name                   = "CasperShare"
				  backup_distribution_point_id = ""
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid distribution point ID`),
			},
			{
				// ZeroBackup
				Config: `
				resource "jamfplatform_pro_file_share_distribution_point" "test" {
				  name                         = "tf-acc-fsdp-refuse"
				  server_name                  = "smb.acc.example.com"
				  file_sharing_connection_type = "SMB"
				  share_name                   = "CasperShare"
				  backup_distribution_point_id = "0"
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid distribution point ID`),
			},
		},
	})
}
