// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /allowedfileextensions endpoint.
// Classic has known concurrency issues when multiple writes hit the same resource
// type — keep these tests serial with any future classic acceptance work in this
// package.

package allowed_file_extension_test

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// extFor builds a run-unique, lowercase, whitespace-free extension value. The variant
// suffix distinguishes records used by different tests (and steps) within the same run so
// they never collide. The "tf" prefix keeps fixtures clearly synthetic.
func extFor(suffix, variant string) string {
	return fmt.Sprintf("tf%s%s", suffix, variant)
}

// testAccCheckAllowedFileExtensionDestroy verifies records created during the test were destroyed.
func testAccCheckAllowedFileExtensionDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_allowed_file_extension" {
				continue
			}
			_, err := c.GetAllowedFileExtensionByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro allowed file extension %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro allowed file extension %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// regexpConflict matches the server's 409 rejection for a duplicate extension. The error
// surfaces the raw SDK message ("...status 409... Duplicate extension"), so match the
// "409" status token — whitespace-free, immune to the line-wrap brittleness Terraform
// introduces when it renders long error details (the §ExpectError regex line-wrap lesson).
func regexpConflict() *regexp.Regexp { return regexp.MustCompile("409") }

// regexpWhitespace matches the schema validator rejection for surrounding whitespace.
// "whitespace" is a single token, immune to the error-detail line wrap.
func regexpWhitespace() *regexp.Regexp { return regexp.MustCompile("whitespace") }

func allowedFileExtensionConfig(ext string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_allowed_file_extension" "test" {
			extension = %q
		}
	`, ext)
}

// TestAccResource_ProAllowedFileExtension_Basic exercises create, replacement, and import
// for the classic /allowedfileextensions endpoint. The endpoint has no update path, so
// changing `extension` forces a replacement (asserted with a plan check) rather than an
// in-place edit.
func TestAccResource_ProAllowedFileExtension_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := extFor(suffix, "a")
	replaced := extFor(suffix, "b")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllowedFileExtensionDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: allowedFileExtensionConfig(original),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_allowed_file_extension.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_allowed_file_extension.test", "extension", original),
				),
			},
			{
				Config: allowedFileExtensionConfig(replaced),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("jamfplatform_pro_allowed_file_extension.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_allowed_file_extension.test", "extension", replaced),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_allowed_file_extension.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProAllowedFileExtension_WhitespaceRejected verifies the schema validator
// rejects an extension with surrounding whitespace at plan time. The server would
// otherwise trim it and produce a post-apply inconsistency.
func TestAccResource_ProAllowedFileExtension_WhitespaceRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      allowedFileExtensionConfig(" jpg "),
				ExpectError: regexpWhitespace(),
			},
		},
	})
}

// TestAccResource_ProAllowedFileExtension_DuplicateRejected verifies the server-enforced
// uniqueness constraint: creating a second record with an identical extension fails with
// a 409 Conflict.
//
// The duplicate check is read-then-write on the classic backend and is NOT race-safe:
// two records created in parallel (Terraform's default) can both slip through before
// either commits. The explicit depends_on serialises creation so "b" POSTs against an
// already-committed "a" — the sequential path that triggers the 409 (confirmed by wire-probe).
func TestAccResource_ProAllowedFileExtension_DuplicateRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	ext := extFor(suffix, "d")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllowedFileExtensionDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_allowed_file_extension" "a" {
						extension = %q
					}

					resource "jamfplatform_pro_allowed_file_extension" "b" {
						extension  = %q
						depends_on = [jamfplatform_pro_allowed_file_extension.a]
					}
				`, ext, ext),
				ExpectError: regexpConflict(),
			},
		},
	})
}

func TestAccDataSource_ProAllowedFileExtension_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	ext := extFor(suffix, "e")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllowedFileExtensionDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_allowed_file_extension" "src" {
						extension = %q
					}

					data "jamfplatform_pro_allowed_file_extension" "lookup" {
						id = jamfplatform_pro_allowed_file_extension.src.id
					}
				`, ext),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_allowed_file_extension.lookup", "extension", "jamfplatform_pro_allowed_file_extension.src", "extension"),
				),
			},
		},
	})
}

func TestAccDataSource_ProAllowedFileExtension_ByExtension(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	ext := extFor(suffix, "f")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllowedFileExtensionDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_allowed_file_extension" "src" {
						extension = %q
					}

					data "jamfplatform_pro_allowed_file_extension" "lookup" {
						extension = jamfplatform_pro_allowed_file_extension.src.extension
					}
				`, ext),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_allowed_file_extension.lookup", "id", "jamfplatform_pro_allowed_file_extension.src", "id"),
				),
			},
		},
	})
}

// TestAccListResource_ProAllowedFileExtension_Basic exercises the
// jamfplatform_pro_allowed_file_extension list resource via the `terraform query` workflow.
func TestAccListResource_ProAllowedFileExtension_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	ext := extFor(suffix, "g")

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAllowedFileExtensionDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_allowed_file_extension" "src" {
						extension = %q
					}
				`, ext),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_allowed_file_extension.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_allowed_file_extension" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = {
								name_substring = %q
							}
						}
					}
				`, ext),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_allowed_file_extension.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_allowed_file_extension.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(ext)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("extension"), KnownValue: knownvalue.StringExact(ext)},
						},
					),
				},
			},
		},
	})
}
