// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /policies endpoint. Classic
// has known concurrency issues when multiple writes hit the same resource
// type — keep these tests serial with any other classic acceptance work.

package policy_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// createDummyComputer creates a minimal classic computer record via the SDK
// and returns its server-assigned ID as a string. Cleanup runs at test end
// via t.Cleanup.
//
// The classic POST endpoint at /computers/id/0 ignores the supplied id of 0
// and assigns a fresh ID; the SDK signature returns only error, so we re-read
// by name to discover the assigned ID.
func createDummyComputer(t *testing.T, name string) string {
	t.Helper()
	c := testhelpers.NewProClassicClient(t)
	ctx := context.Background()
	if err := c.CreateComputerByID(ctx, "0", &proclassic.ComputerPost{
		General: &proclassic.ComputerPostGeneral{Name: &name},
	}); err != nil {
		t.Fatalf("CreateComputerByID(%q): %v", name, err)
	}
	id := testhelpers.ResolveComputerIDByName(ctx, t, name)
	t.Cleanup(func() {
		if err := c.DeleteComputerByID(context.Background(), id); err != nil && !helpers.IsNotFoundError(err) {
			t.Logf("cleanup DeleteComputerByID(%s): %v", id, err)
		}
	})
	return id
}

// createDummyUser creates a minimal classic user record via the SDK and
// returns its server-assigned ID as a string. Cleanup runs at test end via
// t.Cleanup.
func createDummyUser(t *testing.T, name string) string {
	t.Helper()
	c := testhelpers.NewProClassicClient(t)
	ctx := context.Background()
	got, err := c.CreateUserByID(ctx, "0", &proclassic.UserPost{Name: &name})
	if err != nil || got == nil || got.ID == nil {
		t.Fatalf("CreateUserByID(%q): %v", name, err)
	}
	id := fmt.Sprintf("%d", *got.ID)
	t.Cleanup(func() {
		if err := c.DeleteUserByID(context.Background(), id); err != nil && !helpers.IsNotFoundError(err) {
			t.Logf("cleanup DeleteUserByID(%s): %v", id, err)
		}
	})
	return id
}

// packageFixturePath resolves the shared jamf-cli .pkg fixture committed under
// internal/resources/pro/package/test_fixtures/. The path is computed
// relative to this test file so the result is invariant to the working
// directory `go test` was invoked from.
func packageFixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("could not resolve caller path for fixture lookup")
	}
	dir := filepath.Dir(file)
	abs, err := filepath.Abs(filepath.Join(dir, "..", "package", "test_fixtures", name))
	if err != nil {
		t.Fatalf("resolving fixture path %q: %v", name, err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("fixture %q not present at %q: %v", name, abs, err)
	}
	return abs
}

// testAccCheckPolicyDestroy verifies policies created during the test were
// destroyed.
func testAccCheckPolicyDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_policy" {
				continue
			}
			_, err := c.GetPolicyByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro policy %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro policy %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func policyConfigMinimal(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name    = %q
    enabled = true
  }
}
`, name)
}

func policyConfigEnabledFlip(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name    = %q
    enabled = false
  }
}
`, name)
}

func policyConfigSelfService(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name    = %q
    enabled = true
  }
  self_service = {
    use_for_self_service      = true
    self_service_display_name = %q
    display_notifications      = true
    notification_location       = "Self Service"
    notification_subject      = "tf-acc"
    notification_message      = "Test policy"
  }
}
`, name, name)
}

// policyConfigSelfServiceCategories builds a policy in TWO Self Service
// categories with distinct per-entry flags (firstFeatureIn controls the
// first category's feature_in; the second is always feature_in=false). Two
// category fixtures supply the referenced IDs. This is the lossy case the
// pre-migration [0]-only read silently collapsed to a single category.
func policyConfigSelfServiceCategories(name string, firstFeatureIn bool) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_category" "a" {
  name     = %q
  priority = 9
}

resource "jamfplatform_pro_category" "b" {
  name     = %q
  priority = 9
}

resource "jamfplatform_pro_policy" "test" {
  general = {
    name    = %q
    enabled = true
  }
  self_service = {
    use_for_self_service      = true
    self_service_display_name = %q
    categories = [
      {
        id         = jamfplatform_pro_category.a.id
        display_in = true
        feature_in = %t
      },
      {
        id         = jamfplatform_pro_category.b.id
        display_in = true
        feature_in = false
      },
    ]
  }
}
`, name+"-a", name+"-b", name, name, firstFeatureIn)
}

func policyConfigAllComputers(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  scope = {
    targets = {
      all_computers = true
    }
  }
}
`, name)
}

// TestAccPolicyResource_ScopeTargetsNullToPresentTransition is the load-bearing
// regression test for the `targets` nesting under granular per-category scope
// ownership.
//
// Step 1 declares `scope` with only `exclusions`, so the `targets` block is
// absent (unmanaged) and must stay null in state. Step 2 adds
// `targets { all_jss_users = true }` while leaving `all_computers` undeclared —
// the declared flag is owned by Terraform and lands true, while the undeclared
// `all_computers` stays null (owned outside Terraform; the read-side gate must
// not adopt the echoed value). A green step 2 plus the null assertion IS the
// regression coverage.
func TestAccPolicyResource_ScopeTargetsNullToPresentTransition(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-targets-transition-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				// scope present, targets ABSENT → null prior state.
				Config: policyConfigScopeExclusionsOnly(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets"),
						knownvalue.Null(),
					),
				},
			},
			{
				// targets goes null→present; all_computers stays undeclared
				// (unmanaged) and must remain null in state.
				Config: policyConfigScopeExclusionsPlusTargets(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("all_jss_users"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("all_computers"),
						knownvalue.Null(),
					),
				},
			},
		},
	})
}

func policyConfigScopeExclusionsOnly(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  scope = {
    exclusions = {
      directory_service_or_local_user_names = ["tf-acc-excluded-user"]
    }
  }
}
`, name)
}

func policyConfigScopeExclusionsPlusTargets(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  scope = {
    targets = {
      all_jss_users = true
    }
    exclusions = {
      directory_service_or_local_user_names = ["tf-acc-excluded-user"]
    }
  }
}
`, name)
}

// TestAccPolicyResource_Minimal covers the smallest viable policy: name only.
// Verifies Create/Read/Update/Delete + import.
func TestAccPolicyResource_Minimal(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-min-" + suffix
	renamed := name + "-renamed"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigMinimal(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("name"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("enabled"),
						knownvalue.Bool(true),
					),
				},
			},
			{
				Config: policyConfigMinimal(renamed),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("name"),
						knownvalue.StringExact(renamed),
					),
				},
			},
			{
				Config: policyConfigEnabledFlip(renamed),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("enabled"),
						knownvalue.Bool(false),
					),
				},
			},
			{
				// ImportStateVerify: false — policy has ~15 Optional
				// state-gated top-level blocks (scope, self_service,
				// packages, scripts, printers, dock_items, restart_options,
				// maintenance, files_and_processes, user_interaction,
				// disk_encryption, local_accounts, directory_bindings,
				// management_account, efi_password) plus general sub-blocks
				// (date_time_limitations, network_limitations,
				// override_default_settings) — this minimal config declares
				// none of them. Import correctly hydrates them from the
				// server's echoed defaults (see the import-hydration fix),
				// which legitimately differs from this config's nulls. An
				// ImportStateVerifyIgnore list covering all of them would be
				// both unwieldy and fragile against future schema changes;
				// the plain import (still exercised below) is the
				// established pattern for this resource — see
				// TestAccResource_MacOSConfigurationProfile_ImportState for
				// precedent.
				ResourceName:                         "jamfplatform_pro_policy.test",
				ImportState:                          true,
				ImportStateVerify:                    false,
				ImportStateVerifyIdentifierAttribute: "id",
			},
		},
	})
}

// TestAccPolicyResource_SelfServiceNotificationSplit confirms the two
// <notification> elements round-trip cleanly through the schema's split
// display_notifications + notification_location attributes.
func TestAccPolicyResource_SelfServiceNotificationSplit(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-ss-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigSelfService(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("self_service").AtMapKey("display_notifications"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("self_service").AtMapKey("notification_location"),
						knownvalue.StringExact("Self Service"),
					),
				},
			},
		},
	})
}

// TestAccPolicyResource_SelfServiceMultiCategory is the regression test for the
// self_service.categories migration (SingleNested → SetNested). The classic
// /policies wire carries a repeating <category> element; the pre-migration
// model read only index [0], silently dropping every category after the first
// along with its independent display_in/feature_in flags. Step 1 declares two
// categories with distinct feature_in values — a green size-2 set, with one
// element feature_in=true and one feature_in=false, IS the assertion that both
// survive the round-trip. Step 2 flips the first category's feature_in in place
// to exercise per-element flag mutation idempotency.
func TestAccPolicyResource_SelfServiceMultiCategory(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-sscat-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigSelfServiceCategories(name, true),
				ConfigStateChecks: []statecheck.StateCheck{
					// Both categories survive the round-trip — the
					// pre-migration [0]-only read would collapse this to 1.
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("self_service").AtMapKey("categories"),
						knownvalue.SetSizeExact(2),
					),
					// …and the two entries carry DISTINCT feature_in flags
					// (SetPartial consumes each match, so this requires two
					// separate elements). ObjectPartial matches only
					// feature_in, sidestepping the Computed name/id.
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("self_service").AtMapKey("categories"),
						knownvalue.SetPartial([]knownvalue.Check{
							knownvalue.ObjectPartial(map[string]knownvalue.Check{"feature_in": knownvalue.Bool(true)}),
							knownvalue.ObjectPartial(map[string]knownvalue.Check{"feature_in": knownvalue.Bool(false)}),
						}),
					),
				},
			},
			{
				// Flip the first category's feature_in true→false in place;
				// both categories must still round-trip, both now feature_in
				// =false, and the apply must converge cleanly.
				Config: policyConfigSelfServiceCategories(name, false),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("self_service").AtMapKey("categories"),
						knownvalue.SetSizeExact(2),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("self_service").AtMapKey("categories"),
						knownvalue.SetPartial([]knownvalue.Check{
							knownvalue.ObjectPartial(map[string]knownvalue.Check{"feature_in": knownvalue.Bool(false)}),
							knownvalue.ObjectPartial(map[string]knownvalue.Check{"feature_in": knownvalue.Bool(false)}),
						}),
					),
				},
			},
		},
	})
}

// TestAccPolicyResource_AllComputers verifies all_computers=true creates
// without per-computer targets.
func TestAccPolicyResource_AllComputers(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-all-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigAllComputers(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("all_computers"),
						knownvalue.Bool(true),
					),
				},
			},
		},
	})
}

// policyConfigGeneralFull exercises every general top-level attribute the
// schema exposes (excluding category/site reference IDs, which use -1 to
// pin "NONE" so the test is independent of tenant inventory).
func policyConfigGeneralFull(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name                          = %q
    enabled                       = true
    trigger                       = "EVENT"
    trigger_checkin               = true
    trigger_enrollment_complete   = true
    trigger_login                 = true
    trigger_network_state_changed = true
    trigger_startup               = true
    trigger_other                 = "tf-acc-event"
    frequency                     = "Once per computer"
    retry_event                   = "check-in"
    retry_attempts                = 3
    notify_on_each_failed_retry   = true
    limit_to_jamf_pro_assigned_user            = false
    target_drive                  = "/"
    offline                       = false
    category_id                   = "-1"
    site_id                       = "-1"
  }
}
`, name)
}

// policyConfigGeneralWithSubBlocks layers the three nested sub-blocks
// (date_time_limitations, network_limitations, override_default_settings) on
// top of the full general block.
//
// All three retry attributes are cleared explicitly, not just two. Jamf Pro
// keeps a retry configuration only under frequency "Once per computer", and
// every one of the three is Optional+Computed, so a value the previous step set
// carries into this plan through UseNonNullStateForUnknown even though the
// configuration no longer mentions it. Leaving notify_on_each_failed_retry out
// is what made this step fail once the read stopped being sticky, and it is now
// refused at plan time by retryRequiresOncePerComputerValidator.
//
// network_limitations and override_default_settings are declared empty. Every
// attribute in both is Computed-only — Jamf Pro derives each from somewhere
// else and discards a write — so declaring the block is the whole gesture: it
// tells the provider to read the derived values into state. The writable
// counterparts are set on the general block itself: network_requirements drives
// minimum_network_connection, and target_drive drives
// override_default_settings.target_drive.
//
// no_execute_start / no_execute_end round-trip, but only because the write
// side offsets them a full day forward — see no_execute.go. They get their own
// coverage in TestAccPolicyResource_NoExecuteWindowRoundTrips; asserting them
// here too is deliberate, because this is the config that also exercises the
// sibling activation/expiration timestamps in the same element.
func policyConfigGeneralWithSubBlocks(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name                        = %q
    enabled                     = true
    frequency                   = "Ongoing"
    retry_event                 = "none"
    retry_attempts              = -1
    notify_on_each_failed_retry = false
    target_drive                = "/override"
    network_requirements        = "Ethernet"
    category_id                 = "-1"
    site_id                     = "-1"

    date_time_limitations = {
      activation_date  = "2026-01-01 01:00:00"
      expiration_date  = "2027-01-01 01:00:00"
      no_execute_on    = ["Sun", "Sat"]
      no_execute_start = "1:00 AM"
      no_execute_end   = "2:00 AM"
    }

    network_limitations       = {}
    override_default_settings = {}
  }
}
`, name)
}

// TestAccPolicyResource_GeneralFullCoverage exercises every general-section
// attribute (top-level + each nested sub-block) and confirms the wire echoes
// match what was sent. Import-state round-trip for the nested sub-blocks is
// not asserted here — by design (see state_builders.assignPolicyResourceModel)
// Optional+Computed nested sections are only populated on Read when the
// caller already manages them, so a freshly-imported state will not contain
// date_time_limitations / network_limitations / override_default_settings.
// TestAccPolicyResource_Minimal already covers import for the top-level
// general attributes.
func TestAccPolicyResource_GeneralFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-gen-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigGeneralFull(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("trigger_checkin"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("trigger_other"),
						knownvalue.StringExact("tf-acc-event"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("frequency"),
						knownvalue.StringExact("Once per computer"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("retry_event"),
						knownvalue.StringExact("check-in"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("retry_attempts"),
						knownvalue.Int64Exact(3),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("notify_on_each_failed_retry"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("target_drive"),
						knownvalue.StringExact("/"),
					),
				},
			},
			{
				Config: policyConfigGeneralWithSubBlocks(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("date_time_limitations").AtMapKey("activation_date"),
						knownvalue.StringExact("2026-01-01 01:00:00"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("date_time_limitations").AtMapKey("expiration_date"),
						knownvalue.StringExact("2027-01-01 01:00:00"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("date_time_limitations").AtMapKey("no_execute_start"),
						knownvalue.StringExact("1:00 AM"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("date_time_limitations").AtMapKey("no_execute_end"),
						knownvalue.StringExact("2:00 AM"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("date_time_limitations").AtMapKey("no_execute_on"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("Sat"),
							knownvalue.StringExact("Sun"),
						}),
					),
					// network_limitations is a projection. minimum_network_connection
					// follows general.network_requirements = "Ethernet" set above,
					// and any_ip_address follows scope.limitations.network_segment_ids
					// being empty. Neither was configured here — and neither could be.
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("network_limitations").AtMapKey("minimum_network_connection"),
						knownvalue.StringExact("Ethernet"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("network_limitations").AtMapKey("any_ip_address"),
						knownvalue.Bool(true),
					),
					// override_default_settings is a projection too. target_drive
					// follows the general.target_drive = "/override" set above, which
					// is the pairing most likely to regress silently.
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("override_default_settings").AtMapKey("target_drive"),
						knownvalue.StringExact("/override"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("override_default_settings").AtMapKey("force_afp_smb"),
						knownvalue.Bool(false),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("override_default_settings").AtMapKey("distribution_point"),
						knownvalue.StringExact("default"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("general").AtMapKey("override_default_settings").AtMapKey("sus"),
						knownvalue.StringExact("default"),
					),
				},
			},
		},
	})
}

// policyConfigReboot exposes every reboot-section attribute. The
// specify_startup variant is parameterised so the test can sweep all three
// validator-accepted values.
func policyConfigReboot(name, specifyStartup string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  restart_options = {
    message                        = "tf-acc reboot message"
    startup_disk                   = "Current Startup Disk"
    specify_startup                = %q
    no_user_logged_in              = "Restart immediately"
    user_logged_in                 = "Restart if a package or update requires it"
    delay_minutes           = 10
    start_reboot_timer_immediately = true
    file_vault_2_reboot            = false
  }
}
`, name, specifyStartup)
}

// TestAccPolicyResource_RebootFullCoverage exercises every reboot attribute
// and sweeps specify_startup through all three values the validator accepts:
// the wire-empty default, the standard restart label, and the MDM
// kernel-cache-rebuild label.
// Import-state round-trip is not asserted — by design (see
// state_builders.assignPolicyResourceModel) the reboot section is only
// populated on Read when the caller already manages it, so a freshly-imported
// state will not contain the reboot block.
func policyConfigPackageConfigurationDistributionPoint(name, dp string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  packages = {
    distribution_point = %q
  }
}
`, name, dp)
}

// TestAccPolicyResource_PackageConfigurationDistributionPoint exercises the
// top-level `<package_configuration><distribution_point>` wire field added in
// SDK 0.8.1-…46aec40edb28. The wire returns this value as a peer of <packages>.
// Test omits the `packages` set entirely — there is no clean "NONE" value for
// packages[].id, so per-package
// coverage is deferred to PR #5 with a real `jamfplatform_pro_package` fixture.
// Import-state round-trip is not asserted (per
// state_builders.assignPolicyResourceModel, optional sub-blocks are only
// populated on Read when already managed by the caller).
func TestAccPolicyResource_PackageConfigurationDistributionPoint(t *testing.T) {
	testhelpers.AccPreCheck(t)
	// The policy references the "default" distribution point; the server rejects
	// the write (409 "Problem with distribution server") unless a principal DP is
	// configured, so make the tenant's existing cloud DP principal for the test.
	testhelpers.EnsurePrincipalCloudDistributionPoint(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-pkgcfg-dp-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigPackageConfigurationDistributionPoint(name, "default"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("packages").AtMapKey("distribution_point"),
						knownvalue.StringExact("default"),
					),
				},
			},
		},
	})
}

func policyConfigPackageConfigurationPackages(name, pkgName, pkgFileName, pkgSrc, action string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_package" "fixture" {
  display_name        = %q
  file_name           = %q
  package_file_source = %q
  info                = "tf-acc package fixture for jamfplatform_pro_policy package_configuration coverage"
}

resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  packages = {
    distribution_point = "default"
    packages = [
      {
        id     = jamfplatform_pro_package.fixture.id
        action = %q
      },
    ]
  }
}
`, pkgName, pkgFileName, pkgSrc, name, action)
}

// TestAccPolicyResource_PackageConfigurationPackages exercises the
// `packages.packages` set together with the top-level
// distribution_point. A jamfplatform_pro_package resource uploads the
// committed jamf-cli .pkg fixture (shared with the pro/package acc
// suite) so the policy can reference a real package ID. Step 2 swaps the
// per-package action Install → Cache to exercise the Update path on the
// policy's packages set without re-uploading the package.
// Import-state round-trip is not asserted (per
// state_builders.assignPolicyResourceModel, optional sub-blocks are only
// populated on Read when already managed by the caller).
//
// Two tenant prerequisites, in this order. The package fixture uploads its
// binary, which only converges on a JCDS-backed tenant, so RequireJCDSUploads
// runs first: a tenant this test is going to skip on never has its principal-DP
// flag flipped and restored for nothing. Then, as in
// PackageConfigurationDistributionPoint, the "default" distribution_point
// reference requires a principal DP on the tenant.
func TestAccPolicyResource_PackageConfigurationPackages(t *testing.T) {
	testhelpers.AccPreCheck(t)
	testhelpers.RequireJCDSUploads(t)
	testhelpers.EnsurePrincipalCloudDistributionPoint(t)
	suffix := testhelpers.RunSuffix()
	policyName := "tf-acc-policy-pkgcfg-pkgs-" + suffix
	pkgDisplay := "tf-acc-pkg-fixture-" + suffix
	pkgFileName := "tf-acc-pkg-fixture-" + suffix + ".pkg"
	pkgSrc := packageFixturePath(t, "jamf-cli-1.15.0.pkg")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigPackageConfigurationPackages(policyName, pkgDisplay, pkgFileName, pkgSrc, "Install"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("packages").AtMapKey("distribution_point"),
						knownvalue.StringExact("default"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("packages").AtMapKey("packages"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("packages").AtMapKey("packages").AtSliceIndex(0).AtMapKey("action"),
						knownvalue.StringExact("Install"),
					),
				},
			},
			{
				Config: policyConfigPackageConfigurationPackages(policyName, pkgDisplay, pkgFileName, pkgSrc, "Cache"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("packages").AtMapKey("packages").AtSliceIndex(0).AtMapKey("action"),
						knownvalue.StringExact("Cache"),
					),
				},
			},
		},
	})
}

func TestAccPolicyResource_RebootFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-reboot-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigReboot(name, ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("restart_options").AtMapKey("specify_startup"),
						knownvalue.StringExact(""),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("restart_options").AtMapKey("message"),
						knownvalue.StringExact("tf-acc reboot message"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("restart_options").AtMapKey("delay_minutes"),
						knownvalue.Int64Exact(10),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("restart_options").AtMapKey("start_reboot_timer_immediately"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("restart_options").AtMapKey("no_user_logged_in"),
						knownvalue.StringExact("Restart immediately"),
					),
				},
			},
			{
				Config: policyConfigReboot(name, "Standard Restart"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("restart_options").AtMapKey("specify_startup"),
						knownvalue.StringExact("Standard Restart"),
					),
				},
			},
			{
				Config: policyConfigReboot(name, "MDM Restart with Kernel Cache Rebuild"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("restart_options").AtMapKey("specify_startup"),
						knownvalue.StringExact("MDM Restart with Kernel Cache Rebuild"),
					),
				},
			},
		},
	})
}

func policyConfigScripts(policyName, scriptName, priority, scriptPolicyPriority, p4 string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_script" "fixture" {
  name            = %q
  priority        = %q
  info            = "tf-acc script fixture for jamfplatform_pro_policy scripts coverage"
  script_contents = <<-EOT
    #!/bin/sh
    echo "tf-acc-script"
  EOT
}

resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  scripts = {
    scripts = [
      {
        id          = jamfplatform_pro_script.fixture.id
        priority    = %q
        parameter_4 = %q
      },
    ]
  }
}
`, scriptName, priority, policyName, scriptPolicyPriority, p4)
}

// TestAccPolicyResource_ScriptsFullCoverage creates a jamfplatform_pro_script
// fixture (the standalone script resource uses BEFORE/AFTER/AT_REBOOT) and
// references its ID from policy.scripts.scripts. The policy's wire form for
// `priority` is `Before`/`After`/`At Reboot`, distinct from the fixture's
// enum. Step 2 swaps priority and
// parameter_4 to exercise the Update path on the scripts set without
// recreating the fixture.
func TestAccPolicyResource_ScriptsFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	policyName := "tf-acc-policy-scripts-" + suffix
	scriptName := "tf-acc-script-fixture-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigScripts(policyName, scriptName, "AFTER", "Before", "tf-acc-p4-initial"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scripts").AtMapKey("scripts"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scripts").AtMapKey("scripts").AtSliceIndex(0).AtMapKey("priority"),
						knownvalue.StringExact("Before"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scripts").AtMapKey("scripts").AtSliceIndex(0).AtMapKey("parameter_4"),
						knownvalue.StringExact("tf-acc-p4-initial"),
					),
				},
			},
			{
				Config: policyConfigScripts(policyName, scriptName, "AFTER", "After", "tf-acc-p4-updated"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scripts").AtMapKey("scripts").AtSliceIndex(0).AtMapKey("priority"),
						knownvalue.StringExact("After"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scripts").AtMapKey("scripts").AtSliceIndex(0).AtMapKey("parameter_4"),
						knownvalue.StringExact("tf-acc-p4-updated"),
					),
				},
			},
		},
	})
}

func policyConfigPrinters(policyName, printerName, action string, makeDefault bool) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_printer" "fixture" {
  name = %q
  uri  = "ipp://10.1.20.120/"
}

resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  printers = {
    printers = [
      {
        id           = jamfplatform_pro_printer.fixture.id
        action       = %q
        make_default = %t
      },
    ]
  }
}
`, printerName, policyName, action, makeDefault)
}

// TestAccPolicyResource_PrintersFullCoverage creates a jamfplatform_pro_printer
// fixture and references it from policy.printers.printers. Step 2 swaps action
// `Map` → `Unmap` (the UI-canonical values exposed by the schema; the provider
// translates to/from the wire `install`/`uninstall` form via printerActionToWire
// / printerActionFromWire) and toggles make_default to exercise the Update path.
//
// leave_existing_default is gone: it modelled a wire element Jamf Pro answers
// as <leave_existing_default/> whatever is written, including on a policy whose
// printers were configured in the admin UI with the default-printer choice
// toggled both ways. make_default is the attribute that actually carries that
// choice, and it round-trips — which is what the two steps below assert.
func TestAccPolicyResource_PrintersFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	policyName := "tf-acc-policy-printers-" + suffix
	printerName := "tf-acc-printer-fixture-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigPrinters(policyName, printerName, "Map", true),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("printers").AtMapKey("printers"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("printers").AtMapKey("printers").AtSliceIndex(0).AtMapKey("action"),
						knownvalue.StringExact("Map"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("printers").AtMapKey("printers").AtSliceIndex(0).AtMapKey("make_default"),
						knownvalue.Bool(true),
					),
				},
			},
			{
				Config: policyConfigPrinters(policyName, printerName, "Unmap", false),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("printers").AtMapKey("printers").AtSliceIndex(0).AtMapKey("action"),
						knownvalue.StringExact("Unmap"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("printers").AtMapKey("printers").AtSliceIndex(0).AtMapKey("make_default"),
						knownvalue.Bool(false),
					),
				},
			},
		},
	})
}

func policyConfigDockItems(policyName, dockItemName, action string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_dock_item" "fixture" {
  name = %q
  type = "App"
  path = "/Applications/Calculator.app"
}

resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  dock_items = {
    dock_items = [
      {
        id     = jamfplatform_pro_dock_item.fixture.id
        action = %q
      },
    ]
  }
}
`, dockItemName, policyName, action)
}

// TestAccPolicyResource_DockItemsFullCoverage creates a jamfplatform_pro_dock_item
// fixture and references it from policy.dock_items.dock_items. Step 2 swaps action
// `Add To End` → `Add To Beginning` to exercise the Update path.
func TestAccPolicyResource_DockItemsFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	policyName := "tf-acc-policy-dockitems-" + suffix
	dockItemName := "tf-acc-dock-item-fixture-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigDockItems(policyName, dockItemName, "Add To End"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("dock_items").AtMapKey("dock_items"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("dock_items").AtMapKey("dock_items").AtSliceIndex(0).AtMapKey("action"),
						knownvalue.StringExact("Add To End"),
					),
				},
			},
			{
				Config: policyConfigDockItems(policyName, dockItemName, "Add To Beginning"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("dock_items").AtMapKey("dock_items").AtSliceIndex(0).AtMapKey("action"),
						knownvalue.StringExact("Add To Beginning"),
					),
				},
			},
		},
	})
}

func policyConfigMaintenance(name string, updateInventory bool) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  maintenance = {
    update_inventory        = %t
    reset_computer_names    = true
    install_cached_packages = true
    fix_disk_permissions    = true
    fix_byhost_files        = true
    flush_system_caches     = true
    flush_user_caches       = true
    verify_startup_disk     = true
  }
}
`, name, updateInventory)
}

// TestAccPolicyResource_MaintenanceFullCoverage exercises every maintenance
// attribute. Step 2 toggles update_inventory to exercise the Update path.
// The wire-dead `heal` and `prebindings` fields (wire rejected or echoed
// empty regardless of the value sent) were dropped from the schema in the
// same PR as this rename batch.
func TestAccPolicyResource_MaintenanceFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-maint-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigMaintenance(name, true),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("maintenance").AtMapKey("update_inventory"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("maintenance").AtMapKey("verify_startup_disk"),
						knownvalue.Bool(true),
					),
				},
			},
			{
				Config: policyConfigMaintenance(name, false),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("maintenance").AtMapKey("update_inventory"),
						knownvalue.Bool(false),
					),
				},
			},
		},
	})
}

func policyConfigFilesProcesses(name, searchByPath, executeCommand string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  files_and_processes = {
    search_by_path         = %q
    delete_file_if_found   = true
    search_by_filename     = "tf-acc-search-filename"
    update_locate_database = true
    search_by_spotlight    = "tf-acc-spotlight"
    search_for_process     = "tf-acc-process"
    kill_process_if_found  = true
    execute_command        = %q
  }
}
`, name, searchByPath, executeCommand)
}

// TestAccPolicyResource_FilesProcessesFullCoverage exercises every
// files_and_processes attribute. Step 2 perturbs two scalar string fields to
// exercise the Update path.
func TestAccPolicyResource_FilesProcessesFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-filesproc-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigFilesProcesses(name, "/tmp/tf-acc-search", "echo tf-acc-initial"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("files_and_processes").AtMapKey("search_by_path"),
						knownvalue.StringExact("/tmp/tf-acc-search"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("files_and_processes").AtMapKey("delete_file_if_found"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("files_and_processes").AtMapKey("search_by_filename"),
						knownvalue.StringExact("tf-acc-search-filename"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("files_and_processes").AtMapKey("kill_process_if_found"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("files_and_processes").AtMapKey("execute_command"),
						knownvalue.StringExact("echo tf-acc-initial"),
					),
				},
			},
			{
				Config: policyConfigFilesProcesses(name, "/tmp/tf-acc-search-updated", "echo tf-acc-updated"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("files_and_processes").AtMapKey("search_by_path"),
						knownvalue.StringExact("/tmp/tf-acc-search-updated"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("files_and_processes").AtMapKey("execute_command"),
						knownvalue.StringExact("echo tf-acc-updated"),
					),
				},
			},
		},
	})
}

func policyConfigUserInteractionDate(name, startMessage, untilUTC string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  user_interaction = {
    start_message      = %q
    deferral_type      = "date"
    deferral_until_utc = %q
    complete_message   = "tf-acc complete_message"
  }
}
`, name, startMessage, untilUTC)
}

func policyConfigUserInteractionDuration(name, startMessage string, days int) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  user_interaction = {
    start_message    = %q
    deferral_type    = "duration"
    deferral_days    = %d
    complete_message = "tf-acc complete_message"
  }
}
`, name, startMessage, days)
}

func policyConfigUserInteractionNone(name, startMessage string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  user_interaction = {
    start_message    = %q
    deferral_type    = "none"
    complete_message = "tf-acc complete_message"
  }
}
`, name, startMessage)
}

// TestAccPolicyResource_UserInteractionFullCoverage exercises every
// user_interaction attribute and every deferral_type variant, including
// in-place transitions between Date / Duration / None. Wire-probe
// 2026-05-27 confirmed the classic /policies PUT accepts these transitions
// without destroy+recreate as long as the provider emits the off-axis wire
// fields with explicit zero values (which buildPolicyUserInteraction does).
func TestAccPolicyResource_UserInteractionFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-userint-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigUserInteractionDate(name, "tf-acc start_message initial", "2030-01-01T01:00:00.000+0000"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("user_interaction").AtMapKey("start_message"),
						knownvalue.StringExact("tf-acc start_message initial"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("user_interaction").AtMapKey("deferral_type"),
						knownvalue.StringExact("date"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("user_interaction").AtMapKey("deferral_until_utc"),
						knownvalue.StringExact("2030-01-01T01:00:00.000+0000"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("user_interaction").AtMapKey("complete_message"),
						knownvalue.StringExact("tf-acc complete_message"),
					),
				},
			},
			// In-place Date → Duration transition. Wire probe confirmed the
			// classic API zeroes the off-axis field on PUT.
			{
				Config: policyConfigUserInteractionDuration(name, "tf-acc start_message updated", 3),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("user_interaction").AtMapKey("start_message"),
						knownvalue.StringExact("tf-acc start_message updated"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("user_interaction").AtMapKey("deferral_type"),
						knownvalue.StringExact("duration"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("user_interaction").AtMapKey("deferral_days"),
						knownvalue.Int64Exact(3),
					),
				},
			},
			// In-place Duration → None transition.
			{
				Config: policyConfigUserInteractionNone(name, "tf-acc start_message no-deferral"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("user_interaction").AtMapKey("deferral_type"),
						knownvalue.StringExact("none"),
					),
				},
			},
		},
	})
}

func policyConfigDateTimeLimitationBad(name, attr, value string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
    date_time_limitations = {
      %s = %q
    }
  }
}
`, name, attr, value)
}

func policyConfigNoExecuteOnBad(name string, days []string) string {
	quoted := make([]string, len(days))
	for i, d := range days {
		quoted[i] = fmt.Sprintf("%q", d)
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
    date_time_limitations = {
      no_execute_on = [%s]
    }
  }
}
`, name, strings.Join(quoted, ", "))
}

func policyConfigDeferralUntilUtcBad(name, value string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  user_interaction = {
    deferral_type      = "date"
    deferral_until_utc = %q
  }
}
`, name, value)
}

// TestAccPolicyResource_DateTimeLimitationsFormatRejection exercises each
// regex validator on the date_time_limitations sub-block. Every step fails
// at plan time with a message keyed off the validator's own error text — no
// API round-trip occurs, so these are fast and credential-cheap once the
// acc framework is up. Wire formats are documented in policy/validators.go.
func TestAccPolicyResource_DateTimeLimitationsFormatRejection(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-dtl-bad-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      policyConfigDateTimeLimitationBad(name, "activation_date", "06/01/2027 14:30:00"),
				ExpectError: regexp.MustCompile(`Value must be 24-hour`),
			},
			{
				Config:      policyConfigDateTimeLimitationBad(name, "activation_date", "2027-06-01T14:30:00"),
				ExpectError: regexp.MustCompile(`Value must be 24-hour`),
			},
			{
				Config:      policyConfigDateTimeLimitationBad(name, "expiration_date", "2027-12-31"),
				ExpectError: regexp.MustCompile(`Value must be 24-hour`),
			},
			{
				Config:      policyConfigDateTimeLimitationBad(name, "no_execute_start", "17:00"),
				ExpectError: regexp.MustCompile(`12-hour`),
			},
			{
				Config:      policyConfigDateTimeLimitationBad(name, "no_execute_start", "5:00 am"),
				ExpectError: regexp.MustCompile(`12-hour`),
			},
			{
				Config:      policyConfigDateTimeLimitationBad(name, "no_execute_end", "13:00 PM"),
				ExpectError: regexp.MustCompile(`12-hour`),
			},
			{
				Config:      policyConfigDateTimeLimitationBad(name, "no_execute_end", "0:00 AM"),
				ExpectError: regexp.MustCompile(`12-hour`),
			},
			{
				Config:      policyConfigNoExecuteOnBad(name, []string{"Sunday"}),
				ExpectError: regexp.MustCompile(`must be one of`),
			},
			{
				Config:      policyConfigNoExecuteOnBad(name, []string{"Sun", "FOO"}),
				ExpectError: regexp.MustCompile(`must be one of`),
			},
		},
	})
}

// TestAccPolicyResource_DeferralUntilUtcFormatRejection exercises the regex
// validator on user_interaction.deferral_until_utc. Wire format is documented
// in policy/validators.go (ISO-8601 with millisecond precision + four-digit
// numeric offset).
func TestAccPolicyResource_DeferralUntilUtcFormatRejection(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-defer-bad-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      policyConfigDeferralUntilUtcBad(name, "2027-01-01T01:00:00Z"),
				ExpectError: regexp.MustCompile(`Value must be ISO-8601`),
			},
			{
				Config:      policyConfigDeferralUntilUtcBad(name, "2027-01-01 01:00:00.000+0000"),
				ExpectError: regexp.MustCompile(`Value must be ISO-8601`),
			},
			{
				Config:      policyConfigDeferralUntilUtcBad(name, "2027-01-01T01:00:00.000+00:00"),
				ExpectError: regexp.MustCompile(`Value must be ISO-8601`),
			},
		},
	})
}

func policyConfigDiskEncryption(policyName, decName, action string, authRestart bool) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_disk_encryption_configuration" "fixture" {
  name                     = %q
  key_type                 = "Individual"
  file_vault_enabled_users = "Current or Next User"
}

resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  disk_encryption = {
    action                           = %q
    disk_encryption_configuration_id = tonumber(jamfplatform_pro_disk_encryption_configuration.fixture.id)
    auth_restart                     = %t
  }
}
`, decName, policyName, action, authRestart)
}

// TestAccPolicyResource_DiskEncryptionFullCoverage creates a
// jamfplatform_pro_disk_encryption_configuration fixture (key_type=Individual
// is the minimal config — no IRK certificate required) and references its ID
// from policy.disk_encryption. Step 2 toggles auth_restart to exercise the
// Update path. action=remediate silently reverts to apply on the server,
// so this test sticks with action=apply throughout.
func TestAccPolicyResource_DiskEncryptionFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	policyName := "tf-acc-policy-de-" + suffix
	decName := "tf-acc-dec-fixture-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigDiskEncryption(policyName, decName, "apply", false),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("disk_encryption").AtMapKey("action"),
						knownvalue.StringExact("apply"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("disk_encryption").AtMapKey("auth_restart"),
						knownvalue.Bool(false),
					),
				},
			},
			{
				Config: policyConfigDiskEncryption(policyName, decName, "apply", true),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("disk_encryption").AtMapKey("auth_restart"),
						knownvalue.Bool(true),
					),
				},
			},
		},
	})
}

func policyConfigScopeAllJssUsers(name string, allJssUsers bool) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  scope = {
    targets = {
      all_computers = true
      all_jss_users = %t
    }
  }
}
`, name, allJssUsers)
}

// TestAccPolicyResource_ScopeAllJssUsersFullCoverage exercises the
// scope.all_jss_users Bool attribute newly wired into the schema after SDK
// commit d7d755d added the proclassic.PolicyScope.AllJssUsers field. Both
// steps keep all_computers=true so the policy is otherwise valid; step 2
// flips all_jss_users to exercise the Update path.
func TestAccPolicyResource_ScopeAllJssUsersFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-scope-alljss-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigScopeAllJssUsers(name, true),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("all_jss_users"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("all_computers"),
						knownvalue.Bool(true),
					),
				},
			},
			{
				Config: policyConfigScopeAllJssUsers(name, false),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("all_jss_users"),
						knownvalue.Bool(false),
					),
				},
			},
		},
	})
}

func policyConfigScopeTargetsAllFixtures(policyName, buildingName, departmentName, deviceGroupName, userGroupName string, computerID, userID string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_building" "fixture" {
  name = %q
}

resource "jamfplatform_pro_department" "fixture" {
  name = %q
}

resource "jamfplatform_device_group" "fixture" {
  name        = %q
  description = "tf-acc scope fixture"
  group_type  = "static"
  device_type = "computer"
}

resource "jamfplatform_pro_user_group" "fixture" {
  name       = %q
  group_type = "static"
}

resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  scope = {
    targets = {
      computer_ids       = [%q]
      computer_group_ids = [jamfplatform_device_group.fixture.jamf_pro_id]
      building_ids       = [jamfplatform_pro_building.fixture.id]
      department_ids     = [jamfplatform_pro_department.fixture.id]
      user_ids       = [%q]
      user_group_ids = [jamfplatform_pro_user_group.fixture.id]
    }
  }
}
`, buildingName, departmentName, deviceGroupName, userGroupName, policyName, computerID, userID)
}

// TestAccPolicyResource_ScopeTargetsFixtureCoverage exercises every scope
// target sub-block by creating one fixture per category and asserting the
// resulting set membership round-trips through the policy resource. Fixtures
// that have a matching Terraform Pro resource are declared inline in the HCL;
// fixtures the project does not yet expose as resources (classic computer,
// classic user) are created out-of-band via direct SDK calls and cleaned up
// at test end via t.Cleanup.
//
// The static computer group is created via the Platform Services
// jamfplatform_device_group resource and its server-derived `jamf_pro_id`
// attribute bridges into the classic policy's computer_group_ids set per the
// resource description.
func TestAccPolicyResource_ScopeTargetsFixtureCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	policyName := "tf-acc-policy-scope-targets-" + suffix
	buildingName := "tf-acc-building-" + suffix
	departmentName := "tf-acc-department-" + suffix
	deviceGroupName := "tf-acc-device-group-" + suffix
	userGroupName := "tf-acc-user-group-" + suffix
	computerName := "tf-acc-computer-" + suffix
	userName := "tf-acc-user-" + suffix

	computerID := createDummyComputer(t, computerName)
	userID := createDummyUser(t, userName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigScopeTargetsAllFixtures(policyName, buildingName, departmentName, deviceGroupName, userGroupName, computerID, userID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("computer_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("computer_group_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("building_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("department_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("user_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("user_group_ids"),
						knownvalue.SetSizeExact(1),
					),
				},
			},
		},
	})
}

func policyConfigScopeLimitations(policyName, networkSegmentName, ibeaconName string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_network_segment" "fixture" {
  name             = %q
  starting_address = "10.99.0.0"
  ending_address   = "10.99.0.255"
}

resource "jamfplatform_pro_ibeacon" "fixture" {
  name                    = %q
  uuid                    = "759b0599-64e0-416a-8d31-d8e93482a4d7"
  include_any_major_value = true
  include_any_minor_value = true
}

resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  scope = {
    targets = {
      all_computers = true
    }
    limitations = {
      network_segment_ids = [jamfplatform_pro_network_segment.fixture.id]
      ibeacon_ids         = [jamfplatform_pro_ibeacon.fixture.id]
    }
  }
}
`, networkSegmentName, ibeaconName, policyName)
}

// TestAccPolicyResource_ScopeLimitationsFixtureCoverage exercises the
// fixture-backed `scope.limitations` sub-attributes — network segment IDs and
// iBeacon IDs. The `directory_service_or_local_user_names` and
// `directory_service_user_group_names` attributes are NOT exercised here: the
// classic API rejects names that do not resolve against the tenant's LDAP
// integration (`Error: Problem matching limitation user group`), so testing
// them requires fixture LDAP entries the tenant does not generally provide.
// Their wire round-trip is covered indirectly via the policy 6791 baseline.
func TestAccPolicyResource_ScopeLimitationsFixtureCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	policyName := "tf-acc-policy-scope-lim-" + suffix
	networkSegmentName := "tf-acc-netseg-" + suffix
	ibeaconName := "tf-acc-ibeacon-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigScopeLimitations(policyName, networkSegmentName, ibeaconName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("limitations").AtMapKey("network_segment_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("limitations").AtMapKey("ibeacon_ids"),
						knownvalue.SetSizeExact(1),
					),
				},
			},
		},
	})
}

func policyConfigScopeExclusions(policyName, buildingName, departmentName, deviceGroupName, userGroupName, networkSegmentName, ibeaconName string, computerID, userID string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_building" "fixture" {
  name = %q
}

resource "jamfplatform_pro_department" "fixture" {
  name = %q
}

resource "jamfplatform_device_group" "fixture" {
  name        = %q
  description = "tf-acc scope exclusions fixture"
  group_type  = "static"
  device_type = "computer"
}

resource "jamfplatform_pro_user_group" "fixture" {
  name       = %q
  group_type = "static"
}

resource "jamfplatform_pro_network_segment" "fixture" {
  name             = %q
  starting_address = "10.88.0.0"
  ending_address   = "10.88.0.255"
}

resource "jamfplatform_pro_ibeacon" "fixture" {
  name                    = %q
  uuid                    = "759b0599-64e0-416a-8d31-d8e93482a4d7"
  include_any_major_value = true
  include_any_minor_value = true
}

resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  scope = {
    targets = {
      all_computers = true
    }
    exclusions = {
      computer_ids                          = [%q]
      computer_group_ids                    = [jamfplatform_device_group.fixture.jamf_pro_id]
      building_ids                          = [jamfplatform_pro_building.fixture.id]
      department_ids                        = [jamfplatform_pro_department.fixture.id]
      user_ids        = [%q]
      user_group_ids  = [jamfplatform_pro_user_group.fixture.id]
      network_segment_ids = [jamfplatform_pro_network_segment.fixture.id]
      ibeacon_ids         = [jamfplatform_pro_ibeacon.fixture.id]
    }
  }
}
`, buildingName, departmentName, deviceGroupName, userGroupName, networkSegmentName, ibeaconName, policyName, computerID, userID)
}

// TestAccPolicyResource_ScopeExclusionsFixtureCoverage mirrors the targets
// + limitations coverage but routes every fixture ID through
// scope.exclusions. Verifies that the exclusion-side schema parallels the
// target schema and that the SDK builders emit the right wire shape under
// `<scope><exclusions>`. Free-form directory-service name fields are
// omitted for the same reason as in
// TestAccPolicyResource_ScopeLimitationsFixtureCoverage: the classic API
// validates names against the tenant's LDAP integration and rejects names
// that do not resolve.
func TestAccPolicyResource_ScopeExclusionsFixtureCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	policyName := "tf-acc-policy-scope-excl-" + suffix
	buildingName := "tf-acc-building-excl-" + suffix
	departmentName := "tf-acc-department-excl-" + suffix
	deviceGroupName := "tf-acc-device-group-excl-" + suffix
	userGroupName := "tf-acc-user-group-excl-" + suffix
	networkSegmentName := "tf-acc-netseg-excl-" + suffix
	ibeaconName := "tf-acc-ibeacon-excl-" + suffix
	computerName := "tf-acc-computer-excl-" + suffix
	userName := "tf-acc-user-excl-" + suffix

	computerID := createDummyComputer(t, computerName)
	userID := createDummyUser(t, userName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigScopeExclusions(policyName, buildingName, departmentName, deviceGroupName, userGroupName, networkSegmentName, ibeaconName, computerID, userID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("exclusions").AtMapKey("computer_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("exclusions").AtMapKey("computer_group_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("exclusions").AtMapKey("building_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("exclusions").AtMapKey("department_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("exclusions").AtMapKey("user_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("exclusions").AtMapKey("user_group_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("exclusions").AtMapKey("network_segment_ids"),
						knownvalue.SetSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("scope").AtMapKey("exclusions").AtMapKey("ibeacon_ids"),
						knownvalue.SetSizeExact(1),
					),
				},
			},
		},
	})
}

// policyConfigAccountMaintenanceAccounts_initial is the Step 1 fixture
// for TestAccPolicyResource_AccountMaintenanceAccountsFullCoverage. Three
// accounts in order Create → Reset → Delete; password_wo_version = 1 on
// both password-bearing entries.
func policyConfigAccountMaintenanceAccounts_initial(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  local_accounts = [
    {
      action               = "Create"
      username             = "tf-acc-create"
      realname             = "tf-acc create"
      password             = "Sup3rS3cret!"
      password_wo_version  = 1
      home                 = "/Users/tf-acc-create"
      hint                 = "tf-acc hint"
      admin                = true
      filevault_enabled    = false
      secure_token_allowed = true
    },
    {
      action              = "Reset"
      username            = "tf-acc-reset"
      password            = "Resetting123!"
      password_wo_version = 1
    },
    {
      action                            = "Delete"
      username                          = "tf-acc-delete"
      permanently_delete_home_directory = true
    },
  ]
}
`, name)
}

// policyConfigAccountMaintenanceAccounts_appendFourth is the Step 3
// fixture: identical to Step 2 with a fourth Delete account appended
// at the end of the list. Exercises List growth (List length 3 → 4)
// and verifies the existing three indices are not perturbed.
func policyConfigAccountMaintenanceAccounts_appendFourth(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  local_accounts = [
    {
      action               = "Create"
      username             = "tf-acc-create"
      realname             = "tf-acc create"
      password             = "Rotated-pw-1!"
      password_wo_version  = 2
      home                 = "/Users/tf-acc-create"
      hint                 = "tf-acc hint"
      admin                = true
      filevault_enabled    = false
      secure_token_allowed = true
    },
    {
      action              = "Reset"
      username            = "tf-acc-reset"
      password            = "Resetting123!"
      password_wo_version = 1
    },
    {
      action                            = "Delete"
      username                          = "tf-acc-delete"
      permanently_delete_home_directory = true
    },
    {
      action                            = "Delete"
      username                          = "tf-acc-fourth-delete"
      permanently_delete_home_directory = true
    },
  ]
}
`, name)
}

// policyConfigAccountMaintenanceAccounts_rotateFirst is the Step 2
// fixture: identical to Step 1 except the first account's
// password_wo_version is bumped 1 → 2 and its password is changed.
// Exercises the per-element rotation gate. Order is preserved (List
// positional identity must round-trip).
func policyConfigAccountMaintenanceAccounts_rotateFirst(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  local_accounts = [
    {
      action               = "Create"
      username             = "tf-acc-create"
      realname             = "tf-acc create"
      password             = "Rotated-pw-1!"
      password_wo_version  = 2
      home                 = "/Users/tf-acc-create"
      hint                 = "tf-acc hint"
      admin                = true
      filevault_enabled    = false
      secure_token_allowed = true
    },
    {
      action              = "Reset"
      username            = "tf-acc-reset"
      password            = "Resetting123!"
      password_wo_version = 1
    },
    {
      action                            = "Delete"
      username                          = "tf-acc-delete"
      permanently_delete_home_directory = true
    },
  ]
}
`, name)
}

// TestAccPolicyResource_AccountMaintenanceAccountsFullCoverage exercises
// three of the four classic account-maintenance actions in a single policy
// and validates List positional identity across rotation and growth:
//
//   - Create: full account-provisioning attribute set (username, realname,
//     password, home, hint, admin, filevault_enabled, secure_token_allowed).
//   - Reset:  password reset by username.
//   - Delete: removal with `permanently_delete_home_directory = true`. This
//     is the inverted-and-renamed form of the wire field
//     `<archive_home_directory>`. Server receives the inverse on the wire;
//     state stores the UI-canonical semantic.
//
// Step layout: Step 1 creates three accounts. Step 2 rotates the first
// account's `password_wo_version` to prove per-element rotation gating.
// Step 3 appends a fourth Delete account to prove List growth preserves
// positional identity for indices 0–2.
//
// `DisableFileVault` is wired into the schema validator
// (`stringvalidator.OneOf("Create", "Reset", "Delete", "DisableFileVault")`)
// — the wire string is `DisableFileVault` without a trailing `2` despite
// older documentation. The action is NOT exercised in this acceptance test
// because the classic /policies endpoint silently strips
// `<account><action>DisableFileVault</action></account>` entries from new
// policies (round-trip returns no account, framework then reports
// "produced inconsistent result after apply"). The action does survive on
// policies created via the Jamf Pro UI, so the rejection appears to be
// API-only. Manually-probe the precise wire shape needed before adding
// acceptance coverage.
func TestAccPolicyResource_AccountMaintenanceAccountsFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-am-accounts-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigAccountMaintenanceAccounts_initial(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts"),
						knownvalue.ListSizeExact(3),
					),
					// Positional identity: List indices map to the HCL order.
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(0).AtMapKey("username"),
						knownvalue.StringExact("tf-acc-create"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(1).AtMapKey("username"),
						knownvalue.StringExact("tf-acc-reset"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(2).AtMapKey("username"),
						knownvalue.StringExact("tf-acc-delete"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(0).AtMapKey("password_wo_version"),
						knownvalue.Int64Exact(1),
					),
				},
			},
			{
				// Step 2: per-element rotation. Bump first account's
				// password_wo_version 1 → 2 and change its password.
				// Order must hold; the other two accounts are untouched.
				Config: policyConfigAccountMaintenanceAccounts_rotateFirst(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts"),
						knownvalue.ListSizeExact(3),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(0).AtMapKey("password_wo_version"),
						knownvalue.Int64Exact(2),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(1).AtMapKey("password_wo_version"),
						knownvalue.Int64Exact(1),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(0).AtMapKey("username"),
						knownvalue.StringExact("tf-acc-create"),
					),
				},
			},
			{
				// Step 3: list growth. Append a fourth Delete account; the
				// existing three indices must retain their positional
				// identity and wo_version values.
				Config: policyConfigAccountMaintenanceAccounts_appendFourth(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts"),
						knownvalue.ListSizeExact(4),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(0).AtMapKey("username"),
						knownvalue.StringExact("tf-acc-create"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(1).AtMapKey("username"),
						knownvalue.StringExact("tf-acc-reset"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(2).AtMapKey("username"),
						knownvalue.StringExact("tf-acc-delete"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(3).AtMapKey("username"),
						knownvalue.StringExact("tf-acc-fourth-delete"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(0).AtMapKey("password_wo_version"),
						knownvalue.Int64Exact(2),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("local_accounts").AtSliceIndex(1).AtMapKey("password_wo_version"),
						knownvalue.Int64Exact(1),
					),
				},
			},
		},
	})
}

func policyConfigAccountMaintenanceDirectoryBindings(policyName, dbName, dbUsername, dbPassword string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_directory_binding" "fixture" {
  name     = %q
  priority = 1
  type     = "Open Directory"
  domain   = "ldap.tf-acc.example.com"
  username = %q
  password = %q

  open_directory = {
    encrypt_using_ssl     = false
    perform_secure_bind   = false
    use_for_authentication = true
    use_for_contacts       = false
  }
}

resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  directory_bindings = [
    {
      id = jamfplatform_pro_directory_binding.fixture.id
    },
  ]
}
`, dbName, dbUsername, dbPassword, policyName)
}

// TestAccPolicyResource_AccountMaintenanceDirectoryBindingsFullCoverage
// creates a jamfplatform_pro_directory_binding fixture and references it
// from policy.directory_bindings, asserting the set
// round-trips through the policy resource. The fixture uses the Open
// Directory binding type which has the minimal required-field footprint.
func TestAccPolicyResource_AccountMaintenanceDirectoryBindingsFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	policyName := "tf-acc-policy-am-db-" + suffix
	dbName := "tf-acc-db-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigAccountMaintenanceDirectoryBindings(policyName, dbName, "cn=joiner,dc=tf-acc,dc=example,dc=com", "Sup3rS3cret!"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("directory_bindings"),
						knownvalue.SetSizeExact(1),
					),
				},
			},
		},
	})
}

func policyConfigAccountMaintenanceManagementAccount(name, action, managedPassword string, woVersion int64) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  management_account = {
    action                      = %q
    managed_password            = %q
    managed_password_wo_version = %d
  }
}
`, name, action, managedPassword, woVersion)
}

// TestAccPolicyResource_AccountMaintenanceManagementAccountFullCoverage
// exercises the management_account block. Step 1 rotates the management account
// with a literal managed_password; step 2 bumps the WriteOnly version to
// confirm the rotation gate fires without a plaintext value in the diff.
//
// It used to assert managed_password_length as well. That attribute is gone:
// Jamf Pro never returns it, does not parse it (a length of "abc" is accepted
// as readily as 16), and offers no action for a generated-password length to
// apply to — the enum is rotate or doNotChange.
func TestAccPolicyResource_AccountMaintenanceManagementAccountFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-am-ma-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigAccountMaintenanceManagementAccount(name, "rotate", "Sup3rS3cret!", 1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("management_account").AtMapKey("action"),
						knownvalue.StringExact("rotate"),
					),
				},
			},
			{
				Config: policyConfigAccountMaintenanceManagementAccount(name, "rotate", "R0tat3dSecret!", 2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("management_account").AtMapKey("managed_password_wo_version"),
						knownvalue.Int64Exact(2),
					),
				},
			},
		},
	})
}

func policyConfigAccountMaintenanceOpenFirmwareEfiPassword(name, ofMode, ofPassword string, woVersion int64) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  efi_password = {
    of_mode                = %q
    of_password            = %q
    of_password_wo_version = %d
  }
}
`, name, ofMode, ofPassword, woVersion)
}

// TestAccPolicyResource_AccountMaintenanceOpenFirmwareEfiPasswordFullCoverage
// exercises the efi_password block. The plaintext of_password
// is `WriteOnly` (sent on writes, never persisted in state); rotation is
// triggered via `of_password_wo_version`. Step 2 bumps
// `of_password_wo_version` 1 → 2 to exercise both the Update path and the
// WriteOnly rotation gate, keeping of_mode on `command`.
//
// It used to switch of_mode `command` → `full`, which is not a value Jamf Pro
// accepts: the enum is {command, none}. Sending `full` returns HTTP 201 and
// then resets the whole block to `none` AND clears the stored password, so the
// step failed as ".efi_password: inconsistent values for sensitive attribute"
// — the framework masking the real culprit, the non-sensitive of_mode sibling,
// to the nearest non-sensitive parent. of_mode now carries a OneOf validator
// built from the SDK's own enum, so the same mistake is a plan-time error.
func TestAccPolicyResource_AccountMaintenanceOpenFirmwareEfiPasswordFullCoverage(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-am-efi-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigAccountMaintenanceOpenFirmwareEfiPassword(name, "command", "OF-tf-acc-1!", 1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("efi_password").AtMapKey("of_mode"),
						knownvalue.StringExact("command"),
					),
				},
			},
			{
				Config: policyConfigAccountMaintenanceOpenFirmwareEfiPassword(name, "command", "OF-tf-acc-2!", 2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("efi_password").AtMapKey("of_mode"),
						knownvalue.StringExact("command"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("efi_password").AtMapKey("of_password_wo_version"),
						knownvalue.Int64Exact(2),
					),
				},
			},
			{
				// `full` is refused at plan time now rather than silently
				// wiping the block server-side.
				Config:      policyConfigAccountMaintenanceOpenFirmwareEfiPassword(name, "full", "OF-tf-acc-3!", 3),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Attribute efi_password.of_mode value must be one of`),
			},
			{
				// of_mode = "none" clears the payload, and is the other half of
				// the enum.
				Config: policyConfigAccountMaintenanceOpenFirmwareEfiPassword(name, "none", "", 3),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_policy.test",
						tfjsonpath.New("efi_password").AtMapKey("of_mode"),
						knownvalue.StringExact("none"),
					),
				},
			},
		},
	})
}

// TestAccPolicyResource_ScopeLimitationsClearWithEmptySet verifies that a
// declared-but-empty category clears its members on the wire. Under granular
// per-category scope ownership a declared `[]` is the clear gesture: the build
// emits an explicit empty element, and the update's read-merge-write re-emits
// every undeclared category so only the declared one changes. Omitting the
// category instead would leave it as configured outside Terraform.
// Uses a network-segment fixture (no LDAP needed).
func TestAccPolicyResource_ScopeLimitationsClearWithEmptySet(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-limclear-" + suffix
	seg := "tf-acc-netseg-limclear-" + suffix
	cfg := func(segs string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_network_segment" "fixture" {
  name             = %q
  starting_address = "10.98.0.0"
  ending_address   = "10.98.0.255"
}

resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  scope = {
    targets = {
      all_computers = true
    }
    limitations = {
      network_segment_ids = [%s]
    }
  }
}
`, seg, name, segs)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg(`jamfplatform_pro_network_segment.fixture.id`),
				Check:  resource.TestCheckResourceAttr("jamfplatform_pro_policy.test", "scope.limitations.network_segment_ids.#", "1"),
			},
			{
				// Clear to [] — the declared-but-empty category must be emitted
				// as an explicit empty element so the subtree replace clears the
				// segment. The implicit post-step empty-plan check enforces the
				// clear round-tripped.
				Config: cfg(``),
				Check:  resource.TestCheckResourceAttr("jamfplatform_pro_policy.test", "scope.limitations.network_segment_ids.#", "0"),
			},
		},
	})
}

// policyOmitRetainsResourceAddr is the policy under test in the omit-retains
// contract test; the disk-encryption fixture's address is needed too because
// the wire echoes only its numeric id.
const (
	policyOmitRetainsResourceAddr = "jamfplatform_pro_policy.test"
	policyOmitRetainsDecAddr      = "jamfplatform_pro_disk_encryption_configuration.fixture"
)

// policyOmitRetainsNames derives every fixture name from the run-unique base
// name, so the wire assertion can match referenced objects by the name Jamf
// echoes rather than by server-assigned ids the config helper cannot know.
type policyOmitRetainsNames struct {
	Policy, Category, Building, Department, DeviceGroup, SegmentLimitation, SegmentExclusion, Ibeacon, Script, Printer, DockItem, Dec, DirectoryBinding string
}

func newPolicyOmitRetainsNames(base string) policyOmitRetainsNames {
	return policyOmitRetainsNames{
		Policy:            base,
		Category:          base + "-cat",
		Building:          base + "-bld",
		Department:        base + "-dept",
		DeviceGroup:       base + "-grp",
		SegmentLimitation: base + "-seg-lim",
		SegmentExclusion:  base + "-seg-excl",
		Ibeacon:           base + "-ibeacon",
		Script:            base + "-script",
		Printer:           base + "-printer",
		DockItem:          base + "-dock",
		Dec:               base + "-dec",
		DirectoryBinding:  base + "-db",
	}
}

// policyOmitRetainsFixtures declares every sibling object the fully declared
// policy references. It is shared by all three steps so a fixture is never
// destroyed while the policy still points at it; only the policy body shrinks.
func policyOmitRetainsFixtures(n policyOmitRetainsNames) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_category" "fixture" {
  name     = %q
  priority = 9
}

resource "jamfplatform_pro_building" "fixture" {
  name = %q
}

resource "jamfplatform_pro_department" "fixture" {
  name = %q
}

resource "jamfplatform_device_group" "fixture" {
  name        = %q
  description = "tf-acc omit-retains fixture"
  group_type  = "static"
  device_type = "computer"
}

resource "jamfplatform_pro_network_segment" "limitation" {
  name             = %q
  starting_address = "10.77.0.0"
  ending_address   = "10.77.0.255"
}

resource "jamfplatform_pro_network_segment" "exclusion" {
  name             = %q
  starting_address = "10.76.0.0"
  ending_address   = "10.76.0.255"
}

resource "jamfplatform_pro_ibeacon" "fixture" {
  name                    = %q
  uuid                    = "759b0599-64e0-416a-8d31-d8e93482a4d7"
  include_any_major_value = true
  include_any_minor_value = true
}

resource "jamfplatform_pro_script" "fixture" {
  name            = %q
  priority        = "AFTER"
  info            = "tf-acc omit-retains script fixture"
  script_contents = <<-EOT
    #!/bin/sh
    echo "tf-acc-omit-retains"
  EOT
}

resource "jamfplatform_pro_printer" "fixture" {
  name = %q
  uri  = "ipp://10.1.20.121/"
}

resource "jamfplatform_pro_dock_item" "fixture" {
  name = %q
  type = "App"
  path = "/Applications/Calculator.app"
}

resource "jamfplatform_pro_disk_encryption_configuration" "fixture" {
  name                     = %q
  key_type                 = "Individual"
  file_vault_enabled_users = "Current or Next User"
}

resource "jamfplatform_pro_directory_binding" "fixture" {
  name     = %q
  priority = 1
  type     = "Open Directory"
  domain   = "ldap.tf-acc.example.com"
  username = "cn=joiner,dc=tf-acc,dc=example,dc=com"
  password = "Sup3rS3cret!"

  open_directory = {
    encrypt_using_ssl      = false
    perform_secure_bind    = false
    use_for_authentication = true
    use_for_contacts       = false
  }
}
`, n.Category, n.Building, n.Department, n.DeviceGroup, n.SegmentLimitation, n.SegmentExclusion, n.Ibeacon, n.Script, n.Printer, n.DockItem, n.Dec, n.DirectoryBinding)
}

// policyOmitRetainsDependsOn pins the policy after every fixture in all three
// steps. Once a step stops declaring a reference the server still holds it,
// which is the very contract under test, so without this edge Terraform's
// destroy would try to remove the device group and disk encryption
// configuration while the live policy still points at them and both deletes
// are refused as HAS_DEPENDENCIES.
const policyOmitRetainsDependsOn = `
  depends_on = [
    jamfplatform_pro_category.fixture,
    jamfplatform_pro_building.fixture,
    jamfplatform_pro_department.fixture,
    jamfplatform_device_group.fixture,
    jamfplatform_pro_network_segment.limitation,
    jamfplatform_pro_network_segment.exclusion,
    jamfplatform_pro_ibeacon.fixture,
    jamfplatform_pro_script.fixture,
    jamfplatform_pro_printer.fixture,
    jamfplatform_pro_dock_item.fixture,
    jamfplatform_pro_disk_encryption_configuration.fixture,
    jamfplatform_pro_directory_binding.fixture,
  ]
`

// policyOmitRetainsGeneralScalars is the general block every step keeps.
// frequency=Ongoing forbids retry configuration, so the retry pair is pinned
// to its cleared form and category/site to NONE, making the block independent
// of tenant inventory and stable across the three steps.
const policyOmitRetainsGeneralScalars = `
    enabled        = true
    frequency      = "Ongoing"
    retry_event    = "none"
    retry_attempts = -1
    category_id    = "-1"
    site_id        = "-1"
`

// policyOmitRetainsConfig is the fully declared shape for the omit-retains
// contract: every state-gated block, sub-block and collection carries a
// distinctive non-default value so a server that stopped retaining an
// omitted element is caught on content, not on presence. packages and
// self_service.self_service_icon are the two gated sections left out —
// both need an out-of-band upload (JCDS, icon bytes) this test cannot supply.
// Four general leaves are left undeclared because wire probes on 2026-09-06
// showed the server ignores them on both POST and PUT and echoes its own
// value, which would fail step 1 on content rather than on retention.
//
// network_limitations and override_default_settings are declared EMPTY. Every
// child of both is Computed-only — Jamf Pro derives each from a setting that
// lives elsewhere and refuses a write — so declaring the block is the whole
// gesture, and the wire assertions below check the projections rather than
// anything this configuration set. printers.leave_existing_default is gone
// altogether, having modelled a dead wire element. Both changes landed in #387.
//
// The four self_service notification leaves are declared but not asserted: the
// classic GET echoes them only while the tenant-level Self Service
// notifications toggle is on, so an acceptance test must not depend on them.
func policyOmitRetainsConfig(n policyOmitRetainsNames) string {
	return policyOmitRetainsFixtures(n) + fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
%s
    date_time_limitations = {
      activation_date = "2026-02-03 04:05:06"
      expiration_date = "2027-02-03 04:05:06"
      no_execute_on   = ["Tue", "Thu"]
    }
    network_limitations       = {}
    override_default_settings = {}
  }
  scope = {
    targets = {
      computer_group_ids = [jamfplatform_device_group.fixture.jamf_pro_id]
      building_ids       = [jamfplatform_pro_building.fixture.id]
    }
    limitations = {
      network_segment_ids = [jamfplatform_pro_network_segment.limitation.id]
      ibeacon_ids         = [jamfplatform_pro_ibeacon.fixture.id]
    }
    exclusions = {
      department_ids      = [jamfplatform_pro_department.fixture.id]
      network_segment_ids = [jamfplatform_pro_network_segment.exclusion.id]
    }
  }
  self_service = {
    use_for_self_service          = true
    self_service_display_name     = "Omit-retains display name"
    install_button_text           = "Retain me"
    reinstall_button_text         = "Retain me again"
    self_service_description      = "Omit-retains contract description."
    ensure_users_view_description = true
    include_in_featured_category  = true
    display_notifications         = true
    notification_location         = "Self Service and Notification Center"
    notification_subject          = "Omit-retains subject"
    notification_message          = "Omit-retains message"
    categories = [
      {
        id         = jamfplatform_pro_category.fixture.id
        display_in = true
        feature_in = true
      },
    ]
  }
  scripts = {
    scripts = [
      {
        id           = jamfplatform_pro_script.fixture.id
        priority     = "Before"
        parameter_4  = "omit-retains-p4"
        parameter_5  = "omit-retains-p5"
        parameter_11 = "omit-retains-p11"
      },
    ]
  }
  printers = {
    printers = [
      {
        id           = jamfplatform_pro_printer.fixture.id
        action       = "Map"
        make_default = true
      },
    ]
  }
  dock_items = {
    dock_items = [
      {
        id     = jamfplatform_pro_dock_item.fixture.id
        action = "Add To Beginning"
      },
    ]
  }
  local_accounts = [
    {
      action               = "Create"
      username             = "tf-acc-omit"
      realname             = "Omit Retains"
      password             = "Sup3rS3cret!"
      password_wo_version  = 1
      home                 = "/Users/tf-acc-omit"
      hint                 = "omit-retains hint"
      admin                = true
      filevault_enabled    = false
      secure_token_allowed = true
    },
  ]
  directory_bindings = [
    {
      id = jamfplatform_pro_directory_binding.fixture.id
    },
  ]
  management_account = {
    action                      = "rotate"
    managed_password            = "Sup3rS3cret!"
    managed_password_wo_version = 1
  }
  efi_password = {
    of_mode                = "command"
    of_password            = "OF-omit-retains-1!"
    of_password_wo_version = 1
  }
  restart_options = {
    message                        = "Omit-retains reboot message"
    startup_disk                   = "Current Startup Disk"
    specify_startup                = "Standard Restart"
    no_user_logged_in              = "Restart immediately"
    user_logged_in                 = "Restart if a package or update requires it"
    delay_minutes                  = 7
    start_reboot_timer_immediately = true
    file_vault_2_reboot            = true
  }
  maintenance = {
    update_inventory        = false
    reset_computer_names    = true
    install_cached_packages = true
    fix_disk_permissions    = false
    fix_byhost_files        = true
    flush_system_caches     = false
    flush_user_caches       = true
    verify_startup_disk     = true
  }
  files_and_processes = {
    search_by_path         = "/tmp/omit-retains"
    delete_file_if_found   = true
    search_by_filename     = "omit-retains.txt"
    update_locate_database = true
    search_by_spotlight    = "omit-retains-spotlight"
    search_for_process     = "omitretainsd"
    kill_process_if_found  = true
    execute_command        = "echo omit-retains"
  }
  user_interaction = {
    start_message    = "Omit-retains start message"
    deferral_type    = "duration"
    deferral_days    = 3
    complete_message = "Omit-retains complete message"
  }
  disk_encryption = {
    action                           = "apply"
    disk_encryption_configuration_id = tonumber(jamfplatform_pro_disk_encryption_configuration.fixture.id)
    auth_restart                     = true
  }
%s}
`, n.Policy, policyOmitRetainsGeneralScalars, policyOmitRetainsDependsOn)
}

// policyOmitRetainsParentsOnlyConfig keeps every parent that has a state-gated
// child and drops the child: general without its three sub-blocks, scope with
// targets but no limitations or exclusions, self_service without categories,
// and management_account as the sole survivor of the four account-maintenance
// peers so the PUT carries an <account_maintenance> element that names only
// <management_account>. It also drops the Optional+Computed leaf
// self_service.install_button_text. Every other optional block is omitted.
func policyOmitRetainsParentsOnlyConfig(n policyOmitRetainsNames) string {
	return policyOmitRetainsFixtures(n) + fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
%s
  }
  scope = {
    targets = {
      computer_group_ids = [jamfplatform_device_group.fixture.jamf_pro_id]
      building_ids       = [jamfplatform_pro_building.fixture.id]
    }
  }
  self_service = {
    use_for_self_service          = true
    self_service_display_name     = "Omit-retains display name"
    reinstall_button_text         = "Retain me again"
    self_service_description      = "Omit-retains contract description."
    ensure_users_view_description = true
    include_in_featured_category  = true
    display_notifications         = true
    notification_location         = "Self Service and Notification Center"
    notification_subject          = "Omit-retains subject"
    notification_message          = "Omit-retains message"
  }
  management_account = {
    action                      = "rotate"
    managed_password_wo_version = 1
  }
%s}
`, n.Policy, policyOmitRetainsGeneralScalars, policyOmitRetainsDependsOn)
}

// policyOmitRetainsGeneralOnlyConfig drops every optional block, so the PUT
// carries <general> alone.
func policyOmitRetainsGeneralOnlyConfig(n policyOmitRetainsNames) string {
	return policyOmitRetainsFixtures(n) + fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
%s
  }
%s}
`, n.Policy, policyOmitRetainsGeneralScalars, policyOmitRetainsDependsOn)
}

// requireSingleNamed asserts a wire collection holds exactly one element and
// that its name is the fixture the step-1 config referenced. Names are used
// because Jamf echoes them for every referenced object and the config helper
// cannot know server-assigned ids.
func requireSingleNamed[T any](field string, items *[]T, name func(T) *string, want string) error {
	if items == nil || len(*items) != 1 {
		return fmt.Errorf("%s: want exactly one element named %q, got %+v", field, want, items)
	}
	return testhelpers.RequireEqual(field+"[0].name", want, testhelpers.Deref(name((*items)[0])))
}

func idNameName(i proclassic.IDName) *string { return i.Name }

// policyRetainedOnServer asserts the server's copy still carries every value
// the omit-retains config declared in its first step. The disk-encryption
// configuration id is read from the fixture's Terraform state because that is
// the one reference the wire echoes as a bare id.
func policyRetainedOnServer(t *testing.T, n policyOmitRetainsNames) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return func(s *terraform.State) error {
		dec, ok := s.RootModule().Resources[policyOmitRetainsDecAddr]
		if !ok {
			return fmt.Errorf("resource %s not found in state", policyOmitRetainsDecAddr)
		}
		wantDecID := dec.Primary.ID
		return testhelpers.CheckLiveObject(policyOmitRetainsResourceAddr,
			func(ctx context.Context, id string) (*proclassic.Policy, error) {
				return c.GetPolicyByID(ctx, id)
			},
			func(p *proclassic.Policy) error {
				if err := policyGeneralRetained(p.General); err != nil {
					return err
				}
				if err := policyScopeRetained(p.Scope, n); err != nil {
					return err
				}
				if err := policySelfServiceRetained(p.SelfService, n); err != nil {
					return err
				}
				if err := policyLinkedObjectsRetained(p, n); err != nil {
					return err
				}
				if err := policyAccountMaintenanceRetained(p.AccountMaintenance, n); err != nil {
					return err
				}
				return policyOptionsRetained(p, wantDecID)
			})(s)
	}
}

func policyGeneralRetained(g *proclassic.PolicyGeneral) error {
	if g == nil {
		return fmt.Errorf("general: absent")
	}
	dtl := g.DateTimeLimitations
	if dtl == nil {
		return fmt.Errorf("general.date_time_limitations: absent")
	}
	if err := testhelpers.RequireEqual("general.date_time_limitations.activation_date", "2026-02-03 04:05:06", testhelpers.Deref(dtl.ActivationDate)); err != nil {
		return err
	}
	if err := testhelpers.RequireEqual("general.date_time_limitations.expiration_date", "2027-02-03 04:05:06", testhelpers.Deref(dtl.ExpirationDate)); err != nil {
		return err
	}
	if dtl.NoExecuteOn == nil || dtl.NoExecuteOn.Day == nil || len(*dtl.NoExecuteOn.Day) != 2 {
		return fmt.Errorf("general.date_time_limitations.no_execute_on: want [Tue Thu], got %+v", dtl.NoExecuteOn)
	}
	days := strings.Join(*dtl.NoExecuteOn.Day, ",")
	if days != "Tue,Thu" && days != "Thu,Tue" {
		return fmt.Errorf("general.date_time_limitations.no_execute_on: want [Tue Thu], got %q", days)
	}
	nl := g.NetworkLimitations
	if nl == nil {
		return fmt.Errorf("general.network_limitations: absent")
	}
	if err := testhelpers.RequireEqual("general.network_limitations.minimum_network_connection", "No Minimum", testhelpers.Deref(nl.MinimumNetworkConnection)); err != nil {
		return err
	}
	if err := testhelpers.RequireEqual("general.network_limitations.any_ip_address", false, testhelpers.Deref(nl.AnyIPAddress)); err != nil {
		return err
	}
	ods := g.OverrideDefaultSettings
	if ods == nil {
		return fmt.Errorf("general.override_default_settings: absent")
	}
	checks := []error{
		testhelpers.RequireEqual("general.override_default_settings.target_drive", "/", testhelpers.Deref(ods.TargetDrive)),
		testhelpers.RequireEqual("general.override_default_settings.distribution_point", "default", testhelpers.Deref(ods.DistributionPoint)),
		testhelpers.RequireEqual("general.override_default_settings.force_afp_smb", false, testhelpers.Deref(ods.ForceAfpSmb)),
		testhelpers.RequireEqual("general.override_default_settings.sus", "default", testhelpers.Deref(ods.Sus)),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}

func policyScopeRetained(s *proclassic.PolicyScope, n policyOmitRetainsNames) error {
	if s == nil {
		return fmt.Errorf("scope: absent")
	}
	if s.ComputerGroups == nil || s.Buildings == nil {
		return fmt.Errorf("scope.targets: computer_groups or buildings absent")
	}
	if err := requireSingleNamed("scope.computer_groups", s.ComputerGroups.ComputerGroup, idNameName, n.DeviceGroup); err != nil {
		return err
	}
	if err := requireSingleNamed("scope.buildings", s.Buildings.Building, idNameName, n.Building); err != nil {
		return err
	}
	if s.Limitations == nil || s.Limitations.NetworkSegments == nil || s.Limitations.Ibeacons == nil {
		return fmt.Errorf("scope.limitations: want network_segments and ibeacons, got %+v", s.Limitations)
	}
	if err := requireSingleNamed("scope.limitations.network_segments", s.Limitations.NetworkSegments.NetworkSegment, idNameName, n.SegmentLimitation); err != nil {
		return err
	}
	if err := requireSingleNamed("scope.limitations.ibeacons", s.Limitations.Ibeacons.Ibeacon, idNameName, n.Ibeacon); err != nil {
		return err
	}
	if s.Exclusions == nil || s.Exclusions.Departments == nil || s.Exclusions.NetworkSegments == nil {
		return fmt.Errorf("scope.exclusions: want departments and network_segments, got %+v", s.Exclusions)
	}
	if err := requireSingleNamed("scope.exclusions.departments", s.Exclusions.Departments.Department, idNameName, n.Department); err != nil {
		return err
	}
	return requireSingleNamed("scope.exclusions.network_segments", s.Exclusions.NetworkSegments.NetworkSegment,
		func(i proclassic.PolicyScopeExclusionsNetworkSegmentsNetworkSegmentItem) *string { return i.Name }, n.SegmentExclusion)
}

func policySelfServiceRetained(ss *proclassic.PolicySelfService, n policyOmitRetainsNames) error {
	if ss == nil {
		return fmt.Errorf("self_service: absent")
	}
	checks := []error{
		testhelpers.RequireEqual("self_service.use_for_self_service", true, testhelpers.Deref(ss.UseForSelfService)),
		testhelpers.RequireEqual("self_service.self_service_display_name", "Omit-retains display name", testhelpers.Deref(ss.SelfServiceDisplayName)),
		testhelpers.RequireEqual("self_service.install_button_text", "Retain me", testhelpers.Deref(ss.InstallButtonText)),
		testhelpers.RequireEqual("self_service.reinstall_button_text", "Retain me again", testhelpers.Deref(ss.ReinstallButtonText)),
		testhelpers.RequireEqual("self_service.self_service_description", "Omit-retains contract description.", testhelpers.Deref(ss.SelfServiceDescription)),
		testhelpers.RequireEqual("self_service.force_users_to_view_description", true, testhelpers.Deref(ss.ForceUsersToViewDescription)),
		testhelpers.RequireEqual("self_service.feature_on_main_page", true, testhelpers.Deref(ss.FeatureOnMainPage)),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	if ss.SelfServiceCategories == nil {
		return fmt.Errorf("self_service.self_service_categories: absent")
	}
	if err := requireSingleNamed("self_service.self_service_categories", ss.SelfServiceCategories.Category,
		func(c proclassic.PolicySelfServiceSelfServiceCategoriesCategoryItem) *string { return c.Name }, n.Category); err != nil {
		return err
	}
	cat := (*ss.SelfServiceCategories.Category)[0]
	if err := testhelpers.RequireEqual("self_service.self_service_categories[0].display_in", true, testhelpers.Deref(cat.DisplayIn)); err != nil {
		return err
	}
	return testhelpers.RequireEqual("self_service.self_service_categories[0].feature_in", true, testhelpers.Deref(cat.FeatureIn))
}

func policyLinkedObjectsRetained(p *proclassic.Policy, n policyOmitRetainsNames) error {
	if p.Scripts == nil {
		return fmt.Errorf("scripts: absent")
	}
	if err := requireSingleNamed("scripts", p.Scripts.Script, func(s proclassic.PolicyScriptsScriptItem) *string { return s.Name }, n.Script); err != nil {
		return err
	}
	sc := (*p.Scripts.Script)[0]
	checks := []error{
		testhelpers.RequireEqual("scripts[0].priority", "Before", testhelpers.Deref(sc.Priority)),
		testhelpers.RequireEqual("scripts[0].parameter4", "omit-retains-p4", testhelpers.Deref(sc.Parameter4)),
		testhelpers.RequireEqual("scripts[0].parameter5", "omit-retains-p5", testhelpers.Deref(sc.Parameter5)),
		testhelpers.RequireEqual("scripts[0].parameter11", "omit-retains-p11", testhelpers.Deref(sc.Parameter11)),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	if p.Printers == nil {
		return fmt.Errorf("printers: absent")
	}
	if err := requireSingleNamed("printers", p.Printers.Printer, func(pr proclassic.PolicyPrintersPrinterItem) *string { return pr.Name }, n.Printer); err != nil {
		return err
	}
	pr := (*p.Printers.Printer)[0]
	if err := testhelpers.RequireEqual("printers[0].action", proclassic.PolicyPrintersPrinterItemActionInstall, testhelpers.Deref(pr.Action)); err != nil {
		return err
	}
	if err := testhelpers.RequireEqual("printers[0].make_default", true, testhelpers.Deref(pr.MakeDefault)); err != nil {
		return err
	}
	if p.DockItems == nil {
		return fmt.Errorf("dock_items: absent")
	}
	if err := requireSingleNamed("dock_items", p.DockItems.DockItem, func(d proclassic.PolicyDockItemsDockItemItem) *string { return d.Name }, n.DockItem); err != nil {
		return err
	}
	return testhelpers.RequireEqual("dock_items[0].action", "Add To Beginning", testhelpers.Deref((*p.DockItems.DockItem)[0].Action))
}

func policyAccountMaintenanceRetained(am *proclassic.PolicyAccountMaintenance, n policyOmitRetainsNames) error {
	if am == nil {
		return fmt.Errorf("account_maintenance: absent")
	}
	if am.Accounts == nil {
		return fmt.Errorf("account_maintenance.accounts: absent")
	}
	if err := requireSingleNamed("account_maintenance.accounts", am.Accounts.Account,
		func(a proclassic.PolicyAccountMaintenanceAccountsAccountItem) *string { return a.Username }, "tf-acc-omit"); err != nil {
		return err
	}
	acct := (*am.Accounts.Account)[0]
	checks := []error{
		testhelpers.RequireEqual("account_maintenance.accounts[0].action", "Create", testhelpers.Deref(acct.Action)),
		testhelpers.RequireEqual("account_maintenance.accounts[0].realname", "Omit Retains", testhelpers.Deref(acct.Realname)),
		testhelpers.RequireEqual("account_maintenance.accounts[0].home", "/Users/tf-acc-omit", testhelpers.Deref(acct.Home)),
		testhelpers.RequireEqual("account_maintenance.accounts[0].hint", "omit-retains hint", testhelpers.Deref(acct.Hint)),
		testhelpers.RequireEqual("account_maintenance.accounts[0].admin", true, testhelpers.Deref(acct.Admin)),
		testhelpers.RequireEqual("account_maintenance.accounts[0].secure_token_allowed", true, testhelpers.Deref(acct.SecureTokenAllowed)),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	if am.DirectoryBindings == nil {
		return fmt.Errorf("account_maintenance.directory_bindings: absent")
	}
	if err := requireSingleNamed("account_maintenance.directory_bindings", am.DirectoryBindings.Binding, idNameName, n.DirectoryBinding); err != nil {
		return err
	}
	if am.ManagementAccount == nil {
		return fmt.Errorf("account_maintenance.management_account: absent")
	}
	if err := testhelpers.RequireEqual("account_maintenance.management_account.action", proclassic.PolicyAccountMaintenanceManagementAccountActionRotate, testhelpers.Deref(am.ManagementAccount.Action)); err != nil {
		return err
	}
	if am.OpenFirmwareEfiPassword == nil {
		return fmt.Errorf("account_maintenance.open_firmware_efi_password: absent")
	}
	return testhelpers.RequireEqual("account_maintenance.open_firmware_efi_password.of_mode", proclassic.PolicyAccountMaintenanceOpenFirmwareEfiPasswordOfModeCommand, testhelpers.Deref(am.OpenFirmwareEfiPassword.OfMode))
}

func policyOptionsRetained(p *proclassic.Policy, wantDecID string) error {
	r := p.Reboot
	if r == nil {
		return fmt.Errorf("reboot: absent")
	}
	checks := []error{
		testhelpers.RequireEqual("reboot.message", "Omit-retains reboot message", testhelpers.Deref(r.Message)),
		testhelpers.RequireEqual("reboot.startup_disk", "Current Startup Disk", testhelpers.Deref(r.StartupDisk)),
		testhelpers.RequireEqual("reboot.specify_startup", "Standard Restart", testhelpers.Deref(r.SpecifyStartup)),
		testhelpers.RequireEqual("reboot.no_user_logged_in", "Restart immediately", testhelpers.Deref(r.NoUserLoggedIn)),
		testhelpers.RequireEqual("reboot.user_logged_in", "Restart if a package or update requires it", testhelpers.Deref(r.UserLoggedIn)),
		testhelpers.RequireEqual("reboot.minutes_until_reboot", 7, testhelpers.Deref(r.MinutesUntilReboot)),
		testhelpers.RequireEqual("reboot.start_reboot_timer_immediately", true, testhelpers.Deref(r.StartRebootTimerImmediately)),
		testhelpers.RequireEqual("reboot.file_vault_2_reboot", true, testhelpers.Deref(r.FileVault2Reboot)),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	m := p.Maintenance
	if m == nil {
		return fmt.Errorf("maintenance: absent")
	}
	checks = []error{
		testhelpers.RequireEqual("maintenance.recon", false, testhelpers.Deref(m.Recon)),
		testhelpers.RequireEqual("maintenance.reset_name", true, testhelpers.Deref(m.ResetName)),
		testhelpers.RequireEqual("maintenance.install_all_cached_packages", true, testhelpers.Deref(m.InstallAllCachedPackages)),
		testhelpers.RequireEqual("maintenance.permissions", false, testhelpers.Deref(m.Permissions)),
		testhelpers.RequireEqual("maintenance.byhost", true, testhelpers.Deref(m.Byhost)),
		testhelpers.RequireEqual("maintenance.system_cache", false, testhelpers.Deref(m.SystemCache)),
		testhelpers.RequireEqual("maintenance.user_cache", true, testhelpers.Deref(m.UserCache)),
		testhelpers.RequireEqual("maintenance.verify", true, testhelpers.Deref(m.Verify)),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	fp := p.FilesProcesses
	if fp == nil {
		return fmt.Errorf("files_processes: absent")
	}
	checks = []error{
		testhelpers.RequireEqual("files_processes.search_by_path", "/tmp/omit-retains", testhelpers.Deref(fp.SearchByPath)),
		testhelpers.RequireEqual("files_processes.delete_file", true, testhelpers.Deref(fp.DeleteFile)),
		testhelpers.RequireEqual("files_processes.locate_file", "omit-retains.txt", testhelpers.Deref(fp.LocateFile)),
		testhelpers.RequireEqual("files_processes.update_locate_database", true, testhelpers.Deref(fp.UpdateLocateDatabase)),
		testhelpers.RequireEqual("files_processes.spotlight_search", "omit-retains-spotlight", testhelpers.Deref(fp.SpotlightSearch)),
		testhelpers.RequireEqual("files_processes.search_for_process", "omitretainsd", testhelpers.Deref(fp.SearchForProcess)),
		testhelpers.RequireEqual("files_processes.kill_process", true, testhelpers.Deref(fp.KillProcess)),
		testhelpers.RequireEqual("files_processes.run_command", "echo omit-retains", testhelpers.Deref(fp.RunCommand)),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	ui := p.UserInteraction
	if ui == nil {
		return fmt.Errorf("user_interaction: absent")
	}
	checks = []error{
		testhelpers.RequireEqual("user_interaction.message_start", "Omit-retains start message", testhelpers.Deref(ui.MessageStart)),
		testhelpers.RequireEqual("user_interaction.message_finish", "Omit-retains complete message", testhelpers.Deref(ui.MessageFinish)),
		testhelpers.RequireEqual("user_interaction.allow_users_to_defer", true, testhelpers.Deref(ui.AllowUsersToDefer)),
		testhelpers.RequireEqual("user_interaction.allow_deferral_minutes", 3*1440, testhelpers.Deref(ui.AllowDeferralMinutes)),
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	de := p.DiskEncryption
	if de == nil {
		return fmt.Errorf("disk_encryption: absent")
	}
	if err := testhelpers.RequireEqual("disk_encryption.action", "apply", testhelpers.Deref(de.Action)); err != nil {
		return err
	}
	if err := testhelpers.RequireEqual("disk_encryption.disk_encryption_configuration_id", wantDecID, fmt.Sprintf("%d", testhelpers.Deref(de.DiskEncryptionConfigurationID))); err != nil {
		return err
	}
	return testhelpers.RequireEqual("disk_encryption.auth_restart", true, testhelpers.Deref(de.AuthRestart))
}

// TestAccResource_ProPolicy_OmittedBlocksRetained pins the omit-retains
// contract the plan output cannot show. Dropping any state-gated block from
// config plans it as removed, the classic PUT omits the element, and the
// server is expected to keep every value; Terraform state cannot witness
// that, so the assertion is made against the wire object after every step.
// Step 1 declares every gated section this test can build without an upload
// (packages needs JCDS, self_service_icon needs icon bytes). Step 2 keeps
// the parents that have gated children and drops the children — so the scope
// travels through the granular merge, self_service goes out without
// categories, and <account_maintenance> names only <management_account>.
// Step 3 drops every optional block so the PUT carries <general> alone. Each
// step's implicit post-apply plan must be empty. If this test fails on
// content, the endpoint no longer merges for that element and nothing that
// suppresses the removal plan may ship for this resource.
func TestAccResource_ProPolicy_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	n := newPolicyOmitRetainsNames("tf-acc-policy-omit-" + suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyOmitRetainsConfig(n),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyOmitRetainsResourceAddr, "self_service.install_button_text", "Retain me"),
					resource.TestCheckResourceAttr(policyOmitRetainsResourceAddr, "self_service.categories.#", "1"),
					resource.TestCheckResourceAttr(policyOmitRetainsResourceAddr, "scope.exclusions.department_ids.#", "1"),
					policyRetainedOnServer(t, n),
				),
			},
			{
				Config: policyOmitRetainsParentsOnlyConfig(n),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyOmitRetainsResourceAddr, "self_service.install_button_text", "Retain me"),
					resource.TestCheckNoResourceAttr(policyOmitRetainsResourceAddr, "self_service.categories.#"),
					resource.TestCheckNoResourceAttr(policyOmitRetainsResourceAddr, "scope.exclusions.department_ids.#"),
					resource.TestCheckNoResourceAttr(policyOmitRetainsResourceAddr, "scope.limitations.ibeacon_ids.#"),
					resource.TestCheckNoResourceAttr(policyOmitRetainsResourceAddr, "general.date_time_limitations.activation_date"),
					resource.TestCheckNoResourceAttr(policyOmitRetainsResourceAddr, "local_accounts.#"),
					resource.TestCheckNoResourceAttr(policyOmitRetainsResourceAddr, "scripts.scripts.#"),
					policyRetainedOnServer(t, n),
				),
			},
			{
				Config: policyOmitRetainsGeneralOnlyConfig(n),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(policyOmitRetainsResourceAddr, "scope.targets.building_ids.#"),
					resource.TestCheckNoResourceAttr(policyOmitRetainsResourceAddr, "self_service.use_for_self_service"),
					resource.TestCheckNoResourceAttr(policyOmitRetainsResourceAddr, "management_account.action"),
					policyRetainedOnServer(t, n),
				),
			},
		},
	})
}

// TestAccPolicyResource_CreateAccountRequiresHome exercises the validator that
// replaces Jamf Pro's opaque refusal of a create-account entry with no home
// directory.
//
// Jamf Pro answers `409 Problem with create account fields` and names nothing —
// the same message it gives for several unrelated problems — so the failure was
// unattributable until the field was isolated by probe (adding `home` alone
// turns the identical request into a 201). Like the date-format rejections
// above, this fails at plan time with no API round-trip.
func TestAccPolicyResource_CreateAccountRequiresHome(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-policy-acct-home-" + suffix

	withoutHome := fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }

  local_accounts = [
    {
      action              = "Create"
      username            = "tf-acc-admin"
      realname            = "TF Acc Admin"
      password            = "Sup3rS3cret!"
      password_wo_version = 1
      admin               = true
    },
  ]
}
`, name)

	// A non-Create action needs no home, so the same shape must pass validation.
	resetWithoutHome := fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }

  local_accounts = [
    {
      action              = "Reset"
      username            = "tf-acc-admin"
      password            = "Sup3rS3cret!"
      password_wo_version = 1
    },
  ]
}
`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      withoutHome,
				ExpectError: regexp.MustCompile(`home required for a Create account action`),
			},
			{
				Config:             resetWithoutHome,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
