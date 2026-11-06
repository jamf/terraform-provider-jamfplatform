// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package policy_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const deprecationFixtures = `
resource "jamfplatform_pro_script" "fixture" {
  name            = "tf-acc-dep-script-%[1]s"
  priority        = "AFTER"
  script_contents = "#!/bin/sh\necho tf-acc-dep"
}

resource "jamfplatform_pro_printer" "fixture" {
  name = "tf-acc-dep-printer-%[1]s"
  uri  = "ipp://10.1.20.130/"
}

resource "jamfplatform_pro_dock_item" "fixture" {
  name = "tf-acc-dep-dock-%[1]s"
  type = "App"
  path = "/Applications/Calculator.app"
}
`

func policyConfigDeprecatedWrappers(suffix string) string {
	return fmt.Sprintf(deprecationFixtures, suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = "tf-acc-policy-dep-%[1]s"
    date_time_limitations = {
      activation_date = "2026-02-03 04:05:06"
      expiration_date = "2027-02-03 04:05:06"
      no_execute_on   = ["Tue", "Thu"]
    }
  }
  scripts = {
    scripts = [{
      id          = jamfplatform_pro_script.fixture.id
      priority    = "After"
      parameter_4 = "tf-acc-p4"
    }]
  }
  printers = {
    printers = [{
      id     = jamfplatform_pro_printer.fixture.id
      action = "Map"
    }]
  }
  dock_items = {
    dock_items = [{
      id     = jamfplatform_pro_dock_item.fixture.id
      action = "Add To Beginning"
    }]
  }
}
`, suffix)
}

func policyConfigFlatReplacements(suffix string) string {
	return fmt.Sprintf(deprecationFixtures, suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = "tf-acc-policy-dep-%[1]s"
  }
  server_side_limitations = {
    activation_date = "2026-02-03 04:05:06"
    expiration_date = "2027-02-03 04:05:06"
  }
  client_side_limitations = {
    no_execute_on = ["Tue", "Thu"]
  }
  script_assignments = [{
    id          = jamfplatform_pro_script.fixture.id
    priority    = "After"
    parameter_4 = "tf-acc-p4"
  }]
  printer_assignments = [{
    id     = jamfplatform_pro_printer.fixture.id
    action = "Map"
  }]
  dock_item_assignments = [{
    id     = jamfplatform_pro_dock_item.fixture.id
    action = "Add To Beginning"
  }]
}
`, suffix)
}

func policyConfigBothShapes(suffix string) string {
	return fmt.Sprintf(deprecationFixtures, suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = "tf-acc-policy-dep-%[1]s"
  }
  scripts = {
    scripts = [{
      id       = jamfplatform_pro_script.fixture.id
      priority = "After"
    }]
  }
  script_assignments = [{
    id       = jamfplatform_pro_script.fixture.id
    priority = "After"
  }]
}
`, suffix)
}

// TestAccPolicyResource_DeprecatedWrappersMigrateToFlatAttributes walks a
// practitioner through the deprecation: the wrapper blocks still apply and read
// back, switching the configuration to the flat replacements is an in-place
// update with no replacement and nothing left to change afterwards, and naming
// both shapes at once is refused at plan time.
func TestAccPolicyResource_DeprecatedWrappersMigrateToFlatAttributes(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	const addr = "jamfplatform_pro_policy.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigDeprecatedWrappers(suffix),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("scripts").AtMapKey("scripts"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("printers").AtMapKey("printers"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("dock_items").AtMapKey("dock_items"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("general").AtMapKey("date_time_limitations").AtMapKey("activation_date"), knownvalue.StringExact("2026-02-03 04:05:06")),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("script_assignments"), knownvalue.Null()),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("printer_assignments"), knownvalue.Null()),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("dock_item_assignments"), knownvalue.Null()),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("server_side_limitations"), knownvalue.Null()),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("client_side_limitations"), knownvalue.Null()),
				},
			},
			{
				Config:      policyConfigBothShapes(suffix),
				ExpectError: regexp.MustCompile(`(?s)cannot be specified when`),
			},
			{
				Config: policyConfigFlatReplacements(suffix),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("script_assignments"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("script_assignments").AtSliceIndex(0).AtMapKey("parameter_4"), knownvalue.StringExact("tf-acc-p4")),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("printer_assignments").AtSliceIndex(0).AtMapKey("action"), knownvalue.StringExact("Map")),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("dock_item_assignments"), knownvalue.SetSizeExact(1)),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("server_side_limitations").AtMapKey("expiration_date"), knownvalue.StringExact("2027-02-03 04:05:06")),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("client_side_limitations").AtMapKey("no_execute_on"), knownvalue.SetSizeExact(2)),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("scripts"), knownvalue.Null()),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("printers"), knownvalue.Null()),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("dock_items"), knownvalue.Null()),
					statecheck.ExpectKnownValue(addr, tfjsonpath.New("general").AtMapKey("date_time_limitations"), knownvalue.Null()),
				},
			},
			{
				Config: policyConfigFlatReplacements(suffix),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
