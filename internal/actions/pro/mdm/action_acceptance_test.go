// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mdmactions_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// These acceptance tests invoke real MDM commands against live enrolled devices.
// Only the blank push is left to invoke: the fourteen device-effecting commands
// went with POST /v2/mdm/commands at the Platform API GA, taking their tests
// (remote desktop, device lock, clear restrictions password, enhanced log
// collection) with them. The plan-time guards below outlived those actions and
// now ride send_blank_push, which carries the same selectors and validators.
// They are gated on serial-number environment variables so the operator can
// supply and swap target devices without editing the suite; each test skips
// when its variable is unset. Invoking an action from config requires
// lifecycle.action_trigger (Terraform >= 1.14).
//
//	JAMFPLATFORM_ACC_PRO_COMPUTER_SERIAL   — a disposable enrolled computer
//	JAMFPLATFORM_ACC_PRO_COMPUTER_SERIAL_2 — a SECOND disposable enrolled computer,
//	                                     needed only by the mixed-selector test
//	JAMFPLATFORM_ACC_PRO_MOBILE_SERIAL     — a disposable supervised iPhone or iPad,
//	                                     for the lost-mode actions
//
// The lost-mode tests prove only that Jamf Pro accepted the command: a
// simulated device never acknowledges, so nothing here can observe Lost Mode
// switching on. Jamf Pro also folds a repeat of the same command into the one
// already waiting for that device, so a re-run answers success without queuing
// anything new.
const (
	envComputerSerial  = "JAMFPLATFORM_ACC_PRO_COMPUTER_SERIAL"
	envComputerSerial2 = "JAMFPLATFORM_ACC_PRO_COMPUTER_SERIAL_2"
	envMobileSerial    = "JAMFPLATFORM_ACC_PRO_MOBILE_SERIAL"
)

// fireConfig wraps an action block with a terraform_data trigger that invokes it
// after the trigger is created, ordering the action correctly behind any
// fixtures. A successful apply means the command was accepted by the server.
func fireConfig(actionBlock, actionRef string) string {
	return fmt.Sprintf(`
%s

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [%s]
    }
  }
}
`, actionBlock, actionRef)
}

// TestAccAction_ProSendBlankPush_Invoke is the benign smoke test: a blank push
// has no device-side effect beyond prompting a check-in.
func TestAccAction_ProSendBlankPush_Invoke(t *testing.T) {
	serial := testhelpers.AccEnv(envComputerSerial)
	if serial == "" {
		t.Skipf("%s not set; skipping blank push acceptance test", envComputerSerial)
	}
	testhelpers.AccPreCheck(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_send_blank_push" "push" {
  config {
    serial_numbers = [%q]
  }
}`, serial), "action.jamfplatform_pro_send_blank_push.push")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

// TestAccAction_ProSendBlankPush_MixedSelectors covers the additive path: a
// management id and a serial number in one invocation must both be targeted,
// rather than one selector silently winning.
func TestAccAction_ProSendBlankPush_MixedSelectors(t *testing.T) {
	first, second := testhelpers.AccEnv(envComputerSerial), testhelpers.AccEnv(envComputerSerial2)
	if first == "" || second == "" {
		t.Skipf("set %s and %s to run the mixed-selector acceptance test", envComputerSerial, envComputerSerial2)
	}
	testhelpers.AccPreCheck(t)

	// Resolve the second serial through the provider itself so the config
	// exercises management_ids alongside serial_numbers.
	config := fireConfig(fmt.Sprintf(`
data "jamfplatform_devices" "targets" {}

locals {
  second_id = one([
    for d in data.jamfplatform_devices.targets.devices : d.id
    if d.serial_number == %q
  ])
}

action "jamfplatform_pro_send_blank_push" "push" {
  config {
    serial_numbers = [%q]
    management_ids = [local.second_id]
  }
}`, second, first), "action.jamfplatform_pro_send_blank_push.push")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

// TestAccAction_ProCommand_NoTargetFails asserts the plan-time guard: with both
// selectors omitted the run must fail during validation, not part-way through an
// apply that has already commanded nothing.
func TestAccAction_ProCommand_NoTargetFails(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(`
action "jamfplatform_pro_send_blank_push" "push" {
  config {}
}`, "action.jamfplatform_pro_send_blank_push.push")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		// actionvalidator.AtLeastOneOf reports "Missing Attribute Configuration"
		// and names the selectors as a bracketed, space-free list. Both anchors
		// are short enough that Terraform's ~80-column wrapping cannot split
		// them, so neither straddles a line break.
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`(?s)Missing Attribute Configuration.*\[management_ids,serial_numbers\]`),
		}},
	})
}

// TestAccAction_ProCommand_EmptyListFails asserts the SizeAtLeast(1) validator:
// an empty list is not a valid way to say "no devices". Jamf Pro rejects an
// empty id list outright, so catching it at plan time is the better failure.
func TestAccAction_ProCommand_EmptyListFails(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(`
action "jamfplatform_pro_send_blank_push" "push" {
  config {
    serial_numbers = []
  }
}`, "action.jamfplatform_pro_send_blank_push.push")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`(?s)Invalid Attribute Value.*at least 1 element`),
		}},
	})
}

// TestAccAction_ProCommand_UnknownSerialFails covers the bulk resolver's error
// path: an unmatched serial must be named. This is the one place a bad target is
// attributable — the command POST itself fails without identifying any device.
func TestAccAction_ProCommand_UnknownSerialFails(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(`
action "jamfplatform_pro_send_blank_push" "push" {
  config {
    serial_numbers = ["NOSUCHSERIAL0001"]
  }
}`, "action.jamfplatform_pro_send_blank_push.push")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`NOSUCHSERIAL0001`),
		}},
	})
}

// accMobileSerial skips the test unless a disposable mobile device is declared.
func accMobileSerial(t *testing.T) string {
	t.Helper()
	serial := testhelpers.AccEnv(envMobileSerial)
	if serial == "" {
		t.Skipf("%s not set; skipping lost mode acceptance test", envMobileSerial)
	}
	testhelpers.AccPreCheck(t)
	return serial
}

func TestAccAction_ProEnableLostMode_Invoke(t *testing.T) {
	serial := accMobileSerial(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_enable_lost_mode" "lost" {
  config {
    serial_numbers = [%q]
    message        = "This device is lost. Please call the number below."
    phone_number   = "+1 555 0100"
    footnote       = "Reward offered"
  }
}`, serial), "action.jamfplatform_pro_enable_lost_mode.lost")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

// TestAccAction_ProEnableLostMode_PhoneOnly covers the other half of the
// at-least-one rule: a phone number with no message is accepted.
func TestAccAction_ProEnableLostMode_PhoneOnly(t *testing.T) {
	serial := accMobileSerial(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_enable_lost_mode" "lost" {
  config {
    serial_numbers = [%q]
    phone_number   = "+1 555 0100"
  }
}`, serial), "action.jamfplatform_pro_enable_lost_mode.lost")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

func TestAccAction_ProDisableLostMode_Invoke(t *testing.T) {
	serial := accMobileSerial(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_disable_lost_mode" "found" {
  config {
    serial_numbers = [%q]
  }
}`, serial), "action.jamfplatform_pro_disable_lost_mode.found")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

func TestAccAction_ProPlayLostModeSound_Invoke(t *testing.T) {
	serial := accMobileSerial(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_play_lost_mode_sound" "ping" {
  config {
    serial_numbers = [%q]
  }
}`, serial), "action.jamfplatform_pro_play_lost_mode_sound.ping")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

// TestAccAction_ProEnableLostMode_FootnoteAloneFails asserts the plan-time rule
// Jamf Pro enforces at the end of an apply: a footnote does not satisfy the
// message-or-phone requirement. Needs no device, since it fails at validation.
func TestAccAction_ProEnableLostMode_FootnoteAloneFails(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(`
action "jamfplatform_pro_enable_lost_mode" "lost" {
  config {
    serial_numbers = ["NOSUCHSERIAL0001"]
    footnote       = "Reward offered"
  }
}`, "action.jamfplatform_pro_enable_lost_mode.lost")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`(?s)Missing Attribute Configuration.*\[message,phone_number\]`),
		}},
	})
}

// TestAccAction_ProEnableLostMode_BlankMessageFails asserts that whitespace is
// not a message: Jamf Pro itself accepts it, so only plan-time validation stops
// a device locking behind an empty screen.
func TestAccAction_ProEnableLostMode_BlankMessageFails(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(`
action "jamfplatform_pro_enable_lost_mode" "lost" {
  config {
    serial_numbers = ["NOSUCHSERIAL0001"]
    message        = "   "
  }
}`, "action.jamfplatform_pro_enable_lost_mode.lost")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`(?s)Invalid Attribute Value.*visible`),
		}},
	})
}
