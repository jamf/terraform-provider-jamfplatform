// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests talk to the Jamf Pro /v1/volume-purchasing-subscriptions endpoint — the
// admin UI "Notifications" tab under Settings → Volume purchasing.
//
// The write is FULL-REPLACE: every collection and `enabled` is emitted on each
// apply, and an empty set clears the field. The core lifecycle test exercises
// name/enabled/trigger/external-recipient add-and-remove and needs NO fixtures —
// triggers are enum strings and external recipients are plain emails. Two steps
// reference tenant objects and are gated:
//   - location_ids needs real Volume Purchasing location IDs → a
//     jamfplatform_pro_volume_purchasing_location fixture gated on
//     JAMFPLATFORM_ACC_PRO_VPP_TOKEN (a real ABM/ASM .vpptoken; same gate as the location
//     + assignment VPP tests). Token material MUST come from env — never commit it.
//   - internal_recipients references a Jamf Pro account id; the test stands up
//     its own jamfplatform_pro_account fixture and references its id — no env
//     var, no tenant prerequisite.

package volume_purchasing_notification_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const resAddr = "jamfplatform_pro_volume_purchasing_notification.test"

// vppTokenEnvVar holds the base64 `.vpptoken` used to stand up a location fixture.
const vppTokenEnvVar = "JAMFPLATFORM_ACC_PRO_VPP_TOKEN"

func testAccCheckNotificationDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_volume_purchasing_notification" {
				continue
			}
			_, err := c.GetVolumePurchasingSubscriptionV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Volume Purchasing notification %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Volume Purchasing notification %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// lifecycleConfig builds a notification with no tenant references. step selects the
// mutation:
//   - 1: one trigger, one external recipient, enabled.
//   - 2: both triggers, two external recipients, disabled (grow + toggle).
//   - 3: no triggers ([] clears), one external recipient (shrink).
func lifecycleConfig(name string, step int) string {
	var enabled, triggers, externals string
	switch step {
	case 2:
		enabled = "false"
		triggers = `["NO_MORE_LICENSES", "REMOVED_FROM_APP_STORE"]`
		externals = `
    { email = "first@example.com", name = "First Person" },
    { email = "second@example.com", name = "Second Person" },`
	case 3:
		enabled = "true"
		triggers = `[]`
		externals = `
    { email = "first@example.com", name = "First Person" },`
	default:
		enabled = "true"
		triggers = `["NO_MORE_LICENSES"]`
		externals = `
    { email = "first@example.com", name = "First Person" },`
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_volume_purchasing_notification" "test" {
  name     = %[1]q
  enabled  = %[2]s
  triggers = %[3]s
  external_recipients = [%[4]s
  ]
}
`, name, enabled, triggers, externals)
}

func TestAccResource_ProVolumePurchasingNotification(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-vpn-" + suffix
	renamed := "tf-acc-vpn-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: lifecycleConfig(name, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resAddr, "id"),
					resource.TestCheckResourceAttr(resAddr, "name", name),
					resource.TestCheckResourceAttr(resAddr, "enabled", "true"),
					resource.TestCheckResourceAttr(resAddr, "triggers.#", "1"),
					resource.TestCheckResourceAttr(resAddr, "external_recipients.#", "1"),
					resource.TestCheckResourceAttr(resAddr, "site_id", "-1"),
				),
			},
			{
				// Grow: rename, disable, add a trigger and an external recipient.
				Config: lifecycleConfig(renamed, 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "name", renamed),
					resource.TestCheckResourceAttr(resAddr, "enabled", "false"),
					resource.TestCheckResourceAttr(resAddr, "triggers.#", "2"),
					resource.TestCheckResourceAttr(resAddr, "external_recipients.#", "2"),
				),
			},
			{
				// Shrink: clear triggers ([]), drop back to one external recipient,
				// re-enable. The post-step plan fails on any residual diff, so the
				// full-replace clear is enforced implicitly.
				Config: lifecycleConfig(renamed, 3),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "enabled", "true"),
					resource.TestCheckResourceAttr(resAddr, "triggers.#", "0"),
					resource.TestCheckResourceAttr(resAddr, "external_recipients.#", "1"),
				),
			},
			{
				ResourceName:            resAddr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// locationsConfig adds a location fixture and references its id in location_ids.
func locationsConfig(token, suffix, name string, withLocation bool) string {
	locations := "[]"
	if withLocation {
		locations = "[jamfplatform_pro_volume_purchasing_location.vpp.id]"
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_volume_purchasing_location" "vpp" {
  name                                     = "tf-acc-vpn-loc-%[2]s"
  service_token                            = %[1]q
  service_token_wo_version                 = 1
  automatically_populate_purchased_content = true
}

resource "jamfplatform_pro_volume_purchasing_notification" "test" {
  name         = %[3]q
  triggers     = ["NO_MORE_LICENSES"]
  location_ids = %[4]s
}
`, token, suffix, name, locations)
}

// TestAccResource_ProVolumePurchasingNotification_Locations exercises the
// location_ids full-replace path (add then clear). Gated on a real VPP token.
func TestAccResource_ProVolumePurchasingNotification_Locations(t *testing.T) {
	token := testhelpers.AccEnv(vppTokenEnvVar)
	if token == "" {
		t.Skipf("%s not set; skipping location_ids test (needs a VPP location fixture)", vppTokenEnvVar)
	}
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-vpn-loc-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: locationsConfig(token, suffix, name, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "location_ids.#", "1"),
				),
			},
			{
				// Clear ([]) — full-replace empties the collection.
				Config: locationsConfig(token, suffix, name, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "location_ids.#", "0"),
				),
			},
		},
	})
}

// internalRecipientsConfig stands up a jamfplatform_pro_account fixture and
// references its id in internal_recipients (add when withRecipient, else clear
// to []). The account is present in both steps so it is created once; the
// notification clears the recipient before the account is destroyed.
func internalRecipientsConfig(name, suffix string, withRecipient bool) string {
	recipients := "[]"
	if withRecipient {
		recipients = "[jamfplatform_pro_account.recipient.id]"
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_account" "recipient" {
  username      = "tf-acc-vpn-recipient-%[3]s"
  full_name     = "TF Acc VPN Recipient"
  email_address = "tf-acc-vpn-recipient-%[3]s@example.invalid"
  access_level  = "Full Access"
  privilege_set = "Custom"

  password            = "Pr0bePassw0rd-%[3]s"
  password_wo_version = 1

  privileges = {
    jamf_pro_server_objects = ["Read Computers"]
  }
}

resource "jamfplatform_pro_volume_purchasing_notification" "test" {
  name                = %[1]q
  triggers            = ["NO_MORE_LICENSES"]
  internal_recipients = %[2]s
}
`, name, recipients, suffix)
}

// TestAccResource_ProVolumePurchasingNotification_InternalRecipients exercises the
// internal_recipients add-then-clear path against a self-provisioned account.
func TestAccResource_ProVolumePurchasingNotification_InternalRecipients(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-vpn-ir-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: internalRecipientsConfig(name, suffix, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "internal_recipients.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(resAddr, "internal_recipients.*", "jamfplatform_pro_account.recipient", "id"),
				),
			},
			{
				Config: internalRecipientsConfig(name, suffix, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "internal_recipients.#", "0"),
				),
			},
		},
	})
}

func TestAccDataSource_ProVolumePurchasingNotification_BySelectors(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-vpn-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: lifecycleConfig(name, 1) + `
data "jamfplatform_pro_volume_purchasing_notification" "by_id" {
  id = jamfplatform_pro_volume_purchasing_notification.test.id
}
data "jamfplatform_pro_volume_purchasing_notification" "by_name" {
  name = jamfplatform_pro_volume_purchasing_notification.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_volume_purchasing_notification.by_id", "name", resAddr, "name"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_volume_purchasing_notification.by_name", "id", resAddr, "id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_volume_purchasing_notification.by_id", "external_recipients.#", "1"),
				),
			},
		},
	})
}

func TestAccResource_ProVolumePurchasingNotification_DriftRecovery(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-vpn-drift-" + suffix
	cfg := lifecycleConfig(name, 1)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationDestroy(t),
		Steps: []resource.TestStep{
			{Config: cfg, Check: resource.TestCheckResourceAttrSet(resAddr, "id")},
			{
				PreConfig: func() {
					c := pro.New(testhelpers.NewAcceptanceClient(t))
					ctx := context.Background()
					listed, err := c.ListVolumePurchasingSubscriptionsV1(ctx, nil)
					if err != nil {
						t.Fatalf("drift preconfig list: %v", err)
					}
					for _, item := range listed {
						if item.Name == name {
							if err := c.DeleteVolumePurchasingSubscriptionV1(ctx, item.ID); err != nil {
								t.Fatalf("drift preconfig delete: %v", err)
							}
						}
					}
				},
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resAddr, "id"),
					resource.TestCheckResourceAttr(resAddr, "name", name),
				),
			},
		},
	})
}

// ---- ExpectError: plan-time validators (no API call) ---------------------------

func TestAccResource_ProVolumePurchasingNotification_InvalidTrigger(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jamfplatform_pro_volume_purchasing_notification" "test" {
  name     = "tf-acc-vpn-badtrigger"
  triggers = ["BOGUS_TRIGGER"]
}
`,
				// Anchor on a no-space token to dodge TF's ~80-col detail wrap.
				ExpectError: regexp.MustCompile(`NO_MORE_LICENSES`),
			},
		},
	})
}

func TestAccDataSource_ProVolumePurchasingNotification_AmbiguousSelector(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "jamfplatform_pro_volume_purchasing_notification" "bad" {
  id   = "1"
  name = "x"
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}
