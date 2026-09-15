// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package re_enrollment_settings_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// The Re-enrollment settings object is a tenant-wide singleton that always
// exists and cannot be deleted (Delete is state-only by design). Every test
// therefore uses an INVERTED CheckDestroy: after `terraform destroy` the record
// must still be readable on the tenant. Tests leave the record in a known
// explicit state (all six fields set) so there is no ordering dependence
// between tests — the singleton always exists.

const reEnrollmentResourceAddr = "jamfplatform_pro_re_enrollment_settings.test"

// checkReEnrollmentStillExists verifies Delete did not remove the Re-enrollment
// settings record. The Delete handler is documented as state-only, so a GET must
// still succeed after destroy.
func checkReEnrollmentStillExists(t *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		got, err := c.GetReenrollmentSettingsV1(context.Background())
		if err != nil {
			return fmt.Errorf("expected Re-enrollment settings record to persist after destroy: %w", err)
		}
		if got == nil {
			return fmt.Errorf("expected non-nil Re-enrollment settings post-destroy")
		}
		return nil
	}
}

// reEnrollmentConfig renders a full six-field config. The five clear_* toggles
// are Optional+Computed (omit=preserve) and clear_management_history is Required;
// this helper declares all six so the Update/Import/DataSource tests pin every
// value. Omit=preserve is exercised separately in the split-ownership test.
func reEnrollmentConfig(clearPolicyLogs, clearLocationInfo, clearLocationHistory, clearExtAttrs, clearSUPlans bool, clearManagementHistory string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_re_enrollment_settings" "test" {
			clear_policy_logs                  = %t
			clear_location_information         = %t
			clear_location_information_history = %t
			clear_extension_attributes         = %t
			clear_software_update_plans        = %t
			clear_management_history           = %q
		}
	`, clearPolicyLogs, clearLocationInfo, clearLocationHistory, clearExtAttrs, clearSUPlans, clearManagementHistory)
}

// TestAccResource_ProReEnrollmentSettings_Update drives the full Update
// round-trip. Step 1 sets a mix of the five bools plus the enum; step 2 flips
// all five bools and changes the enum. Every attribute is asserted on both
// steps. This is the highest-value test — it proves the full-replace PUT and
// state round-trip across every non-RequiresReplace attribute.
func TestAccResource_ProReEnrollmentSettings_Update(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkReEnrollmentStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: reEnrollmentConfig(true, false, true, false, true, "DELETE_EVERYTHING_EXCEPT_ACKNOWLEDGED"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "id", "singleton"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_policy_logs", "true"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_location_information", "false"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_location_information_history", "true"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_extension_attributes", "false"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_software_update_plans", "true"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_management_history", "DELETE_EVERYTHING_EXCEPT_ACKNOWLEDGED"),
				),
			},
			{
				// Flip all five bools and change the enum.
				Config: reEnrollmentConfig(false, true, false, true, false, "DELETE_NOTHING"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_policy_logs", "false"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_location_information", "true"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_location_information_history", "false"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_extension_attributes", "true"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_software_update_plans", "false"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_management_history", "DELETE_NOTHING"),
				),
			},
		},
	})
}

// TestAccResource_ProReEnrollmentSettings_SplitOwnership proves the omit=preserve
// contract for an Optional+Computed toggle (clear_location_information) on this
// full-replace singleton, across all three transitions:
//
//   - Step 1 (create = adopt): the singleton already has clear_location_information
//     set true out of band; the config OMITS it. The GET-on-create merge must adopt
//     the existing true rather than resetting it to the false default. This is the
//     create-adopt behaviour — a plain USFU-only conversion would clobber it here.
//   - Step 2 (update = preserve): a UI edit flips it to false out of band while the
//     config still omits it and changes only the enum; UseStateForUnknown must carry
//     the live false forward, not revert to step 1's true or reset to default.
//   - Step 3 (take over): declaring the toggle explicitly lets Terraform own it.
//
// true/false are the discriminators — a clobber would surface as the false default.
func TestAccResource_ProReEnrollmentSettings_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)

	// setLocationInfoOutOfBand simulates a UI edit: read the current settings, flip
	// clear_location_information, and write the full object back (full-replace).
	setLocationInfoOutOfBand := func(v bool) func() {
		return func() {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			ctx := context.Background()
			got, err := c.GetReenrollmentSettingsV1(ctx)
			if err != nil {
				t.Fatalf("out-of-band GET: %v", err)
			}
			got.IsFlushLocationInformationEnabled = &v
			if _, err := c.UpdateReenrollmentSettingsV1(ctx, got); err != nil {
				t.Fatalf("out-of-band PUT: %v", err)
			}
		}
	}

	checkServerLocationInfo := func(want bool) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetReenrollmentSettingsV1(context.Background())
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if got.IsFlushLocationInformationEnabled == nil || *got.IsFlushLocationInformationEnabled != want {
				return fmt.Errorf("isFlushLocationInformationEnabled = %v, want %v", got.IsFlushLocationInformationEnabled, want)
			}
			return nil
		}
	}

	// Config omits clear_location_information; declares the enum anchor + one other
	// toggle so the unrelated change in step 2 is the enum.
	cfg := func(enum string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_re_enrollment_settings" "test" {
				clear_policy_logs        = true
				clear_management_history = %q
			}
		`, enum)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkReEnrollmentStillExists(t),
		Steps: []resource.TestStep{
			{
				// Pin clear_location_information=true on the singleton BEFORE create,
				// then create with it omitted. GET-on-create must adopt true, not reset
				// to the false default — the create-adopt behaviour.
				PreConfig: setLocationInfoOutOfBand(true),
				Config:    cfg("DELETE_NOTHING"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_policy_logs", "true"),
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_location_information", "true"),
					checkServerLocationInfo(true),
				),
			},
			{
				// UI flips clear_location_information to false out of band; config still
				// omits it and changes only the enum. The live false must survive — not
				// reverted to step 1's true, not reset to default.
				PreConfig: setLocationInfoOutOfBand(false),
				Config:    cfg("DELETE_ERRORS"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_management_history", "DELETE_ERRORS"),
					// State adopts the out-of-band value (Computed) and preserves it.
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_location_information", "false"),
					checkServerLocationInfo(false),
				),
			},
			{
				// Declaring the toggle explicitly lets Terraform take it back over —
				// flip it from the step-2 false to true to prove the override writes.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_re_enrollment_settings" "test" {
						clear_policy_logs          = true
						clear_location_information = true
						clear_management_history   = %q
					}
				`, "DELETE_ERRORS"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(reEnrollmentResourceAddr, "clear_location_information", "true"),
					checkServerLocationInfo(true),
				),
			},
		},
	})
}

// TestAccResource_ProReEnrollmentSettings_Import exercises the import
// round-trip. All six attributes are non-sub-block scalars, so ImportStateVerify
// is safe here (the singleton importer's post-import Read populates every base
// attribute). The second step asserts the non-singleton import guard.
func TestAccResource_ProReEnrollmentSettings_Import(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkReEnrollmentStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: reEnrollmentConfig(true, true, true, true, true, "DELETE_EVERYTHING"),
			},
			{
				ResourceName:      reEnrollmentResourceAddr,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
			},
			{
				ResourceName:  reEnrollmentResourceAddr,
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_ProReEnrollmentSettings_InvalidEnum verifies the OneOf
// validator on clear_management_history rejects an unknown value. The regex
// matches a single contiguous enum literal embedded in the validator's allowed
// list — no spaces, so it survives Terraform's ~80-col error wrapping.
func TestAccResource_ProReEnrollmentSettings_InvalidEnum(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      reEnrollmentConfig(true, true, true, true, true, "BOGUS"),
				ExpectError: regexp.MustCompile(`DELETE_EVERYTHING_EXCEPT_ACKNOWLEDGED`),
			},
		},
	})
}

// TestAccDataSource_ProReEnrollmentSettings_Basic applies the resource then
// reads it back through the data source and checks a couple of attributes.
func TestAccDataSource_ProReEnrollmentSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkReEnrollmentStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: reEnrollmentConfig(true, false, false, false, false, "DELETE_ERRORS") + `
					data "jamfplatform_pro_re_enrollment_settings" "ds" {
						depends_on = [jamfplatform_pro_re_enrollment_settings.test]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_re_enrollment_settings.ds", "id", "singleton"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_re_enrollment_settings.ds", "clear_policy_logs", "true"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_re_enrollment_settings.ds", "clear_management_history", "DELETE_ERRORS"),
				),
			},
		},
	})
}
