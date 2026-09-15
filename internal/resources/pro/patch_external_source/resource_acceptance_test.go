// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /patchexternalsources endpoint.
// Classic has known concurrency issues when multiple writes hit the same
// resource type — keep these tests serial with any future classic acceptance
// work in this package.

package patch_external_source_test

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckPatchExternalSourceDestroy verifies sources created during the test
// were destroyed.
func testAccCheckPatchExternalSourceDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_patch_external_source" {
				continue
			}
			_, err := c.GetPatchExternalSourceByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro patch external source %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro patch external source %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

const patchExternalSourceResourceAddr = "jamfplatform_pro_patch_external_source.test"

// patchExternalSourceLive fetches the server's copy of the source under test
// and hands it to assert, so the drop step can prove on the wire that the port
// was cleared to 0 rather than retained — a null in state cannot tell the two
// apart, because the state builder folds both an empty and a zero port to
// null (#384).
func patchExternalSourceLive(t *testing.T, assert func(*proclassic.PatchExternalSource) error) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return testhelpers.CheckLiveObject(patchExternalSourceResourceAddr, c.GetPatchExternalSourceByID, assert)
}

// TestAccResource_ProPatchExternalSource_Basic exercises create, a multi-attribute
// in-place update (toggling all three Optional+Computed bools and changing
// host_name + port), a step that drops port and proves the clear on the wire,
// and import for the classic /patchexternalsources endpoint. The update step
// also implicitly verifies the GET-after-Update path (classic
// UpdatePatchExternalSourceByID returns 201 with empty body).
func TestAccResource_ProPatchExternalSource_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-patch-ext-src-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchExternalSourceDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_patch_external_source" "test" {
						name                           = %q
						enabled                        = true
						host_name                      = "definitions.example.com/v2/"
						port                           = 8443
						ssl_enabled                    = true
						certificate_validation_enabled = false
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_patch_external_source.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "enabled", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "host_name", "definitions.example.com/v2/"),
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "port", "8443"),
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "ssl_enabled", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "certificate_validation_enabled", "false"),
				),
			},
			{
				// Mutate every non-RequiresReplace attribute: toggle all three
				// bools and change host_name + port.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_patch_external_source" "test" {
						name                           = %q
						enabled                        = false
						host_name                      = "definitions.updated.com/v3/"
						port                           = 9000
						ssl_enabled                    = false
						certificate_validation_enabled = true
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "enabled", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "host_name", "definitions.updated.com/v3/"),
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "port", "9000"),
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "ssl_enabled", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "certificate_validation_enabled", "true"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_patch_external_source" "test" {
						name                           = %q
						enabled                        = false
						host_name                      = "definitions.updated.com/v3/"
						ssl_enabled                    = false
						certificate_validation_enabled = true
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("jamfplatform_pro_patch_external_source.test", "port"),
					resource.TestCheckResourceAttr("jamfplatform_pro_patch_external_source.test", "host_name", "definitions.updated.com/v3/"),
					patchExternalSourceLive(t, func(s *proclassic.PatchExternalSource) error {
						return testhelpers.RequireEqual("port", 0, testhelpers.Deref(s.Port))
					}),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_patch_external_source.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func TestAccDataSource_ProPatchExternalSource_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-patch-ext-src-ds-id-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchExternalSourceDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_patch_external_source" "src" {
						name        = %q
						host_name   = "definitions.datajar.mobi/v2/"
						ssl_enabled = true
					}

					data "jamfplatform_pro_patch_external_source" "lookup" {
						id = jamfplatform_pro_patch_external_source.src.id
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_patch_external_source.lookup", "name", "jamfplatform_pro_patch_external_source.src", "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_patch_external_source.lookup", "host_name", "definitions.datajar.mobi/v2/"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_patch_external_source.lookup", "ssl_enabled", "true"),
					// Only that the attribute is present. Its contents come from a
					// third-party definitions host via Jamf Pro, and the data source
					// treats a failed catalog fetch as non-fatal (warning + empty
					// list) — so asserting on entry 0 tests datajar's uptime, not
					// this provider. Entry mapping is covered by the unit tests in
					// internal/common/availabletitles.
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_patch_external_source.lookup", "available_titles.#"),
				),
			},
		},
	})
}

func TestAccDataSource_ProPatchExternalSource_ByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-patch-ext-src-ds-name-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchExternalSourceDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_patch_external_source" "src" {
						name        = %q
						host_name   = "definitions.datajar.mobi/v2/"
						ssl_enabled = true
					}

					data "jamfplatform_pro_patch_external_source" "lookup" {
						name = jamfplatform_pro_patch_external_source.src.name
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_patch_external_source.lookup", "id", "jamfplatform_pro_patch_external_source.src", "id"),
					// Presence only — see the ByID test for why entry 0 is not asserted.
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_patch_external_source.lookup", "available_titles.#"),
				),
			},
		},
	})
}

// TestAccListResource_ProPatchExternalSource_Basic exercises the
// jamfplatform_pro_patch_external_source list resource via the `terraform query`
// workflow. The classic list endpoint returns only id+name per item, so the
// query check asserts only name/DisplayName.
func TestAccListResource_ProPatchExternalSource_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-patch-ext-src-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchExternalSourceDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_patch_external_source" "src" {
						name      = %q
						host_name = "definitions.example.com/v2/"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_patch_external_source.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_patch_external_source" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_patch_external_source.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_patch_external_source.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							// host_name is NOT in the summary row, and it is Required on
							// the schema — a list result carrying null for it generates
							// configuration the provider refuses to plan. This pins the
							// per-item GET.
							{Path: tfjsonpath.New("host_name"), KnownValue: knownvalue.StringExact("definitions.example.com/v2/")},
						},
					),
				},
			},
		},
	})
}
