//go:build acceptance

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_prestage_enrollment_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// accCleanupOrphans registers a t.Cleanup that force-deletes any
// prestages or ADE fixtures whose display name contains the test's
// unique suffix. Runs unconditionally — Terraform's own
// `terraform destroy` step is the primary teardown, but mid-step
// failures (or destroy itself erroring) can leak fixtures on the
// tenant. The cleanup is idempotent: 404 / INVALID_ID are tolerated.
func accCleanupOrphans(t *testing.T, suffix string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		c := pro.New(testhelpers.NewAcceptanceClient(t))

		// Computer prestages — list, match displayName by suffix, DELETE.
		prestages, err := c.ListComputerPrestagesV3(ctx, nil)
		if err != nil {
			t.Logf("orphan cleanup: list computer prestages failed: %v", err)
		} else {
			for _, p := range prestages {
				if !strings.Contains(p.DisplayName, suffix) {
					continue
				}
				if err := c.DeleteComputerPrestageV3(ctx, p.ID); err != nil && !helpers.IsNotFoundError(err) {
					t.Logf("orphan cleanup: delete prestage %s (%s) failed: %v", p.ID, p.DisplayName, err)
				} else {
					t.Logf("orphan cleanup: deleted prestage %s (%s)", p.ID, p.DisplayName)
				}
			}
		}

		// ADE fixtures — list, match name by suffix, DELETE.
		ades, err := c.ListDeviceEnrollmentsV1(ctx, nil)
		if err != nil {
			t.Logf("orphan cleanup: list ADE instances failed: %v", err)
			return
		}
		for _, a := range ades {
			if a.ID == nil || a.Name == "" || !strings.Contains(a.Name, suffix) {
				continue
			}
			if err := c.DeleteDeviceEnrollmentV1(ctx, *a.ID); err != nil && !helpers.IsNotFoundError(err) {
				t.Logf("orphan cleanup: delete ADE %s (%s) failed: %v", *a.ID, a.Name, err)
			} else {
				t.Logf("orphan cleanup: deleted ADE %s (%s)", *a.ID, a.Name)
			}
		}
	})
}

const (
	resourceName  = "jamfplatform_pro_computer_prestage_enrollment.test"
	adeFixtureRef = "jamfplatform_pro_automated_device_enrollment.fixture.id"
)

// requireADETokenBlob skips the test when JAMFPLATFORM_ACC_PRO_DEP_TOKEN is not set.
// The env var must hold the base64-encoded `.p7m` server token downloaded
// from Apple Business Manager / Apple School Manager — the same token blob
// the sibling jamfplatform_pro_automated_device_enrollment acc tests
// consume. Each test creates an ADE fixture from this blob, builds a
// prestage that depends on it, and lets TF destroy both at the end.
func requireADETokenBlob(t *testing.T) string {
	t.Helper()
	v := testhelpers.AccEnv("JAMFPLATFORM_ACC_PRO_DEP_TOKEN")
	if v == "" {
		t.Skip("JAMFPLATFORM_ACC_PRO_DEP_TOKEN not set — acc tests upload a real ADE server token (.p7m base64) and create an ADE fixture before exercising the prestage.")
	}
	return v
}

// requireADESerialFixture skips the test when JAMFPLATFORM_ACC_PRO_DEP_SERIAL is
// not set. Holds a real device serial number bound to the uploaded ADE
// token; without it scope writes return DEVICE_DOES_NOT_EXIST_ON_TOKEN.
func requireADESerialFixture(t *testing.T) string {
	t.Helper()
	v := testhelpers.AccEnv("JAMFPLATFORM_ACC_PRO_DEP_SERIAL")
	if v == "" {
		t.Skip("JAMFPLATFORM_ACC_PRO_DEP_SERIAL not set — scope acc test requires a real ADE-bound device serial.")
	}
	return v
}

// requireADESerial2Fixture skips when JAMFPLATFORM_ACC_PRO_DEP_SERIAL2 is unset.
// A second real ADE-bound serial used by the multi-serial diff test
// (exercises the add+remove paths that drive scope_serial_numbers
// changes between non-empty sets).
func requireADESerial2Fixture(t *testing.T) string {
	t.Helper()
	v := testhelpers.AccEnv("JAMFPLATFORM_ACC_PRO_DEP_SERIAL2")
	if v == "" {
		t.Skip("JAMFPLATFORM_ACC_PRO_DEP_SERIAL2 not set — multi-serial diff acc test requires a second real ADE-bound device serial.")
	}
	return v
}

// adeFixtureBlock emits a Terraform resource block that creates an ADE
// instance from the supplied base64 token. Returned HCL is concatenated
// with the prestage block in each test config.
func adeFixtureBlock(suffix, tokenB64 string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_automated_device_enrollment" "fixture" {
  name                    = %q
  server_token            = %q
  server_token_wo_version = 1
}
`, "tf-acc-prestage-ade-"+suffix, tokenB64)
}

// importStateVerifyIgnore mirrors spike §13 — WriteOnly secrets and their
// `_wo_version` rotation triggers are never echoed back by Jamf Pro.
var importStateVerifyIgnore = []string{
	"timeouts",
	"recovery_lock_password",
	"recovery_lock_password_wo_version",
	"account_settings.admin_password",
	"account_settings.admin_password_wo_version",
	// profile_uuid is a server-generated MDM profile UUID that Jamf Pro
	// populates ASYNCHRONOUSLY (the "information out of date" window). A plain
	// GET can return it empty even on an otherwise-ready prestage, so its value
	// does not reliably round-trip on import — Step 1's check already documents
	// it as not-assertable at create time. It is not user-settable, so verifying
	// it on import adds no coverage; ignore it like the other non-round-tripping
	// attributes above.
	"profile_uuid",
}

// --- §13 #1: Minimal Create + Import + simple Update --------------------------

func TestAccResource_ProComputerPrestageEnrollment_Minimal(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-computer-prestage-minimal-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + testAccComputerPrestageMinimalConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "display_name", name),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "device_enrollment_program_instance_id"),
					// `profile_uuid` is server-generated but may be empty
					// immediately after Create on a freshly-uploaded ADE
					// fixture — Jamf populates it asynchronously. The
					// attribute exists in state; the value is not assertable
					// at this point in the lifecycle. Drift-recovery in a
					// later Read cycle catches any persistent emptiness.
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: importStateVerifyIgnore,
			},
			{
				Config: adeFixtureBlock(suffix, token) + testAccComputerPrestageMinimalConfig(name+"-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "display_name", name+"-renamed"),
				),
			},
		},
	})
}

// --- §13 #2: Full Update round-trip -----------------------------------------

func TestAccResource_ProComputerPrestageEnrollment_Full_UpdateRoundTrip(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	// require_authentication = true (V1) is rejected by Jamf Pro with
	// PREREQUISITE_NOT_MET unless a Directory Service is configured on the
	// tenant. Stand up the shared Okta LDAP fixture and order the prestage
	// after it via depends_on. The fixture HCL is included in BOTH steps so
	// TF does not destroy the directory service between Create and Update.
	ldapEnv := testhelpers.RequireOktaLdapEnv(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-computer-prestage-full-" + suffix
	ldapFixture := testhelpers.LdapServerFixture("tf-acc-prestage-ldap-"+suffix, ldapEnv)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + ldapFixture + testAccComputerPrestageFullConfigV1(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "display_name", name),
					resource.TestCheckResourceAttr(resourceName, "support_phone_number", "+44-1-555-0100"),
					resource.TestCheckResourceAttr(resourceName, "skip_setup_items.filevault", "true"),
					resource.TestCheckResourceAttr(resourceName, "location_information.username", "tf-acc-user"),
					resource.TestCheckResourceAttr(resourceName, "purchasing_information.apple_care_id", "AC-1"),
					resource.TestCheckResourceAttr(resourceName, "account_settings.payload_configured", "true"),
					resource.TestCheckResourceAttr(resourceName, "account_settings.admin_username", "ladmin1"),
				),
			},
			{
				Config: adeFixtureBlock(suffix, token) + ldapFixture + testAccComputerPrestageFullConfigV2(name+"-v2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "display_name", name+"-v2"),
					resource.TestCheckResourceAttr(resourceName, "support_phone_number", "+44-1-555-0200"),
					resource.TestCheckResourceAttr(resourceName, "skip_setup_items.filevault", "false"),
					resource.TestCheckResourceAttr(resourceName, "skip_setup_items.icloud_storage", "true"),
					resource.TestCheckResourceAttr(resourceName, "location_information.username", "tf-acc-user-v2"),
					resource.TestCheckResourceAttr(resourceName, "purchasing_information.life_expectancy", "5"),
					resource.TestCheckResourceAttr(resourceName, "account_settings.admin_username", "ladmin2"),
					resource.TestCheckResourceAttr(resourceName, "account_settings.admin_password_wo_version", "2"),
				),
			},
			// Step 3 — REMOVE all four Optional-only nested blocks that were
			// managed in steps 1-2. Before the gate-on-target fix this Update
			// crashed with "Provider produced inconsistent result after apply:
			// .location_information: was null, but now cty.ObjectVal(...)" (and
			// the WriteOnly-masked "inconsistent values for sensitive
			// attribute" on account_settings). A clean apply here is the
			// regression guard; the framework's implicit post-apply plan must
			// also be empty, proving the removed blocks stay null on refresh
			// rather than re-appearing as drift.
			{
				Config: adeFixtureBlock(suffix, token) + ldapFixture + testAccComputerPrestageBlocksRemovedConfig(name+"-v2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "display_name", name+"-v2"),
					resource.TestCheckNoResourceAttr(resourceName, "skip_setup_items.filevault"),
					resource.TestCheckNoResourceAttr(resourceName, "location_information.username"),
					resource.TestCheckNoResourceAttr(resourceName, "purchasing_information.apple_care_id"),
					resource.TestCheckNoResourceAttr(resourceName, "account_settings.admin_username"),
				),
			},
		},
	})
}

// --- §13 #3 / #4: Recovery Lock mutually-exclusive shapes -------------------

func TestAccResource_ProComputerPrestageEnrollment_RecoveryLockManual(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccComputerPrestageRecoveryLockManualConfig("tf-acc-rl-manual-"+suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable_recovery_lock", "true"),
					resource.TestCheckResourceAttr(resourceName, "recovery_lock_password_type", "MANUAL"),
				),
			},
		},
	})
}

func TestAccResource_ProComputerPrestageEnrollment_RecoveryLockRandom(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccComputerPrestageRecoveryLockRandomConfig("tf-acc-rl-random-"+suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enable_recovery_lock", "true"),
					resource.TestCheckResourceAttr(resourceName, "recovery_lock_password_type", "RANDOM"),
				),
			},
		},
	})
}

// --- §13 #5 / #6: AccountSettings prefill modes -----------------------------

func TestAccResource_ProComputerPrestageEnrollment_AccountSettingsPrefillCustom(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccComputerPrestagePrefillCustomConfig("tf-acc-prefill-custom-"+suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "account_settings.prefill_type", "CUSTOM"),
					resource.TestCheckResourceAttr(resourceName, "account_settings.prefill_account_full_name", "Acc Test"),
					resource.TestCheckResourceAttr(resourceName, "account_settings.prefill_account_user_name", "acctest"),
				),
			},
		},
	})
}

func TestAccResource_ProComputerPrestageEnrollment_AccountSettingsPrefillDeviceOwner(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccComputerPrestagePrefillDeviceOwnerConfig("tf-acc-prefill-device-owner-"+suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "account_settings.prefill_type", "DEVICE_OWNER"),
				),
			},
		},
	})
}

// --- §13 #7: PSSO disabled (default) -----------------------------------------

func TestAccResource_ProComputerPrestageEnrollment_NoPSSOFields(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccComputerPrestageMinimalConfig("tf-acc-no-psso-"+suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "psso_enabled", "false"),
				),
			},
		},
	})
}

// --- §13 #8: Scope round-trip ------------------------------------------------

func TestAccResource_ProComputerPrestageEnrollment_ScopeAssignments(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	serial := requireADESerialFixture(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-scope-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + testAccComputerPrestageScopeConfig(name, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "0"),
				),
			},
			{
				Config: adeFixtureBlock(suffix, token) + testAccComputerPrestageScopeConfig(name, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "1"),
				),
			},
			{
				Config: adeFixtureBlock(suffix, token) + testAccComputerPrestageScopeConfig(name, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "0"),
				),
			},
		},
	})
}

// --- §13 #8a: Scope multi-serial add/remove diff ---------------------------

// TestAccResource_ProComputerPrestageEnrollment_ScopeMultiSerialDiff
// exercises the (POST add + POST remove-multiple) diff logic in
// `applyScope` across every transition shape:
//
//	Step 1: [] → [s1]                  (pure add of one serial)
//	Step 2: [s1] → [s1, s2]            (pure add when scope is non-empty)
//	Step 3: [s1, s2] → [s2]            (pure remove of one of two serials)
//	Step 4: [s2] → [s1]                (combined add + remove in one apply)
//	Step 5: [s1] → []                  (pure remove of last serial)
//
// Gated on `JAMFPLATFORM_ACC_PRO_DEP_TOKEN`, `JAMFPLATFORM_ACC_PRO_DEP_SERIAL`, and
// `JAMFPLATFORM_ACC_PRO_DEP_SERIAL2` — all three must be set, both serials must
// be present on the uploaded ADE token, and neither serial may be
// scoped to any other PreStage at the time the test starts.
func TestAccResource_ProComputerPrestageEnrollment_ScopeMultiSerialDiff(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	s1 := requireADESerialFixture(t)
	s2 := requireADESerial2Fixture(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-scope-multi-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: [] → [s1]
			{
				Config: adeFixtureBlock(suffix, token) + testAccComputerPrestageMultiScopeConfig(name, []string{s1}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "1"),
					resource.TestCheckTypeSetElemAttr(resourceName, "scope_serial_numbers.*", s1),
				),
			},
			// Step 2: [s1] → [s1, s2]
			{
				Config: adeFixtureBlock(suffix, token) + testAccComputerPrestageMultiScopeConfig(name, []string{s1, s2}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "2"),
					resource.TestCheckTypeSetElemAttr(resourceName, "scope_serial_numbers.*", s1),
					resource.TestCheckTypeSetElemAttr(resourceName, "scope_serial_numbers.*", s2),
				),
			},
			// Step 3: [s1, s2] → [s2]  (pure remove)
			{
				Config: adeFixtureBlock(suffix, token) + testAccComputerPrestageMultiScopeConfig(name, []string{s2}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "1"),
					resource.TestCheckTypeSetElemAttr(resourceName, "scope_serial_numbers.*", s2),
				),
			},
			// Step 4: [s2] → [s1]  (combined add + remove)
			{
				Config: adeFixtureBlock(suffix, token) + testAccComputerPrestageMultiScopeConfig(name, []string{s1}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "1"),
					resource.TestCheckTypeSetElemAttr(resourceName, "scope_serial_numbers.*", s1),
				),
			},
			// Step 5: [s1] → []  (pure remove of last serial)
			{
				Config: adeFixtureBlock(suffix, token) + testAccComputerPrestageMultiScopeConfig(name, []string{}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "0"),
				),
			},
		},
	})
}

func testAccComputerPrestageMultiScopeConfig(name string, serials []string) string {
	parts := make([]string, 0, len(serials))
	for _, s := range serials {
		parts = append(parts, fmt.Sprintf("%q", s))
	}
	scope := "[" + strings.Join(parts, ", ") + "]"
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  location_information   = {}
  purchasing_information = {}
  account_settings       = {}

  scope_serial_numbers = %s
}
`, name, adeFixtureRef, scope)
}

// --- §13 #8b: Scope ALREADY_SCOPED conflict ---------------------------------

// TestAccResource_ProComputerPrestageEnrollment_ScopeAlreadyScopedConflict
// verifies the provider's user-facing error path when a serial is assigned
// to one PreStage and a second PreStage tries to claim the same serial.
// Jamf Pro enforces single-PreStage-per-serial with `400 ALREADY_SCOPED`;
// the provider rewraps that diagnostic with guidance.
func TestAccResource_ProComputerPrestageEnrollment_ScopeAlreadyScopedConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	serial := requireADESerialFixture(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: create both PreStages, scope serial to A only.
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccTwoComputerPrestagesScopeConflictConfig("tf-acc-conflict-"+suffix, serial, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_computer_prestage_enrollment.a", "scope_serial_numbers.#", "1"),
					resource.TestCheckResourceAttr("jamfplatform_pro_computer_prestage_enrollment.b", "scope_serial_numbers.#", "0"),
				),
			},
			// Step 2: try to scope the same serial to B without removing
			// from A. Jamf returns 400 ALREADY_SCOPED; the provider
			// surfaces a user-facing "scope conflict" error.
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccTwoComputerPrestagesScopeConflictConfig("tf-acc-conflict-"+suffix, serial, true),
				ExpectError: regexp.MustCompile(`scope conflict|ALREADY_SCOPED`),
				// scope conflict matches both the documented `400
				// ALREADY_SCOPED` path and the 500-empty-errors
				// fallback wording the provider emits when Jamf
				// returns the bug-shaped 500 instead of the proper
				// 400.
			},
		},
	})
}

// testAccTwoComputerPrestagesScopeConflictConfig emits two prestage blocks
// (a and b). When `bClaims` is false, only a is scoped to the serial; when
// true, both attempt to claim the same serial (and Jamf rejects b's
// attempt).
func testAccTwoComputerPrestagesScopeConflictConfig(nameBase, serial string, bClaims bool) string {
	bScope := "[]"
	if bClaims {
		bScope = fmt.Sprintf(`[%q]`, serial)
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "a" {
  display_name                          = "%s-a"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  location_information   = {}
  purchasing_information = {}
  account_settings       = {}

  scope_serial_numbers = [%q]
}

resource "jamfplatform_pro_computer_prestage_enrollment" "b" {
  display_name                          = "%s-b"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  location_information   = {}
  purchasing_information = {}
  account_settings       = {}

  scope_serial_numbers = %s

  depends_on = [jamfplatform_pro_computer_prestage_enrollment.a]
}
`, nameBase, adeFixtureRef, serial, nameBase, adeFixtureRef, bScope)
}

// --- §13 #9: anchor_certificates silent-rollback hard-error path ------------

func TestAccResource_ProComputerPrestageEnrollment_AnchorCertificatesRollback(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-anchor-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + testAccComputerPrestageMinimalConfig(name),
				Check:  resource.TestCheckResourceAttrSet(resourceName, "id"),
			},
			{
				Config:      adeFixtureBlock(suffix, token) + testAccComputerPrestageAnchorsConfig(name, `["dGVzdA=="]`),
				ExpectError: regexp.MustCompile(`did not commit|anchor_certificates`),
			},
		},
	})
}

// --- §13 #10: ExpectError per declared validator ---------------------------

func TestAccResource_ProComputerPrestageEnrollment_ExpectError_RandomConflictsWithPassword(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-random-with-pwd-%s"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  enable_recovery_lock              = true
  recovery_lock_password_type       = "RANDOM"
  recovery_lock_password            = "conflict-with-RANDOM"
  recovery_lock_password_wo_version = 1
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`conflicts with recovery_lock_password_type = RANDOM`),
			},
		},
	})
}

func TestAccResource_ProComputerPrestageEnrollment_ExpectError_PasswordRequiresEnable(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-pwd-disabled-%s"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  enable_recovery_lock              = false
  recovery_lock_password_type       = "MANUAL"
  recovery_lock_password            = "ShouldFailHere"
  recovery_lock_password_wo_version = 1
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`enable_recovery_lock = true`),
			},
		},
	})
}

func TestAccResource_ProComputerPrestageEnrollment_ExpectError_PrefillCustomRequiresNames(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-prefill-custom-missing-names-%s"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  account_settings = {
    payload_configured = true
    prefill_type       = "CUSTOM"
  }
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`prefill_account_(full|user)_name is required`),
			},
		},
	})
}

func TestAccResource_ProComputerPrestageEnrollment_ExpectError_PssoModesMutuallyExclusive(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-psso-conflict-%s"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  psso_enabled               = true
  platform_sso_app_bundle_id = "com.okta.mobile"
  psso_config_profile_id     = "123"
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`exclusive`),
			},
		},
	})
}

func TestAccResource_ProComputerPrestageEnrollment_ExpectError_AttendedPssoConflictsWithEnrollmentCustomization(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-attended-ec-conflict-%s"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  psso_enabled                = true
  psso_config_profile_id      = "123"
  enrollment_customization_id = "5"
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`enrollment customization|incompatible`),
			},
		},
	})
}

func TestAccResource_ProComputerPrestageEnrollment_ExpectError_BadRecoveryLockType(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-bad-rl-type-%s"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  recovery_lock_password_type = "BOGUS_VALUE"
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`must be one of:\s+\["MANUAL"\s+"RANDOM"\]`),
			},
		},
	})
}

func TestAccResource_ProComputerPrestageEnrollment_ExpectError_BadPrefillType(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-bad-prefill-%s"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  account_settings = {
    prefill_type = "UNKNOWN"
  }
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`must be one of:\s+\["CUSTOM"\s+"DEVICE_OWNER"\]`),
			},
		},
	})
}

func TestAccResource_ProComputerPrestageEnrollment_ExpectError_BadUserAccountType(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-bad-uat-%s"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  account_settings = {
    user_account_type = "ROOT"
  }
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`must be one of:\s+\["ADMINISTRATOR"\s+"STANDARD"\s+"SKIP"\]`),
			},
		},
	})
}

func TestAccResource_ProComputerPrestageEnrollment_ExpectError_BadMinOsTarget(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-bad-min-os-%s"
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  prestage_minimum_os_target_version_type = "LATEST_ONLY"
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`must be one of`),
			},
		},
	})
}

// --- Config helpers (prestage block only — ADE fixture is concatenated by caller) ---

func testAccComputerPrestageMinimalConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  location_information   = {}
  purchasing_information = {}
  account_settings       = {}
  skip_setup_items       = {}

  scope_serial_numbers = []
}
`, name, adeFixtureRef)
}

func testAccComputerPrestageFullConfigV1(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  default_prestage                      = false
  support_phone_number                  = "+44-1-555-0100"
  support_email_address                 = "ops@example.test"
  department                            = "Operations"
  require_authentication                = true
  authentication_prompt                 = "Welcome to acceptance"
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  skip_setup_items = {
    filevault      = true
    siri           = true
    icloud_storage = false
  }

  location_information = {
    username = "tf-acc-user"
    email    = "tf-acc@example.test"
  }

  purchasing_information = {
    purchased       = true
    apple_care_id   = "AC-1"
    life_expectancy = 3
  }

  account_settings = {
    payload_configured                           = true
    local_admin_account_enabled                  = true
    admin_username                               = "ladmin1"
    admin_password                               = "InitialP@ss1"
    admin_password_wo_version                    = 1
    user_account_type                            = "ADMINISTRATOR"
    prefill_primary_account_info_feature_enabled = true
    prefill_type                                 = "DEVICE_OWNER"
  }

  scope_serial_numbers = []

  depends_on = [%[3]s]
}
`, name, adeFixtureRef, testhelpers.LdapFixtureResourceAddr)
}

// testAccComputerPrestageBlocksRemovedConfig is V2 with every Optional-only
// nested block (skip_setup_items, location_information, purchasing_information,
// account_settings) OMITTED. Used to exercise the remove-on-update transition:
// blocks managed in prior steps disappear from config, so the plan is null for
// each and the provider must write null (not repopulate from the wire GET).
func testAccComputerPrestageBlocksRemovedConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = false
  mdm_removable                         = false
  default_prestage                      = false
  support_phone_number                  = "+44-1-555-0200"
  support_email_address                 = "newops@example.test"
  department                            = "NewOps"
  require_authentication                = false
  authentication_prompt                 = "Updated prompt"
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = true
  keep_existing_site_membership         = true
  auto_advance_setup                    = true
  install_profiles_during_setup         = false
  prevent_activation_lock               = true
  enable_device_based_activation_lock   = false

  custom_package_ids = []

  scope_serial_numbers = []

  depends_on = [%[3]s]
}
`, name, adeFixtureRef, testhelpers.LdapFixtureResourceAddr)
}

func testAccComputerPrestageFullConfigV2(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = false
  mdm_removable                         = false
  default_prestage                      = false
  support_phone_number                  = "+44-1-555-0200"
  support_email_address                 = "newops@example.test"
  department                            = "NewOps"
  require_authentication                = false
  authentication_prompt                 = "Updated prompt"
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = true
  keep_existing_site_membership         = true
  auto_advance_setup                    = true
  install_profiles_during_setup         = false
  prevent_activation_lock               = true
  enable_device_based_activation_lock   = false

  skip_setup_items = {
    filevault                   = false
    siri                        = false
    icloud_storage              = true
    additional_privacy_settings = true
  }

  location_information = {
    username = "tf-acc-user-v2"
    realname = "Updated User"
    email    = "tf-acc-v2@example.test"
  }

  purchasing_information = {
    purchased       = true
    apple_care_id   = "AC-2"
    life_expectancy = 5
    vendor          = "Acme"
  }

  account_settings = {
    payload_configured                           = true
    local_admin_account_enabled                  = true
    admin_username                               = "ladmin2"
    admin_password                               = "RotatedP@ss2"
    admin_password_wo_version                    = 2
    user_account_type                            = "ADMINISTRATOR"
    prefill_primary_account_info_feature_enabled = true
    prefill_type                                 = "DEVICE_OWNER"
  }

  custom_package_ids = []

  scope_serial_numbers = []

  depends_on = [%[3]s]
}
`, name, adeFixtureRef, testhelpers.LdapFixtureResourceAddr)
}

func testAccComputerPrestageRecoveryLockManualConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  enable_recovery_lock              = true
  recovery_lock_password_type       = "MANUAL"
  recovery_lock_password            = "RecoveryP@ss123"
  recovery_lock_password_wo_version = 1

  location_information   = {}
  purchasing_information = {}
  account_settings       = {}
}
`, name, adeFixtureRef)
}

func testAccComputerPrestageRecoveryLockRandomConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  enable_recovery_lock        = true
  recovery_lock_password_type = "RANDOM"

  location_information   = {}
  purchasing_information = {}
  account_settings       = {}
}
`, name, adeFixtureRef)
}

func testAccComputerPrestagePrefillCustomConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  account_settings = {
    payload_configured                           = true
    prefill_primary_account_info_feature_enabled = true
    prefill_type                                 = "CUSTOM"
    prefill_account_full_name                    = "Acc Test"
    prefill_account_user_name                    = "acctest"
  }

  location_information   = {}
  purchasing_information = {}
}
`, name, adeFixtureRef)
}

func testAccComputerPrestagePrefillDeviceOwnerConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  account_settings = {
    payload_configured                           = true
    prefill_primary_account_info_feature_enabled = true
    prefill_type                                 = "DEVICE_OWNER"
  }

  location_information   = {}
  purchasing_information = {}
}
`, name, adeFixtureRef)
}

func testAccComputerPrestageScopeConfig(name, serial string) string {
	scope := "[]"
	if serial != "" {
		scope = fmt.Sprintf(`[%q]`, serial)
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  location_information   = {}
  purchasing_information = {}
  account_settings       = {}

  scope_serial_numbers = %s
}
`, name, adeFixtureRef, scope)
}

func testAccComputerPrestageAnchorsConfig(name, anchors string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_computer_prestage_enrollment" "test" {
  display_name                          = %q
  mandatory                             = true
  mdm_removable                         = true
  require_authentication                = false
  device_enrollment_program_instance_id = %s
  keep_existing_location_information    = false
  keep_existing_site_membership         = false
  auto_advance_setup                    = false
  install_profiles_during_setup         = false
  prevent_activation_lock               = false
  enable_device_based_activation_lock   = false

  anchor_certificates = %s

  location_information   = {}
  purchasing_information = {}
  account_settings       = {}
}
`, name, adeFixtureRef, anchors)
}
