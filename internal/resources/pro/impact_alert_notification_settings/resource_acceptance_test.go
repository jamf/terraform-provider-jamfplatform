// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package impact_alert_notification_settings_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// checkSingletonRecordStillExists verifies the Jamf Pro Impact Alert Notification settings
// record persists on the tenant after Terraform destroys the resource from state.
// Canonical singleton acceptance check: the remote Delete is a no-op, so the API must
// still return the record (with whatever value was last applied) post-destroy.
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "Impact Alert Notification settings", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetImpactAlertNotificationSettingsV1(ctx)
	})
}

// TestAccResource_ProImpactAlertNotificationSettings_Basic mutates every toggle across two
// Update steps against a real tenant. Each step is independently valid w.r.t. the
// server-enforced confirmation-code ↔ alert dependency: step 1 enables both alerts and
// both confirmation codes; step 2 disables everything (confirmation codes are turned off
// in the same apply as their alerts, never leaving a confcode=true/alert=false
// intermediate). Singleton resources have no remote Delete, so CheckDestroy verifies the
// record PERSISTS on the tenant after Terraform stops managing it.
func TestAccResource_ProImpactAlertNotificationSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const addr = "jamfplatform_pro_impact_alert_notification_settings.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_impact_alert_notification_settings" "test" {
						deployable_objects_alert_enabled             = true
						deployable_objects_confirmation_code_enabled = true
						scopeable_objects_alert_enabled              = true
						scopeable_objects_confirmation_code_enabled  = true
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "singleton"),
					resource.TestCheckResourceAttr(addr, "deployable_objects_alert_enabled", "true"),
					resource.TestCheckResourceAttr(addr, "deployable_objects_confirmation_code_enabled", "true"),
					resource.TestCheckResourceAttr(addr, "scopeable_objects_alert_enabled", "true"),
					resource.TestCheckResourceAttr(addr, "scopeable_objects_confirmation_code_enabled", "true"),
				),
			},
			{
				Config: `
					resource "jamfplatform_pro_impact_alert_notification_settings" "test" {
						deployable_objects_alert_enabled             = false
						deployable_objects_confirmation_code_enabled = false
						scopeable_objects_alert_enabled              = false
						scopeable_objects_confirmation_code_enabled  = false
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "deployable_objects_alert_enabled", "false"),
					resource.TestCheckResourceAttr(addr, "deployable_objects_confirmation_code_enabled", "false"),
					resource.TestCheckResourceAttr(addr, "scopeable_objects_alert_enabled", "false"),
					resource.TestCheckResourceAttr(addr, "scopeable_objects_confirmation_code_enabled", "false"),
				),
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
				},
			},
		},
	})
}

// TestAccResource_ProImpactAlertNotificationSettings_CreateAdopt proves the singleton
// GET-on-create-adopt: when a toggle is omitted from HCL on the FIRST apply, the provider
// reads the live settings and re-sends the existing value rather than resetting it to
// false on the full-replace PUT. Without GET-on-create-adopt this regresses — create-omit
// sends false and clobbers the admin's value.
func TestAccResource_ProImpactAlertNotificationSettings_CreateAdopt(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const addr = "jamfplatform_pro_impact_alert_notification_settings.test"

	// Pin a known state out of band BEFORE Terraform creates the resource:
	// deployable_objects_alert_enabled=true (the discriminator), every other toggle false.
	setBaselineOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		body := &pro.ImpactAlertNotificationSettingsV1{
			DeployableObjectsAlertEnabled:            true,
			DeployableObjectsConfirmationCodeEnabled: false,
			ScopeableObjectsAlertEnabled:             false,
			ScopeableObjectsConfirmationCodeEnabled:  false,
		}
		if err := c.UpdateImpactAlertNotificationSettingsV1(context.Background(), body); err != nil {
			t.Fatalf("out-of-band baseline PUT: %v", err)
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				// Create declaring NOTHING; every toggle omitted. GET-on-create-adopt must
				// preserve the out-of-band deployable_objects_alert_enabled=true.
				PreConfig: setBaselineOutOfBand,
				Config: `
					resource "jamfplatform_pro_impact_alert_notification_settings" "test" {}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Adopted from the live settings (not reset to false).
					resource.TestCheckResourceAttr(addr, "deployable_objects_alert_enabled", "true"),
					resource.TestCheckResourceAttr(addr, "scopeable_objects_alert_enabled", "false"),
				),
			},
		},
	})
}

// TestAccResource_ProImpactAlertNotificationSettings_RejectsNonSingletonImport verifies the
// ImportState guard: any identifier other than "singleton" must fail with a clear error
// rather than silently normalizing to the singleton ID.
func TestAccResource_ProImpactAlertNotificationSettings_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_impact_alert_notification_settings" "test" {}
				`,
			},
			{
				ResourceName:  "jamfplatform_pro_impact_alert_notification_settings.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_ProImpactAlertNotificationSettings_RejectsConfWithoutAlert verifies the
// plan-time ConfigValidator rejects a confirmation code declared without its matching
// alert (both fields explicitly declared). The error summary is short and renders on its
// own line, so the regex matches it without whitespace-wrap concerns.
func TestAccResource_ProImpactAlertNotificationSettings_RejectsConfWithoutAlert(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_impact_alert_notification_settings" "test" {
						deployable_objects_alert_enabled             = false
						deployable_objects_confirmation_code_enabled = true
					}
				`,
				ExpectError: regexp.MustCompile(`Confirmation code requires its alert`),
			},
		},
	})
}

func TestAccDataSource_ProImpactAlertNotificationSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_impact_alert_notification_settings" "src" {
						deployable_objects_alert_enabled = true
					}

					data "jamfplatform_pro_impact_alert_notification_settings" "lookup" {
						depends_on = [jamfplatform_pro_impact_alert_notification_settings.src]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_impact_alert_notification_settings.lookup", "id", "singleton"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_impact_alert_notification_settings.lookup", "deployable_objects_alert_enabled", "jamfplatform_pro_impact_alert_notification_settings.src", "deployable_objects_alert_enabled"),
				),
			},
		},
	})
}
