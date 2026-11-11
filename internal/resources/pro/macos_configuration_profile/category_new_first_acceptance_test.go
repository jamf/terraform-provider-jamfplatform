// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package macos_configuration_profile_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_MacOSConfigurationProfile_SelfServiceCategoryCreatedFirst
// creates a category and a profile listing it ahead of an existing,
// lower-id category, in one apply (issue #568). Jamf Pro returns the list
// sorted by id, so the read must keep the new category first. The created id is
// unknown in the plan but known by Create, because Terraform re-plans before
// applying; this test passes on the pre-AlignToPrior code too and pins the
// end-to-end shape, while the unit tests cover the keyless path.
func TestAccResource_MacOSConfigurationProfile_SelfServiceCategoryCreatedFirst(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-align-mcp-newfirst-" + suffix
	const addr = "jamfplatform_pro_macos_configuration_profile.test"

	existing := fmt.Sprintf(`
resource "jamfplatform_pro_category" "existing" {
  name     = "tf-align-mcp-existing-%[1]s"
  priority = 9
}
`, suffix)
	withProfile := existing + fmt.Sprintf(`
resource "jamfplatform_pro_category" "created" {
  name     = "tf-align-mcp-created-%[1]s"
  priority = 9
}

resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name     = %[2]q
    payloads = <<EOF
%[3]s
EOF
  }
  self_service = {
    categories = [
      { id = jamfplatform_pro_category.created.id, feature_in = true },
      { id = jamfplatform_pro_category.existing.id },
    ]
  }
}
`, suffix, name, minimalPayload)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: existing},
			{
				Config: withProfile,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh:  []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, profileCategories+".#", "2"),
					resource.TestCheckResourceAttrPair(addr, profileCategories+".0.id", "jamfplatform_pro_category.created", "id"),
					resource.TestCheckResourceAttrPair(addr, profileCategories+".0.name", "jamfplatform_pro_category.created", "name"),
					resource.TestCheckResourceAttr(addr, profileCategories+".0.feature_in", "true"),
					resource.TestCheckResourceAttrPair(addr, profileCategories+".1.id", "jamfplatform_pro_category.existing", "id"),
					resource.TestCheckResourceAttrPair(addr, profileCategories+".1.name", "jamfplatform_pro_category.existing", "name"),
					resource.TestCheckResourceAttr(addr, profileCategories+".1.feature_in", "false"),
				),
			},
			{
				Config:   withProfile,
				PlanOnly: true,
			},
		},
	})
}
