// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package jamf_parent_settings_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// The Jamf Parent settings object is a tenant-wide LIVE singleton that always
// exists and cannot be deleted (Delete is state-only by design). Every
// tenant-writing test therefore:
//   - uses an INVERTED CheckDestroy — after `terraform destroy` the record must
//     still be readable on the tenant; and
//   - captures the tenant's pre-test settings up front and restores them
//     byte-for-byte via t.Cleanup as the final step, so the suite leaves the
//     live singleton exactly as it found it.
//
// device_group_id needs a real MOBILE device group (the server validates the
// id with a 400 otherwise), so each tenant-writing test mints a static mobile
// device group fixture via the proclassic SDK and deletes it in cleanup —
// never a hardcoded tenant id. The fixture is created BEFORE the baseline
// restore is registered so cleanup (LIFO) restores the settings away from the
// fixture group before the group itself is deleted.
//
// Safelisted app names/bundle ids are placeholders (com.example.*) only —
// never tenant data.

const parentResourceAddr = "jamfplatform_pro_jamf_parent_settings.test"

// createMobileDeviceGroupFixture mints a static mobile device group for
// device_group_id to point at and registers its deletion in cleanup. The name
// carries the run-wide unique suffix to avoid cross-run collisions.
func createMobileDeviceGroupFixture(t *testing.T) int {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	name := "tf-acc-parent-settings-fixture-" + testhelpers.RunSuffix()
	isSmart := false
	got, err := c.CreateMobileDeviceGroupByID(ctx, "0", &proclassic.MobileDeviceGroup{
		Name:    &name,
		IsSmart: &isSmart,
	})
	if err != nil || got == nil || got.ID == nil {
		t.Fatalf("CreateMobileDeviceGroupByID(%q): %v", name, err)
	}
	id := *got.ID
	t.Cleanup(func() {
		if err := c.DeleteMobileDeviceGroupByID(context.Background(), fmt.Sprintf("%d", id)); err != nil && !helpers.IsNotFoundError(err) {
			t.Logf("cleanup DeleteMobileDeviceGroupByID(%d): %v", id, err)
		}
	})
	return id
}

// restoreParentBaseline reads the tenant's live Jamf Parent settings and
// registers a cleanup that writes them back after the test (including after
// the destroy step), restoring the pre-test state. The GET and PUT share one
// struct, so the baseline round-trips verbatim — including the unmodeled
// allowTemplates. A tenant whose Jamf Parent page was never configured
// (deviceGroupId 0) cannot be restored via PUT (the server 400s on group id
// 0); that surfaces as the restore error below.
func restoreParentBaseline(t *testing.T) {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	baseline, err := c.GetParentAppSettingsV1(context.Background())
	if err != nil {
		t.Fatalf("failed to read Jamf Parent settings baseline: %v", err)
	}
	// Defensive: restrictedTimes and timezoneId are mandatory on every PUT; a
	// tenant should never return a nil map or empty zone, but a restore must
	// not 500 if it does.
	if baseline.RestrictedTimes == nil {
		baseline.RestrictedTimes = map[string]pro.TimeFrame{}
	}
	if baseline.TimezoneID == "" {
		baseline.TimezoneID = "UTC"
	}
	t.Cleanup(func() {
		if _, err := c.UpdateParentAppSettingsV1(context.Background(), baseline); err != nil {
			t.Errorf("failed to restore Jamf Parent settings baseline: %v", err)
		}
	})
}

// checkParentSettingsStillExists verifies Delete did not remove the Jamf
// Parent settings record. The Delete handler is documented as state-only, so a
// GET must still succeed after destroy.
func checkParentSettingsStillExists(t *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		got, err := c.GetParentAppSettingsV1(context.Background())
		if err != nil {
			return fmt.Errorf("expected Jamf Parent settings record to persist after destroy: %w", err)
		}
		if got == nil {
			return fmt.Errorf("expected non-nil Jamf Parent settings post-destroy")
		}
		return nil
	}
}

// parentConfig renders a config with the given device group, enabled flag, and
// raw HCL for the restricted_times map and the safelisted_apps set.
func parentConfig(groupID int, enabled bool, restrictedTimes, safelist string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_jamf_parent_settings" "test" {
			enabled          = %t
			timezone         = "Europe/London"
			device_group_id  = %d
			restricted_times = %s
			safelisted_apps  = %s
		}
	`, enabled, groupID, restrictedTimes, safelist)
}

const twoDayTimes = `{
		MONDAY = { begin_time = "08:30:00", end_time = "15:30:00" }
		FRIDAY = { begin_time = "09:00:00", end_time = "14:00:00" }
	}`

const twoAppSafelist = `[
		{
			name      = "Example Calculator"
			bundle_id = "com.example.calculator"
		},
		{
			name      = "Example Notes"
			bundle_id = "com.example.notes"
		},
	]`

// TestAccResource_ProJamfParentSettings_Update drives the full Update
// round-trip: a full-field apply with a partial restricted_times map
// (MONDAY+FRIDAY), changed times plus day-set grow (WEDNESDAY) and shrink,
// the empty-map step (no restrictions), the enabled toggle (disable retains
// every other field — wire-probed), and the safelist `[]` clear.
func TestAccResource_ProJamfParentSettings_Update(t *testing.T) {
	testhelpers.AccPreCheck(t)
	groupID := createMobileDeviceGroupFixture(t)
	restoreParentBaseline(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkParentSettingsStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: parentConfig(groupID, true, twoDayTimes, twoAppSafelist),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(parentResourceAddr, "id", "singleton"),
					resource.TestCheckResourceAttr(parentResourceAddr, "enabled", "true"),
					resource.TestCheckResourceAttr(parentResourceAddr, "timezone", "Europe/London"),
					resource.TestCheckResourceAttr(parentResourceAddr, "device_group_id", fmt.Sprintf("%d", groupID)),
					resource.TestCheckResourceAttr(parentResourceAddr, "restricted_times.%", "2"),
					resource.TestCheckResourceAttr(parentResourceAddr, "restricted_times.MONDAY.begin_time", "08:30:00"),
					resource.TestCheckResourceAttr(parentResourceAddr, "restricted_times.MONDAY.end_time", "15:30:00"),
					resource.TestCheckResourceAttr(parentResourceAddr, "restricted_times.FRIDAY.begin_time", "09:00:00"),
					resource.TestCheckResourceAttr(parentResourceAddr, "safelisted_apps.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs(parentResourceAddr, "safelisted_apps.*", map[string]string{
						"name":      "Example Calculator",
						"bundle_id": "com.example.calculator",
					}),
				),
			},
			{
				// Change MONDAY's times and grow the day set (+WEDNESDAY).
				Config: parentConfig(groupID, true, `{
					MONDAY    = { begin_time = "07:45:00", end_time = "16:15:00" }
					WEDNESDAY = { begin_time = "10:00:00", end_time = "12:00:00" }
					FRIDAY    = { begin_time = "09:00:00", end_time = "14:00:00" }
				}`, twoAppSafelist),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(parentResourceAddr, "restricted_times.%", "3"),
					resource.TestCheckResourceAttr(parentResourceAddr, "restricted_times.MONDAY.begin_time", "07:45:00"),
					resource.TestCheckResourceAttr(parentResourceAddr, "restricted_times.WEDNESDAY.begin_time", "10:00:00"),
				),
			},
			{
				// Shrink the day set back to one day — the server stores only
				// the present keys (no zero-fill, wire-probed).
				Config: parentConfig(groupID, true, `{
					MONDAY = { begin_time = "07:45:00", end_time = "16:15:00" }
				}`, twoAppSafelist),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(parentResourceAddr, "restricted_times.%", "1"),
					resource.TestCheckResourceAttr(parentResourceAddr, "restricted_times.MONDAY.end_time", "16:15:00"),
				),
			},
			{
				// Empty map = no restrictions (valid wire shape).
				Config: parentConfig(groupID, true, `{}`, twoAppSafelist),
				Check:  resource.TestCheckResourceAttr(parentResourceAddr, "restricted_times.%", "0"),
			},
			{
				// Toggle off: every other field must survive (no disable
				// coupling, wire-probed).
				Config: parentConfig(groupID, false, `{}`, twoAppSafelist),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(parentResourceAddr, "enabled", "false"),
					resource.TestCheckResourceAttr(parentResourceAddr, "timezone", "Europe/London"),
					resource.TestCheckResourceAttr(parentResourceAddr, "safelisted_apps.#", "2"),
				),
			},
			{
				// `[]` clears the safelist entirely.
				Config: parentConfig(groupID, false, `{}`, `[]`),
				Check:  resource.TestCheckResourceAttr(parentResourceAddr, "safelisted_apps.#", "0"),
			},
		},
	})
}

// TestAccResource_ProJamfParentSettings_SplitOwnership proves the
// omit=preserve contract (§768.2) on allow_clear_passcode, the representative
// co-managed field:
//
//   - Step 1 (create = adopt): the field is set out of band BEFORE create and
//     the config OMITS it. The GET-on-create merge must adopt the existing
//     value rather than letting the full-replace PUT reset it to false.
//   - Step 2 (update = preserve): a UI edit changes it out of band while the
//     config still omits it and flips only enabled; the refreshed state +
//     UseStateForUnknown must carry the live value forward.
//   - Step 3 (take over): declaring the field lets Terraform own it.
func TestAccResource_ProJamfParentSettings_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	groupID := createMobileDeviceGroupFixture(t)
	restoreParentBaseline(t)

	// setAllowClearPasscodeOutOfBand simulates a UI edit: read the live
	// settings and write the full object back (one struct both directions)
	// with only allowClearPasscode changed.
	setAllowClearPasscodeOutOfBand := func(v bool) func() {
		return func() {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			ctx := context.Background()
			got, err := c.GetParentAppSettingsV1(ctx)
			if err != nil {
				t.Fatalf("out-of-band GET: %v", err)
			}
			got.AllowClearPasscode = &v
			if got.DeviceGroupID == 0 {
				// An unconfigured tenant cannot round-trip; point the write at
				// the fixture group so the PUT is accepted.
				got.DeviceGroupID = groupID
			}
			if _, err := c.UpdateParentAppSettingsV1(ctx, got); err != nil {
				t.Fatalf("out-of-band PUT: %v", err)
			}
		}
	}

	checkServerAllowClearPasscode := func(want bool) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetParentAppSettingsV1(context.Background())
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if got.AllowClearPasscode == nil || *got.AllowClearPasscode != want {
				return fmt.Errorf("allowClearPasscode = %v, want %v", got.AllowClearPasscode, want)
			}
			return nil
		}
	}

	// Config omits allow_clear_passcode; enabled is the unrelated managed
	// field changed in step 2.
	cfg := func(enabled bool) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_jamf_parent_settings" "test" {
				enabled          = %t
				timezone         = "Europe/London"
				device_group_id  = %d
				restricted_times = {}
			}
		`, enabled, groupID)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkParentSettingsStillExists(t),
		Steps: []resource.TestStep{
			{
				// Pin the field on the singleton BEFORE create, then create
				// with it omitted: the GET-on-create merge must adopt true,
				// not let the full-replace PUT reset it to false.
				PreConfig: setAllowClearPasscodeOutOfBand(true),
				Config:    cfg(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(parentResourceAddr, "enabled", "true"),
					resource.TestCheckResourceAttr(parentResourceAddr, "allow_clear_passcode", "true"),
					checkServerAllowClearPasscode(true),
				),
			},
			{
				// UI changes it out of band; config still omits it and flips
				// only enabled. The live false must survive the write.
				PreConfig: setAllowClearPasscodeOutOfBand(false),
				Config:    cfg(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(parentResourceAddr, "enabled", "false"),
					resource.TestCheckResourceAttr(parentResourceAddr, "allow_clear_passcode", "false"),
					checkServerAllowClearPasscode(false),
				),
			},
			{
				// Declaring the field explicitly lets Terraform take it over.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_jamf_parent_settings" "test" {
						enabled              = false
						timezone             = "Europe/London"
						device_group_id      = %d
						restricted_times     = {}
						allow_clear_passcode = true
					}
				`, groupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(parentResourceAddr, "allow_clear_passcode", "true"),
					checkServerAllowClearPasscode(true),
				),
			},
		},
	})
}

// TestAccResource_ProJamfParentSettings_AllowTemplatesRoundTrip pins the
// §768.3 round-trip of the unmodeled allowTemplates field: set it out of band
// to the non-baseline value, apply a Terraform change, and assert the value
// survived the provider's full-replace PUT (without the round-trip, every
// Terraform write would reset it to the server default true).
func TestAccResource_ProJamfParentSettings_AllowTemplatesRoundTrip(t *testing.T) {
	testhelpers.AccPreCheck(t)
	groupID := createMobileDeviceGroupFixture(t)
	restoreParentBaseline(t)

	// want holds the out-of-band value the provider's PUT must not disturb;
	// written by the PreConfig below before the step that asserts it.
	var want bool

	flipAllowTemplatesOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		got, err := c.GetParentAppSettingsV1(ctx)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}
		// Flip to the non-current value (nil counts as the server default
		// true, so flip to false).
		want = got.AllowTemplates == nil || !*got.AllowTemplates
		got.AllowTemplates = &want
		if _, err := c.UpdateParentAppSettingsV1(ctx, got); err != nil {
			t.Fatalf("out-of-band PUT: %v", err)
		}
	}

	checkServerAllowTemplates := func() resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetParentAppSettingsV1(context.Background())
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if got.AllowTemplates == nil || *got.AllowTemplates != want {
				return fmt.Errorf("allowTemplates = %v, want %v — the §768.3 round-trip failed", got.AllowTemplates, want)
			}
			return nil
		}
	}

	cfg := func(enabled bool) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_jamf_parent_settings" "test" {
				enabled          = %t
				timezone         = "Europe/London"
				device_group_id  = %d
				restricted_times = {}
			}
		`, enabled, groupID)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkParentSettingsStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: cfg(true),
			},
			{
				// Set allowTemplates out of band, then apply a Terraform
				// change (flip enabled): the provider's GET → overlay → PUT
				// must carry the out-of-band value through unchanged.
				PreConfig: flipAllowTemplatesOutOfBand,
				Config:    cfg(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(parentResourceAddr, "enabled", "false"),
					checkServerAllowTemplates(),
				),
			},
		},
	})
}

// TestAccResource_ProJamfParentSettings_Import exercises the import
// round-trip with the canonical singleton id, then asserts the non-singleton
// import guard. The full config keeps every attribute populated so the
// post-import Read reproduces the pre-import state exactly.
func TestAccResource_ProJamfParentSettings_Import(t *testing.T) {
	testhelpers.AccPreCheck(t)
	groupID := createMobileDeviceGroupFixture(t)
	restoreParentBaseline(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkParentSettingsStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: parentConfig(groupID, true, twoDayTimes, twoAppSafelist),
			},
			{
				ResourceName:      parentResourceAddr,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
			},
			{
				ResourceName:  parentResourceAddr,
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_ProJamfParentSettings_InvalidDayKey verifies the map-key
// validator rejects a non-uppercase-day key at plan time (no tenant write).
// The server enforces strict-UPPERCASE java.time.DayOfWeek keys with a 400;
// the validator surfaces that before any API call. The regex matches the
// validator's summary line, which survives Terraform's ~80-col error wrapping.
func TestAccResource_ProJamfParentSettings_InvalidDayKey(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_jamf_parent_settings" "test" {
						timezone        = "Europe/London"
						device_group_id = 1
						restricted_times = {
							FUNDAY = { begin_time = "08:30:00", end_time = "15:30:00" }
						}
					}
				`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Value Match`),
			},
		},
	})
}

// TestAccResource_ProJamfParentSettings_InvalidTime verifies the HH:MM:SS
// validator rejects a non-canonical time at plan time (no tenant write). The
// regex matches a contiguous no-space token so it survives Terraform's
// ~80-col error wrapping.
func TestAccResource_ProJamfParentSettings_InvalidTime(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_jamf_parent_settings" "test" {
						timezone        = "Europe/London"
						device_group_id = 1
						restricted_times = {
							MONDAY = { begin_time = "25:99:00", end_time = "15:30:00" }
						}
					}
				`,
				ExpectError: regexp.MustCompile(`HH:MM:SS`),
			},
		},
	})
}
