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

func twoConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_ebook" "one" {
  general = {
    name            = %[1]q
    url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
    file_type       = "PDF"
    version         = "1.0"
    deployment_type = "Make Available in Self Service"
  }
}

resource "jamfplatform_pro_ebook" "two" {
  depends_on = [jamfplatform_pro_ebook.one]

  general = {
    name            = %[2]q
    url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
    file_type       = "PDF"
    version         = "1.0"
    deployment_type = "Make Available in Self Service"
  }
}
`, nameOne, nameTwo)
}

func TestAccResource_ProEbook_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-ebook-one-" + suffix
	two := "tf-acc-collide-ebook-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: twoConfig(one, two)},
			{
				Config:      twoConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProEbook_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-ebook-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)Error\s+creating.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
