// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Drift-detection coverage for issue #387. Kept in its own file so the
// omit-retains contract tests in resource_acceptance_test.go stay untouched.

package patch_policy_test

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

const patchPolicyDriftResourceAddr = "jamfplatform_pro_patch_policy.test"

// patchPolicyDriftConfig declares the attributes the drift test mutates
// server-side, all of them echoed unconditionally by the classic
// /patchpolicies GET.
//
// user_interaction.notifications is deliberately absent. Four of its six fields
// are echoed only while the tenant-level Self Service notifications toggle is
// on, and an acceptance test must not depend on a tenant setting it does not
// manage; the other two (message, type) are never echoed at all. Both halves
// are covered by the unit tests in drift_test.go instead — see
// flattenUserInteraction.
func patchPolicyDriftConfig(suffix string) string {
	return fixtureTitle(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_patch_policy" "test" {
			software_title_configuration_id = jamfplatform_pro_patch_software_title.title.id
			name                            = "tf-acc-pp-drift-%[1]s"
			target_version                  = %[2]q
			enabled                         = true
			distribution_method             = "selfservice"
			allow_downgrade                 = true
			patch_unknown                   = true

			user_interaction = {
				install_button_text      = "Update"
				self_service_description = "Declared by Terraform."

				deadlines = {
					enabled = true
					period  = 7
				}

				grace_period = {
					duration                    = 15
					notification_center_subject = "Declared by Terraform."
					message                     = "Declared by Terraform."
				}
			}
		}
	`, suffix, accTitleVersion)
}

// mutatePatchPolicyOutOfBand rewrites the managed attributes straight through
// the classic endpoint, standing in for an administrator editing the patch
// policy in the Jamf Pro UI.
func mutatePatchPolicyOutOfBand(t *testing.T, id string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	mutatedName := "tf-acc-pp-drift-mutated"
	mutatedMethod := "prompt"
	mutatedButton := "Mutated Button"
	mutatedText := "Mutated outside Terraform."
	no := false
	period := 1
	duration := 1
	if err := c.UpdatePatchPolicyByID(context.Background(), id, &proclassic.PatchPolicy{
		General: &proclassic.PatchPolicyGeneral{
			Name:               &mutatedName,
			Enabled:            &no,
			DistributionMethod: &mutatedMethod,
			AllowDowngrade:     &no,
			PatchUnknown:       &no,
		},
		UserInteraction: &proclassic.PatchPolicyUserInteraction{
			InstallButtonText:      &mutatedButton,
			SelfServiceDescription: &mutatedText,
			Deadlines:              &proclassic.PatchPolicyUserInteractionDeadlines{DeadlineEnabled: &no, DeadlinePeriod: &period},
			GracePeriod: &proclassic.PatchPolicyUserInteractionGracePeriod{
				GracePeriodDuration:       &duration,
				NotificationCenterSubject: &mutatedText,
				Message:                   &mutatedText,
			},
		},
	}); err != nil {
		t.Fatalf("out-of-band update of patch policy %s failed: %s", id, err)
	}
}

// capturePatchPolicyID records the resource id so a later step's PreConfig can
// reach the object directly.
func capturePatchPolicyID(into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[patchPolicyDriftResourceAddr]
		if !ok {
			return fmt.Errorf("%s missing from state", patchPolicyDriftResourceAddr)
		}
		*into = rs.Primary.ID
		return nil
	}
}

// TestAccResource_ProPatchPolicy_DriftIsReported pins the wire-authoritative
// read at the acceptance level: a change made outside the workspace must plan
// as an in-place update. Before issue #387 the refresh in step 2 adopted
// nothing and the plan was empty.
func TestAccResource_ProPatchPolicy_DriftIsReported(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pp-drift-" + suffix

	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPatchPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: patchPolicyDriftConfig(suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "name", name),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "enabled", "true"),
					capturePatchPolicyID(&id),
				),
			},
			{
				PreConfig: func() { mutatePatchPolicyOutOfBand(t, id) },
				Config:    patchPolicyDriftConfig(suffix),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(patchPolicyDriftResourceAddr, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "name", name),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "enabled", "true"),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "distribution_method", "selfservice"),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "allow_downgrade", "true"),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "patch_unknown", "true"),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "user_interaction.install_button_text", "Update"),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "user_interaction.self_service_description", "Declared by Terraform."),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "user_interaction.deadlines.enabled", "true"),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "user_interaction.deadlines.period", "7"),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "user_interaction.grace_period.duration", "15"),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "user_interaction.grace_period.notification_center_subject", "Declared by Terraform."),
					resource.TestCheckResourceAttr(patchPolicyDriftResourceAddr, "user_interaction.grace_period.message", "Declared by Terraform."),
				),
			},
		},
	})
}
