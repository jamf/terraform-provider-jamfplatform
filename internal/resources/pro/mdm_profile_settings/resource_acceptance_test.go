// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mdm_profile_settings_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// checkSingletonRecordStillExists verifies the Jamf Pro device communication settings
// record persists on the tenant after Terraform destroys the resource from state.
// Canonical singleton acceptance check: the remote Delete is a no-op, so the API
// must still return the record (with whatever value was last applied) post-destroy.
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "device communication settings", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetDeviceCommunicationSettingsV1(ctx)
	})
}

// TestAccResource_ProMDMProfileSettings_Basic mutates all six settings on, then
// off, exercising both Update paths against a real tenant. Singleton resources have no
// remote Delete, so CheckDestroy is wired to verify the record PERSISTS on the tenant
// after Terraform stops managing it (the opposite of the standard pattern).
func TestAccResource_ProMDMProfileSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_mdm_profile_settings" "test" {
						auto_renew_computer_profile_when_ca_renewed      = true
						auto_renew_computer_profile_before_expiry        = true
						computer_profile_expiration_limit_days           = 180
						auto_renew_mobile_device_profile_when_ca_renewed  = true
						auto_renew_mobile_device_profile_before_expiry    = true
						mobile_device_profile_expiration_limit_days       = 180
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "id", "singleton"),
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "auto_renew_computer_profile_when_ca_renewed", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "auto_renew_computer_profile_before_expiry", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "computer_profile_expiration_limit_days", "180"),
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "auto_renew_mobile_device_profile_when_ca_renewed", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "auto_renew_mobile_device_profile_before_expiry", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "mobile_device_profile_expiration_limit_days", "180"),
				),
			},
			{
				Config: `
					resource "jamfplatform_pro_mdm_profile_settings" "test" {
						auto_renew_computer_profile_when_ca_renewed      = false
						auto_renew_computer_profile_before_expiry        = false
						computer_profile_expiration_limit_days           = 90
						auto_renew_mobile_device_profile_when_ca_renewed  = false
						auto_renew_mobile_device_profile_before_expiry    = false
						mobile_device_profile_expiration_limit_days       = 90
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "auto_renew_computer_profile_when_ca_renewed", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "auto_renew_computer_profile_before_expiry", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "computer_profile_expiration_limit_days", "90"),
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "auto_renew_mobile_device_profile_when_ca_renewed", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "auto_renew_mobile_device_profile_before_expiry", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_mdm_profile_settings.test", "mobile_device_profile_expiration_limit_days", "90"),
				),
			},
			{
				ResourceName:      "jamfplatform_pro_mdm_profile_settings.test",
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

// TestAccResource_ProMDMProfileSettings_RejectsNonSingletonImport verifies the
// ImportState guard added in resource.go: any identifier other than "singleton" must
// fail with a clear error rather than silently normalizing to the singleton ID.
func TestAccResource_ProMDMProfileSettings_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_mdm_profile_settings" "test" {
						auto_renew_computer_profile_when_ca_renewed      = false
						auto_renew_computer_profile_before_expiry        = false
						computer_profile_expiration_limit_days           = 90
						auto_renew_mobile_device_profile_when_ca_renewed = false
						auto_renew_mobile_device_profile_before_expiry   = false
						mobile_device_profile_expiration_limit_days      = 90
					}
				`,
			},
			{
				ResourceName:  "jamfplatform_pro_mdm_profile_settings.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

func TestAccDataSource_ProMDMProfileSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_mdm_profile_settings" "src" {
						auto_renew_computer_profile_when_ca_renewed      = true
						auto_renew_computer_profile_before_expiry        = true
						computer_profile_expiration_limit_days           = 120
						auto_renew_mobile_device_profile_when_ca_renewed = true
						auto_renew_mobile_device_profile_before_expiry   = true
						mobile_device_profile_expiration_limit_days      = 120
					}

					data "jamfplatform_pro_mdm_profile_settings" "lookup" {
						depends_on = [jamfplatform_pro_mdm_profile_settings.src]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mdm_profile_settings.lookup", "id", "singleton"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_mdm_profile_settings.lookup", "auto_renew_computer_profile_when_ca_renewed", "jamfplatform_pro_mdm_profile_settings.src", "auto_renew_computer_profile_when_ca_renewed"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_mdm_profile_settings.lookup", "computer_profile_expiration_limit_days", "jamfplatform_pro_mdm_profile_settings.src", "computer_profile_expiration_limit_days"),
				),
			},
		},
	})
}
