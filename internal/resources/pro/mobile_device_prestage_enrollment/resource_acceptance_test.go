//go:build acceptance

// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment_test

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

const (
	resourceName  = "jamfplatform_pro_mobile_device_prestage_enrollment.test"
	adeFixtureRef = "jamfplatform_pro_automated_device_enrollment.fixture.id"
)

// accCleanupOrphans registers a t.Cleanup that force-deletes any mobile-device
// prestages or ADE fixtures whose display name contains the test's unique
// suffix. Runs unconditionally — Terraform's own `terraform destroy` step is
// the primary teardown, but mid-step failures (or destroy itself erroring) can
// leak fixtures on the tenant. The cleanup is idempotent: 404 / INVALID_ID are
// tolerated.
func accCleanupOrphans(t *testing.T, suffix string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		c := pro.New(testhelpers.NewAcceptanceClient(t))

		// Mobile device prestages — list, match displayName by suffix, DELETE.
		prestages, err := c.ListMobileDevicePrestagesV3(ctx, nil)
		if err != nil {
			t.Logf("orphan cleanup: list mobile device prestages failed: %v", err)
		} else {
			for _, p := range prestages {
				if !strings.Contains(p.DisplayName, suffix) {
					continue
				}
				if err := c.DeleteMobileDevicePrestageV3(ctx, p.ID); err != nil && !helpers.IsNotFoundError(err) {
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

// requireADETokenBlob skips the test when JAMFPLATFORM_ACC_PRO_DEP_TOKEN is not set.
// The env var must hold the base64-encoded `.p7m` server token downloaded from
// Apple Business Manager / Apple School Manager — the same token blob the
// sibling jamfplatform_pro_automated_device_enrollment acc tests consume. Each
// test creates an ADE fixture from this blob, builds a prestage that depends on
// it, and lets TF destroy both at the end.
func requireADETokenBlob(t *testing.T) string {
	t.Helper()
	v := testhelpers.AccEnv("JAMFPLATFORM_ACC_PRO_DEP_TOKEN")
	if v == "" {
		t.Skip("JAMFPLATFORM_ACC_PRO_DEP_TOKEN not set — acc tests upload a real ADE server token (.p7m base64) and create an ADE fixture before exercising the prestage.")
	}
	return v
}

// requireADESerialFixture skips the test when JAMFPLATFORM_ACC_PRO_DEP_MOBILE_SERIAL is
// not set. Holds a real mobile-device serial number bound to the uploaded ADE
// token; without it scope writes return DEVICE_DOES_NOT_EXIST_ON_TOKEN. Mobile
// uses its own env var (distinct from the computer prestage's
// JAMFPLATFORM_ACC_PRO_DEP_SERIAL) because mobile-device serials differ from computer
// serials on the same token.
func requireADESerialFixture(t *testing.T) string {
	t.Helper()
	v := testhelpers.AccEnv("JAMFPLATFORM_ACC_PRO_DEP_MOBILE_SERIAL")
	if v == "" {
		t.Skip("JAMFPLATFORM_ACC_PRO_DEP_MOBILE_SERIAL not set — scope acc test requires a real ADE-bound mobile-device serial.")
	}
	return v
}

// requireADEMultiSerialsFixture skips unless JAMFPLATFORM_ACC_PRO_DEP_MOBILE_SERIALS
// holds at least `min` comma-separated mobile-device serials bound to the ADE
// token. Used by the multi-serial scope round-trip test.
func requireADEMultiSerialsFixture(t *testing.T, min int) []string {
	t.Helper()
	raw := testhelpers.AccEnv("JAMFPLATFORM_ACC_PRO_DEP_MOBILE_SERIALS")
	if raw == "" {
		t.Skipf("JAMFPLATFORM_ACC_PRO_DEP_MOBILE_SERIALS not set — multi-serial scope acc test requires %d comma-separated ADE-bound mobile-device serials.", min)
	}
	var serials []string
	for s := range strings.SplitSeq(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			serials = append(serials, s)
		}
	}
	if len(serials) < min {
		t.Skipf("JAMFPLATFORM_ACC_PRO_DEP_MOBILE_SERIALS has %d serials; multi-serial scope acc test needs at least %d.", len(serials), min)
	}
	return serials
}

// adeFixtureBlock emits a Terraform resource block that creates an ADE instance
// from the supplied base64 token. Returned HCL is concatenated with the
// prestage block in each test config.
func adeFixtureBlock(suffix, tokenB64 string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_automated_device_enrollment" "fixture" {
  name                    = %q
  server_token            = %q
  server_token_wo_version = 1
}
`, "tf-acc-mdprestage-ade-"+suffix, tokenB64)
}

// importStateVerifyIgnore mirrors spike §13 — mobile prestages carry NO
// WriteOnly secrets, so only `timeouts` is ignored.
var importStateVerifyIgnore = []string{
	"timeouts",
}

// --- §13 #1: Minimal Create + Import + simple Update ------------------------

func TestAccResource_ProMobileDevicePrestageEnrollment_Minimal(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-mobile-prestage-minimal-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageMinimalConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "display_name", name),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "device_enrollment_program_instance_id"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: importStateVerifyIgnore,
			},
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageMinimalConfig(name+"-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "display_name", name+"-renamed"),
				),
			},
		},
	})
}

// --- §13 #2: Full Update round-trip (incl. prestage_device_names add+remove) -

func TestAccResource_ProMobileDevicePrestageEnrollment_Full_UpdateRoundTrip(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	// require_authentication = true (config V1) needs a Directory Service on the
	// tenant or Create returns PREREQUISITE_NOT_MET; stand up the shared Okta
	// LDAP server fixture and depend on it.
	ldapEnv := testhelpers.RequireOktaLdapEnv(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-mobile-prestage-full-" + suffix
	ldapFixture := testhelpers.LdapServerFixture(name, ldapEnv)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: ldapFixture + adeFixtureBlock(suffix, token) + testAccMobilePrestageFullConfigV1(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "display_name", name),
					resource.TestCheckResourceAttr(resourceName, "support_phone_number", "+44-1-555-0100"),
					resource.TestCheckResourceAttr(resourceName, "skip_setup_items.biometric", "true"),
					resource.TestCheckResourceAttr(resourceName, "location_information.username", "tf-acc-user"),
					resource.TestCheckResourceAttr(resourceName, "purchasing_information.apple_care_id", "AC-1"),
					resource.TestCheckResourceAttr(resourceName, "names.assign_names_using", "List of Names"),
					resource.TestCheckResourceAttr(resourceName, "names.prestage_device_names.#", "2"),
				),
			},
			{
				Config: ldapFixture + adeFixtureBlock(suffix, token) + testAccMobilePrestageFullConfigV2(name+"-v2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "display_name", name+"-v2"),
					resource.TestCheckResourceAttr(resourceName, "support_phone_number", "+44-1-555-0200"),
					resource.TestCheckResourceAttr(resourceName, "skip_setup_items.biometric", "false"),
					resource.TestCheckResourceAttr(resourceName, "skip_setup_items.siri", "true"),
					resource.TestCheckResourceAttr(resourceName, "location_information.username", "tf-acc-user-v2"),
					resource.TestCheckResourceAttr(resourceName, "purchasing_information.life_expectancy", "5"),
					// prestage_device_names list shrinks from 2 → 1 (remove path).
					resource.TestCheckResourceAttr(resourceName, "names.prestage_device_names.#", "1"),
				),
			},
			// Step 3 — REMOVE all four Optional-only nested blocks
			// (skip_setup_items, names, location_information,
			// purchasing_information) managed in steps 1-2. Before the
			// gate-on-target fix this Update crashed with "Provider produced
			// inconsistent result after apply: ...: was null, but now
			// cty.ObjectVal(...)". A clean apply is the regression guard; the
			// framework's implicit post-apply empty-plan check proves the
			// removed blocks stay null on refresh rather than re-appearing.
			{
				Config: ldapFixture + adeFixtureBlock(suffix, token) + testAccMobilePrestageBlocksRemovedConfig(name+"-v2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "display_name", name+"-v2"),
					resource.TestCheckNoResourceAttr(resourceName, "skip_setup_items.biometric"),
					resource.TestCheckNoResourceAttr(resourceName, "names.assign_names_using"),
					resource.TestCheckNoResourceAttr(resourceName, "location_information.username"),
					resource.TestCheckNoResourceAttr(resourceName, "purchasing_information.apple_care_id"),
				),
			},
		},
	})
}

// --- §13 #3: Shared iPad storage quota --------------------------------------

func TestAccResource_ProMobileDevicePrestageEnrollment_SharedIpad_StorageQuota(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-mobile-prestage-storage-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageStorageQuotaConfig(name, 5),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "multi_user", "true"),
					resource.TestCheckResourceAttr(resourceName, "use_storage_quota_size", "true"),
					// Read-only: populated by Jamf Pro, never set in HCL.
					resource.TestCheckResourceAttrSet(resourceName, "storage_quota_size_megabytes"),
				),
			},
			// Update an unrelated shared-iPad field; storage_quota_size_megabytes
			// stays server-authoritative (read-only) and must not perturb the plan.
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageStorageQuotaConfig(name, 10),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "maximum_shared_accounts", "10"),
					resource.TestCheckResourceAttrSet(resourceName, "storage_quota_size_megabytes"),
				),
			},
		},
	})
}

// --- §13 #4: Shared iPad temporary session ----------------------------------

func TestAccResource_ProMobileDevicePrestageEnrollment_SharedIpad_TemporarySession(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-mobile-prestage-tempsession-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageTemporarySessionConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "multi_user", "true"),
					resource.TestCheckResourceAttr(resourceName, "temporary_session_only", "true"),
				),
			},
		},
	})
}

// --- §13 #5: Names happy-path per mode --------------------------------------

func TestAccResource_ProMobileDevicePrestageEnrollment_Names_SerialNumbers(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccMobilePrestageNamesModeConfig("tf-acc-names-serial-"+suffix, `
  names = {
    assign_names_using = "Serial Numbers"
    device_name_prefix = "iPad-"
  }
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "names.assign_names_using", "Serial Numbers"),
					resource.TestCheckResourceAttr(resourceName, "names.device_name_prefix", "iPad-"),
				),
			},
		},
	})
}

func TestAccResource_ProMobileDevicePrestageEnrollment_Names_SingleName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccMobilePrestageNamesModeConfig("tf-acc-names-single-"+suffix, `
  names = {
    assign_names_using = "Single Name"
    single_device_name = "Shared-iPad"
  }
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "names.assign_names_using", "Single Name"),
					resource.TestCheckResourceAttr(resourceName, "names.single_device_name", "Shared-iPad"),
				),
			},
		},
	})
}

func TestAccResource_ProMobileDevicePrestageEnrollment_Names_ListOfNames(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccMobilePrestageNamesModeConfig("tf-acc-names-list-"+suffix, `
  names = {
    assign_names_using = "List of Names"
    prestage_device_names = [
      { device_name = "iPad-1" },
      { device_name = "iPad-2" },
    ]
  }
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "names.assign_names_using", "List of Names"),
					resource.TestCheckResourceAttr(resourceName, "names.prestage_device_names.#", "2"),
				),
			},
		},
	})
}

func TestAccResource_ProMobileDevicePrestageEnrollment_Names_DefaultNames(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccMobilePrestageNamesModeConfig("tf-acc-names-default-"+suffix, `
  names = {
    assign_names_using = "Default Names"
  }
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "names.assign_names_using", "Default Names"),
				),
			},
		},
	})
}

// --- §13 #5b: ExpectError — Single Name requires single_device_name ---------

func TestAccResource_ProMobileDevicePrestageEnrollment_ExpectError_SingleNameRequiresName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) +
					testAccMobilePrestageNamesModeConfig("tf-acc-expect-single-"+suffix, `
  names = {
    assign_names_using = "Single Name"
  }
`),
				ExpectError: regexp.MustCompile(`single_device_name is required`),
			},
		},
	})
}

// --- §13 #5c: ExpectError — storage quota ⊻ temporary session ---------------

func TestAccResource_ProMobileDevicePrestageEnrollment_ExpectError_StorageVsTempSession(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-storage-temp-%s"
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  multi_user              = true
  supervised              = true
  prevent_activation_lock = true
  use_storage_quota_size  = true
  temporary_session_only  = true
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`mutually-exclusive|conflicts with temporary_session_only`),
			},
		},
	})
}

// --- §13 #5d: ExpectError — temporary_session_timeout below minimum ---------

func TestAccResource_ProMobileDevicePrestageEnrollment_ExpectError_TempSessionTimeoutBelowMin(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-temp-timeout-%s"
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  multi_user                        = true
  supervised                        = true
  prevent_activation_lock           = true
  temporary_session_only            = true
  enforce_temporary_session_timeout = true
  temporary_session_timeout         = 15
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`at-least-30|below minimum`),
			},
		},
	})
}

// --- §13 #6: ExpectError — bad min-OS enforcement enum ----------------------

func TestAccResource_ProMobileDevicePrestageEnrollment_ExpectError_BadMinOsIos(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-bad-min-os-ios-%s"
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  prestage_minimum_os_target_version_type_ios = "LATEST_ONLY"
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`must be one of`),
			},
		},
	})
}

func TestAccResource_ProMobileDevicePrestageEnrollment_ExpectError_BadMinOsIpad(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = "tf-acc-expect-bad-min-os-ipad-%s"
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  prestage_minimum_os_target_version_type_ipad = "LATEST_ONLY"
}
`, suffix, adeFixtureRef),
				ExpectError: regexp.MustCompile(`must be one of`),
			},
		},
	})
}

// --- §13 #6: MinOsSpecific happy path (iOS + iPadOS specific versions) -------

func TestAccResource_ProMobileDevicePrestageEnrollment_MinOsSpecific(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-mobile-minos-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  prestage_minimum_os_target_version_type_ios  = "MINIMUM_OS_SPECIFIC_VERSION"
  minimum_os_specific_version_ios              = "17.1"
  prestage_minimum_os_target_version_type_ipad = "MINIMUM_OS_SPECIFIC_VERSION"
  minimum_os_specific_version_ipad             = "17.1"

  location_information   = {}
  purchasing_information = {}
  names                  = {}

  scope_serial_numbers = []
}
`, name, adeFixtureRef),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "prestage_minimum_os_target_version_type_ios", "MINIMUM_OS_SPECIFIC_VERSION"),
					resource.TestCheckResourceAttr(resourceName, "minimum_os_specific_version_ios", "17.1"),
					resource.TestCheckResourceAttr(resourceName, "prestage_minimum_os_target_version_type_ipad", "MINIMUM_OS_SPECIFIC_VERSION"),
					resource.TestCheckResourceAttr(resourceName, "minimum_os_specific_version_ipad", "17.1"),
				),
			},
		},
	})
}

// --- §13 #9: anchor_certificates silent-rollback hard-error path ------------

func TestAccResource_ProMobileDevicePrestageEnrollment_AnchorCertificatesRollback(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-mobile-anchor-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageMinimalConfig(name),
				Check:  resource.TestCheckResourceAttrSet(resourceName, "id"),
			},
			{
				Config:      adeFixtureBlock(suffix, token) + testAccMobilePrestageAnchorsConfig(name, `["dGVzdA=="]`),
				ExpectError: regexp.MustCompile(`did not commit|did not round-trip|anchor_certificates`),
			},
		},
	})
}

// --- §13 #7: Scope round-trip ([] → [serial] → []) --------------------------

func TestAccResource_ProMobileDevicePrestageEnrollment_ScopeAssignments(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	serial := requireADESerialFixture(t)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-mobile-scope-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageScopeConfig(name, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "0"),
				),
			},
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageScopeConfig(name, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "1"),
					resource.TestCheckTypeSetElemAttr(resourceName, "scope_serial_numbers.*", serial),
				),
			},
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageScopeConfig(name, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "0"),
				),
			},
		},
	})
}

// --- §13 #7b: Multi-serial scope round-trip --------------------------------
//
// Exercises the scope_serial_numbers Set<String> full-rewrite with several
// serials: create with all → remove one (partial removal) → clear. Verifies
// set membership round-trips order-independently and that a shrink reconciles.
func TestAccResource_ProMobileDevicePrestageEnrollment_ScopeMultipleSerials(t *testing.T) {
	testhelpers.AccPreCheck(t)
	token := requireADETokenBlob(t)
	serials := requireADEMultiSerialsFixture(t, 2)
	suffix := testhelpers.RunSuffix()
	accCleanupOrphans(t, suffix)
	name := "tf-acc-mobile-scope-multi-" + suffix

	all := serials
	subset := serials[:len(serials)-1] // drop the last one

	allChecks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", fmt.Sprintf("%d", len(all))),
	}
	for _, s := range all {
		allChecks = append(allChecks, resource.TestCheckTypeSetElemAttr(resourceName, "scope_serial_numbers.*", s))
	}
	subsetChecks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", fmt.Sprintf("%d", len(subset))),
	}
	for _, s := range subset {
		subsetChecks = append(subsetChecks, resource.TestCheckTypeSetElemAttr(resourceName, "scope_serial_numbers.*", s))
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageScopeListConfig(name, all),
				Check:  resource.ComposeAggregateTestCheckFunc(allChecks...),
			},
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageScopeListConfig(name, subset),
				Check:  resource.ComposeAggregateTestCheckFunc(subsetChecks...),
			},
			{
				Config: adeFixtureBlock(suffix, token) + testAccMobilePrestageScopeListConfig(name, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "scope_serial_numbers.#", "0"),
				),
			},
		},
	})
}

// --- Config helpers (prestage block only — ADE fixture concatenated by caller) ---

func testAccMobilePrestageMinimalConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  location_information   = {}
  purchasing_information = {}
  skip_setup_items       = {}
  names                  = {}

  scope_serial_numbers = []
}
`, name, adeFixtureRef)
}

func testAccMobilePrestageFullConfigV1(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  depends_on = [jamfplatform_pro_ldap_server.acc_ldap]

  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }
  mandatory                             = true
  mdm_removable                         = true
  supervised                            = true
  support_phone_number                  = "+44-1-555-0100"
  support_email_address                 = "ops@example.test"
  department                            = "Operations"
  require_authentication                = true
  authentication_prompt                 = "Welcome to acceptance"

  skip_setup_items = {
    biometric = true
    siri      = false
    location  = true
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

  names = {
    assign_names_using = "List of Names"
    manage_names       = true
    prestage_device_names = [
      { device_name = "iPad-1" },
      { device_name = "iPad-2" },
    ]
  }

  scope_serial_numbers = []
}
`, name, adeFixtureRef)
}

func testAccMobilePrestageFullConfigV2(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  depends_on = [jamfplatform_pro_ldap_server.acc_ldap]

  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }
  mandatory                             = false
  mdm_removable                         = false
  supervised                            = true
  support_phone_number                  = "+44-1-555-0200"
  support_email_address                 = "newops@example.test"
  department                            = "NewOps"
  require_authentication                = false
  authentication_prompt                 = "Updated prompt"

  skip_setup_items = {
    biometric = false
    siri      = true
    location  = false
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

  names = {
    assign_names_using = "List of Names"
    manage_names       = true
    prestage_device_names = [
      { device_name = "iPad-1" },
    ]
  }

  scope_serial_numbers = []
}
`, name, adeFixtureRef)
}

// testAccMobilePrestageBlocksRemovedConfig is V2 with every Optional-only
// nested block (skip_setup_items, names, location_information,
// purchasing_information) OMITTED. Exercises the remove-on-update transition:
// blocks managed in prior steps disappear from config, so the plan is null for
// each and the provider must write null (not repopulate from the wire GET).
func testAccMobilePrestageBlocksRemovedConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  depends_on = [jamfplatform_pro_ldap_server.acc_ldap]

  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }
  mandatory                             = false
  mdm_removable                         = false
  supervised                            = true
  support_phone_number                  = "+44-1-555-0200"
  support_email_address                 = "newops@example.test"
  department                            = "NewOps"
  require_authentication                = false
  authentication_prompt                 = "Updated prompt"

  scope_serial_numbers = []
}
`, name, adeFixtureRef)
}

// storage_quota_size_megabytes is read-only (Computed) — the server
// recalculates it on every change, so it is NOT set in HCL; the test only
// asserts it is populated. maximum_shared_accounts drives the update step.
func testAccMobilePrestageStorageQuotaConfig(name string, maxUsers int) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  multi_user              = true
  supervised              = true
  prevent_activation_lock = true
  use_storage_quota_size  = true
  maximum_shared_accounts = %d

  location_information   = {}
  purchasing_information = {}
  names                  = {}

  scope_serial_numbers = []
}
`, name, adeFixtureRef, maxUsers)
}

func testAccMobilePrestageTemporarySessionConfig(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  multi_user                        = true
  supervised                        = true
  prevent_activation_lock           = true
  temporary_session_only            = true
  enforce_temporary_session_timeout = true
  temporary_session_timeout         = 60

  location_information   = {}
  purchasing_information = {}
  names                  = {}

  scope_serial_numbers = []
}
`, name, adeFixtureRef)
}

// testAccMobilePrestageNamesModeConfig emits a prestage block with the supplied
// `names = { ... }` HCL fragment spliced in.
func testAccMobilePrestageNamesModeConfig(name, namesBlock string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  location_information   = {}
  purchasing_information = {}
%s
  scope_serial_numbers = []
}
`, name, adeFixtureRef, namesBlock)
}

func testAccMobilePrestageAnchorsConfig(name, anchors string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  anchor_certificates = %s

  location_information   = {}
  purchasing_information = {}
  names                  = {}

  scope_serial_numbers = []
}
`, name, adeFixtureRef, anchors)
}

// testAccMobilePrestageScopeListConfig renders scope_serial_numbers from a
// slice (nil/empty → []).
func testAccMobilePrestageScopeListConfig(name string, serials []string) string {
	quoted := make([]string, 0, len(serials))
	for _, s := range serials {
		quoted = append(quoted, fmt.Sprintf("%q", s))
	}
	scope := "[" + strings.Join(quoted, ", ") + "]"
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  location_information   = {}
  purchasing_information = {}
  names                  = {}

  scope_serial_numbers = %s
}
`, name, adeFixtureRef, scope)
}

func testAccMobilePrestageScopeConfig(name, serial string) string {
	scope := "[]"
	if serial != "" {
		scope = fmt.Sprintf(`[%q]`, serial)
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_prestage_enrollment" "test" {
  display_name                          = %q
  device_enrollment_program_instance_id = %s
  timezone                              = "UTC"

  timeouts = {
    create = "1m"
    read   = "1m"
    update = "1m"
    delete = "1m"
  }

  location_information   = {}
  purchasing_information = {}
  names                  = {}

  scope_serial_numbers = %s
}
`, name, adeFixtureRef, scope)
}
