// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mac_app_store_app_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccListResource_ProMacApp_SelfServiceHydration is the #520 regression
// test. `terraform query -generate-config-out` runs the list with
// include_resource, where state starts empty; the icon and category links used
// to stay null there although the applied state and the wire carried them, so
// the generated config dropped them. Asserting the filtered count alone, as
// TestAccListResource_ProMacApp does, cannot see that.
func TestAccListResource_ProMacApp_SelfServiceHydration(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-macapp-listss-" + suffix
	iconA, _ := testhelpers.WriteIconFixtures(t, suffix)

	cfg := testhelpers.CategorySwapFixtures("tf-acc-macapp-listss", suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_icon" "a" {
  icon_file_source = %q
}

resource "jamfplatform_pro_mac_app_store_app" "test" {
  general = {
    name            = %q
    version         = "1.0"
    bundle_id       = "com.example.tfacc.macapp.listss"
    url             = "https://apps.apple.com/app/id000000005"
    deployment_type = "Make Available in Self Service"
  }
  self_service = {
    self_service_icon = {
      id = jamfplatform_pro_icon.a.id
    }
    self_service_categories = [
      { id = jamfplatform_pro_category.a.id, display_in = true },
    ]
  }
}
`, iconA, name)

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check:  resource.TestCheckResourceAttrSet(macAppResourceAddr, "self_service.self_service_icon.id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
provider "jamfplatform" {}

list "jamfplatform_pro_mac_app_store_app" "test" {
  provider         = jamfplatform
  include_resource = true

  config {
    filter = {
      name_substring = %q
    }
  }
}
`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_mac_app_store_app.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_mac_app_store_app.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("self_service").AtMapKey("self_service_icon").AtMapKey("id"), KnownValue: knownvalue.NotNull()},
							{Path: tfjsonpath.New("self_service").AtMapKey("self_service_categories"), KnownValue: knownvalue.SetSizeExact(1)},
						},
					),
				},
			},
		},
	})
}
