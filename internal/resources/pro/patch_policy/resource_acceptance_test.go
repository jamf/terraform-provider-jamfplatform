// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /patchpolicies endpoint.
// Classic has known concurrency issues when multiple writes hit the same
// resource type — keep these tests serial with any future classic acceptance
// work in this package.
//
// FIXTURE CHAIN (mandatory): a patch policy can only target a version that has a
// package assigned on its title. Each test therefore stands up:
//   jamfplatform_pro_package          (metadata-only)
//     → jamfplatform_pro_patch_software_title (name_id "285"/source_id 1, with
//        version_packages = { "8.33.2.2" = <package id> })
//        → jamfplatform_pro_patch_policy (software_title_configuration_id =
//           <title id>, target_version = "8.33.2.2").
//
// The fixtures use the real "8x8 Work" patch catalog entry: name_id "285",
// source_id 1, with version "8.33.2.2". If the catalog entry or that version is
// removed from the test tenant's patch source, these tests will need updating.

package patch_policy_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
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

const (
	accTitleNameID   = "285" // 8x8 Work
	accTitleSourceID = 1
	accTitleVersion  = "8.33.2.2"
	patchPolicyType  = "jamfplatform_pro_patch_policy"
)

// testAccCheckPatchPolicyDestroy verifies patch policies created during the test
// were destroyed.
func testAccCheckPatchPolicyDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != patchPolicyType {
				continue
			}
			_, err := c.GetPatchPolicyByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro patch policy %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro patch policy %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// fixtureTitle returns the package + patch software title fixture HCL shared by
// every step. The title assigns package A to the target version so the policy
// can target it.
func fixtureTitle(suffix string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_package" "pkg_a" {
			display_name = "tf-acc-pp-pkgA-%[1]s"
			file_name    = "tf-acc-pp-pkgA-%[1]s.pkg"
		}

		resource "jamfplatform_pro_patch_software_title" "title" {
			name      = "tf-acc 8x8 Work pp %[1]s"
			name_id   = %[2]q
			source_id = %[3]d

			version_packages = {
				%[4]q = jamfplatform_pro_package.pkg_a.id
			}
		}
	`, suffix, accTitleNameID, accTitleSourceID, accTitleVersion)
}

// TestAccResource_ProPatchPolicy_Basic exercises create, then a multi-attribute
// in-place update mutating every non-RequiresReplace writable attribute:
// enabled, distribution_method (selfservice→prompt), allow_downgrade,
// patch_unknown, scope (add a computer_group / building, then clear the
// building with a declared `[]`), and several user_interaction fields
// (reminders / deadline / grace). It asserts the computed server-derived
// fields populate throughout. Finally import with justified ignores.
func TestAccResource_ProPatchPolicy_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	bld := "tf-acc-pp-bld-" + suffix
	grp := "tf-acc-pp-grp-" + suffix
	// computer_group_ids uses a self-minted smart computer group via
	// jamfplatform_device_group (whose jamf_pro_id is the classic group ID the
	// scope wants). The title fixture sets no site, so a siteless Full-JSS smart
	// group is accepted and the policy can run enabled=true — mirrors the
	// app_installer acceptance pattern. The building is a real fixture.

	// Step 1: create as Self Service with a minimal user_interaction block.
	stepCreate := fixtureTitle(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_patch_policy" "test" {
			software_title_configuration_id = jamfplatform_pro_patch_software_title.title.id
			name                            = "tf-acc-pp-%[1]s"
			target_version                  = %[2]q
			enabled                         = false
			distribution_method             = "selfservice"
			allow_downgrade                 = false
			patch_unknown                   = false

			user_interaction = {
				install_button_text = "Update"

				deadlines = {
					enabled = true
					period  = 7
				}
			}
		}
	`, suffix, accTitleVersion)

	// Step 2: flip every writable general bool/enum, add scope (the default smart
	// group + a building), and mutate user_interaction reminders/deadline/grace.
	stepUpdate := fixtureTitle(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_building" "bld" {
			name = %[3]q
		}

		resource "jamfplatform_device_group" "grp" {
			name        = %[4]q
			group_type  = "smart"
			device_type = "computer"
			description = "Patch policy acceptance fixture"
			criteria = [
				{ criteria = "Operating System Version", operator = "greater than or equal", value = "10.0" },
			]
		}

		resource "jamfplatform_pro_patch_policy" "test" {
			software_title_configuration_id = jamfplatform_pro_patch_software_title.title.id
			name                            = "tf-acc-pp-%[1]s"
			target_version                  = %[2]q
			enabled                         = true
			distribution_method             = "prompt"
			allow_downgrade                 = true
			patch_unknown                   = true

			scope = {
				targets = {
					computer_group_ids = [jamfplatform_device_group.grp.jamf_pro_id]
					building_ids       = [jamfplatform_pro_building.bld.id]
				}
			}

			user_interaction = {
				install_button_text      = "Install Now"
				self_service_description = "Updated description"

				notifications = {
					enabled = true
					subject = "Update available"
					reminders = {
						enabled   = true
						frequency = 12
					}
				}

				deadlines = {
					enabled = true
					period  = 3
				}

				grace_period = {
					duration                    = 30
					notification_center_subject = "Heads up"
				}
			}
		}
	`, suffix, accTitleVersion, bld, grp)

	// Step 3: clear the building target with an explicit `[]` (keep the group)
	// to exercise the declared-clear path. Under granular scope ownership,
	// omitting building_ids instead would drop it from state and leave the
	// category to the admin UI (preserved by the read-merge-write update), not
	// clear it.
	stepScopeShrink := fixtureTitle(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_building" "bld" {
			name = %[3]q
		}

		resource "jamfplatform_device_group" "grp" {
			name        = %[4]q
			group_type  = "smart"
			device_type = "computer"
			description = "Patch policy acceptance fixture"
			criteria = [
				{ criteria = "Operating System Version", operator = "greater than or equal", value = "10.0" },
			]
		}

		resource "jamfplatform_pro_patch_policy" "test" {
			software_title_configuration_id = jamfplatform_pro_patch_software_title.title.id
			name                            = "tf-acc-pp-%[1]s"
			target_version                  = %[2]q
			enabled                         = true
			distribution_method             = "prompt"
			allow_downgrade                 = true
			patch_unknown                   = true

			scope = {
				targets = {
					computer_group_ids = [jamfplatform_device_group.grp.jamf_pro_id]
					building_ids       = []
				}
			}

			user_interaction = {
				install_button_text = "Install Now"

				deadlines = {
					enabled = true
					period  = 3
				}
			}
		}
	`, suffix, accTitleVersion, bld, grp)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: stepCreate,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(patchPolicyType+".test", "id"),
					resource.TestCheckResourceAttrPair(patchPolicyType+".test", "software_title_configuration_id", "jamfplatform_pro_patch_software_title.title", "id"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "target_version", accTitleVersion),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "distribution_method", "selfservice"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "allow_downgrade", "false"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "patch_unknown", "false"),
					// Server-derived fields populate from the patch definition.
					resource.TestCheckResourceAttrSet(patchPolicyType+".test", "release_date"),
					resource.TestCheckResourceAttrSet(patchPolicyType+".test", "incremental_update"),
					resource.TestCheckResourceAttrSet(patchPolicyType+".test", "reboot"),
					resource.TestCheckResourceAttrSet(patchPolicyType+".test", "minimum_os"),
					resource.TestCheckResourceAttrSet(patchPolicyType+".test", "kill_apps.#"),
				),
			},
			{
				Config: stepUpdate,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchPolicyType+".test", "enabled", "true"),
					resource.TestCheckResourceAttrPair(patchPolicyType+".test", "scope.targets.computer_group_ids.0", "jamfplatform_device_group.grp", "jamf_pro_id"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "distribution_method", "prompt"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "allow_downgrade", "true"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "patch_unknown", "true"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "scope.targets.computer_group_ids.#", "1"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "scope.targets.building_ids.#", "1"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "user_interaction.install_button_text", "Install Now"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "user_interaction.notifications.reminders.frequency", "12"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "user_interaction.deadlines.period", "3"),
					resource.TestCheckResourceAttr(patchPolicyType+".test", "user_interaction.grace_period.duration", "30"),
					// Server-derived still present.
					resource.TestCheckResourceAttrSet(patchPolicyType+".test", "kill_apps.#"),
				),
			},
			{
				Config: stepScopeShrink,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchPolicyType+".test", "scope.targets.computer_group_ids.#", "1"),
					// The declared `[]` clear round-trips as an empty set — the
					// canonical "no members" value for a managed category.
					resource.TestCheckResourceAttr(patchPolicyType+".test", "scope.targets.building_ids.#", "0"),
				),
			},
			{
				ResourceName:      patchPolicyType + ".test",
				ImportState:       true,
				ImportStateVerify: true,
				// timeouts: framework-only, never round-trips.
				// scope: import hydrates every category; apply keeps
				// declared-only, so the hydrated import state legitimately
				// differs from this subset-scope config.
				// user_interaction: Optional state-gated block — import hydrates
				// it fully while this config declares a subset.
				// software_title_configuration_id: GET does NOT echo it (it is a
				// create-time-only path parameter, wire-probed); it cannot be
				// reconstructed on import.
				ImportStateVerifyIgnore: []string{"timeouts", "scope", "user_interaction", "software_title_configuration_id"},
			},
		},
	})
}

// TestAccResource_ProPatchPolicy_InvalidDistributionMethod asserts the
// distribution_method OneOf validator rejects an unknown value at plan time. The
// error detail wraps at ~80 cols; the regex anchors on the no-space token
// "selfservice" to avoid a whitespace wrap point.
func TestAccResource_ProPatchPolicy_InvalidDistributionMethod(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fixtureTitle(suffix) + fmt.Sprintf(`
					resource "jamfplatform_pro_patch_policy" "test" {
						software_title_configuration_id = jamfplatform_pro_patch_software_title.title.id
						name                            = "tf-acc-pp-invalid-%[1]s"
						target_version                  = %[2]q
						distribution_method             = "notarealmethod"
					}
				`, suffix, accTitleVersion),
				ExpectError: regexp.MustCompile(`selfservice`),
			},
		},
	})
}

// TestAccDataSource_ProPatchPolicy_ByID looks up a freshly-created patch policy
// by ID.
func TestAccDataSource_ProPatchPolicy_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fixtureTitle(suffix) + fmt.Sprintf(`
					resource "jamfplatform_pro_patch_policy" "src" {
						software_title_configuration_id = jamfplatform_pro_patch_software_title.title.id
						name                            = "tf-acc-pp-ds-%[1]s"
						target_version                  = %[2]q
					}

					data "jamfplatform_pro_patch_policy" "lookup" {
						id = jamfplatform_pro_patch_policy.src.id
					}
				`, suffix, accTitleVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_patch_policy.lookup", "name", "jamfplatform_pro_patch_policy.src", "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_patch_policy.lookup", "target_version", accTitleVersion),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_patch_policy.lookup", "kill_apps.#"),
				),
			},
		},
	})
}

// TestAccListResource_ProPatchPolicy_Basic exercises the list resource via the
// `terraform query` workflow. The Pro v2 collection surfaces the policy display
// name, so DisplayName / the filter match on the name.
//
// It is also the only check on the assumption the enumeration rests on: that a
// Pro v2 patch policy id addresses the same policy on the ProClassic by-id path.
// `include_resource = true` hydrates each result through the classic read using
// the v2 id, and a mismatch drops the policy from the result set entirely rather
// than returning it with null attributes, so it is the ExpectResourceKnownValues
// lookup itself — finding no result under the expected display name — that fails
// the step. Both assertions below read the hydrated resource rather than the
// enumeration, so either one catches that; target_version is asserted alongside
// the name to cover flattenGeneral's mapping of a classic-only field, not
// because it pins the id equality independently.
func TestAccListResource_ProPatchPolicy_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pp-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fixtureTitle(suffix) + fmt.Sprintf(`
					resource "jamfplatform_pro_patch_policy" "src" {
						software_title_configuration_id = jamfplatform_pro_patch_software_title.title.id
						name                            = %[1]q
						target_version                  = %[2]q
					}
				`, name, accTitleVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_patch_policy.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_patch_policy" "test" {
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
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_patch_policy.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("target_version"), KnownValue: knownvalue.StringExact(accTitleVersion)},
						},
					),
				},
			},
		},
	})
}

// patchPolicyOmitRetainsFixtures returns the title chain plus every sibling
// object the omit-retains steps reference: a smart computer group and a
// building for the targets, a second building and a department for the
// exclusions, and a network segment and an iBeacon that are both limited and
// excluded.
func patchPolicyOmitRetainsFixtures(suffix string) string {
	return fixtureTitle(suffix) + fmt.Sprintf(`
		resource "jamfplatform_device_group" "grp" {
			name        = "tf-acc-pp-omit-grp-%[1]s"
			group_type  = "smart"
			device_type = "computer"
			description = "Patch policy omit-retains fixture"
			criteria = [
				{ criteria = "Operating System Version", operator = "greater than or equal", value = "10.0" },
			]
		}

		resource "jamfplatform_pro_building" "b1" {
			name = "tf-acc-pp-omit-bldg1-%[1]s"
		}

		resource "jamfplatform_pro_building" "b2" {
			name = "tf-acc-pp-omit-bldg2-%[1]s"
		}

		resource "jamfplatform_pro_department" "d1" {
			name = "tf-acc-pp-omit-dept-%[1]s"
		}

		resource "jamfplatform_pro_network_segment" "n1" {
			name             = "tf-acc-pp-omit-ns-%[1]s"
			starting_address = "10.214.0.1"
			ending_address   = "10.214.0.254"
		}

		resource "jamfplatform_pro_ibeacon" "i1" {
			name  = "tf-acc-pp-omit-ibeacon-%[1]s"
			uuid  = "5a3f7a1e-6c2d-4b8e-9f10-2d3c4b5a6e7f"
			major = 7
			minor = 11
		}
	`, suffix)
}

// patchPolicyOmitRetainsConfig is the fully declared shape for the omit-retains
// contract: every state-gated block the wire can show — scope targets /
// limitations / exclusions and user_interaction with its deadlines and
// grace_period — carries a value distinct from the server default so that a
// server which stopped retaining an omitted element is caught on content, not
// on presence. user_interaction.notifications (and its nested reminders) is
// deliberately absent: the classic GET never returns a <notifications>
// element, neither after a create that carried one nor after a PUT that did
// (wire-observed 2026-09-06 on this tenant, both distribution methods), so
// no value declared there can be verified against the server and the block
// stays out rather than pretend the test covers it.
func patchPolicyOmitRetainsConfig(suffix, name string) string {
	return patchPolicyOmitRetainsFixtures(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_patch_policy" "test" {
			software_title_configuration_id = jamfplatform_pro_patch_software_title.title.id
			name                            = %[1]q
			target_version                  = %[2]q
			enabled                         = false
			distribution_method             = "selfservice"

			scope = {
				targets = {
					computer_group_ids = [jamfplatform_device_group.grp.jamf_pro_id]
					building_ids       = [jamfplatform_pro_building.b1.id]
				}
				limitations = {
					network_segment_ids = [jamfplatform_pro_network_segment.n1.id]
					ibeacon_ids         = [jamfplatform_pro_ibeacon.i1.id]
				}
				exclusions = {
					building_ids        = [jamfplatform_pro_building.b2.id]
					department_ids      = [jamfplatform_pro_department.d1.id]
					network_segment_ids = [jamfplatform_pro_network_segment.n1.id]
					ibeacon_ids         = [jamfplatform_pro_ibeacon.i1.id]
				}
			}

			user_interaction = {
				install_button_text      = "Retain me"
				self_service_description = "Omit-retains contract description."

				deadlines = {
					enabled = true
					period  = 5
				}

				grace_period = {
					duration                    = 45
					notification_center_subject = "Retained grace subject"
					message                     = "Retained grace message"
				}
			}
		}
	`, name, accTitleVersion)
}

// patchPolicyOmitRetainsParentsOnlyConfig keeps the two blocks that have gated
// children but drops the children: scope keeps its targets and loses
// limitations and exclusions (so the scope goes through the granular merge),
// user_interaction keeps install_button_text and loses deadlines,
// grace_period and the Optional+Computed self_service_description leaf.
func patchPolicyOmitRetainsParentsOnlyConfig(suffix, name string) string {
	return patchPolicyOmitRetainsFixtures(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_patch_policy" "test" {
			software_title_configuration_id = jamfplatform_pro_patch_software_title.title.id
			name                            = %[1]q
			target_version                  = %[2]q
			enabled                         = false
			distribution_method             = "selfservice"

			scope = {
				targets = {
					computer_group_ids = [jamfplatform_device_group.grp.jamf_pro_id]
					building_ids       = [jamfplatform_pro_building.b1.id]
				}
			}

			user_interaction = {
				install_button_text = "Retain me"
			}
		}
	`, name, accTitleVersion)
}

// patchPolicyOmitRetainsGeneralOnlyConfig drops every optional block, so the
// PUT carries <general> alone. The fixtures stay so the server-side references
// they back remain valid while the policy still holds them.
func patchPolicyOmitRetainsGeneralOnlyConfig(suffix, name string) string {
	return patchPolicyOmitRetainsFixtures(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_patch_policy" "test" {
			software_title_configuration_id = jamfplatform_pro_patch_software_title.title.id
			name                            = %[1]q
			target_version                  = %[2]q
			enabled                         = false
			distribution_method             = "selfservice"
		}
	`, name, accTitleVersion)
}

// patchPolicyStateInt reads an integer attribute of a sibling fixture out of
// Terraform state so the wire assertion can compare scope members by value
// rather than count.
func patchPolicyStateInt(s *terraform.State, addr, attr string) (int, error) {
	rs, ok := s.RootModule().Resources[addr]
	if !ok {
		return 0, fmt.Errorf("fixture %s not found in state", addr)
	}
	raw, ok := rs.Primary.Attributes[attr]
	if !ok {
		return 0, fmt.Errorf("fixture %s: attribute %s not in state", addr, attr)
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("fixture %s: %s %q is not an integer: %w", addr, attr, raw, err)
	}
	return id, nil
}

// requireOneIDName asserts an id/name scope category holds exactly the one
// member with the given id.
func requireOneIDName(field string, items *[]proclassic.IDName, wantID int) error {
	if items == nil || len(*items) != 1 {
		return fmt.Errorf("%s: want exactly one member, got %+v", field, items)
	}
	return testhelpers.RequireEqual(field+"[0].id", wantID, testhelpers.Deref((*items)[0].ID))
}

// patchPolicyRetainedOnServer asserts the server's copy still carries every
// value the omit-retains config declared in its first step.
func patchPolicyRetainedOnServer(t *testing.T) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return func(s *terraform.State) error {
		grp, err := patchPolicyStateInt(s, "jamfplatform_device_group.grp", "jamf_pro_id")
		if err != nil {
			return err
		}
		b1, err := patchPolicyStateInt(s, "jamfplatform_pro_building.b1", "id")
		if err != nil {
			return err
		}
		b2, err := patchPolicyStateInt(s, "jamfplatform_pro_building.b2", "id")
		if err != nil {
			return err
		}
		d1, err := patchPolicyStateInt(s, "jamfplatform_pro_department.d1", "id")
		if err != nil {
			return err
		}
		n1, err := patchPolicyStateInt(s, "jamfplatform_pro_network_segment.n1", "id")
		if err != nil {
			return err
		}
		i1, err := patchPolicyStateInt(s, "jamfplatform_pro_ibeacon.i1", "id")
		if err != nil {
			return err
		}
		return testhelpers.CheckLiveObject(patchPolicyType+".test",
			func(ctx context.Context, id string) (*proclassic.PatchPolicy, error) {
				return c.GetPatchPolicyByID(ctx, id)
			},
			func(p *proclassic.PatchPolicy) error {
				sc := p.Scope
				if sc == nil {
					return fmt.Errorf("scope: absent")
				}
				if sc.ComputerGroups == nil {
					return fmt.Errorf("scope.targets.computer_groups: absent")
				}
				if err := requireOneIDName("scope.targets.computer_groups", sc.ComputerGroups.ComputerGroup, grp); err != nil {
					return err
				}
				if sc.Buildings == nil {
					return fmt.Errorf("scope.targets.buildings: absent")
				}
				if err := requireOneIDName("scope.targets.buildings", sc.Buildings.Building, b1); err != nil {
					return err
				}
				if sc.Limitations == nil {
					return fmt.Errorf("scope.limitations: absent")
				}
				if sc.Limitations.NetworkSegments == nil {
					return fmt.Errorf("scope.limitations.network_segments: absent")
				}
				if err := requireOneIDName("scope.limitations.network_segments", sc.Limitations.NetworkSegments.NetworkSegment, n1); err != nil {
					return err
				}
				if sc.Limitations.Ibeacons == nil {
					return fmt.Errorf("scope.limitations.ibeacons: absent")
				}
				if err := requireOneIDName("scope.limitations.ibeacons", sc.Limitations.Ibeacons.Ibeacon, i1); err != nil {
					return err
				}
				if sc.Exclusions == nil {
					return fmt.Errorf("scope.exclusions: absent")
				}
				if sc.Exclusions.Buildings == nil {
					return fmt.Errorf("scope.exclusions.buildings: absent")
				}
				if err := requireOneIDName("scope.exclusions.buildings", sc.Exclusions.Buildings.Building, b2); err != nil {
					return err
				}
				if sc.Exclusions.Departments == nil {
					return fmt.Errorf("scope.exclusions.departments: absent")
				}
				if err := requireOneIDName("scope.exclusions.departments", sc.Exclusions.Departments.Department, d1); err != nil {
					return err
				}
				if sc.Exclusions.NetworkSegments == nil {
					return fmt.Errorf("scope.exclusions.network_segments: absent")
				}
				if err := requireOneIDName("scope.exclusions.network_segments", sc.Exclusions.NetworkSegments.NetworkSegment, n1); err != nil {
					return err
				}
				if sc.Exclusions.Ibeacons == nil {
					return fmt.Errorf("scope.exclusions.ibeacons: absent")
				}
				if err := requireOneIDName("scope.exclusions.ibeacons", sc.Exclusions.Ibeacons.Ibeacon, i1); err != nil {
					return err
				}

				ui := p.UserInteraction
				if ui == nil {
					return fmt.Errorf("user_interaction: absent")
				}
				if err := testhelpers.RequireEqual("user_interaction.install_button_text", "Retain me", testhelpers.Deref(ui.InstallButtonText)); err != nil {
					return err
				}
				if err := testhelpers.RequireEqual("user_interaction.self_service_description", "Omit-retains contract description.", testhelpers.Deref(ui.SelfServiceDescription)); err != nil {
					return err
				}
				if ui.Deadlines == nil {
					return fmt.Errorf("user_interaction.deadlines: absent")
				}
				if err := testhelpers.RequireEqual("user_interaction.deadlines.enabled", true, testhelpers.Deref(ui.Deadlines.DeadlineEnabled)); err != nil {
					return err
				}
				if err := testhelpers.RequireEqual("user_interaction.deadlines.period", 5, testhelpers.Deref(ui.Deadlines.DeadlinePeriod)); err != nil {
					return err
				}
				if ui.GracePeriod == nil {
					return fmt.Errorf("user_interaction.grace_period: absent")
				}
				if err := testhelpers.RequireEqual("user_interaction.grace_period.duration", 45, testhelpers.Deref(ui.GracePeriod.GracePeriodDuration)); err != nil {
					return err
				}
				if err := testhelpers.RequireEqual("user_interaction.grace_period.notification_center_subject", "Retained grace subject", testhelpers.Deref(ui.GracePeriod.NotificationCenterSubject)); err != nil {
					return err
				}
				return testhelpers.RequireEqual("user_interaction.grace_period.message", "Retained grace message", testhelpers.Deref(ui.GracePeriod.Message))
			})(s)
	}
}

// TestAccResource_ProPatchPolicy_OmittedBlocksRetained pins the omit-retains
// contract the plan output cannot show: dropping scope limitations and
// exclusions, and the user_interaction deadlines / grace_period sub-blocks,
// from config plans them as removed, but the classic
// PUT omits the elements and the server keeps every value. Step 2 keeps
// scope.targets and a one-field user_interaction so the scope goes through
// the granular merge and the user_interaction PUT omits its children; step 3
// drops both blocks so the PUT carries <general> alone. Each step's implicit
// post-apply plan must be empty, which is what makes the contract usable. If
// this test fails on content, the endpoint no longer merges and nothing that
// suppresses the removal plan may ship for this resource.
func TestAccResource_ProPatchPolicy_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pp-omit-" + suffix
	addr := patchPolicyType + ".test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: patchPolicyOmitRetainsConfig(suffix, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "user_interaction.install_button_text", "Retain me"),
					resource.TestCheckResourceAttr(addr, "user_interaction.grace_period.duration", "45"),
					resource.TestCheckResourceAttr(addr, "scope.exclusions.ibeacon_ids.#", "1"),
					patchPolicyRetainedOnServer(t),
				),
			},
			{
				Config: patchPolicyOmitRetainsParentsOnlyConfig(suffix, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "scope.targets.building_ids.#", "1"),
					resource.TestCheckNoResourceAttr(addr, "scope.limitations.ibeacon_ids.#"),
					resource.TestCheckNoResourceAttr(addr, "scope.exclusions.building_ids.#"),
					resource.TestCheckResourceAttr(addr, "user_interaction.install_button_text", "Retain me"),
					resource.TestCheckNoResourceAttr(addr, "user_interaction.deadlines.period"),
					resource.TestCheckNoResourceAttr(addr, "user_interaction.grace_period.duration"),
					patchPolicyRetainedOnServer(t),
				),
			},
			{
				Config: patchPolicyOmitRetainsGeneralOnlyConfig(suffix, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(addr, "scope.targets.building_ids.#"),
					resource.TestCheckNoResourceAttr(addr, "user_interaction.install_button_text"),
					patchPolicyRetainedOnServer(t),
				),
			},
		},
	})
}
