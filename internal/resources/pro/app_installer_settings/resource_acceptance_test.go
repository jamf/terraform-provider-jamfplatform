// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package app_installer_settings_test

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

// checkSingletonRecordStillExists verifies the App Installer global settings record
// persists on the tenant after Terraform destroys the resource from state.
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "App Installer settings", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetAppInstallerGlobalSettingsV1(ctx)
	})
}

// TestAccResource_ProAppInstallerSettings_Basic creates settings with both blocks,
// updates one, then clears both to confirm full-replace semantics.
func TestAccResource_ProAppInstallerSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_app_installer_settings" "test" {
					  deployment_settings = {
					    batch_size       = 1000
					    batch_frequency  = 60
					    days             = ["MONDAY", "WEDNESDAY", "FRIDAY"]
					    server_time_from = "08:00:00Z"
					    server_time_to   = "17:00:00Z"
					  }
					  end_user_experience = {
					    notification_frequency  = 2
					    notification_message    = "Update pending"
					    update_deadline         = 24
					    force_quit_message      = "Please quit and save your work"
					    force_quit_grace_period = 10
					    update_complete_message = "Update complete"
					    relaunch = true
					    suppress = false
					  }
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "id", "singleton"),
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "deployment_settings.batch_size", "1000"),
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "deployment_settings.batch_frequency", "60"),
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "deployment_settings.server_time_from", "08:00:00Z"),
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "deployment_settings.server_time_to", "17:00:00Z"),
					resource.TestCheckTypeSetElemAttr("jamfplatform_pro_app_installer_settings.test", "deployment_settings.days.*", "MONDAY"),
					resource.TestCheckTypeSetElemAttr("jamfplatform_pro_app_installer_settings.test", "deployment_settings.days.*", "WEDNESDAY"),
					resource.TestCheckTypeSetElemAttr("jamfplatform_pro_app_installer_settings.test", "deployment_settings.days.*", "FRIDAY"),
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "end_user_experience.notification_frequency", "2"),
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "end_user_experience.update_deadline", "24"),
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "end_user_experience.force_quit_grace_period", "10"),
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "end_user_experience.relaunch", "true"),
				),
			},
			{
				// Update: change deployment controls only, leave EUX unchanged.
				Config: `
					resource "jamfplatform_pro_app_installer_settings" "test" {
					  deployment_settings = {
					    batch_size      = 500
					    batch_frequency = 120
					  }
					  end_user_experience = {
					    notification_frequency = 2
					    update_deadline        = 24
					    relaunch               = true
					    suppress               = false
					  }
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "deployment_settings.batch_size", "500"),
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "deployment_settings.batch_frequency", "120"),
				),
			},
			{
				// Omitting both blocks carries prior state forward via USFU (omit = preserve).
				// The plan must show no diff — no update is triggered.
				Config: `
					resource "jamfplatform_pro_app_installer_settings" "test" {}
				`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// TestAccResource_ProAppInstallerSettings_Import verifies that import captures
// actual server values (value-based normalization, not state-gated).
func TestAccResource_ProAppInstallerSettings_Import(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_app_installer_settings" "test" {
					  deployment_settings = {
					    batch_size = 1000
					  }
					}
				`,
				Check: resource.TestCheckResourceAttr(
					"jamfplatform_pro_app_installer_settings.test",
					"deployment_settings.batch_size", "1000",
				),
			},
			{
				ResourceName:  "jamfplatform_pro_app_installer_settings.test",
				ImportState:   true,
				ImportStateId: "singleton",
				ImportStateCheck: func(s []*terraform.InstanceState) error {
					for _, state := range s {
						if v, ok := state.Attributes["deployment_settings.batch_size"]; ok {
							if v != "1000" {
								return fmt.Errorf("import: expected batch_size=1000, got %s", v)
							}
							return nil
						}
					}
					return fmt.Errorf("import: deployment_settings.batch_size not found in state")
				},
			},
		},
	})
}

// TestAccResource_ProAppInstallerSettings_RejectsNonSingletonImport verifies that
// any identifier other than "singleton" is rejected on import.
func TestAccResource_ProAppInstallerSettings_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_app_installer_settings" "test" {}
				`,
			},
			{
				ResourceName:  "jamfplatform_pro_app_installer_settings.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_ProAppInstallerSettings_SplitOwnership proves the omit=preserve
// contract at the block level for this full-replace singleton across three transitions:
//
//   - Step 1 (create = adopt): deployment_settings.batch_size is set out-of-band to
//     1000 BEFORE the resource is adopted. The config omits deployment_settings entirely.
//     GET-on-create must carry the live 1000 into state, not clobber it.
//   - Step 2 (update = preserve): batch_size changed out-of-band to 2000; the config
//     still omits deployment_settings and only modifies end_user_experience. GET-on-update
//     must carry the live 2000 forward.
//   - Step 3 (take-over): the config explicitly declares deployment_settings; Terraform
//     now owns and drift-reverts it.
func TestAccResource_ProAppInstallerSettings_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)

	setBatchSizeOutOfBand := func(batchSize int) func() {
		return func() {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			ctx := context.Background()
			got, err := c.GetAppInstallerGlobalSettingsV1(ctx)
			if err != nil {
				t.Fatalf("out-of-band GET: %v", err)
			}
			if got.DeploymentProcessControls == nil {
				got.DeploymentProcessControls = &pro.AppInstallersDeploymentProcessControls{}
			}
			got.DeploymentProcessControls.CommandsBatchSize = &batchSize
			if _, err := c.UpdateAppInstallerGlobalSettingsV1(ctx, got); err != nil {
				t.Fatalf("out-of-band PUT: %v", err)
			}
		}
	}

	checkServerBatchSize := func(want int) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetAppInstallerGlobalSettingsV1(context.Background())
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if got.DeploymentProcessControls == nil || got.DeploymentProcessControls.CommandsBatchSize == nil {
				return fmt.Errorf("CommandsBatchSize not set on server, want %d", want)
			}
			if *got.DeploymentProcessControls.CommandsBatchSize != want {
				return fmt.Errorf("CommandsBatchSize = %d, want %d", *got.DeploymentProcessControls.CommandsBatchSize, want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				// Set batch_size=1000 out of band, then adopt with deployment_settings omitted.
				// GET-on-create must preserve 1000.
				PreConfig: setBatchSizeOutOfBand(1000),
				Config: `
					resource "jamfplatform_pro_app_installer_settings" "test" {
					  end_user_experience = {
					    notification_frequency = 2
					    update_deadline        = 24
					  }
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "deployment_settings.batch_size", "1000"),
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "end_user_experience.notification_frequency", "2"),
					checkServerBatchSize(1000),
				),
			},
			{
				// Change batch_size out of band to 2000; config still omits deployment_settings.
				// PreConfig runs before Terraform's refresh: the refresh (part of terraform apply)
				// reads 2000 from the server and updates state. USFU then carries the refreshed
				// state {2000} into plan as a known value — the else branch in buildMergedInput
				// sends 2000, preserving the out-of-band value. The update fires only because
				// EUX changed; deployment_settings is untouched. omit = preserve confirmed.
				PreConfig: setBatchSizeOutOfBand(2000),
				Config: `
					resource "jamfplatform_pro_app_installer_settings" "test" {
					  end_user_experience = {
					    notification_frequency = 4
					    update_deadline        = 48
					  }
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "end_user_experience.notification_frequency", "4"),
					checkServerBatchSize(2000),
				),
			},
			{
				// Take over: Terraform now owns deployment_settings and drift-reverts batch_size.
				Config: `
					resource "jamfplatform_pro_app_installer_settings" "test" {
					  deployment_settings = {
					    batch_size = 500
					  }
					  end_user_experience = {
					    notification_frequency = 4
					    update_deadline        = 48
					  }
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_app_installer_settings.test", "deployment_settings.batch_size", "500"),
					checkServerBatchSize(500),
				),
			},
		},
	})
}

// TestAccDataSource_ProAppInstallerSettings_Basic reads the data source and checks it
// reflects what was set by the resource.
func TestAccDataSource_ProAppInstallerSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_app_installer_settings" "src" {
					  end_user_experience = {
					    notification_frequency = 4
					    update_deadline        = 48
					    relaunch               = false
					    suppress               = true
					  }
					}

					data "jamfplatform_pro_app_installer_settings" "lookup" {
					  depends_on = [jamfplatform_pro_app_installer_settings.src]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_app_installer_settings.lookup", "id", "singleton"),
					resource.TestCheckResourceAttrPair(
						"data.jamfplatform_pro_app_installer_settings.lookup", "end_user_experience.notification_frequency",
						"jamfplatform_pro_app_installer_settings.src", "end_user_experience.notification_frequency",
					),
				),
			},
		},
	})
}
