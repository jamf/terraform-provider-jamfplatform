// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /computerinvitations endpoint.
// The endpoint is create + delete only — every attribute is RequiresReplace —
// so a content change destroys and recreates the invitation (minting a new
// `invitation` code). Reconcile + CheckDestroy use GET-by-id, never the LIST
// endpoint, which lags newly created invitations.

package computer_invitation_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const computerInvitationResourceAddress = "jamfplatform_pro_computer_invitation.test"

// testAccCheckComputerInvitationDestroy verifies invitations created during the
// test were destroyed. Uses GET-by-id (the LIST endpoint lags new creates).
func testAccCheckComputerInvitationDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_computer_invitation" {
				continue
			}
			_, err := c.GetComputerInvitationByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro computer invitation %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro computer invitation %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_ProComputerInvitation_Basic exercises create + read, then a
// RequiresReplace mutation that must destroy + recreate and mint a new
// `invitation` code, then import.
func TestAccResource_ProComputerInvitation_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	var firstInvitation string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckComputerInvitationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_computer_invitation" "test" {
						invitation_type                 = "USER_INITIATED_URL"
						expiration_date                 = "2030-12-31 23:59:00"
						multiple_uses_allowed            = true
						create_account_if_does_not_exist = true
						hide_account                     = true
						lock_down_ssh                    = true
						ssh_username                     = "jamfmgmt"
						ssh_password                     = "S3cret-Passw0rd!"
						ssh_password_wo_version          = 1
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(computerInvitationResourceAddress, "id"),
					resource.TestCheckResourceAttrSet(computerInvitationResourceAddress, "invitation"),
					resource.TestCheckResourceAttr(computerInvitationResourceAddress, "invitation_type", "USER_INITIATED_URL"),
					// Drift reconciled at minute granularity — configured value preserved.
					resource.TestCheckResourceAttr(computerInvitationResourceAddress, "expiration_date", "2030-12-31 23:59:00"),
					resource.TestCheckResourceAttr(computerInvitationResourceAddress, "multiple_uses_allowed", "true"),
					resource.TestCheckResourceAttr(computerInvitationResourceAddress, "ssh_username", "jamfmgmt"),
					resource.TestCheckResourceAttrSet(computerInvitationResourceAddress, "expiration_date_epoch"),
					resource.TestCheckResourceAttrSet(computerInvitationResourceAddress, "expiration_date_utc"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources[computerInvitationResourceAddress]
						if !ok {
							return fmt.Errorf("resource not found in state")
						}
						firstInvitation = rs.Primary.Attributes["invitation"]
						return nil
					},
				),
			},
			{
				// Change a RequiresReplace attribute → destroy + recreate.
				Config: `
					resource "jamfplatform_pro_computer_invitation" "test" {
						invitation_type                 = "USER_INITIATED_URL"
						expiration_date                 = "2030-12-31 23:59:00"
						multiple_uses_allowed            = false
						create_account_if_does_not_exist = true
						hide_account                     = true
						lock_down_ssh                    = true
						ssh_username                     = "jamfmgmt"
						ssh_password                     = "S3cret-Passw0rd!"
						ssh_password_wo_version          = 1
					}
				`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(computerInvitationResourceAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(computerInvitationResourceAddress, "multiple_uses_allowed", "false"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources[computerInvitationResourceAddress]
						if !ok {
							return fmt.Errorf("resource not found in state")
						}
						if got := rs.Primary.Attributes["invitation"]; got == firstInvitation {
							return fmt.Errorf("expected a new invitation code after replace, but it was unchanged (%s)", got)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      computerInvitationResourceAddress,
				ImportState:       true,
				ImportStateVerify: true,
				// Server-derived / WriteOnly attributes are not part of the
				// imported config and are justified ignores:
				//   - ssh_password / ssh_password_wo_version: WriteOnly secret +
				//     its rotation companion are never read back.
				//   - invitation_status / times_used / invited_user_uuid: pure
				//     server-derived runtime fields.
				//   - expiration_date: a finite value drifts ~1s on the wire and
				//     the managed resource preserves the user's configured value,
				//     but import has no prior config to preserve against — it
				//     adopts the drifted server echo, so the two cannot match as
				//     plain strings. The drift reconcile lives in the assign
				//     builder, not a StringSemanticEquals, so it does not apply
				//     to ImportStateVerify's comparison.
				//   - timeouts: provider-only block, never persisted server-side.
				// (expiration_date_epoch / _utc are verbatim-server on every
				// read and DO round-trip, so they are intentionally NOT ignored.)
				ImportStateVerifyIgnore: []string{
					"ssh_password",
					"ssh_password_wo_version",
					"invitation_status",
					"times_used",
					"invited_user_uuid",
					"expiration_date",
					"timeouts",
				},
			},
		},
	})
}

// TestAccResource_ProComputerInvitation_InvalidType asserts the OneOf validator
// rejects an invitation_type Jamf Pro does not recognise (PRESTAGE 409s on the
// wire, unlike the recognised types).
func TestAccResource_ProComputerInvitation_InvalidType(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckComputerInvitationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_computer_invitation" "test" {
						invitation_type = "PRESTAGE"
						ssh_username    = "jamfmgmt"
					}
				`,
				ExpectError: regexp.MustCompile(`(?s)invitation_type.*USER_INITIATED_URL`),
			},
		},
	})
}

// TestAccResource_ProComputerInvitation_Unlimited covers the Unlimited
// expiration sentinel round-trip.
func TestAccResource_ProComputerInvitation_Unlimited(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckComputerInvitationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_computer_invitation" "test" {
						invitation_type = "USER_INITIATED_URL"
						expiration_date = "Unlimited"
						ssh_username    = "jamfmgmt"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(computerInvitationResourceAddress, "expiration_date", "Unlimited"),
					resource.TestCheckResourceAttr(computerInvitationResourceAddress, "expiration_date_utc", "Unlimited"),
				),
			},
		},
	})
}
