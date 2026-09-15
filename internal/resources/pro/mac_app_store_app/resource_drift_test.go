// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Drift-detection coverage for issue #387. Kept in its own file so the
// omit-retains contract tests in resource_acceptance_test.go stay untouched.

package mac_app_store_app_test

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

// macAppDriftConfig declares the attributes the drift test mutates
// server-side, all of them echoed unconditionally by the classic
// /macapplications GET.
//
// is_free is deliberately absent: Jamf Pro resolves it from the App Store
// listing, so it keeps a sticky read and cannot report drift (see
// flattenMacAppGeneral). So are the notification_* fields, which are echoed
// only while the tenant-level Self Service notifications toggle is on — an
// acceptance test must not depend on a tenant setting it does not manage, so
// that half is covered by the unit tests in drift_test.go instead.
func macAppDriftConfig(name, buttonText string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name            = %q
				version         = "1.0"
				bundle_id       = "com.example.tfacc.macapp.drift"
				url             = "https://apps.apple.com/app/id000000001"
				deployment_type = "Make Available in Self Service"
			}
			self_service = {
				install_button_text             = %q
				self_service_description        = "Declared by Terraform."
				force_users_to_view_description  = true
				feature_on_main_page            = true
			}
		}
	`, name, buttonText)
}

// mutateMacAppOutOfBand rewrites the managed attributes straight through the
// classic endpoint, standing in for an administrator editing the app in the
// Jamf Pro UI.
func mutateMacAppOutOfBand(t *testing.T, id string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	mutatedButton := "Mutated Button"
	mutatedDesc := "Mutated outside Terraform."
	mutatedDeployment := "Install Automatically/Prompt Users to Install"
	no := false
	if err := c.UpdateMacApplicationByID(context.Background(), id, &proclassic.MacApplication{
		General: &proclassic.MacApplicationGeneral{DeploymentType: &mutatedDeployment},
		SelfService: &proclassic.MacApplicationSelfService{
			InstallButtonText:           &mutatedButton,
			SelfServiceDescription:      &mutatedDesc,
			ForceUsersToViewDescription: &no,
			FeatureOnMainPage:           &no,
		},
	}); err != nil {
		t.Fatalf("out-of-band update of mac app %s failed: %s", id, err)
	}
}

// captureMacAppID records the resource id so a later step's PreConfig can reach
// the object directly.
func captureMacAppID(into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[macAppResourceAddr]
		if !ok {
			return fmt.Errorf("%s missing from state", macAppResourceAddr)
		}
		*into = rs.Primary.ID
		return nil
	}
}

// TestAccResource_ProMacAppStoreApp_DriftIsReported pins the wire-authoritative
// read at the acceptance level: a change made outside the workspace must plan
// as an in-place update. Before issue #387 the refresh in step 2 adopted
// nothing and the plan was empty.
func TestAccResource_ProMacAppStoreApp_DriftIsReported(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-drift-" + suffix
	const buttonText = "Get it"

	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: macAppDriftConfig(name, buttonText),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.install_button_text", buttonText),
					captureMacAppID(&id),
				),
			},
			{
				PreConfig: func() { mutateMacAppOutOfBand(t, id) },
				Config:    macAppDriftConfig(name, buttonText),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(macAppResourceAddr, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.deployment_type", "Make Available in Self Service"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.install_button_text", buttonText),
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.self_service_description", "Declared by Terraform."),
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.force_users_to_view_description", "true"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.feature_on_main_page", "true"),
				),
			},
		},
	})
}
