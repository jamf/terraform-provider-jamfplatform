// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package local_admin_password_settings_test

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

// The LAPS settings object is a tenant-wide singleton that always exists and
// cannot be deleted (Delete is state-only). Every test uses an INVERTED
// CheckDestroy: after `terraform destroy` the record must still be readable.
//
// LAPS is a live security setting, so each test snapshots the current settings
// and restores them via t.Cleanup — the tenant is left exactly as found.

const lapsResourceAddr = "jamfplatform_pro_local_admin_password_settings.test"

// snapshotAndRestoreLAPS captures the live LAPS settings now and registers a
// cleanup that writes them back, so the test leaves the tenant as found.
func snapshotAndRestoreLAPS(t *testing.T) {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	baseline, err := c.GetLocalAdminPasswordSettingsV2(ctx)
	if err != nil {
		t.Fatalf("snapshot LAPS settings: %v", err)
	}
	t.Cleanup(func() {
		if _, err := c.UpdateLocalAdminPasswordSettingsV2(context.Background(), lapsResponseToRequest(baseline)); err != nil {
			t.Errorf("restore LAPS settings: %v", err)
		}
	})
}

// lapsResponseToRequest mirrors a settings response into the request shape the
// SDK Update call expects. The two types carry identical fields; the SDK split
// them so a read result cannot be passed straight back to a write.
func lapsResponseToRequest(r *pro.LapsSettingsResponseV2) *pro.LapsSettingsRequestV2 {
	return &pro.LapsSettingsRequestV2{
		AutoDeployEnabled:        r.AutoDeployEnabled,
		AutoRotateEnabled:        r.AutoRotateEnabled,
		AutoRotateExpirationTime: r.AutoRotateExpirationTime,
		PasswordRotationTime:     r.PasswordRotationTime,
	}
}

// checkLAPSStillExists verifies Delete did not remove the record.
func checkLAPSStillExists(t *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		got, err := c.GetLocalAdminPasswordSettingsV2(context.Background())
		if err != nil {
			return fmt.Errorf("expected LAPS settings record to persist after destroy: %w", err)
		}
		if got == nil {
			return fmt.Errorf("expected non-nil LAPS settings post-destroy")
		}
		return nil
	}
}

func lapsConfig(enabled bool, rotationInterval, rotationAfterViewing string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_local_admin_password_settings" "test" {
			laps_for_prestage_accounts_enabled = %t
			rotation_interval                  = %q
			rotation_after_viewing_interval    = %q
		}
	`, enabled, rotationInterval, rotationAfterViewing)
}

// TestAccResource_ProLocalAdminPasswordSettings_Update drives the full Update
// round-trip across every attribute, including the Never <-> duration transition.
func TestAccResource_ProLocalAdminPasswordSettings_Update(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreLAPS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkLAPSStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: lapsConfig(true, "7 days", "1 day"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lapsResourceAddr, "id", "singleton"),
					resource.TestCheckResourceAttr(lapsResourceAddr, "laps_for_prestage_accounts_enabled", "true"),
					resource.TestCheckResourceAttr(lapsResourceAddr, "rotation_interval", "7 days"),
					resource.TestCheckResourceAttr(lapsResourceAddr, "rotation_after_viewing_interval", "1 day"),
				),
			},
			{
				// Flip the toggle, switch automatic rotation off, change the viewing interval.
				Config: lapsConfig(false, "Never", "12 hours"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lapsResourceAddr, "laps_for_prestage_accounts_enabled", "false"),
					resource.TestCheckResourceAttr(lapsResourceAddr, "rotation_interval", "Never"),
					resource.TestCheckResourceAttr(lapsResourceAddr, "rotation_after_viewing_interval", "12 hours"),
				),
			},
			{
				// Turn automatic rotation back on with a different duration.
				Config: lapsConfig(true, "180 days", "7 days"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lapsResourceAddr, "rotation_interval", "180 days"),
					resource.TestCheckResourceAttr(lapsResourceAddr, "rotation_after_viewing_interval", "7 days"),
				),
			},
		},
	})
}

// TestAccResource_ProLocalAdminPasswordSettings_NeverPreservesDormantExpiration
// proves the omit=preserve property for the dormant rotation expiration: with
// rotation_interval=Never, an out-of-band change to the (dormant) expiration must
// not surface a diff, because read short-circuits to "Never" on the disabled flag.
func TestAccResource_ProLocalAdminPasswordSettings_NeverPreservesDormantExpiration(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreLAPS(t)

	setDormantExpiration := func(seconds int) func() {
		return func() {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			ctx := context.Background()
			got, err := c.GetLocalAdminPasswordSettingsV2(ctx)
			if err != nil {
				t.Fatalf("out-of-band GET: %v", err)
			}
			req := lapsResponseToRequest(got)
			req.AutoRotateEnabled = false
			req.AutoRotateExpirationTime = seconds
			if _, err := c.UpdateLocalAdminPasswordSettingsV2(ctx, req); err != nil {
				t.Fatalf("out-of-band PUT: %v", err)
			}
		}
	}

	cfg := lapsConfig(true, "Never", "1 hour")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkLAPSStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lapsResourceAddr, "rotation_interval", "Never"),
				),
			},
			{
				// Mutate the dormant expiration out of band; the plan must stay empty
				// (Never short-circuits on the disabled flag, ignoring the dormant value).
				PreConfig:          setDormantExpiration(5184000),
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// TestAccResource_ProLocalAdminPasswordSettings_SplitOwnership proves the
// omit=preserve contract for an Optional+Computed control
// (laps_for_prestage_accounts_enabled) on this full-replace singleton:
//
//   - Step 1 (create = adopt): the toggle is set true out of band; the config
//     omits it. The GET-on-create merge must adopt true, not reset it.
//   - Step 2 (update = preserve): a UI edit flips it false out of band while the
//     config still omits it and changes only rotation_after_viewing_interval;
//     UseStateForUnknown must carry the live false forward.
//   - Step 3 (take over): declaring the toggle lets Terraform own it again.
func TestAccResource_ProLocalAdminPasswordSettings_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreLAPS(t)

	setDeployOutOfBand := func(v bool) func() {
		return func() {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			ctx := context.Background()
			got, err := c.GetLocalAdminPasswordSettingsV2(ctx)
			if err != nil {
				t.Fatalf("out-of-band GET: %v", err)
			}
			req := lapsResponseToRequest(got)
			req.AutoDeployEnabled = v
			if _, err := c.UpdateLocalAdminPasswordSettingsV2(ctx, req); err != nil {
				t.Fatalf("out-of-band PUT: %v", err)
			}
		}
	}

	checkServerDeploy := func(want bool) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetLocalAdminPasswordSettingsV2(context.Background())
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if got.AutoDeployEnabled != want {
				return fmt.Errorf("autoDeployEnabled = %v, want %v", got.AutoDeployEnabled, want)
			}
			return nil
		}
	}

	// Config omits the toggle; declares both rotation controls so step 2's change
	// is the rotation-after-viewing interval.
	cfg := func(afterViewing string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_local_admin_password_settings" "test" {
				rotation_interval               = "7 days"
				rotation_after_viewing_interval = %q
			}
		`, afterViewing)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkLAPSStillExists(t),
		Steps: []resource.TestStep{
			{
				PreConfig: setDeployOutOfBand(true),
				Config:    cfg("1 hour"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lapsResourceAddr, "laps_for_prestage_accounts_enabled", "true"),
					checkServerDeploy(true),
				),
			},
			{
				PreConfig: setDeployOutOfBand(false),
				Config:    cfg("3 hours"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lapsResourceAddr, "rotation_after_viewing_interval", "3 hours"),
					// State adopts the out-of-band value (Computed) and preserves it.
					resource.TestCheckResourceAttr(lapsResourceAddr, "laps_for_prestage_accounts_enabled", "false"),
					checkServerDeploy(false),
				),
			},
			{
				Config: `
					resource "jamfplatform_pro_local_admin_password_settings" "test" {
						laps_for_prestage_accounts_enabled = true
						rotation_interval                  = "7 days"
						rotation_after_viewing_interval    = "3 hours"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(lapsResourceAddr, "laps_for_prestage_accounts_enabled", "true"),
					checkServerDeploy(true),
				),
			},
		},
	})
}

// TestAccResource_ProLocalAdminPasswordSettings_Import exercises the import
// round-trip. All attributes are scalars, so ImportStateVerify is safe.
func TestAccResource_ProLocalAdminPasswordSettings_Import(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreLAPS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkLAPSStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: lapsConfig(true, "30 days", "3 days"),
			},
			{
				ResourceName:      lapsResourceAddr,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
			},
			{
				ResourceName:  lapsResourceAddr,
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_ProLocalAdminPasswordSettings_InvalidEnums verifies the OneOf
// validators reject unknown values. Regex fragments avoid spaces so they survive
// Terraform's ~80-col error wrapping.
func TestAccResource_ProLocalAdminPasswordSettings_InvalidEnums(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      lapsConfig(true, "90 days", "1 day"),
				ExpectError: regexp.MustCompile(`Never`),
			},
			{
				Config:      lapsConfig(true, "7 days", "2 hours"),
				ExpectError: regexp.MustCompile(`hours`),
			},
		},
	})
}

// TestAccDataSource_ProLocalAdminPasswordSettings_Basic applies the resource then
// reads it back through the data source.
func TestAccDataSource_ProLocalAdminPasswordSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreLAPS(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkLAPSStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: lapsConfig(true, "60 days", "3 hours") + `
					data "jamfplatform_pro_local_admin_password_settings" "ds" {
						depends_on = [jamfplatform_pro_local_admin_password_settings.test]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_local_admin_password_settings.ds", "id", "singleton"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_local_admin_password_settings.ds", "laps_for_prestage_accounts_enabled", "true"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_local_admin_password_settings.ds", "rotation_interval", "60 days"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_local_admin_password_settings.ds", "rotation_after_viewing_interval", "3 hours"),
				),
			},
		},
	})
}
