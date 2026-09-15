// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /webhooks endpoint. Classic
// has known concurrency issues when multiple writes hit the same resource type
// — keep these tests serial with any future classic acceptance work here.
//
// Coverage (maintainer ask): the apply-managed authentication_type values (NONE,
// BASIC, HEADER, HASH_SIGNATURE) with in-place transitions between them — MTLS is
// enum-valid (covered by the schema test) but not apply-tested as it needs a
// client certificate supplied through the admin UI; every one of
// the 23 events walked in-place; a smart-group fixture (jamfplatform_device_group)
// driving SmartGroupComputerMembershipChange + smart_group_id, including the
// clear-on-event-change path; password (WriteOnly) rotation; one ExpectError per
// cross-field validator and per OneOf enum; import round-trips; and drift
// recovery for a server-side toggle.

package webhook_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const webhookResourceAddr = "jamfplatform_pro_webhook.test"

// allWebhookEvents mirrors the wire-probed enum (WEBHOOK_SPIKE.md §3). Kept here
// independently so the acc test fails loudly if the production list drifts.
var allWebhookEvents = []string{
	"ComputerAdded", "ComputerCheckIn", "ComputerInventoryCompleted",
	"ComputerPatchPolicyCompleted", "ComputerPolicyFinished", "ComputerPushCapabilityChanged",
	"DeviceAddedToDEP", "DeviceRateLimited", "JSSShutdown", "JSSStartup",
	"MobileDeviceCheckIn", "MobileDeviceCommandCompleted", "MobileDeviceEnrolled",
	"MobileDeviceInventoryCompleted", "MobileDevicePushSent", "MobileDeviceUnEnrolled",
	"PatchSoftwareTitleUpdated", "PushSent", "RestAPIOperation", "SCEPChallenge",
	"SmartGroupComputerMembershipChange", "SmartGroupMobileDeviceMembershipChange",
	"SmartGroupUserMembershipChange",
}

// testAccCheckWebhookDestroy verifies records created during the test were
// destroyed.
func testAccCheckWebhookDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_webhook" {
				continue
			}
			_, err := c.GetWebhookByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro webhook %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro webhook %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// ---- config builders -----------------------------------------------------------

func webhookNoneConfig(name, url, event, contentType string, enabled bool) string {
	// authentication_type is set explicitly to NONE: it is Optional+Computed
	// with UseStateForUnknown, so omitting it RETAINS the prior value (e.g. when
	// transitioning back from BASIC/HEADER/HASH_SIGNATURE). Resetting to NONE is
	// an explicit config change, not an omission — mirror that here.
	return fmt.Sprintf(`
		resource "jamfplatform_pro_webhook" "test" {
			name                = %q
			url                 = %q
			event               = %q
			content_type        = %q
			enabled             = %t
			authentication_type = "NONE"
		}
	`, name, url, event, contentType, enabled)
}

func webhookEventOnlyConfig(name, event string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_webhook" "test" {
			name  = %q
			url   = "https://example.com/walk"
			event = %q
		}
	`, name, event)
}

func webhookBasicConfig(name, username, password string, woVersion int) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_webhook" "test" {
			name                = %q
			url                 = "https://example.com/basic"
			event               = "ComputerCheckIn"
			authentication_type = "BASIC"
			username            = %q
			password            = %q
			password_wo_version = %d
		}
	`, name, username, password, woVersion)
}

func webhookHeaderConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_webhook" "test" {
			name                = %q
			url                 = "https://example.com/header"
			event               = "MobileDeviceEnrolled"
			authentication_type = "HEADER"
			header              = jsonencode({ Authorization = "Bearer abc123" })
		}
	`, name)
}

func webhookHashSignatureConfig(name, secret, algo string, woVersion int) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_webhook" "test" {
			name                = %q
			url                 = "https://example.com/hash"
			event               = "ComputerPolicyFinished"
			authentication_type = "HASH_SIGNATURE"
			password            = %q
			password_wo_version = %d
			hash_algorithm      = %q
		}
	`, name, secret, woVersion, algo)
}

// smartGroupDeviceGroupFixture declares a smart computer device group whose
// Computed `jamf_pro_id` bridges the Platform Services group to its Jamf Pro
// classic numeric ID — exactly what the webhook <smart_group_id> consumes.
const smartGroupDeviceGroupFixture = `
	resource "jamfplatform_device_group" "sg" {
		name        = "%s-sg"
		group_type  = "smart"
		device_type = "computer"
		description = "Managed by Terraform acc test"
		criteria = [{
			criteria = "Serial Number"
			operator = "like"
			value    = ""
		}]
	}
`

// smartGroupWebhookConfig targets the device-group fixture's bridged classic ID
// via tonumber(.jamf_pro_id). enableDisplayFields toggles the only writable
// group-object knob.
func smartGroupWebhookConfig(name string, enableDisplayFields bool) string {
	return fmt.Sprintf(smartGroupDeviceGroupFixture+`
		resource "jamfplatform_pro_webhook" "test" {
			name                                   = %q
			url                                    = "https://example.com/smartgroup"
			event                                  = "SmartGroupComputerMembershipChange"
			smart_group_id                         = tonumber(jamfplatform_device_group.sg.jamf_pro_id)
			enable_display_fields_for_group_object = %t
		}
	`, name, name, enableDisplayFields)
}

// smartGroupClearedConfig keeps the fixture declared (so it is not destroyed
// mid-test) but flips the webhook to a non-smart event with no smart_group_id,
// exercising the -1 clear-on-event-change path.
func smartGroupClearedConfig(name string) string {
	return fmt.Sprintf(smartGroupDeviceGroupFixture+`
		resource "jamfplatform_pro_webhook" "test" {
			name  = %q
			url   = "https://example.com/smartgroup"
			event = "ComputerAdded"
		}
	`, name, name)
}

// webhookLiveAuthFields fetches the server's copy of the webhook under test
// and asserts the stored username and header, so the auth-type transitions can
// prove on the wire that the always-emitted empty elements cleared the fields
// the new type does not use — a null in state cannot tell a cleared value from
// one the classic merge retained and the state builder reconciled away (#384).
func webhookLiveAuthFields(t *testing.T, wantUsername, wantHeader string) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return testhelpers.CheckLiveObject(webhookResourceAddr, c.GetWebhookByID, func(w *proclassic.Webhook) error {
		if err := testhelpers.RequireEqual("username", wantUsername, testhelpers.Deref(w.Username)); err != nil {
			return err
		}
		return testhelpers.RequireEqual("header", wantHeader, testhelpers.Deref(w.Header))
	})
}

// ---- tests ----------------------------------------------------------------------

// TestAccResource_ProWebhook_NoneLifecycle covers the NONE happy path, a full
// attribute-mutation update, and an ImportStateVerify round-trip.
func TestAccResource_ProWebhook_NoneLifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-pro-webhook-none-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: webhookNoneConfig(name, "https://example.com/a", "ComputerAdded", "text/xml", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(webhookResourceAddr, "id"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "authentication_type", "NONE"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "content_type", "text/xml"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "enabled", "true"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "connection_timeout", "5"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "read_timeout", "2"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "hash_algorithm", "SHA256"),
					resource.TestCheckNoResourceAttr(webhookResourceAddr, "username"),
				),
			},
			{
				// Mutate every non-RequiresReplace attribute.
				Config: webhookNoneConfig(name, "https://example.com/b", "ComputerCheckIn", "application/json", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResourceAddr, "url", "https://example.com/b"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "event", "ComputerCheckIn"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "content_type", "application/json"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "enabled", "false"),
				),
			},
			{
				ResourceName:            webhookResourceAddr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password", "password_wo_version", "timeouts"},
			},
		},
	})
}

// TestAccResource_ProWebhook_AuthTransitions walks every supported auth type
// in-place, asserting the server-side clearing of inactive fields converges.
func TestAccResource_ProWebhook_AuthTransitions(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-pro-webhook-auth-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookDestroy(t),
		Steps: []resource.TestStep{
			{ // NONE
				Config: webhookNoneConfig(name, "https://example.com/auth", "ComputerAdded", "text/xml", true),
				Check:  resource.TestCheckResourceAttr(webhookResourceAddr, "authentication_type", "NONE"),
			},
			{ // NONE -> BASIC
				Config: webhookBasicConfig(name, "webhookuser", "basicpass", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResourceAddr, "authentication_type", "BASIC"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "username", "webhookuser"),
					// password is WriteOnly — never surfaced in state.
					resource.TestCheckNoResourceAttr(webhookResourceAddr, "password"),
				),
			},
			{ // BASIC -> HEADER (username must clear server-side)
				Config: webhookHeaderConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResourceAddr, "authentication_type", "HEADER"),
					resource.TestCheckResourceAttrSet(webhookResourceAddr, "header"),
					resource.TestCheckNoResourceAttr(webhookResourceAddr, "username"),
					webhookLiveAuthFields(t, "", `{"Authorization":"Bearer abc123"}`),
				),
			},
			{ // HEADER -> HASH_SIGNATURE (header must clear; >=16 char secret)
				Config: webhookHashSignatureConfig(name, "sixteencharsecret!!", "SHA512", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResourceAddr, "authentication_type", "HASH_SIGNATURE"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "hash_algorithm", "SHA512"),
					resource.TestCheckNoResourceAttr(webhookResourceAddr, "header"),
					webhookLiveAuthFields(t, "", ""),
				),
			},
			{ // HASH_SIGNATURE -> NONE
				Config: webhookNoneConfig(name, "https://example.com/auth", "ComputerAdded", "text/xml", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResourceAddr, "authentication_type", "NONE"),
					resource.TestCheckNoResourceAttr(webhookResourceAddr, "header"),
					resource.TestCheckNoResourceAttr(webhookResourceAddr, "username"),
					webhookLiveAuthFields(t, "", ""),
				),
			},
		},
	})
}

// TestAccResource_ProWebhook_AllEvents walks all 23 events in-place on a single
// webhook. Smart-group events carry no smart_group_id (the server stores the -1
// sentinel), so no fixture is needed; this covers every enum value plus the
// non-smart↔smart transitions.
func TestAccResource_ProWebhook_AllEvents(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-pro-webhook-events-" + testhelpers.RunSuffix()

	steps := make([]resource.TestStep, 0, len(allWebhookEvents))
	for _, ev := range allWebhookEvents {
		steps = append(steps, resource.TestStep{
			Config: webhookEventOnlyConfig(name, ev),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(webhookResourceAddr, "event", ev),
				resource.TestCheckResourceAttrSet(webhookResourceAddr, "id"),
			),
		})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookDestroy(t),
		Steps:                    steps,
	})
}

// TestAccResource_ProWebhook_SmartGroup drives the smart-group fixture path:
// create with smart_group_id from a smart computer device group, toggle the
// display-fields flag, then flip to a non-smart event so smart_group_id clears.
func TestAccResource_ProWebhook_SmartGroup(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-pro-webhook-sg-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: smartGroupWebhookConfig(name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResourceAddr, "event", "SmartGroupComputerMembershipChange"),
					resource.TestCheckResourceAttrSet(webhookResourceAddr, "smart_group_id"),
					resource.TestCheckResourceAttrPair(webhookResourceAddr, "smart_group_id", "jamfplatform_device_group.sg", "jamf_pro_id"),
					resource.TestCheckResourceAttr(webhookResourceAddr, "enable_display_fields_for_group_object", "false"),
				),
			},
			{
				// Toggle the display-fields flag (the only writable group-object knob).
				Config: smartGroupWebhookConfig(name, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResourceAddr, "enable_display_fields_for_group_object", "true"),
					resource.TestCheckResourceAttrSet(webhookResourceAddr, "smart_group_id"),
				),
			},
			{
				// Flip to a non-smart event: smart_group_id must clear to null.
				Config: smartGroupClearedConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(webhookResourceAddr, "event", "ComputerAdded"),
					resource.TestCheckNoResourceAttr(webhookResourceAddr, "smart_group_id"),
				),
			},
		},
	})
}

// TestAccResource_ProWebhook_PasswordRotation confirms bumping
// password_wo_version re-sends the BASIC password without error, and an
// unchanged version is a no-op (the secret is retained server-side).
func TestAccResource_ProWebhook_PasswordRotation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-pro-webhook-rot-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: webhookBasicConfig(name, "rotuser", "firstpass", 1),
				Check:  resource.TestCheckResourceAttr(webhookResourceAddr, "username", "rotuser"),
			},
			{
				// Bump wo_version + change the secret → re-sent.
				Config: webhookBasicConfig(name, "rotuser", "secondpass", 2),
				Check:  resource.TestCheckResourceAttr(webhookResourceAddr, "password_wo_version", "2"),
			},
			{
				// Same version, different (ignored) secret → password omitted on the
				// wire; plan is stable, username unchanged.
				Config: webhookBasicConfig(name, "rotuser", "thirdpass", 2),
				Check:  resource.TestCheckResourceAttr(webhookResourceAddr, "password_wo_version", "2"),
			},
		},
	})
}

// TestAccResource_ProWebhook_DriftRecovery makes an out-of-band change (toggle
// enabled via the SDK) and confirms Terraform detects and re-converges it.
func TestAccResource_ProWebhook_DriftRecovery(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-pro-webhook-drift-" + testhelpers.RunSuffix()
	cfg := webhookNoneConfig(name, "https://example.com/drift", "ComputerAdded", "text/xml", true)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check:  resource.TestCheckResourceAttr(webhookResourceAddr, "enabled", "true"),
			},
			{
				// Mutate the server out from under Terraform, then expect a plan.
				PreConfig: func() {
					c := proclassic.New(testhelpers.NewAcceptanceClient(t))
					ctx := context.Background()
					got, err := c.GetWebhookByName(ctx, name)
					if err != nil {
						t.Fatalf("drift preconfig: get webhook: %v", err)
					}
					disabled := false
					if err := c.UpdateWebhookByID(ctx, strconv.Itoa(*got.ID), &proclassic.Webhook{
						Name: got.Name, URL: got.URL, Event: got.Event, Enabled: &disabled,
					}); err != nil {
						t.Fatalf("drift preconfig: disable webhook: %v", err)
					}
				},
				Config:             cfg,
				ExpectNonEmptyPlan: false, // applies cleanly back to enabled=true
				Check:              resource.TestCheckResourceAttr(webhookResourceAddr, "enabled", "true"),
			},
		},
	})
}

// TestAccResource_ProWebhook_ValidatorErrors asserts one ExpectError per
// cross-field validator and per OneOf enum.
func TestAccResource_ProWebhook_ValidatorErrors(t *testing.T) {
	testhelpers.AccPreCheck(t)
	base := "tf-acc-pro-webhook-val-" + testhelpers.RunSuffix()

	cases := []struct {
		name   string
		config string
		expect *regexp.Regexp
	}{
		{
			name: "username requires basic",
			config: fmt.Sprintf(`
				resource "jamfplatform_pro_webhook" "test" {
					name = %q
					url = "https://e.com/x"
					event = "ComputerAdded"
					authentication_type = "NONE"
					username = "bob"
				}`, base+"-u"),
			expect: regexp.MustCompile("username requires BASIC"),
		},
		{
			name: "password requires basic or hash",
			config: fmt.Sprintf(`
				resource "jamfplatform_pro_webhook" "test" {
					name = %q
					url = "https://e.com/x"
					event = "ComputerAdded"
					authentication_type = "HEADER"
					header = "{}"
					password = "somesecretvalue!"
					password_wo_version = 1
				}`, base+"-p"),
			expect: regexp.MustCompile("password requires BASIC"),
		},
		{
			name: "header requires header auth",
			config: fmt.Sprintf(`
				resource "jamfplatform_pro_webhook" "test" {
					name = %q
					url = "https://e.com/x"
					event = "ComputerAdded"
					authentication_type = "NONE"
					header = "{}"
				}`, base+"-h"),
			expect: regexp.MustCompile("header requires HEADER"),
		},
		{
			name: "basic requires username",
			config: fmt.Sprintf(`
				resource "jamfplatform_pro_webhook" "test" {
					name = %q
					url = "https://e.com/x"
					event = "ComputerAdded"
					authentication_type = "BASIC"
					password = "somesecretvalue!"
					password_wo_version = 1
				}`, base+"-bu"),
			expect: regexp.MustCompile("username required for BASIC"),
		},
		{
			name: "header auth requires header",
			config: fmt.Sprintf(`
				resource "jamfplatform_pro_webhook" "test" {
					name = %q
					url = "https://e.com/x"
					event = "ComputerAdded"
					authentication_type = "HEADER"
				}`, base+"-hh"),
			expect: regexp.MustCompile("header required for HEADER"),
		},
		{
			name: "smart_group_id requires smart event",
			config: fmt.Sprintf(`
				resource "jamfplatform_pro_webhook" "test" {
					name = %q
					url = "https://e.com/x"
					event = "ComputerAdded"
					smart_group_id = 29
				}`, base+"-s"),
			expect: regexp.MustCompile("smart_group_id requires a SmartGroup"),
		},
		{
			name: "invalid authentication_type",
			config: fmt.Sprintf(`
				resource "jamfplatform_pro_webhook" "test" {
					name = %q
					url = "https://e.com/x"
					event = "ComputerAdded"
					authentication_type = "OAUTH"
				}`, base+"-a"),
			expect: regexp.MustCompile(`authentication_type`),
		},
		{
			name: "invalid event",
			config: fmt.Sprintf(`
				resource "jamfplatform_pro_webhook" "test" {
					name = %q
					url = "https://e.com/x"
					event = "NotAnEvent"
				}`, base+"-e"),
			expect: regexp.MustCompile(`event`),
		},
		{
			name: "invalid content_type",
			config: fmt.Sprintf(`
				resource "jamfplatform_pro_webhook" "test" {
					name = %q
					url = "https://e.com/x"
					event = "ComputerAdded"
					content_type = "text/plain"
				}`, base+"-c"),
			expect: regexp.MustCompile(`content_type`),
		},
		{
			name: "invalid hash_algorithm",
			config: fmt.Sprintf(`
				resource "jamfplatform_pro_webhook" "test" {
					name = %q
					url = "https://e.com/x"
					event = "ComputerAdded"
					hash_algorithm = "SHA1"
				}`, base+"-ha"),
			expect: regexp.MustCompile(`hash_algorithm`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      tc.config,
						PlanOnly:    true,
						ExpectError: tc.expect,
					},
				},
			})
		})
	}
}
