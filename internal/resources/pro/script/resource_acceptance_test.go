// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package script_test

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

// testAccCheckScriptDestroy verifies scripts created during the test were destroyed.
func testAccCheckScriptDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_script" {
				continue
			}
			_, err := c.GetScriptV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro script %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro script %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func TestAccResource_ProScript_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-script-" + suffix
	nameUpdated := "tf-acc-pro-script-updated-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckScriptDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_script" "test" {
						name            = %q
						priority        = "AFTER"
						info            = "tf-acc info"
						notes           = "tf-acc notes"
						os_requirements = "13.0.x,14.0.x"
						parameter_4     = "param4-label"
						script_contents = "#!/bin/sh\necho hello"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_script.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_script.test", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_pro_script.test", "priority", "AFTER"),
					resource.TestCheckResourceAttr("jamfplatform_pro_script.test", "parameter_4", "param4-label"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_script" "test" {
						name            = %q
						priority        = "BEFORE"
						script_contents = "#!/bin/sh\necho updated"
					}
				`, nameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_script.test", "name", nameUpdated),
					resource.TestCheckResourceAttr("jamfplatform_pro_script.test", "priority", "BEFORE"),
				),
			},
			{
				ResourceName:      "jamfplatform_pro_script.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
				},
			},
		},
	})
}

func TestAccDataSource_ProScript_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-script-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckScriptDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_script" "src" {
						name            = %q
						priority        = "AFTER"
						script_contents = "echo ds-test"
					}

					data "jamfplatform_pro_script" "lookup" {
						id = jamfplatform_pro_script.src.id
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_script.lookup", "name", "jamfplatform_pro_script.src", "name"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_script.lookup", "priority", "jamfplatform_pro_script.src", "priority"),
				),
			},
		},
	})
}

// TestAccListResource_ProScript_Basic exercises the jamfplatform_pro_script list
// resource via the `terraform query` workflow.
func TestAccListResource_ProScript_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-script-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckScriptDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_script" "src" {
						name            = %q
						priority        = "AFTER"
						script_contents = "echo list-test"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_script.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_script" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_script.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_script.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("priority"), KnownValue: knownvalue.StringExact("AFTER")},
						},
					),
				},
			},
		},
	})
}

func TestAccDataSource_ProScripts_FilterByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scripts-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckScriptDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_script" "src" {
						name            = %q
						priority        = "AFTER"
						script_contents = "echo plural-test"
					}

					data "jamfplatform_pro_scripts" "lookup" {
						filter = [
							{
								selector = "name"
								argument = jamfplatform_pro_script.src.name
							}
						]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_scripts.lookup", "scripts.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_scripts.lookup", "scripts.0.name", name),
				),
			},
		},
	})
}

// TestAccResource_ProScript_SplitOwnership proves the omit=preserve contract for
// the Optional+Computed `script_contents` on the full-replace /v1/scripts endpoint:
// when the field is omitted from HCL, an out-of-band edit (simulating the Jamf Pro
// UI) survives an unrelated Terraform change (a name update) rather than being
// wiped — and an explicit "" still clears it. Without Optional+Computed +
// UseStateForUnknown this regresses: the name-change PUT drops the field and
// full-replace wipes the body.
func TestAccResource_ProScript_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-script-split-" + suffix
	nameUpdated := "tf-acc-pro-script-split-upd-" + suffix
	const addr = "jamfplatform_pro_script.test"
	const tfContents = "#!/bin/sh\necho tf-owned"   // initial TF-declared body
	const uiContents = "#!/bin/sh\necho ui-managed" // later set out-of-band (UI)

	var scriptID string

	// setContentsOutOfBand simulates a UI edit: GET the script, set scriptContents,
	// PUT it back (a full-object write, like the admin console does).
	setContentsOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		got, err := c.GetScriptV1(ctx, scriptID)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}
		v := uiContents
		got.ScriptContents = &v
		if _, err := c.UpdateScriptV1(ctx, scriptID, got); err != nil {
			t.Fatalf("out-of-band PUT: %v", err)
		}
	}

	checkServerContents := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetScriptV1(context.Background(), scriptID)
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if helpers.DerefString(got.ScriptContents) != want {
				return fmt.Errorf("script_contents = %q, want %q", helpers.DerefString(got.ScriptContents), want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckScriptDestroy(t),
		Steps: []resource.TestStep{
			{
				// Create with a TF-declared body, so the next step proves the UI value
				// is preserved AND not reverted to this prior TF-owned value.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_script" "test" {
						name            = %q
						priority        = "AFTER"
						script_contents = %q
					}
				`, name, tfContents),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckResourceAttr(addr, "script_contents", tfContents),
					func(s *terraform.State) error {
						scriptID = s.RootModule().Resources[addr].Primary.ID
						return nil
					},
				),
			},
			{
				// Admin overwrites the body in the UI to a DIFFERENT value; config now
				// REMOVES script_contents and changes only the name. The UI value must
				// survive — neither wiped by the full-replace PUT nor reverted to the
				// prior TF-owned value.
				PreConfig: setContentsOutOfBand,
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_script" "test" {
						name     = %q
						priority = "AFTER"
					}
				`, nameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "name", nameUpdated),
					// State adopts the out-of-band value (Computed) and preserves it.
					resource.TestCheckResourceAttr(addr, "script_contents", uiContents),
					checkServerContents(uiContents),
				),
			},
			{
				// Explicit "" clears it (full-replace), proving TF can still take over.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_script" "test" {
						name            = %q
						priority        = "AFTER"
						script_contents = ""
					}
				`, nameUpdated),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "script_contents", ""),
					checkServerContents(""),
				),
			},
		},
	})
}

// TestAccResource_ProScript_CategorySwap is the regression test for the
// derived-name consistency rule (STYLE_GUIDE §886). `category_name` is a
// Computed-only field derived from the mutable `category_id`. If it carries
// stringplanmodifier.UseStateForUnknown(), step 2 (swapping category_id to a
// differently-named category, in place) pins the stale prior name into the
// plan while apply reads back the new name — Terraform Core then aborts with
// "Provider produced inconsistent result after apply" on category_name.
// With `category_name` modelled as plain Computed (no UseStateForUnknown), the
// plan value goes Unknown and the post-apply name is accepted. This test FAILS
// on the buggy schema and PASSES once the modifier is removed.
func TestAccResource_ProScript_CategorySwap(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-script-catswap-" + suffix
	catA := "tf-acc-cat-a-" + suffix
	catB := "tf-acc-cat-b-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckScriptDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_category" "a" {
						name     = %q
						priority = 9
					}
					resource "jamfplatform_pro_category" "b" {
						name     = %q
						priority = 9
					}
					resource "jamfplatform_pro_script" "test" {
						name            = %q
						priority        = "AFTER"
						script_contents = "#!/bin/sh\necho catswap"
						category_id     = jamfplatform_pro_category.a.id
					}
				`, catA, catB, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("jamfplatform_pro_script.test", "category_id", "jamfplatform_pro_category.a", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_script.test", "category_name", catA),
				),
			},
			{
				// Swap category_id in place to a differently-named category.
				// This is the step that trips the derived-name inconsistency.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_category" "a" {
						name     = %q
						priority = 9
					}
					resource "jamfplatform_pro_category" "b" {
						name     = %q
						priority = 9
					}
					resource "jamfplatform_pro_script" "test" {
						name            = %q
						priority        = "AFTER"
						script_contents = "#!/bin/sh\necho catswap"
						category_id     = jamfplatform_pro_category.b.id
					}
				`, catA, catB, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("jamfplatform_pro_script.test", "category_id", "jamfplatform_pro_category.b", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_script.test", "category_name", catB),
				),
			},
		},
	})
}
