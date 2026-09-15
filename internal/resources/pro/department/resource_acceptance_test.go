// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package department_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckDepartmentDestroy verifies departments created during the test were destroyed.
func testAccCheckDepartmentDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_department" {
				continue
			}
			_, err := c.GetDepartmentV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro department %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro department %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// departmentConfig returns a department resource config with the supplied name.
func departmentConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_department" "test" {
			name = %q
		}
	`, name)
}

// TestAccResource_ProDepartment_Basic walks create → update → import for a Jamf Pro
// department. There are no optional fields to partially populate; the framework's
// implicit post-apply plan check fails any step that round-trips dirty.
func TestAccResource_ProDepartment_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	nameInitial := "tf-acc-pro-department-" + suffix
	nameUpdated := "tf-acc-pro-department-updated-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDepartmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: departmentConfig(nameInitial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_department.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_department.test", "name", nameInitial),
				),
			},
			{
				Config: departmentConfig(nameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_department.test", "name", nameUpdated),
				),
			},
			{
				ResourceName:      "jamfplatform_pro_department.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
				},
			},
		},
	})
}

func TestAccDataSource_ProDepartment_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-department-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDepartmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_department" "src" {
						name = %q
					}

					data "jamfplatform_pro_department" "lookup" {
						id = jamfplatform_pro_department.src.id
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_department.lookup", "name", "jamfplatform_pro_department.src", "name"),
				),
			},
		},
	})
}

func TestAccDataSource_ProDepartments_FilterByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-departments-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDepartmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_department" "src" {
						name = %q
					}

					data "jamfplatform_pro_departments" "lookup" {
						filter = [
							{
								selector = "name"
								argument = jamfplatform_pro_department.src.name
							}
						]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_departments.lookup", "departments.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_departments.lookup", "departments.0.name", name),
				),
			},
		},
	})
}

// TestAccListResource_ProDepartment_Basic exercises the jamfplatform_pro_department
// list resource via the `terraform query` workflow.
func TestAccListResource_ProDepartment_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-department-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDepartmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_department" "src" {
						name = %q
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_department.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_department" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = [
								{
									selector = "name"
									argument = %q
								}
							]
						}
					}
				`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_department.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_department.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
						},
					),
				},
			},
		},
	})
}
