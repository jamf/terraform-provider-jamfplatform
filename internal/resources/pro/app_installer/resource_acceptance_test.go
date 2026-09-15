// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package app_installer_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const resourceAddr = "jamfplatform_pro_app_installer.test"

// titleName is a stable catalog title in the test tenant. A Jamf-published
// title is used deliberately: Jamf owns the catalog entry, so it stays healthy.
// A third-party title, "010 Editor" (catalog ID 518), was previously found
// server-corrupt — its /titles/{id} GET 500s and the deployments LIST duplicates
// rows backed by it, which is outside this provider's control, and nuking the
// tenant did not clear it.
const titleName = "Jamf Composer"

// catalogDS resolves the title through the catalog data source; the deployment
// references the resolved title_name (exercising the DS → resource wiring rather
// than hard-coding the name).
const catalogDS = `
data "jamfplatform_pro_app_installer_titles" "catalog" {
	name_substring = "Jamf Composer"
}
`

// titleNameRef is the HCL expression for the resolved title name.
const titleNameRef = "data.jamfplatform_pro_app_installer_titles.catalog.titles[0].title_name"

// testAccCheckAppInstallerDestroy verifies deployments created during the test
// were destroyed.
func testAccCheckAppInstallerDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_app_installer" {
				continue
			}
			_, err := c.GetAppInstallerDeploymentV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking App Installer deployment %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("App Installer deployment %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_ProAppInstaller_Basic exercises create + import of a minimal
// SELF_SERVICE / AUTOMATIC deployment referenced by title name. Under AUTOMATIC
// the server reports the latest version in selected_version. Import exercises
// the app_title_id → app_title_name reverse-resolve.
func TestAccResource_ProAppInstaller_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-app-installer-" + suffix
	cfg := catalogDS + fmt.Sprintf(`
		resource "jamfplatform_pro_app_installer" "test" {
			name            = %q
			app_title_name  = %s
			deployment_type = "SELF_SERVICE"
			update_behavior = "AUTOMATIC"
		}
	`, name, titleNameRef)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppInstallerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceAddr, "id"),
					resource.TestCheckResourceAttr(resourceAddr, "name", name),
					resource.TestCheckResourceAttr(resourceAddr, "app_title_name", titleName),
					resource.TestCheckResourceAttrSet(resourceAddr, "app_title_id"),
					resource.TestCheckResourceAttr(resourceAddr, "deployment_type", "SELF_SERVICE"),
					resource.TestCheckResourceAttr(resourceAddr, "update_behavior", "AUTOMATIC"),
					resource.TestCheckResourceAttrSet(resourceAddr, "latest_available_version"),
				),
			},
			{
				// Drift-recovery / plan stability: re-applying the same config
				// must yield an empty plan, proving the server-computed attrs
				// (selected_version, latest_available_version, title_available_in_ais,
				// version_removed) reconcile and don't churn.
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      resourceAddr,
				ImportState:       true,
				ImportStateVerify: true,
				// notification_settings / self_service_settings: Optional
				// state-gated blocks this config never declares. Import
				// hydrates them from the server's echoed defaults (correct —
				// see the import-hydration fix), which legitimately differs
				// from this config's null. Not verified here.
				ImportStateVerifyIgnore: []string{"notification_settings", "self_service_settings"},
			},
		},
	})
}

// groupFixtures returns a smart computer group (via jamfplatform_device_group,
// whose jamf_pro_id is the numeric classic ID the deployment's smart_group_id
// wants) and a category. The group has no site, so the deployment that uses it
// must NOT set site_id — Jamf restricts a site-scoped deployment to smart groups
// belonging to that site, and a Full-JSS (siteless) group is rejected as "not
// existing smart group". site_id is exercised separately (see the SiteScoped
// test) where no smart group is needed.
func groupFixtures(suffix string) string {
	return catalogDS + fmt.Sprintf(`
		resource "jamfplatform_device_group" "grp" {
			name        = "tf-acc-ai-grp-%[1]s"
			group_type  = "smart"
			device_type = "computer"
			description = "App Installer acceptance fixture"
			criteria = [
				{ criteria = "Operating System Version", operator = "greater than or equal", value = "10.0" },
			]
		}
		resource "jamfplatform_pro_category" "cat" {
			name     = "tf-acc-ai-cat-%[1]s"
			priority = 5
		}
	`, suffix)
}

// TestAccResource_ProAppInstaller_ComprehensiveScopeAndBlocks exercises the full
// surface: enabled=true scoped to a real smart group, category_id,
// install_predefined_config_profiles, trigger_admin_notifications, the complete
// notification_settings field set, and self_service_settings with categories +
// featured — then mutates every block field and finally clears both blocks
// (scope retained). Both nested blocks are full-replace. site_id is intentionally
// not set here (the fixture group is siteless); see the SiteScoped test.
func TestAccResource_ProAppInstaller_ComprehensiveScopeAndBlocks(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-app-installer-full-" + suffix
	header := groupFixtures(suffix)

	withBlocks := header + fmt.Sprintf(`
		resource "jamfplatform_pro_app_installer" "test" {
			name                               = %q
			enabled                            = true
			trigger_admin_notifications        = true
			install_predefined_config_profiles = true
			app_title_name                     = %s
			deployment_type                    = "SELF_SERVICE"
			update_behavior                    = "AUTOMATIC"

			smart_group_id = jamfplatform_device_group.grp.jamf_pro_id
			category_id    = jamfplatform_pro_category.cat.id

			notification_settings = {
				notification_message  = "Update available"
				notification_interval = 4
				deadline_message      = "Please update soon"
				deadline              = 48
				quit_delay            = 60
				complete_message      = "Update complete"
				relaunch              = true
				suppress              = false
			}
			self_service_settings = {
				description                    = "Install from Self Service"
				force_view_description         = true
				include_in_featured_category   = true
				include_in_compliance_category = false
				categories = [
					{ category_id = jamfplatform_pro_category.cat.id, featured = true },
				]
			}
		}
	`, name, titleNameRef)

	changedBlocks := header + fmt.Sprintf(`
		resource "jamfplatform_pro_app_installer" "test" {
			name                               = %q
			enabled                            = true
			trigger_admin_notifications        = false
			install_predefined_config_profiles = false
			app_title_name                     = %s
			deployment_type                    = "SELF_SERVICE"
			update_behavior                    = "AUTOMATIC"

			smart_group_id = jamfplatform_device_group.grp.jamf_pro_id

			notification_settings = {
				notification_message = "Please update"
				deadline             = 24
				suppress             = true
			}
			self_service_settings = {
				description                  = "Updated description"
				include_in_featured_category = false
				categories = [
					{ category_id = jamfplatform_pro_category.cat.id, featured = false },
				]
			}
		}
	`, name, titleNameRef)

	clearedBlocks := header + fmt.Sprintf(`
		resource "jamfplatform_pro_app_installer" "test" {
			name            = %q
			enabled         = true
			app_title_name  = %s
			deployment_type = "SELF_SERVICE"
			update_behavior = "AUTOMATIC"
			smart_group_id  = jamfplatform_device_group.grp.jamf_pro_id
		}
	`, name, titleNameRef)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppInstallerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: withBlocks,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddr, "enabled", "true"),
					resource.TestCheckResourceAttr(resourceAddr, "trigger_admin_notifications", "true"),
					resource.TestCheckResourceAttr(resourceAddr, "install_predefined_config_profiles", "true"),
					resource.TestCheckResourceAttrPair(resourceAddr, "smart_group_id", "jamfplatform_device_group.grp", "jamf_pro_id"),
					resource.TestCheckResourceAttrPair(resourceAddr, "category_id", "jamfplatform_pro_category.cat", "id"),
					resource.TestCheckResourceAttr(resourceAddr, "notification_settings.notification_interval", "4"),
					resource.TestCheckResourceAttr(resourceAddr, "notification_settings.deadline", "48"),
					resource.TestCheckResourceAttr(resourceAddr, "notification_settings.quit_delay", "60"),
					resource.TestCheckResourceAttr(resourceAddr, "notification_settings.relaunch", "true"),
					resource.TestCheckResourceAttr(resourceAddr, "self_service_settings.categories.#", "1"),
					resource.TestCheckResourceAttr(resourceAddr, "self_service_settings.categories.0.featured", "true"),
				),
			},
			{
				Config: changedBlocks,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddr, "trigger_admin_notifications", "false"),
					resource.TestCheckResourceAttr(resourceAddr, "install_predefined_config_profiles", "false"),
					resource.TestCheckResourceAttr(resourceAddr, "notification_settings.notification_message", "Please update"),
					resource.TestCheckResourceAttr(resourceAddr, "notification_settings.deadline", "24"),
					resource.TestCheckResourceAttr(resourceAddr, "notification_settings.suppress", "true"),
					// Fields dropped from the changed notification block are cleared
					// (full-replace), so they read back null.
					resource.TestCheckNoResourceAttr(resourceAddr, "notification_settings.quit_delay"),
					resource.TestCheckResourceAttr(resourceAddr, "self_service_settings.description", "Updated description"),
					resource.TestCheckResourceAttr(resourceAddr, "self_service_settings.categories.0.featured", "false"),
				),
			},
			{
				// Cleared: both blocks removed from config. With Optional-only
				// state-gating, Read preserves the null and does not re-import
				// the server-echoed default blocks. Scope is retained.
				Config: clearedBlocks,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddr, "enabled", "true"),
					resource.TestCheckResourceAttrPair(resourceAddr, "smart_group_id", "jamfplatform_device_group.grp", "jamf_pro_id"),
					resource.TestCheckNoResourceAttr(resourceAddr, "notification_settings.%"),
					resource.TestCheckNoResourceAttr(resourceAddr, "self_service_settings.%"),
				),
			},
		},
	})
}

// TestAccResource_ProAppInstaller_AutomaticInstallManual covers the remaining
// enum values: deployment_type=INSTALL_AUTOMATICALLY and update_behavior=MANUAL,
// scoped to a real smart group so enabled=true is valid.
func TestAccResource_ProAppInstaller_AutomaticInstallManual(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-app-installer-auto-" + suffix
	cfg := groupFixtures(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_app_installer" "test" {
			name            = %q
			enabled         = true
			app_title_name  = %s
			deployment_type = "INSTALL_AUTOMATICALLY"
			update_behavior = "MANUAL"
			smart_group_id  = jamfplatform_device_group.grp.jamf_pro_id
		}
	`, name, titleNameRef)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppInstallerDestroy(t),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(resourceAddr, "enabled", "true"),
				resource.TestCheckResourceAttr(resourceAddr, "deployment_type", "INSTALL_AUTOMATICALLY"),
				resource.TestCheckResourceAttr(resourceAddr, "update_behavior", "MANUAL"),
				resource.TestCheckResourceAttrSet(resourceAddr, "selected_version"),
			),
		}},
	})
}

// NOTE: site_id is intentionally not acceptance-tested. It is plumbed and
// round-trips (Optional+Computed, server default "-1"), but exercising it
// requires a site-scoped deployment, and (a) a site-scoped deployment can only
// reference a smart group belonging to that site (Full-JSS fixture groups are
// rejected as "not existing smart group"), and (b) the acceptance API client
// lacks site-based privileges to delete site-scoped deployments (observed 403),
// so such a test could not reliably clean up. Covered manually instead.

// TestAccResource_ProAppInstaller_BadTitleName asserts the plan-time
// app_title_name preflight surfaces an unknown title before apply.
func TestAccResource_ProAppInstaller_BadTitleName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-app-installer-bad-" + suffix
	cfg := fmt.Sprintf(`
		resource "jamfplatform_pro_app_installer" "test" {
			name            = %q
			app_title_name  = "Definitely Not A Real App Title"
			deployment_type = "SELF_SERVICE"
			update_behavior = "AUTOMATIC"
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      cfg,
			ExpectError: regexp.MustCompile("Unknown App Installer title"),
		}},
	})
}
