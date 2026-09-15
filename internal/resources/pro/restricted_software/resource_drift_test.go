// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Drift-detection coverage for issue #387: a change made to a
// Terraform-managed attribute outside the workspace must be reported by the
// next plan. Kept in its own file so the omit-retains contract tests in
// resource_acceptance_test.go stay untouched.
//
// The mutation is issued through the SDK rather than the Jamf Pro UI because
// they reach the same endpoint: the reproduction in #387 was confirmed both
// ways.

package restricted_software_test

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

// driftConfig declares every general attribute the drift test mutates
// server-side. All five are echoed faithfully by the classic
// /restrictedsoftware GET (Jamf Pro 11.31.1, wire-probed 2026-09-06).
func driftConfig(name, processName, message string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_restricted_software" "test" {
			general = {
				name                                 = %q
				process_name                         = %q
				display_message                      = %q
				restrict_exact_process_name          = true
				kill_process                         = true
				delete_application                   = true
				send_email_notification_on_violation = true
			}
		}
	`, name, processName, message)
}

// mutateRestrictedSoftwareOutOfBand rewrites the managed attributes straight
// through the classic endpoint, standing in for an administrator editing the
// record in the Jamf Pro UI.
func mutateRestrictedSoftwareOutOfBand(t *testing.T, id string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	mutated := "Mutated outside Terraform."
	no := false
	if err := c.UpdateRestrictedSoftwareByID(context.Background(), id, &proclassic.RestrictedSoftware{
		General: &proclassic.RestrictedSoftwareGeneral{
			DisplayMessage:        &mutated,
			MatchExactProcessName: &no,
			KillProcess:           &no,
			DeleteExecutable:      &no,
			SendNotification:      &no,
		},
	}); err != nil {
		t.Fatalf("out-of-band update of restricted software %s failed: %s", id, err)
	}
}

// captureID records the resource id from state so a later step's PreConfig can
// reach the object directly.
func captureID(addr string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[addr]
		if !ok {
			return fmt.Errorf("%s missing from state", addr)
		}
		*into = rs.Primary.ID
		return nil
	}
}

// TestAccResource_ProRestrictedSoftware_DriftIsReported is the acceptance-level
// regression for issue #387. Before that fix the flatteners returned the value
// already in state for every one of these attributes, so step 2's refresh
// adopted nothing, the plan was empty, and a Jamf Pro UI edit to a managed
// attribute was invisible indefinitely. The plan check on step 2 is the
// assertion: the refresh at the head of that step must see the mutated server
// values and plan them back to the configured ones, so an out-of-band change
// shows up as an in-place update.
func TestAccResource_ProRestrictedSoftware_DriftIsReported(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-restsw-drift-" + suffix
	const message = "Declared by Terraform."

	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRestrictedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: driftConfig(name, "Chess.app", message),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.display_message", message),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.kill_process", "true"),
					captureID(restrictedSoftwareResourceAddr, &id),
				),
			},
			{
				PreConfig: func() { mutateRestrictedSoftwareOutOfBand(t, id) },
				Config:    driftConfig(name, "Chess.app", message),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(restrictedSoftwareResourceAddr, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.display_message", message),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.restrict_exact_process_name", "true"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.kill_process", "true"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.delete_application", "true"),
					resource.TestCheckResourceAttr(restrictedSoftwareResourceAddr, "general.send_email_notification_on_violation", "true"),
				),
			},
		},
	})
}
