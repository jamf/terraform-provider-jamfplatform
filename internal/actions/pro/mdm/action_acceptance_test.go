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

// onePixelPNG is a valid 1x1 PNG, base64-encoded, which is all the wallpaper
// tests need: Jamf Pro does not look at the image.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestAccAction_ProSetWallpaper_Image(t *testing.T) {
	serial := accMobileSerial(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_set_wallpaper" "paper" {
  config {
    serial_numbers = [%q]
    location       = "lock_screen"
    image          = %q
  }
}`, serial, onePixelPNG), "action.jamfplatform_pro_set_wallpaper.paper")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

// TestAccAction_ProSetWallpaper_IconID names icon 1, which every Jamf Pro tenant
// holds. Uploading one here would leave it behind: Jamf Pro has no way to delete
// an icon, and each upload mints a new id.
func TestAccAction_ProSetWallpaper_IconID(t *testing.T) {
	serial := accMobileSerial(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_set_wallpaper" "paper" {
  config {
    serial_numbers = [%q]
    location       = "home_screen"
    icon_id        = "1"
  }
}`, serial), "action.jamfplatform_pro_set_wallpaper.paper")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

// TestAccAction_ProSetWallpaper_ManagementIDs covers the resolution path: the
// command cannot address a device by Management ID, so the action looks up the
// serial number first.
func TestAccAction_ProSetWallpaper_ManagementIDs(t *testing.T) {
	serial := accMobileSerial(t)

	config := fireConfig(fmt.Sprintf(`
data "jamfplatform_devices" "targets" {}

locals {
  device_id = one([
    for d in data.jamfplatform_devices.targets.devices : d.id
    if d.serial_number == %q
  ])
}

action "jamfplatform_pro_set_wallpaper" "paper" {
  config {
    management_ids = [local.device_id]
    location       = "lock_and_home_screens"
    image          = %q
  }
}`, serial, onePixelPNG), "action.jamfplatform_pro_set_wallpaper.paper")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

func TestAccAction_ProSetWallpaper_NeedsAnImageSource(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(`
action "jamfplatform_pro_set_wallpaper" "paper" {
  config {
    serial_numbers = ["NOSUCHSERIAL0001"]
    location       = "lock_screen"
  }
}`, "action.jamfplatform_pro_set_wallpaper.paper")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`(?s)Missing Attribute Configuration.*\[image,icon_id\]`),
		}},
	})
}

// TestAccAction_ProSetWallpaper_BadImageFails asserts the plan-time image check,
// which exists because Jamf Pro accepts any bytes and the device is the first to
// reject them.
func TestAccAction_ProSetWallpaper_BadImageFails(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(`
action "jamfplatform_pro_set_wallpaper" "paper" {
  config {
    serial_numbers = ["NOSUCHSERIAL0001"]
    location       = "lock_screen"
    image          = base64encode("not an image")
  }
}`, "action.jamfplatform_pro_set_wallpaper.paper")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`(?s)Invalid Wallpaper Image.*PNG`),
		}},
	})
}

func TestAccAction_ProSetWallpaper_BadLocationFails(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_set_wallpaper" "paper" {
  config {
    serial_numbers = ["NOSUCHSERIAL0001"]
    location       = "desktop"
    image          = %q
  }
}`, onePixelPNG), "action.jamfplatform_pro_set_wallpaper.paper")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`(?s)Invalid Attribute Value.*lock_screen`),
		}},
	})
}

// TestAccAction_ProSetWallpaper_UnknownSerialFails covers the failure Jamf Pro
// cannot attribute: an unmatched device answers "Unable to match mobile_device"
// without naming it, so the action names the serials it sent.
func TestAccAction_ProSetWallpaper_UnknownSerialFails(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_set_wallpaper" "paper" {
  config {
    serial_numbers = ["NOSUCHSERIAL0001"]
    location       = "lock_screen"
    image          = %q
  }
}`, onePixelPNG), "action.jamfplatform_pro_set_wallpaper.paper")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`NOSUCHSERIAL0001`),
		}},
	})
}

// accComputerSerial skips the test unless a disposable computer is declared.
func accComputerSerial(t *testing.T) string {
	t.Helper()
	serial := testhelpers.AccEnv(envComputerSerial)
	if serial == "" {
		t.Skipf("%s not set; skipping computer command acceptance test", envComputerSerial)
	}
	testhelpers.AccPreCheck(t)
	return serial
}

// TestAccAction_ProClearPasscode_LooksUpTheToken leaves unlock_token out, so the
// action reads it from the device's inventory. A simulated device holds one.
func TestAccAction_ProClearPasscode_LooksUpTheToken(t *testing.T) {
	serial := accMobileSerial(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_clear_passcode" "clear" {
  config {
    serial_number = %q
  }
}`, serial), "action.jamfplatform_pro_clear_passcode.clear")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

// TestAccAction_ProClearPasscode_ExplicitToken skips the lookup. Jamf Pro does
// not validate the token, so a placeholder is accepted.
func TestAccAction_ProClearPasscode_ExplicitToken(t *testing.T) {
	serial := accMobileSerial(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_clear_passcode" "clear" {
  config {
    serial_number = %q
    unlock_token  = "dGVzdA=="
  }
}`, serial), "action.jamfplatform_pro_clear_passcode.clear")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

func TestAccAction_ProClearPasscode_BothSelectorsFail(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(`
action "jamfplatform_pro_clear_passcode" "clear" {
  config {
    serial_number = "NOSUCHSERIAL0001"
    management_id = "00000000-0000-0000-0000-000000000000"
  }
}`, "action.jamfplatform_pro_clear_passcode.clear")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`(?s)Invalid Attribute Combination.*\[management_id,serial_number\]`),
		}},
	})
}

func TestAccAction_ProClearPasscode_UnknownSerialFails(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(`
action "jamfplatform_pro_clear_passcode" "clear" {
  config {
    serial_number = "NOSUCHSERIAL0001"
  }
}`, "action.jamfplatform_pro_clear_passcode.clear")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`NOSUCHSERIAL0001`),
		}},
	})
}

func TestAccAction_ProSetRecoveryLock_Invoke(t *testing.T) {
	serial := accComputerSerial(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_set_recovery_lock" "lock" {
  config {
    serial_numbers = [%q]
    password       = "tf-acc-Recovery-1"
  }
}`, serial), "action.jamfplatform_pro_set_recovery_lock.lock")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

func TestAccAction_ProClearRecoveryLock_Invoke(t *testing.T) {
	serial := accComputerSerial(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_clear_recovery_lock" "unlock" {
  config {
    serial_numbers = [%q]
  }
}`, serial), "action.jamfplatform_pro_clear_recovery_lock.unlock")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

// TestAccAction_ProSetRecoveryLock_EmptyPasswordFails asserts that an empty
// password is refused here. Jamf Pro would take it as a request to clear the
// lock, so clearing has its own action.
func TestAccAction_ProSetRecoveryLock_EmptyPasswordFails(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(`
action "jamfplatform_pro_set_recovery_lock" "lock" {
  config {
    serial_numbers = ["NOSUCHSERIAL0001"]
    password       = ""
  }
}`, "action.jamfplatform_pro_set_recovery_lock.lock")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`(?s)Invalid Attribute Value.*at least 1`),
		}},
	})
}

func TestAccAction_ProSetAutoAdminPassword_Invoke(t *testing.T) {
	serial := accComputerSerial(t)

	config := fireConfig(fmt.Sprintf(`
action "jamfplatform_pro_set_auto_admin_password" "admin" {
  config {
    serial_number = %q
    guid          = "00000000-0000-0000-0000-000000000000"
    password      = "tf-acc-Admin-1"
  }
}`, serial), "action.jamfplatform_pro_set_auto_admin_password.admin")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    []resource.TestStep{{Config: config}},
	})
}

func TestAccAction_ProSetAutoAdminPassword_NeedsAGUID(t *testing.T) {
	testhelpers.AccPreCheck(t)

	config := fireConfig(`
action "jamfplatform_pro_set_auto_admin_password" "admin" {
  config {
    serial_number = "NOSUCHSERIAL0001"
    password      = "tf-acc-Admin-1"
  }
}`, "action.jamfplatform_pro_set_auto_admin_password.admin")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile(`(?s)Missing required argument.*guid`),
		}},
	})
}
