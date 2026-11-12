//go:build acceptance

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProMobileDevicePrestageEnrollment_ReferenceClear proves "-1" is the
// clear value for enrollment_site_id and location_information.building_id /
// department_id (#573, wire-probed 2026-11-12): "" and "0" are refused at plan
// time, "-1" clears each on the server, and the cleared state plans empty. Before
// the refusal "" was rolled back by a bug-shaped 500 and "0" drew a 400.
func TestAccResource_ProMobileDevicePrestageEnrollment_ReferenceClear(t *testing.T) {
	testhelpers.AccPreCheck(t)
	adeID := requireSharedADE(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-mobile-prestage-refclear-" + suffix

	config := func(siteID, buildingID, departmentID string) string {
		return adeFixtureBlock(adeID) + fmt.Sprintf(`
resource "jamfplatform_pro_site" "clear" {
  name = "tf-acc-mps-siteclear-%[1]s"
}
resource "jamfplatform_pro_building" "clear" {
  name = "tf-acc-mps-bldclear-%[1]s"
}
resource "jamfplatform_pro_department" "clear" {
  name = "tf-acc-mps-depclear-%[1]s"
}
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = %[2]q
  device_enrollment_program_instance_id = %[3]s
  timezone                              = "UTC"
  enrollment_site_id                    = %[4]s

  location_information = {
    building_id   = %[5]s
    department_id = %[6]s
  }
  purchasing_information = {}
  skip_setup_items       = {}
  names                  = {}
  scope_serial_numbers   = []
}
`, suffix, name, adeFixtureRef, siteID, buildingID, departmentID)
	}
	real := []string{"jamfplatform_pro_site.clear.id", "jamfplatform_pro_building.clear.id", "jamfplatform_pro_department.clear.id"}
	cleared := []string{`"-1"`, `"-1"`, `"-1"`}
	with := func(i int, v string) (string, string, string) {
		a := append([]string{}, cleared...)
		a[i] = v
		return a[0], a[1], a[2]
	}
	refusal := func(i int, v, noun string) resource.TestStep {
		s, b, d := with(i, v)
		return resource.TestStep{Config: config(s, b, d), PlanOnly: true, ExpectError: regexp.MustCompile("Invalid " + noun + " ID")}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			refusal(0, `""`, "site"), refusal(0, `"0"`, "site"),
			refusal(1, `""`, "building"), refusal(1, `"0"`, "building"),
			refusal(2, `""`, "department"), refusal(2, `"0"`, "department"),
			{
				Config: config(real[0], real[1], real[2]),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(resourceName, "enrollment_site_id", "jamfplatform_pro_site.clear", "id"),
					resource.TestCheckResourceAttrPair(resourceName, "location_information.building_id", "jamfplatform_pro_building.clear", "id"),
					resource.TestCheckResourceAttrPair(resourceName, "location_information.department_id", "jamfplatform_pro_department.clear", "id"),
				),
			},
			{
				Config: config(cleared[0], cleared[1], cleared[2]),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enrollment_site_id", "-1"),
					resource.TestCheckResourceAttr(resourceName, "location_information.building_id", "-1"),
					resource.TestCheckResourceAttr(resourceName, "location_information.department_id", "-1"),
				),
			},
			{
				Config:           config(cleared[0], cleared[1], cleared[2]),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
		},
	})
}

// TestAccResource_ProMobileDevicePrestageEnrollment_ReferenceRefusals proves the plan-time refusals for the
// reference IDs Jamf Pro cannot clear to the value written (#573, wire-probed
// 2026-11-12): an enrollment customization takes a positive ID or "0" and
// refuses "-1" and "", and the other references refuse "" and "0".
func TestAccResource_ProMobileDevicePrestageEnrollment_ReferenceRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	adeID := requireSharedADE(t)
	name := "tf-acc-refusals-" + testhelpers.RunSuffix()

	withAttr := func(cfg, attr, value string) string {
		i := strings.LastIndex(cfg, "\n}\n")
		return cfg[:i] + "\n  " + attr + " = " + value + cfg[i:]
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: withAttr(adeFixtureBlock(adeID)+testAccMobilePrestageMinimalConfig(name), "enrollment_customization_id", `""`), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid enrollment customization ID`)},
			{Config: withAttr(adeFixtureBlock(adeID)+testAccMobilePrestageMinimalConfig(name), "enrollment_customization_id", `"-1"`), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid enrollment customization ID`)},
			{Config: withAttr(adeFixtureBlock(adeID)+testAccMobilePrestageMinimalConfig(name), "rts_config_profile_id", `""`), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid configuration profile ID`)},
			{Config: withAttr(adeFixtureBlock(adeID)+testAccMobilePrestageMinimalConfig(name), "rts_config_profile_id", `"0"`), PlanOnly: true, ExpectError: regexp.MustCompile(`Invalid configuration profile ID`)},
		},
	})
}
