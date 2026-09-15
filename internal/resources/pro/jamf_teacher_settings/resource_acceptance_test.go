// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package jamf_teacher_settings_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// The Jamf Teacher settings object is a tenant-wide LIVE singleton that always
// exists and cannot be deleted (Delete is state-only by design). Every test
// therefore:
//   - uses an INVERTED CheckDestroy — after `terraform destroy` the record must
//     still be readable on the tenant; and
//   - captures the tenant's pre-test settings up front and restores them
//     byte-for-byte via t.Cleanup as the final step, so the suite leaves the
//     live singleton exactly as it found it.
//
// Safelisted app names/bundle ids are placeholders (com.example.*) only —
// never tenant data.

const teacherResourceAddr = "jamfplatform_pro_jamf_teacher_settings.test"

// restoreTeacherBaseline reads the tenant's live Jamf Teacher settings and
// registers a cleanup that writes them back after the test (including after
// the destroy step), restoring the pre-test state.
func restoreTeacherBaseline(t *testing.T) {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	baseline, err := c.GetTeacherAppSettingsV1(context.Background())
	if err != nil {
		t.Fatalf("failed to read Jamf Teacher settings baseline: %v", err)
	}
	t.Cleanup(func() {
		if _, err := c.UpdateTeacherAppSettingsV1(context.Background(), requestFromResponse(baseline)); err != nil {
			t.Errorf("failed to restore Jamf Teacher settings baseline: %v", err)
		}
	})
}

// requestFromResponse converts a GET response into the full-replace PUT body
// that reproduces it. An empty autoClear re-persists as null (the wire's clear
// sentinel), so the echo round-trips.
func requestFromResponse(b *pro.TeacherSettingsResponse) *pro.TeacherSettingsRequest {
	tz := b.TimezoneID
	if tz == "" {
		// Defensive: timezoneId is mandatory on every PUT; a tenant should
		// never return an empty one, but a restore must not 500 if it does.
		tz = "UTC"
	}
	enabled := b.IsEnabled
	autoClear := b.AutoClear
	maxRes := b.MaxRestrictionLengthSeconds
	apps := make([]pro.SafelistedApp, len(b.SafelistedApps))
	copy(apps, b.SafelistedApps)
	return &pro.TeacherSettingsRequest{
		IsEnabled:                   &enabled,
		TimezoneID:                  &tz,
		AutoClear:                   &autoClear,
		MaxRestrictionLengthSeconds: &maxRes,
		SafelistedApps:              &apps,
	}
}

// checkTeacherSettingsStillExists verifies Delete did not remove the Jamf
// Teacher settings record. The Delete handler is documented as state-only, so a
// GET must still succeed after destroy.
func checkTeacherSettingsStillExists(t *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		got, err := c.GetTeacherAppSettingsV1(context.Background())
		if err != nil {
			return fmt.Errorf("expected Jamf Teacher settings record to persist after destroy: %w", err)
		}
		if got == nil {
			return fmt.Errorf("expected non-nil Jamf Teacher settings post-destroy")
		}
		return nil
	}
}

// teacherFullConfig renders a config declaring every attribute, with two
// placeholder safelisted apps.
func teacherFullConfig(enabled bool, endTime string, maxSeconds int) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_jamf_teacher_settings" "test" {
			enabled                          = %t
			timezone                         = "Europe/London"
			restrictions_end_time            = %q
			maximum_restriction_time_seconds = %d
			safelisted_apps = [
				{
					name      = "Example Calculator"
					bundle_id = "com.example.calculator"
				},
				{
					name      = "Example Notes"
					bundle_id = "com.example.notes"
				},
			]
		}
	`, enabled, endTime, maxSeconds)
}

// TestAccResource_ProJamfTeacherSettings_Update drives the full Update
// round-trip: a full-field apply, then the enabled toggle true→false→true
// (disable retains every other field — wire-probed), then the two clear shapes
// (`restrictions_end_time = ""` and `safelisted_apps = []`).
func TestAccResource_ProJamfTeacherSettings_Update(t *testing.T) {
	testhelpers.AccPreCheck(t)
	restoreTeacherBaseline(t)

	checkServerAutoClear := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetTeacherAppSettingsV1(context.Background())
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if got.AutoClear != want {
				return fmt.Errorf("autoClear = %q, want %q", got.AutoClear, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkTeacherSettingsStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: teacherFullConfig(true, "17:30:00", 3600),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(teacherResourceAddr, "id", "singleton"),
					resource.TestCheckResourceAttr(teacherResourceAddr, "enabled", "true"),
					resource.TestCheckResourceAttr(teacherResourceAddr, "timezone", "Europe/London"),
					resource.TestCheckResourceAttr(teacherResourceAddr, "restrictions_end_time", "17:30:00"),
					resource.TestCheckResourceAttr(teacherResourceAddr, "maximum_restriction_time_seconds", "3600"),
					resource.TestCheckResourceAttr(teacherResourceAddr, "safelisted_apps.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs(teacherResourceAddr, "safelisted_apps.*", map[string]string{
						"name":      "Example Calculator",
						"bundle_id": "com.example.calculator",
					}),
				),
			},
			{
				// Toggle off: every other field must survive (no disable coupling).
				Config: teacherFullConfig(false, "17:30:00", 3600),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(teacherResourceAddr, "enabled", "false"),
					resource.TestCheckResourceAttr(teacherResourceAddr, "restrictions_end_time", "17:30:00"),
					resource.TestCheckResourceAttr(teacherResourceAddr, "maximum_restriction_time_seconds", "3600"),
					resource.TestCheckResourceAttr(teacherResourceAddr, "safelisted_apps.#", "2"),
				),
			},
			{
				// Toggle back on.
				Config: teacherFullConfig(true, "17:30:00", 3600),
				Check:  resource.TestCheckResourceAttr(teacherResourceAddr, "enabled", "true"),
			},
			{
				// Clear step: "" is the documented clear sentinel for
				// restrictions_end_time (server persists null, echoes "");
				// state keeps the declared "" per the reconcile. Shrink the
				// safelist to one entry in the same step.
				Config: `
					resource "jamfplatform_pro_jamf_teacher_settings" "test" {
						enabled                          = true
						timezone                         = "Europe/London"
						restrictions_end_time            = ""
						maximum_restriction_time_seconds = 3600
						safelisted_apps = [
							{
								name      = "Example Calculator"
								bundle_id = "com.example.calculator"
							},
						]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(teacherResourceAddr, "restrictions_end_time", ""),
					checkServerAutoClear(""),
					resource.TestCheckResourceAttr(teacherResourceAddr, "safelisted_apps.#", "1"),
				),
			},
			{
				// `[]` clears the safelist entirely.
				Config: `
					resource "jamfplatform_pro_jamf_teacher_settings" "test" {
						enabled                          = true
						timezone                         = "Europe/London"
						restrictions_end_time            = ""
						maximum_restriction_time_seconds = 3600
						safelisted_apps                  = []
					}
				`,
				Check: resource.TestCheckResourceAttr(teacherResourceAddr, "safelisted_apps.#", "0"),
			},
		},
	})
}

// TestAccResource_ProJamfTeacherSettings_SplitOwnership proves the
// omit=preserve contract (§768.2) on maximum_restriction_time_seconds, the
// representative co-managed field:
//
//   - Step 1 (create = adopt): the field is set out of band BEFORE create and
//     the config OMITS it. The GET-on-create merge must adopt the existing
//     value rather than letting the full-replace PUT reset it to null.
//   - Step 2 (update = preserve): a UI edit changes it out of band while the
//     config still omits it and flips only enabled; the refreshed state +
//     UseStateForUnknown must carry the live value forward.
//   - Step 3 (take over): declaring the field lets Terraform own it.
func TestAccResource_ProJamfTeacherSettings_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	restoreTeacherBaseline(t)

	// setMaxRestrictionOutOfBand simulates a UI edit: read the live settings
	// and write the full object back (full-replace) with only the maximum
	// restriction time changed.
	setMaxRestrictionOutOfBand := func(v int) func() {
		return func() {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			ctx := context.Background()
			got, err := c.GetTeacherAppSettingsV1(ctx)
			if err != nil {
				t.Fatalf("out-of-band GET: %v", err)
			}
			body := requestFromResponse(got)
			body.MaxRestrictionLengthSeconds = &v
			if _, err := c.UpdateTeacherAppSettingsV1(ctx, body); err != nil {
				t.Fatalf("out-of-band PUT: %v", err)
			}
		}
	}

	checkServerMaxRestriction := func(want int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetTeacherAppSettingsV1(context.Background())
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if got.MaxRestrictionLengthSeconds != want {
				return fmt.Errorf("maxRestrictionLengthSeconds = %d, want %d", got.MaxRestrictionLengthSeconds, want)
			}
			return nil
		}
	}

	// Config omits maximum_restriction_time_seconds; enabled is the unrelated
	// managed field changed in step 2.
	cfg := func(enabled bool) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_jamf_teacher_settings" "test" {
				enabled  = %t
				timezone = "Europe/London"
			}
		`, enabled)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkTeacherSettingsStillExists(t),
		Steps: []resource.TestStep{
			{
				// Pin the field on the singleton BEFORE create, then create
				// with it omitted: the GET-on-create merge must adopt 11100,
				// not let the full-replace PUT null it.
				PreConfig: setMaxRestrictionOutOfBand(11100),
				Config:    cfg(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(teacherResourceAddr, "enabled", "true"),
					resource.TestCheckResourceAttr(teacherResourceAddr, "maximum_restriction_time_seconds", "11100"),
					checkServerMaxRestriction(11100),
				),
			},
			{
				// UI changes it out of band; config still omits it and flips
				// only enabled. The live 22200 must survive the write.
				PreConfig: setMaxRestrictionOutOfBand(22200),
				Config:    cfg(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(teacherResourceAddr, "enabled", "false"),
					resource.TestCheckResourceAttr(teacherResourceAddr, "maximum_restriction_time_seconds", "22200"),
					checkServerMaxRestriction(22200),
				),
			},
			{
				// Declaring the field explicitly lets Terraform take it over.
				Config: `
					resource "jamfplatform_pro_jamf_teacher_settings" "test" {
						enabled                          = false
						timezone                         = "Europe/London"
						maximum_restriction_time_seconds = 3600
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(teacherResourceAddr, "maximum_restriction_time_seconds", "3600"),
					checkServerMaxRestriction(3600),
				),
			},
		},
	})
}

// TestAccResource_ProJamfTeacherSettings_Import exercises the import
// round-trip with the canonical singleton id, then asserts the non-singleton
// import guard. The full config keeps every attribute populated so the
// post-import Read reproduces the pre-import state exactly.
func TestAccResource_ProJamfTeacherSettings_Import(t *testing.T) {
	testhelpers.AccPreCheck(t)
	restoreTeacherBaseline(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkTeacherSettingsStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: teacherFullConfig(true, "16:00:00", 7200),
			},
			{
				ResourceName:      teacherResourceAddr,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
			},
			{
				ResourceName:  teacherResourceAddr,
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_ProJamfTeacherSettings_InvalidEndTime verifies the HH:MM:SS
// validator rejects a non-canonical time at plan time (no tenant write). The
// regex matches a contiguous no-space token so it survives Terraform's ~80-col
// error wrapping.
func TestAccResource_ProJamfTeacherSettings_InvalidEndTime(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_jamf_teacher_settings" "test" {
						timezone              = "Europe/London"
						restrictions_end_time = "17:30"
					}
				`,
				ExpectError: regexp.MustCompile(`HH:MM:SS`),
			},
		},
	})
}
