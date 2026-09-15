// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /mobiledeviceinvitations
// endpoint. The endpoint is create + delete only — every attribute is
// RequiresReplace — so a content change destroys and recreates the invitation
// (minting a new `invitation` code). Reconcile + CheckDestroy use GET-by-id,
// never the LIST endpoint, which lags newly created invitations.

package mobile_device_invitation_test

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

const mobileDeviceInvitationResourceAddress = "jamfplatform_pro_mobile_device_invitation.test"

// testAccCheckMobileDeviceInvitationDestroy verifies invitations created during
// the test were destroyed. Uses GET-by-id (the LIST endpoint lags new creates).
func testAccCheckMobileDeviceInvitationDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_mobile_device_invitation" {
				continue
			}
			_, err := c.GetMobileDeviceInvitationByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro mobile device invitation %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro mobile device invitation %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_ProMobileDeviceInvitation_Basic exercises create + read, then
// a RequiresReplace mutation that must destroy + recreate and mint a new
// `invitation` code, then import.
func TestAccResource_ProMobileDeviceInvitation_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	var firstInvitation string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileDeviceInvitationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_mobile_device_invitation" "test" {
						invitation_type       = "USER_INITIATED_URL"
						expiration_date       = "2030-12-31 23:59:00"
						multiple_uses_allowed = true
						require_login         = true
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(mobileDeviceInvitationResourceAddress, "id"),
					resource.TestCheckResourceAttrSet(mobileDeviceInvitationResourceAddress, "invitation"),
					resource.TestCheckResourceAttr(mobileDeviceInvitationResourceAddress, "invitation_type", "USER_INITIATED_URL"),
					// Drift reconciled at minute granularity — configured value preserved.
					resource.TestCheckResourceAttr(mobileDeviceInvitationResourceAddress, "expiration_date", "2030-12-31 23:59:00"),
					resource.TestCheckResourceAttr(mobileDeviceInvitationResourceAddress, "multiple_uses_allowed", "true"),
					resource.TestCheckResourceAttr(mobileDeviceInvitationResourceAddress, "require_login", "true"),
					resource.TestCheckResourceAttrSet(mobileDeviceInvitationResourceAddress, "expiration_date_epoch"),
					resource.TestCheckResourceAttrSet(mobileDeviceInvitationResourceAddress, "expiration_date_utc"),
					resource.TestCheckResourceAttrSet(mobileDeviceInvitationResourceAddress, "target_ios"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources[mobileDeviceInvitationResourceAddress]
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
					resource "jamfplatform_pro_mobile_device_invitation" "test" {
						invitation_type       = "USER_INITIATED_URL"
						expiration_date       = "2030-12-31 23:59:00"
						multiple_uses_allowed = false
						require_login         = true
					}
				`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(mobileDeviceInvitationResourceAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileDeviceInvitationResourceAddress, "multiple_uses_allowed", "false"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources[mobileDeviceInvitationResourceAddress]
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
				ResourceName:      mobileDeviceInvitationResourceAddress,
				ImportState:       true,
				ImportStateVerify: true,
				// Server-derived attributes not part of the imported config — all
				// justified ignores:
				//   - expiration_date: a finite value drifts ~1s on the wire and the
				//     managed resource preserves the user's configured value, but
				//     import has no prior config to preserve against — it adopts the
				//     drifted server echo, so the two cannot match as plain strings.
				//     The drift reconcile lives in the assign builder, not a
				//     StringSemanticEquals, so it does not apply to ImportStateVerify.
				//   - last_action / date_sent / date_sent_utc / date_sent_epoch: pure
				//     server-derived runtime fields populated only after the email is
				//     sent; not config-driven.
				//   - timeouts: provider-only block, never persisted server-side.
				// (expiration_date_epoch / _utc are verbatim-server on every read and
				// DO round-trip, so they are intentionally NOT ignored.)
				ImportStateVerifyIgnore: []string{
					"expiration_date",
					"last_action",
					"date_sent",
					"date_sent_utc",
					"date_sent_epoch",
					"timeouts",
				},
			},
		},
	})
}

// TestAccResource_ProMobileDeviceInvitation_InvalidType asserts the OneOf
// validator rejects an invitation_type outside the user-creatable set.
func TestAccResource_ProMobileDeviceInvitation_InvalidType(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileDeviceInvitationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_mobile_device_invitation" "test" {
						invitation_type = "DEP_CUSTOM_ENROLL"
					}
				`,
				ExpectError: regexp.MustCompile(`(?s)invitation_type.*USER_INITIATED_URL`),
			},
		},
	})
}

// TestAccResource_ProMobileDeviceInvitation_Unlimited covers the Unlimited
// expiration sentinel round-trip.
func TestAccResource_ProMobileDeviceInvitation_Unlimited(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileDeviceInvitationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_mobile_device_invitation" "test" {
						invitation_type = "USER_INITIATED_URL"
						expiration_date = "Unlimited"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileDeviceInvitationResourceAddress, "expiration_date", "Unlimited"),
					resource.TestCheckResourceAttr(mobileDeviceInvitationResourceAddress, "expiration_date_utc", "Unlimited"),
				),
			},
		},
	})
}
