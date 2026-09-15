// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Drift-detection coverage for issue #387. Kept in its own file so the
// omit-retains contract tests in resource_acceptance_test.go stay untouched.
//
// jamfplatform_pro_policy is the resource the sticky read was introduced for,
// and the one that carried the most of it — 74 of the 182 sites in #387. The
// fields exercised here span five of its sections, chosen because each is
// echoed faithfully by the classic /policies GET (Jamf Pro 11.31.1,
// wire-probed 2026-09-06). files_processes.execute_command is the sharpest
// case in the issue: a shell command a policy runs, editable in the Jamf Pro
// UI, that Terraform used to report as unchanged forever.

package policy_test

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

const policyDriftResourceAddr = "jamfplatform_pro_policy.test"

// policyDriftConfig declares the attributes the drift test mutates
// server-side, one per affected section.
func policyDriftConfig(name, command string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_policy" "test" {
			general = {
				name             = %q
				enabled          = true
				frequency        = "Once per computer"
				trigger_checkin  = true
				trigger_login    = true
				trigger_other    = "tf-acc-387"
				target_drive     = "/"
			}
			files_and_processes = {
				execute_command   = %q
				search_by_path    = "/tf-acc-387/path"
				search_for_process = "tf-acc-387-proc"
				kill_process_if_found = true
			}
			maintenance = {
				update_inventory     = true
				fix_disk_permissions = true
			}
			restart_options = {
				message                        = "Declared by Terraform."
				startup_disk                   = "Current Startup Disk"
				no_user_logged_in              = "Restart immediately"
				user_logged_in                 = "Restart"
				delay_minutes                  = 5
				start_reboot_timer_immediately = true
			}
			user_interaction = {
				start_message    = "Starting."
				complete_message = "Done."
			}
		}
	`, name, command)
}

// mutatePolicyOutOfBand rewrites the managed attributes straight through the
// classic endpoint, standing in for an administrator editing the policy in the
// Jamf Pro UI.
func mutatePolicyOutOfBand(t *testing.T, id string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	mutatedCommand := "echo mutated-outside-terraform"
	mutatedTrigger := "mutated-387"
	mutatedMessage := "Mutated outside Terraform."
	no := false
	delay := 30
	if err := c.UpdatePolicyByID(context.Background(), id, &proclassic.PolicyPost{
		General: &proclassic.PolicyPostGeneral{
			Enabled:        &no,
			TriggerCheckin: &no,
			TriggerLogin:   &no,
			TriggerOther:   &mutatedTrigger,
		},
		FilesProcesses: &proclassic.PolicyPostFilesProcesses{RunCommand: &mutatedCommand},
		Maintenance:    &proclassic.PolicyPostMaintenance{Recon: &no, Permissions: &no},
		Reboot:         &proclassic.PolicyPostReboot{Message: &mutatedMessage, MinutesUntilReboot: &delay},
		UserInteraction: &proclassic.PolicyPostUserInteraction{
			MessageStart:  &mutatedMessage,
			MessageFinish: &mutatedMessage,
		},
	}); err != nil {
		t.Fatalf("out-of-band update of policy %s failed: %s", id, err)
	}
}

// capturePolicyID records the resource id so a later step's PreConfig can reach
// the object directly.
func capturePolicyID(into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[policyDriftResourceAddr]
		if !ok {
			return fmt.Errorf("%s missing from state", policyDriftResourceAddr)
		}
		*into = rs.Primary.ID
		return nil
	}
}

// TestAccResource_ProPolicy_DriftIsReported pins the wire-authoritative read at
// the acceptance level: a change made outside the workspace must plan as an
// in-place update. Before issue #387 the refresh in step 2 adopted nothing and
// the plan was empty however far the server had moved.
func TestAccResource_ProPolicy_DriftIsReported(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-policy-drift-" + suffix
	const command = "echo declared-by-terraform"

	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyDriftConfig(name, command),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "files_and_processes.execute_command", command),
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "general.enabled", "true"),
					capturePolicyID(&id),
				),
			},
			{
				PreConfig: func() { mutatePolicyOutOfBand(t, id) },
				Config:    policyDriftConfig(name, command),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(policyDriftResourceAddr, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "general.enabled", "true"),
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "general.trigger_checkin", "true"),
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "general.trigger_login", "true"),
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "general.trigger_other", "tf-acc-387"),
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "files_and_processes.execute_command", command),
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "maintenance.update_inventory", "true"),
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "maintenance.fix_disk_permissions", "true"),
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "restart_options.message", "Declared by Terraform."),
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "restart_options.delay_minutes", "5"),
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "user_interaction.start_message", "Starting."),
					resource.TestCheckResourceAttr(policyDriftResourceAddr, "user_interaction.complete_message", "Done."),
				),
			},
		},
	})
}
