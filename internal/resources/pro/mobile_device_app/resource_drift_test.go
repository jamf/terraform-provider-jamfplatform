// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Drift-detection coverage for issue #387. Kept in its own file so the
// omit-retains contract tests in resource_acceptance_test.go stay untouched.

package mobile_device_app_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// mobileAppDriftConfig declares the attributes the drift test mutates
// server-side, all of them echoed unconditionally by the classic
// /mobiledeviceapplications GET.
//
// host_externally is deliberately absent: the write does not persist while
// external_url is set, so it keeps a sticky read and cannot report drift (see
// flattenMobileAppGeneral). So are after_install_button_text and the
// notification_* fields, each echoed only while its own gate is on
// (general.make_available_after_install for the first, the tenant-level Self
// Service notifications toggle for the rest) — an acceptance test must not
// depend on a tenant setting it does not manage, so that half is covered by the
// unit tests in drift_test.go instead.
//
// The distribution method travels as general.deploy_automatically, not
// general.deployment_type: the latter is a read-only mirror of the former and a
// write to it is discarded. The mutation below still moves deployment_type,
// because moving deploy_automatically is how you move it.
func mobileAppDriftConfig(name, buttonText string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name                                   = %q
				version                                = "1.0"
				bundle_id                              = "com.example.tfacc.mobileapp.drift"
				os_type                                = "iOS"
				deploy_automatically                   = false
				keep_app_updated_on_devices            = true
				remove_app_when_mdm_profile_is_removed = true
				prevent_backup_of_app_data             = true
				allow_user_to_delete                   = true
			}
			self_service = {
				install_button_text      = %q
				self_service_description = "Declared by Terraform."
				feature_on_main_page     = true
			}
		}
	`, name, buttonText)
}

// mutateMobileAppOutOfBand rewrites the managed attributes straight through the
// classic endpoint, standing in for an administrator editing the app in the
// Jamf Pro UI.
func mutateMobileAppOutOfBand(t *testing.T, id string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	mutatedButton := "Mutated Button"
	mutatedDesc := "Mutated outside Terraform."
	no := false
	yes := true
	if err := c.UpdateMobileDeviceApplicationByID(context.Background(), id, &proclassic.MobileDeviceApplication{
		General: &proclassic.MobileDeviceApplicationGeneral{
			// deploy_automatically, not deployment_type: a write to
			// deployment_type is discarded, so mutating it would not stand in
			// for an administrator's edit at all.
			DeployAutomatically:              &yes,
			KeepAppUpdatedOnDevices:          &no,
			RemoveAppWhenMDMProfileIsRemoved: &no,
			PreventBackupOfAppData:           &no,
			AllowUserToDelete:                &no,
		},
		SelfService: &proclassic.MobileDeviceApplicationSelfService{
			SelfServiceInstallButtonText: &mutatedButton,
			SelfServiceDescription:       &mutatedDesc,
			FeatureOnMainPage:            &no,
		},
	}); err != nil {
		t.Fatalf("out-of-band update of mobile device app %s failed: %s", id, err)
	}
}

// captureMobileAppID records the resource id so a later step's PreConfig can
// reach the object directly.
func captureMobileAppID(into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[mobileAppResourceAddr]
		if !ok {
			return fmt.Errorf("%s missing from state", mobileAppResourceAddr)
		}
		*into = rs.Primary.ID
		return nil
	}
}

// TestAccResource_ProMobileDeviceApp_DriftIsReported pins the
// wire-authoritative read at the acceptance level: a change made outside the
// workspace must plan as an in-place update. Before issue #387 the refresh in
// step 2 adopted nothing and the plan was empty.
func TestAccResource_ProMobileDeviceApp_DriftIsReported(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-drift-" + suffix
	const buttonText = "Get it"

	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mobileAppDriftConfig(name, buttonText),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "self_service.install_button_text", buttonText),
					captureMobileAppID(&id),
				),
			},
			{
				PreConfig: func() { mutateMobileAppOutOfBand(t, id) },
				Config:    mobileAppDriftConfig(name, buttonText),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(mobileAppResourceAddr, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.deploy_automatically", "false"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.deployment_type", "Make Available in Self Service"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.keep_app_updated_on_devices", "true"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.remove_app_when_mdm_profile_is_removed", "true"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.prevent_backup_of_app_data", "true"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.allow_user_to_delete", "true"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "self_service.install_button_text", buttonText),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "self_service.self_service_description", "Declared by Terraform."),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "self_service.feature_on_main_page", "true"),
				),
			},
		},
	})
}
