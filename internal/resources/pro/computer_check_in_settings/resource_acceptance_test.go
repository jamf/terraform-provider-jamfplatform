// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package computer_check_in_settings_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// checkSingletonRecordStillExists verifies the Jamf Pro Client Check-In settings
// record persists on the tenant after Terraform destroys the resource from state.
// Canonical singleton acceptance check: the remote Delete is a no-op, so the API
// must still return the record (with whatever value was last applied) post-destroy.
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "Client Check-In settings", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetCheckInSettingsV3(ctx)
	})
}

// TestAccResource_ProComputerCheckInSettings_Basic mutates every attribute across two Update
// steps against a real tenant (step 1: frequency 15 + all bools true; step 2:
// frequency 30 + all bools false), exercising both Update paths. Singleton resources
// have no remote Delete, so CheckDestroy verifies the record PERSISTS on the tenant
// after Terraform stops managing it (the opposite of the standard pattern).
func TestAccResource_ProComputerCheckInSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const addr = "jamfplatform_pro_computer_check_in_settings.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_computer_check_in_settings" "test" {
						check_in_frequency                  = 15
						create_startup_script               = true
						startup_log                          = true
						startup_policies                     = true
						startup_ssh                          = true
						create_login_hook                    = true
						login_hook_log                       = true
						login_hook_policies                  = true
						allow_network_state_change_triggers  = true
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "singleton"),
					resource.TestCheckResourceAttr(addr, "check_in_frequency", "15"),
					resource.TestCheckResourceAttr(addr, "create_startup_script", "true"),
					resource.TestCheckResourceAttr(addr, "startup_log", "true"),
					resource.TestCheckResourceAttr(addr, "startup_policies", "true"),
					resource.TestCheckResourceAttr(addr, "startup_ssh", "true"),
					resource.TestCheckResourceAttr(addr, "create_login_hook", "true"),
					resource.TestCheckResourceAttr(addr, "login_hook_log", "true"),
					resource.TestCheckResourceAttr(addr, "login_hook_policies", "true"),
					resource.TestCheckResourceAttr(addr, "allow_network_state_change_triggers", "true"),
				),
			},
			{
				Config: `
					resource "jamfplatform_pro_computer_check_in_settings" "test" {
						check_in_frequency                  = 30
						create_startup_script               = false
						startup_log                          = false
						startup_policies                     = false
						startup_ssh                          = false
						create_login_hook                    = false
						login_hook_log                       = false
						login_hook_policies                  = false
						allow_network_state_change_triggers  = false
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "check_in_frequency", "30"),
					resource.TestCheckResourceAttr(addr, "create_startup_script", "false"),
					resource.TestCheckResourceAttr(addr, "startup_log", "false"),
					resource.TestCheckResourceAttr(addr, "startup_policies", "false"),
					resource.TestCheckResourceAttr(addr, "startup_ssh", "false"),
					resource.TestCheckResourceAttr(addr, "create_login_hook", "false"),
					resource.TestCheckResourceAttr(addr, "login_hook_log", "false"),
					resource.TestCheckResourceAttr(addr, "login_hook_policies", "false"),
					resource.TestCheckResourceAttr(addr, "allow_network_state_change_triggers", "false"),
				),
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
				},
			},
		},
	})
}

// TestAccResource_ProComputerCheckInSettings_CreateAdopt proves the singleton
// GET-on-create-adopt: when a toggle is omitted from HCL on the FIRST apply, the
// provider reads the live settings and re-sends the existing value rather than
// resetting it to false on the full-replace PUT. Without GET-on-create-adopt this
// regresses — create-omit sends false and clobbers the admin's value.
func TestAccResource_ProComputerCheckInSettings_CreateAdopt(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const addr = "jamfplatform_pro_computer_check_in_settings.test"

	// Pin a known state out of band BEFORE Terraform creates the resource:
	// startup_ssh=true (the discriminator), every other toggle false, frequency 15.
	setBaselineOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		freq := 15
		tr, fa := true, false
		body := &pro.ClientCheckInV3{
			CheckInFrequency:                 &freq,
			CreateStartupScript:              &fa,
			StartupLog:                       &fa,
			StartupPolicies:                  &fa,
			StartupSsh:                       &tr,
			CreateHooks:                      &fa,
			HookLog:                          &fa,
			HookPolicies:                     &fa,
			EnableLocalConfigurationProfiles: &fa,
		}
		if _, err := c.UpdateCheckInSettingsV3(context.Background(), body); err != nil {
			t.Fatalf("out-of-band baseline PUT: %v", err)
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				// Create declaring ONLY the Required frequency; every toggle omitted.
				// GET-on-create-adopt must preserve the out-of-band startup_ssh=true.
				PreConfig: setBaselineOutOfBand,
				Config: `
					resource "jamfplatform_pro_computer_check_in_settings" "test" {
						check_in_frequency = 15
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "check_in_frequency", "15"),
					// Adopted from the live settings (not reset to false).
					resource.TestCheckResourceAttr(addr, "startup_ssh", "true"),
					resource.TestCheckResourceAttr(addr, "create_startup_script", "false"),
				),
			},
		},
	})
}

// TestAccResource_ProComputerCheckInSettings_RejectsNonSingletonImport verifies the ImportState
// guard: any identifier other than "singleton" must fail with a clear error rather
// than silently normalizing to the singleton ID.
func TestAccResource_ProComputerCheckInSettings_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_computer_check_in_settings" "test" {
						check_in_frequency = 15
					}
				`,
			},
			{
				ResourceName:  "jamfplatform_pro_computer_check_in_settings.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_ProComputerCheckInSettings_RejectsInvalidFrequency verifies the
// int64validator.OneOf(5,15,30,60) validator rejects an out-of-set frequency at plan
// time. The error detail wraps at ~80 cols; the regex matches a whitespace-tolerant
// token (\s+ absorbs an inserted newline+indent) to stay robust against the wrap.
func TestAccResource_ProComputerCheckInSettings_RejectsInvalidFrequency(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_computer_check_in_settings" "test" {
						check_in_frequency = 7
					}
				`,
				ExpectError: regexp.MustCompile(`must\s+be\s+one\s+of`),
			},
		},
	})
}

func TestAccDataSource_ProComputerCheckInSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_computer_check_in_settings" "src" {
						check_in_frequency = 15
					}

					data "jamfplatform_pro_computer_check_in_settings" "lookup" {
						depends_on = [jamfplatform_pro_computer_check_in_settings.src]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_computer_check_in_settings.lookup", "id", "singleton"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_computer_check_in_settings.lookup", "check_in_frequency", "jamfplatform_pro_computer_check_in_settings.src", "check_in_frequency"),
				),
			},
		},
	})
}
