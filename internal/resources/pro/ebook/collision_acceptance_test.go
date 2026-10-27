// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ebook_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProEbook_NameCollision proves the plan-time check fires: a second resource
// whose name differs from the first only in case is refused at plan, before any write.
func TestAccResource_ProEbook_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-ebook-" + testhelpers.RunSuffix()
	existing := fmt.Sprintf(`
					resource "jamfplatform_pro_ebook" "existing" {
						general = {
							name            = %q
							url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
							file_type       = "PDF"
							version         = "1.0"
							deployment_type = "Make Available in Self Service"
						}
					}
				`, name)
	duplicate := fmt.Sprintf(`
					resource "jamfplatform_pro_ebook" "duplicate" {
						general = {
							name            = %q
							url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
							file_type       = "PDF"
							version         = "1.0"
							deployment_type = "Make Available in Self Service"
						}
					}
				`, strings.ToUpper(name))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: existing},
			{
				Config:      existing + duplicate,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro`),
			},
		},
	})
}
