// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /restrictedsoftware endpoint.
// Classic has known concurrency issues when multiple writes hit the same
// resource type — keep these tests serial with any future classic acceptance
// work in this package.
//
// No pre-existing tenant target objects are required: scope happy-paths use
// all_computers = true, exclusion users are free-text local usernames, and
// general.site is left unset (defaults to "None"). This sidesteps the
// HTTP 409 "bad scope reference" invariant (a target/exclusion ID that does not
// exist is rejected) without provisioning fixtures.

package restricted_software_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const restrictedSoftwareResourceAddr = "jamfplatform_pro_restricted_software.test"

// testAccCheckRestrictedSoftwareDestroy verifies records created during the test
// were destroyed.
func testAccCheckRestrictedSoftwareDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_restricted_software" {
				continue
			}
			_, err := c.GetRestrictedSoftwareByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro restricted software %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro restricted software %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// generalOnlyConfig is the import-stable shape: only the required general
// fields. The importer populates general post-Read but leaves the optional
// scope block null, so ImportStateVerify must run against a general-only config.
func generalOnlyConfig(name, processName string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_restricted_software" "test" {
			general = {
				name         = %q
				process_name = %q
			}
		}
	`, name, processName)
}

// generalFullWithSiteConfig sets every mutable general attribute explicitly,
// including site_id sourced from a jamfplatform_pro_site fixture (a real site
// is required — an unknown site ID returns HTTP 409 "Problem with site ID").
func generalFullWithSiteConfig(name, processName, message string, restrictExact, sendEmail, kill, del bool) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_site" "test" {
			name = "%s-site"
		}

		resource "jamfplatform_pro_restricted_software" "test" {
			general = {
				name                                 = %q
				process_name                         = %q
				restrict_exact_process_name          = %t
				send_email_notification_on_violation = %t
				kill_process                         = %t
				delete_application                   = %t
				display_message                      = %q
				site_id                              = jamfplatform_pro_site.test.id
			}
		}
	`, name, name, processName, restrictExact, sendEmail, kill, del, message)
}

// scopeTargetedConfig sets a targeted scope (a department target sourced from a
// jamfplatform_pro_department fixture) plus a free-text user-exclusion set.
// A targeted apply needs a real object — this is the applied targeted-scope
// happy-path, distinct from the all_computers variant and the PlanOnly
// conflict test.
func scopeTargetedConfig(name, processName, excludedUsers string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_department" "test" {
			name = "%s-dept"
		}

		resource "jamfplatform_pro_restricted_software" "test" {
			general = {
				name         = %q
				process_name = %q
			}
			scope = {
				targets = {
					department_ids = [jamfplatform_pro_department.test.id]
				}
				exclusions = {
					directory_service_or_local_user_names = [%s]
				}
			}
		}
	`, name, name, processName, excludedUsers)
}

// TestAccResource_ProRestrictedSoftware_Basic exercises create, computed-default
// resolution, import, and an in-place general update. The mutated general bools
// and process_name verify the GET-after-Update path (classic PUT returns 201
// with an empty body).
func TestAccResource_ProRestrictedSoftware_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-restsw-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRestrictedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: generalOnlyConfig(name, "Chess.app"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(restrictedSoftwareResourceAddr, "id"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.name", name),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.process_name", "Chess.app"),
					// Server-applied defaults: match_exact_process_name=true, the rest false.
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.restrict_exact_process_name", "true"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.send_email_notification_on_violation", "false"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.kill_process", "false"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.delete_application", "false"),
					// No site assigned: site_id stays the "-1" sentinel; site_name is
					// null (absent from state) because DerivedRefName refuses to trust
					// the flaky server echo of "NONE" for the sentinel.
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.site_id", "-1"),
					resource.TestCheckNoResourceAttr(restrictedSoftwareResourceAddr, "general.site_name"),
				),
			},
			{
				ResourceName:      restrictedSoftwareResourceAddr,
				ImportState:       true,
				ImportStateVerify: true,
				// scope: import hydrates every category; apply keeps
				// declared-only (this general-only config declares none).
				ImportStateVerifyIgnore: []string{"timeouts", "scope"},
			},
			{
				// In-place update: flip every mutable general bool, change the
				// process name, set a display message, and assign a site (every
				// non-RequiresReplace general attribute is mutated here).
				//
				// restrict_exact_process_name stays true because
				// delete_application depends on it: Jamf Pro can only identify
				// the application to delete from an exact process name, and
				// silently clears the flag otherwise. This step used to pass
				// false alongside delete_application = true, which the sticky
				// read hid; it is now a plan-time error, and the pairing has its
				// own step below.
				Config: generalFullWithSiteConfig(name, "Chess2.app", "This application is restricted.", true, true, true, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.process_name", "Chess2.app"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.restrict_exact_process_name", "true"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.send_email_notification_on_violation", "true"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.kill_process", "true"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.delete_application", "true"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.display_message", "This application is restricted."),
					resource.TestCheckResourceAttrPair(restrictedSoftwareResourceAddr, "general.site_id", "jamfplatform_pro_site.test", "id"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.site_name", name+"-site"),
				),
			},
			{
				// The cross-field gate: exact match off with delete_application
				// on is refused at plan time rather than silently discarded.
				Config:      generalFullWithSiteConfig(name, "Chess2.app", "This application is restricted.", false, true, true, true),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`requires general.restrict_exact_process_name`),
			},
			{
				// Turning the exact match off is fine once delete_application
				// goes with it — which is also what Jamf Pro does on its own.
				Config: generalFullWithSiteConfig(name, "Chess2.app", "This application is restricted.", false, true, true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.restrict_exact_process_name", "false"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.delete_application", "false"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.kill_process", "true"),
				),
			},
		},
	})
}

// scopeAllComputersConfig keeps the department fixture declared (so it is not
// destroyed mid-test) but switches the record scope to all_computers with an
// explicitly cleared department target. Under granular scope ownership the
// explicit `[]` is the clear gesture — omitting department_ids instead would
// drop it from state and leave the category to the admin UI (though here the
// all_computers flag wipes the conflicting targets regardless).
func scopeAllComputersConfig(name, processName, excludedUsers string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_department" "test" {
			name = "%s-dept"
		}

		resource "jamfplatform_pro_restricted_software" "test" {
			general = {
				name         = %q
				process_name = %q
			}
			scope = {
				targets = {
					all_computers  = true
					department_ids = []
				}
				exclusions = {
					directory_service_or_local_user_names = [%s]
				}
			}
		}
	`, name, name, processName, excludedUsers)
}

// TestAccResource_ProRestrictedSoftware_ScopeUpdate exercises an applied
// targeted scope (a department target), a nested-set add+remove on the
// free-text user exclusions, and a target-removal transition to all_computers.
func TestAccResource_ProRestrictedSoftware_ScopeUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-restsw-scope-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRestrictedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: scopeTargetedConfig(name, "Solitaire.app", `"alice", "bob"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "scope.targets.department_ids.#", "1"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "scope.exclusions.directory_service_or_local_user_names.#", "2"),
					resource.TestCheckTypeSetElemAttr(restrictedSoftwareResourceAddr, "scope.exclusions.directory_service_or_local_user_names.*", "alice"),
					resource.TestCheckTypeSetElemAttr(restrictedSoftwareResourceAddr, "scope.exclusions.directory_service_or_local_user_names.*", "bob"),
				),
			},
			{
				// Nested-set churn: remove "alice", add "carol" (target unchanged).
				Config: scopeTargetedConfig(name, "Solitaire.app", `"bob", "carol"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "scope.targets.department_ids.#", "1"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "scope.exclusions.directory_service_or_local_user_names.#", "2"),
					resource.TestCheckTypeSetElemAttr(restrictedSoftwareResourceAddr, "scope.exclusions.directory_service_or_local_user_names.*", "bob"),
					resource.TestCheckTypeSetElemAttr(restrictedSoftwareResourceAddr, "scope.exclusions.directory_service_or_local_user_names.*", "carol"),
				),
			},
			{
				// Declared-clear transition: switch to all_computers and clear
				// department_ids with an explicit `[]` (under granular ownership,
				// omitting the category would leave it unmanaged and preserved
				// rather than cleared).
				Config: scopeAllComputersConfig(name, "Solitaire.app", `"bob", "carol"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "scope.targets.all_computers", "true"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "scope.targets.department_ids.#", "0"),
				),
			},
		},
	})
}

// TestAccResource_ProRestrictedSoftware_AllComputersConflict asserts the shared
// scope validator rejects all_computers = true alongside a computer target.
func TestAccResource_ProRestrictedSoftware_AllComputersConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-restsw-allc-" + suffix
	config := fmt.Sprintf(`
		resource "jamfplatform_pro_restricted_software" "test" {
			general = {
				name         = %q
				process_name = "Chess.app"
			}
			scope = {
				targets = {
					all_computers      = true
					computer_group_ids = ["1"]
				}
			}
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("Conflicts with all-flag"),
			},
		},
	})
}

// TestAccResource_ProRestrictedSoftware_ScopeExclusionsClearWithEmptySet
// verifies the declared-`[]` clear gesture under granular scope ownership: an
// explicitly empty directory_service_or_local_user_names must be emitted as an
// empty element so the scope subtree replace clears it (omitting the category
// instead would leave it unmanaged and preserved by the read-merge-write
// update). Uses free-text local usernames (the same category the ScopeUpdate
// test exercises), so no fixtures are required.
func TestAccResource_ProRestrictedSoftware_ScopeExclusionsClearWithEmptySet(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-rs-exclclear-" + suffix
	cfg := func(users string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_restricted_software" "test" {
  general = {
    name         = %q
    process_name = "Solitaire.app"
  }
  scope = {
    targets = {
      all_computers = true
    }
    exclusions = {
      directory_service_or_local_user_names = [%s]
    }
  }
}
`, name, users)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRestrictedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg(`"tf-acc-exclude-user"`),
				Check:  resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "scope.exclusions.directory_service_or_local_user_names.#", "1"),
			},
			{
				// Clear to [] — the declared empty category must be emitted as an
				// explicit empty element (omission would preserve it under granular
				// ownership). Implicit post-step empty-plan enforces the round-trip.
				Config: cfg(``),
				Check:  resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "scope.exclusions.directory_service_or_local_user_names.#", "0"),
			},
		},
	})
}

// restrictedSoftwareOmitRetainsFixtures declares the tenant objects the
// omit-retains configs scope against: one department, building and static
// computer group for the targets tab and a second of each for the exclusions
// tab, so every ID-keyed category on both tabs carries a real, distinct member.
// The Platform device groups bridge to the classic scope through jamf_pro_id.
func restrictedSoftwareOmitRetainsFixtures(suffix string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_department" "target" {
			name = "tf-acc-rs-omit-dept-target-%[1]s"
		}

		resource "jamfplatform_pro_department" "excluded" {
			name = "tf-acc-rs-omit-dept-excluded-%[1]s"
		}

		resource "jamfplatform_pro_building" "target" {
			name = "tf-acc-rs-omit-bldg-target-%[1]s"
		}

		resource "jamfplatform_pro_building" "excluded" {
			name = "tf-acc-rs-omit-bldg-excluded-%[1]s"
		}

		resource "jamfplatform_device_group" "target" {
			name        = "tf-acc-rs-omit-group-target-%[1]s"
			description = "tf-acc omit-retains scope target fixture"
			group_type  = "static"
			device_type = "computer"
		}

		resource "jamfplatform_device_group" "excluded" {
			name        = "tf-acc-rs-omit-group-excluded-%[1]s"
			description = "tf-acc omit-retains scope exclusion fixture"
			group_type  = "static"
			device_type = "computer"
		}
	`, suffix)
}

// restrictedSoftwareOmitRetainsConfig is the fully declared shape for the
// omit-retains contract: both scope tabs carry a member in every ID-keyed
// category plus a free-text excluded user, and general carries non-default
// values, so a server that stopped retaining an omitted element is caught on
// content, not on presence. Restricted software has no limitations tab.
func restrictedSoftwareOmitRetainsConfig(name, suffix string) string {
	return restrictedSoftwareOmitRetainsFixtures(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_restricted_software" "test" {
			general = {
				name            = %q
				process_name    = "Chess.app"
				kill_process    = true
				display_message = "Omit-retains contract message."
			}
			scope = {
				targets = {
					department_ids     = [jamfplatform_pro_department.target.id]
					building_ids       = [jamfplatform_pro_building.target.id]
					computer_group_ids = [jamfplatform_device_group.target.jamf_pro_id]
				}
				exclusions = {
					department_ids                        = [jamfplatform_pro_department.excluded.id]
					building_ids                          = [jamfplatform_pro_building.excluded.id]
					computer_group_ids                    = [jamfplatform_device_group.excluded.jamf_pro_id]
					directory_service_or_local_user_names = ["tf-acc-omit-retains-user"]
				}
			}
		}
	`, name)
}

// restrictedSoftwareOmitRetainsTargetsOnlyConfig keeps scope.targets but drops
// scope.exclusions and the Optional+Computed general.display_message, so the
// PUT re-emits the scope from the granular merge with the exclusions folded in
// from the live object rather than from config.
func restrictedSoftwareOmitRetainsTargetsOnlyConfig(name, suffix string) string {
	return restrictedSoftwareOmitRetainsFixtures(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_restricted_software" "test" {
			general = {
				name         = %q
				process_name = "Chess.app"
				kill_process = true
			}
			scope = {
				targets = {
					department_ids     = [jamfplatform_pro_department.target.id]
					building_ids       = [jamfplatform_pro_building.target.id]
					computer_group_ids = [jamfplatform_device_group.target.jamf_pro_id]
				}
			}
		}
	`, name)
}

// restrictedSoftwareOmitRetainsGeneralOnlyConfig drops the scope block, so the
// PUT carries <general> alone. The fixtures stay declared so the server's
// retained scope keeps pointing at live objects, and depends_on keeps the
// destroy order the dropped references no longer imply: the Platform
// device-groups API refuses to delete a group the retained scope still names
// (422 HAS_DEPENDENCIES), so the record must go before its groups.
func restrictedSoftwareOmitRetainsGeneralOnlyConfig(name, suffix string) string {
	return restrictedSoftwareOmitRetainsFixtures(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_restricted_software" "test" {
			depends_on = [jamfplatform_device_group.target, jamfplatform_device_group.excluded]

			general = {
				name         = %q
				process_name = "Chess.app"
				kill_process = true
			}
		}
	`, name)
}

// stateAttr returns one attribute of a resource in the Terraform state, so a
// wire assertion can compare against the id a fixture was actually allocated.
func stateAttr(s *terraform.State, addr, key string) (string, error) {
	rs, ok := s.RootModule().Resources[addr]
	if !ok {
		return "", fmt.Errorf("fixture %s not found in state", addr)
	}
	v, ok := rs.Primary.Attributes[key]
	if !ok || v == "" {
		return "", fmt.Errorf("fixture %s has no %s in state", addr, key)
	}
	return v, nil
}

// requireSingleID asserts a classic id/name collection holds exactly one member
// carrying the wanted id.
func requireSingleID(field, want string, items *[]proclassic.IDName) error {
	if items == nil || len(*items) != 1 {
		return fmt.Errorf("%s: want exactly one member, got %+v", field, items)
	}
	return testhelpers.RequireEqual(field+"[0].id", want, strconv.Itoa(testhelpers.Deref((*items)[0].ID)))
}

// restrictedSoftwareRetainedOnServer asserts the server's copy still carries
// every value the omit-retains config declared in its first step. The fixture
// ids are read from the Terraform state rather than assumed, so the check
// compares against what the tenant allocated.
func restrictedSoftwareRetainedOnServer(t *testing.T) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return func(s *terraform.State) error {
		ids := map[string]string{}
		for _, f := range []struct{ key, addr, attr string }{
			{"department.target", "jamfplatform_pro_department.target", "id"},
			{"department.excluded", "jamfplatform_pro_department.excluded", "id"},
			{"building.target", "jamfplatform_pro_building.target", "id"},
			{"building.excluded", "jamfplatform_pro_building.excluded", "id"},
			{"group.target", "jamfplatform_device_group.target", "jamf_pro_id"},
			{"group.excluded", "jamfplatform_device_group.excluded", "jamf_pro_id"},
		} {
			v, err := stateAttr(s, f.addr, f.attr)
			if err != nil {
				return err
			}
			ids[f.key] = v
		}
		return testhelpers.CheckLiveObject(restrictedSoftwareResourceAddr,
			func(ctx context.Context, id string) (*proclassic.RestrictedSoftware, error) {
				return c.GetRestrictedSoftwareByID(ctx, id)
			},
			func(rs *proclassic.RestrictedSoftware) error {
				if rs.General == nil {
					return fmt.Errorf("general: absent")
				}
				if err := testhelpers.RequireEqual("general.kill_process", true, testhelpers.Deref(rs.General.KillProcess)); err != nil {
					return err
				}
				if err := testhelpers.RequireEqual("general.display_message", "Omit-retains contract message.", testhelpers.Deref(rs.General.DisplayMessage)); err != nil {
					return err
				}
				if rs.Scope == nil {
					return fmt.Errorf("scope: absent")
				}
				sc := rs.Scope
				if err := testhelpers.RequireEqual("scope.all_computers", false, testhelpers.Deref(sc.AllComputers)); err != nil {
					return err
				}
				if sc.Departments == nil || sc.Buildings == nil || sc.ComputerGroups == nil {
					return fmt.Errorf("scope.targets: a category is absent: %+v", sc)
				}
				if err := requireSingleID("scope.targets.departments", ids["department.target"], sc.Departments.Department); err != nil {
					return err
				}
				if err := requireSingleID("scope.targets.buildings", ids["building.target"], sc.Buildings.Building); err != nil {
					return err
				}
				if err := requireSingleID("scope.targets.computer_groups", ids["group.target"], sc.ComputerGroups.ComputerGroup); err != nil {
					return err
				}
				x := sc.Exclusions
				if x == nil || x.Departments == nil || x.Buildings == nil || x.ComputerGroups == nil || x.Users == nil {
					return fmt.Errorf("scope.exclusions: a category is absent: %+v", x)
				}
				if err := requireSingleID("scope.exclusions.departments", ids["department.excluded"], x.Departments.Department); err != nil {
					return err
				}
				if err := requireSingleID("scope.exclusions.buildings", ids["building.excluded"], x.Buildings.Building); err != nil {
					return err
				}
				if err := requireSingleID("scope.exclusions.computer_groups", ids["group.excluded"], x.ComputerGroups.ComputerGroup); err != nil {
					return err
				}
				if x.Users.User == nil || len(*x.Users.User) != 1 {
					return fmt.Errorf("scope.exclusions.users: want exactly one user, got %+v", x.Users)
				}
				return testhelpers.RequireEqual("scope.exclusions.users[0].name", "tf-acc-omit-retains-user", testhelpers.Deref((*x.Users.User)[0].Name))
			})(s)
	}
}

// TestAccResource_ProRestrictedSoftware_OmittedBlocksRetained pins the
// omit-retains contract the plan output cannot show: dropping scope.exclusions,
// then the whole scope, from config plans them as removed, but the classic PUT
// omits the elements and the server keeps every value. Step 2 keeps
// scope.targets so the scope goes through the granular read-merge-write, which
// must fold the live exclusions back in rather than emitting them empty; it
// also drops the Optional+Computed general.display_message. Step 3 drops scope
// too so the PUT carries <general> alone. Each step's implicit post-apply plan
// must be empty, which is what makes the contract usable. If this test fails
// on content, the endpoint no longer merges and nothing that suppresses the
// removal plan may ship for this resource.
func TestAccResource_ProRestrictedSoftware_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-rs-omit-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRestrictedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: restrictedSoftwareOmitRetainsConfig(name, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "scope.exclusions.directory_service_or_local_user_names.#", "1"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.display_message", "Omit-retains contract message."),
					restrictedSoftwareRetainedOnServer(t),
				),
			},
			{
				Config: restrictedSoftwareOmitRetainsTargetsOnlyConfig(name, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(restrictedSoftwareResourceAddr, "scope.exclusions.directory_service_or_local_user_names.#"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "scope.targets.department_ids.#", "1"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.display_message", "Omit-retains contract message."),
					restrictedSoftwareRetainedOnServer(t),
				),
			},
			{
				Config: restrictedSoftwareOmitRetainsGeneralOnlyConfig(name, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(restrictedSoftwareResourceAddr, "scope.targets.department_ids.#"),
					restrictedSoftwareRetainedOnServer(t),
				),
			},
		},
	})
}
