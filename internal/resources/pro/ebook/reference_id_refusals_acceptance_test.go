// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ebook_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProEbook_ReferenceIDRefusals proves the reference IDs that
// Jamf Pro cannot clear to an empty value or 0 are refused at plan time
// (#573), before an apply could fail with an inconsistent result.
func TestAccResource_ProEbook_ReferenceIDRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// EmptyIcon
				Config: `
				resource "jamfplatform_pro_ebook" "test" {
				  general = {
				    name            = "tf-acc-ebook-refuse"
				    url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
				    file_type       = "PDF"
				    version         = "1.0"
				    deployment_type = "Make Available in Self Service"
				  }
				  self_service = {
				    icon_id = ""
				  }
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid icon ID`),
			},
			{
				// MinusOneIcon
				Config: `
				resource "jamfplatform_pro_ebook" "test" {
				  general = {
				    name            = "tf-acc-ebook-refuse"
				    url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
				    file_type       = "PDF"
				    version         = "1.0"
				    deployment_type = "Make Available in Self Service"
				  }
				  self_service = {
				    icon_id = "-1"
				  }
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid icon ID`),
			},
			{
				// ZeroIcon
				Config: `
				resource "jamfplatform_pro_ebook" "test" {
				  general = {
				    name            = "tf-acc-ebook-refuse"
				    url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
				    file_type       = "PDF"
				    version         = "1.0"
				    deployment_type = "Make Available in Self Service"
				  }
				  self_service = {
				    icon_id = "0"
				  }
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid icon ID`),
			},
		},
	})
}
