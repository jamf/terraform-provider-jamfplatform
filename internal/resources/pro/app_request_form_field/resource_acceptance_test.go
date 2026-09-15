// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package app_request_form_field_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const fieldAddr = "jamfplatform_pro_app_request_form_field.test"

// titleFor builds a run-unique, synthetic form field title.
func titleFor(suffix, variant string) string {
	return fmt.Sprintf("ZTFACC Field %s %s", suffix, variant)
}

func testAccCheckAppRequestFormFieldDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_app_request_form_field" {
				continue
			}
			_, err := c.GetAppRequestFormInputFieldV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro App Request form field %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro App Request form field %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func fieldConfig(title, description string, priority int) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_app_request_form_field" "test" {
			title       = %q
			description = %q
			priority    = %d
		}
	`, title, description, priority)
}

// TestAccResource_ProAppRequestFormField_Basic exercises create, in-place update (title is
// mutable, not RequiresReplace), and import.
func TestAccResource_ProAppRequestFormField_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := titleFor(suffix, "A")
	renamed := titleFor(suffix, "A2")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppRequestFormFieldDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fieldConfig(original, "Why do you need this app?", 5),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(fieldAddr, "id"),
					resource.TestCheckResourceAttr(fieldAddr, "title", original),
					resource.TestCheckResourceAttr(fieldAddr, "description", "Why do you need this app?"),
					resource.TestCheckResourceAttr(fieldAddr, "priority", "5"),
				),
			},
			{
				// Rename + re-prioritise + clear description — all in place (no replacement).
				Config: fieldConfig(renamed, "", 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(fieldAddr, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(fieldAddr, "title", renamed),
					resource.TestCheckResourceAttr(fieldAddr, "description", ""),
					resource.TestCheckResourceAttr(fieldAddr, "priority", "2"),
				),
			},
			{
				ResourceName:            fieldAddr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProAppRequestFormField_Ordering creates two independent fields with
// distinct priorities and confirms both persist — exercising the one-resource-per-field
// model (cross-field independent writes, no reorder call).
func TestAccResource_ProAppRequestFormField_Ordering(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	first := titleFor(suffix, "first")
	second := titleFor(suffix, "second")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppRequestFormFieldDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_app_request_form_field" "first" {
						title    = %q
						priority = 10
					}
					resource "jamfplatform_pro_app_request_form_field" "second" {
						title    = %q
						priority = 20
					}
				`, first, second),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_app_request_form_field.first", "priority", "10"),
					resource.TestCheckResourceAttr("jamfplatform_pro_app_request_form_field.second", "priority", "20"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_app_request_form_field.first", "id"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_app_request_form_field.second", "id"),
				),
			},
		},
	})
}

func TestAccDataSource_ProAppRequestFormField_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	title := titleFor(suffix, "byid")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppRequestFormFieldDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_app_request_form_field" "src" {
						title    = %q
						priority = 1
					}
					data "jamfplatform_pro_app_request_form_field" "lookup" {
						id = jamfplatform_pro_app_request_form_field.src.id
					}
				`, title),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_app_request_form_field.lookup", "title", "jamfplatform_pro_app_request_form_field.src", "title"),
				),
			},
		},
	})
}

func TestAccDataSource_ProAppRequestFormField_ByTitle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	title := titleFor(suffix, "bytitle")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppRequestFormFieldDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_app_request_form_field" "src" {
						title    = %q
						priority = 1
					}
					data "jamfplatform_pro_app_request_form_field" "lookup" {
						title      = jamfplatform_pro_app_request_form_field.src.title
						depends_on = [jamfplatform_pro_app_request_form_field.src]
					}
				`, title),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_app_request_form_field.lookup", "id", "jamfplatform_pro_app_request_form_field.src", "id"),
				),
			},
		},
	})
}

// TestAccDataSource_ProAppRequestFormField_AmbiguousTitle confirms the by-title data source
// surfaces the SDK's ambiguous-match error when two fields share a title. Titles are NOT
// unique on the server (wire-probed), so this is the documented failure mode — there is no
// duplicate-title create rejection to assert.
func TestAccDataSource_ProAppRequestFormField_AmbiguousTitle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	title := titleFor(suffix, "dup")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppRequestFormFieldDestroy(t),
		Steps: []resource.TestStep{
			{
				// Two fields, same title — both create successfully (no uniqueness constraint).
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_app_request_form_field" "a" {
						title    = %q
						priority = 1
					}
					resource "jamfplatform_pro_app_request_form_field" "b" {
						title      = %q
						priority   = 2
						depends_on = [jamfplatform_pro_app_request_form_field.a]
					}
				`, title, title),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_app_request_form_field" "a" {
						title    = %q
						priority = 1
					}
					resource "jamfplatform_pro_app_request_form_field" "b" {
						title      = %q
						priority   = 2
						depends_on = [jamfplatform_pro_app_request_form_field.a]
					}
					data "jamfplatform_pro_app_request_form_field" "ambiguous" {
						title      = %q
						depends_on = [jamfplatform_pro_app_request_form_field.a, jamfplatform_pro_app_request_form_field.b]
					}
				`, title, title, title),
				ExpectError: regexp.MustCompile("ambiguous"),
			},
		},
	})
}

// TestAccListResource_ProAppRequestFormField_Basic exercises the list resource via the
// `terraform query` workflow with a provider-side name_substring filter.
func TestAccListResource_ProAppRequestFormField_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	title := titleFor(suffix, "list")

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppRequestFormFieldDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_app_request_form_field" "src" {
						title    = %q
						priority = 1
					}
				`, title),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_app_request_form_field.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_app_request_form_field" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = {
								name_substring = %q
							}
						}
					}
				`, title),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_app_request_form_field.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_app_request_form_field.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(title)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("title"), KnownValue: knownvalue.StringExact(title)},
						},
					),
				},
			},
		},
	})
}
