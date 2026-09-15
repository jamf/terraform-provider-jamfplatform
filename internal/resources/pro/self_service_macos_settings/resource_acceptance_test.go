// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package self_service_macos_settings_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// samlEnvVar gates the Saml acceptance test: authentication_type = "Saml" requires
// "Single Sign-On for Self Service for macOS" to be enabled in the tenant's SSO settings
// (rejected with PREREQUISITE_NOT_MET otherwise). Set to any non-empty value to opt in.
const samlEnvVar = "JAMFPLATFORM_ACC_PRO_SELF_SERVICE_SAML"

// requireSsoForMacosSelfService skips the test unless the tenant currently has
// ssoForMacOsSelfServiceEnabled = true. The toggle cannot be enabled programmatically here:
// a GET→flip→PUT round-trip of the SSO settings object fails server-side validation
// ("SAML settings validation failed" — the GET echo is not re-PUT-able), so each gated run
// needs the toggle re-enabled in the UI first. It needs re-enabling every run because the
// test's NotRequired step makes Jamf Pro switch the toggle off (wire-probed 2026-06-10:
// that transition is the only Self Service write that touches SSO settings).
func requireSsoForMacosSelfService(t *testing.T) {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	sso, err := c.GetSsoSettingsV3(context.Background())
	if err != nil {
		t.Fatalf("reading SSO settings to check the Saml prerequisite: %v", err)
	}
	if !sso.SsoForMacOsSelfServiceEnabled {
		t.Skip(`"Single Sign-On for Self Service for macOS" is disabled on the tenant; enable it in Settings > Single Sign-On and re-run (the NotRequired step switches it off again, so this is per-run)`)
	}
}

// snapshotAndRestoreSettings captures the tenant's live Self Service settings before the
// test mutates them and restores the snapshot when the test finishes (pass or fail), so
// acceptance runs leave the tenant as found. The restore PUT sends the snapshot verbatim —
// the GET always carries a complete, valid object.
func snapshotAndRestoreSettings(t *testing.T) {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	snapshot, err := c.GetSelfServiceSettingsV1(context.Background())
	if err != nil {
		t.Fatalf("snapshot of Self Service settings before test: %v", err)
	}
	t.Cleanup(func() {
		if _, err := c.UpdateSelfServiceSettingsV1(context.Background(), snapshot); err != nil {
			t.Errorf("restoring Self Service settings snapshot after test: %v", err)
		}
	})
}

// checkSingletonRecordStillExists verifies the Jamf Pro Self Service settings record
// persists on the tenant after Terraform destroys the resource from state. Canonical
// singleton acceptance check: the remote Delete is a no-op, so the API must still return
// the record (with whatever value was last applied) post-destroy.
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "Self Service settings", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetSelfServiceSettingsV1(ctx)
	})
}

// TestAccResource_ProSelfServiceMacosSettings_Basic drives every attribute across two Update
// steps against a real tenant, covering each enum value that needs no fixture or tenant
// prerequisite (BROWSE + category id has its own test; "Saml" is gated behind samlEnvVar —
// it writes through to the tenant's SSO settings and 400s when SAML is unavailable for
// macOS). fido2_enabled is exercised under Basic (wire-probed: accepted and retained inert).
// Singleton resources have no remote Delete, so CheckDestroy verifies the record PERSISTS
// after Terraform stops managing it; the snapshot helper then restores the tenant's
// original settings.
func TestAccResource_ProSelfServiceMacosSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreSettings(t)

	const addr = "jamfplatform_pro_self_service_macos_settings.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						install_automatically               = true
						install_location                    = "/Applications"
						login_method                        = "Anonymous"
						authentication_type                 = "Basic"
						keychain_credential_storage_enabled = true
						fido2_enabled                       = false
						notifications_enabled               = true
						alert_user_approved_mdm             = true
						default_landing_page                = "HOME"
						default_home_category_id            = -1
						bookmarks_display_name              = "Bookmarks"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "singleton"),
					resource.TestCheckResourceAttr(addr, "install_automatically", "true"),
					resource.TestCheckResourceAttr(addr, "install_location", "/Applications"),
					resource.TestCheckResourceAttr(addr, "login_method", "Anonymous"),
					resource.TestCheckResourceAttr(addr, "authentication_type", "Basic"),
					resource.TestCheckResourceAttr(addr, "keychain_credential_storage_enabled", "true"),
					resource.TestCheckResourceAttr(addr, "fido2_enabled", "false"),
					resource.TestCheckResourceAttr(addr, "notifications_enabled", "true"),
					resource.TestCheckResourceAttr(addr, "alert_user_approved_mdm", "true"),
					resource.TestCheckResourceAttr(addr, "default_landing_page", "HOME"),
					resource.TestCheckResourceAttr(addr, "default_home_category_id", "-1"),
					resource.TestCheckResourceAttr(addr, "bookmarks_display_name", "Bookmarks"),
				),
			},
			{
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						install_automatically               = false
						install_location                    = "/Applications"
						login_method                        = "Required"
						authentication_type                 = "Basic"
						keychain_credential_storage_enabled = false
						fido2_enabled                       = true
						notifications_enabled               = false
						alert_user_approved_mdm             = false
						default_landing_page                = "HISTORY"
						default_home_category_id            = -1
						bookmarks_display_name              = "Websites"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "install_automatically", "false"),
					resource.TestCheckResourceAttr(addr, "login_method", "Required"),
					resource.TestCheckResourceAttr(addr, "keychain_credential_storage_enabled", "false"),
					resource.TestCheckResourceAttr(addr, "fido2_enabled", "true"),
					resource.TestCheckResourceAttr(addr, "notifications_enabled", "false"),
					resource.TestCheckResourceAttr(addr, "alert_user_approved_mdm", "false"),
					resource.TestCheckResourceAttr(addr, "default_landing_page", "HISTORY"),
					resource.TestCheckResourceAttr(addr, "bookmarks_display_name", "Websites"),
				),
			},
			{
				// Remaining enum values; NotRequired = the UI's login checkbox off.
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						login_method         = "NotRequired"
						default_landing_page = "NOTIFICATIONS"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "login_method", "NotRequired"),
					resource.TestCheckResourceAttr(addr, "default_landing_page", "NOTIFICATIONS"),
					// Omitted fields preserved from the previous step (omit = preserve).
					resource.TestCheckResourceAttr(addr, "bookmarks_display_name", "Websites"),
					resource.TestCheckResourceAttr(addr, "authentication_type", "Basic"),
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

// TestAccResource_ProSelfServiceMacosSettings_Saml exercises the SSO authentication type and
// the login_method → NotRequired coercion. Gated behind samlEnvVar plus a live prerequisite
// check: "Saml" requires the tenant's "Single Sign-On for Self Service for macOS" toggle
// (400 PREREQUISITE_NOT_MET otherwise). Wire-probed 2026-06-10: authType writes never touch
// that toggle, but the NotRequired step in this test makes Jamf Pro switch it OFF alongside
// coercing authType to Basic — the snapshot restore puts the Self Service settings back but
// CANNOT re-enable the toggle (see requireSsoForMacosSelfService), so re-enable it in the UI
// before each gated run.
func TestAccResource_ProSelfServiceMacosSettings_Saml(t *testing.T) {
	testhelpers.AccPreCheck(t)
	if testhelpers.AccEnv(samlEnvVar) == "" {
		t.Skipf("%s not set; skipping Saml acceptance test (requires SSO for Self Service enabled on the tenant)", samlEnvVar)
	}
	requireSsoForMacosSelfService(t)
	snapshotAndRestoreSettings(t)

	const addr = "jamfplatform_pro_self_service_macos_settings.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						login_method        = "Required"
						authentication_type = "Saml"
						fido2_enabled       = true
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "login_method", "Required"),
					resource.TestCheckResourceAttr(addr, "authentication_type", "Saml"),
					resource.TestCheckResourceAttr(addr, "fido2_enabled", "true"),
				),
			},
			{
				// Disable user login while "Saml" rides in via UseStateForUnknown: the server
				// keeps the write but coerces the stored authentication type back to Basic.
				// ModifyPlan must predict this (plan Unknown) or the apply fails with
				// "inconsistent result after apply" — the regression behind this test.
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						login_method = "NotRequired"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "login_method", "NotRequired"),
					resource.TestCheckResourceAttr(addr, "authentication_type", "Basic"),
				),
			},
		},
	})
}

// TestAccResource_ProSelfServiceMacosSettings_RejectsSamlWithoutLogin verifies the plan-time
// ConfigValidator rejects authentication_type = "Saml" declared together with login_method =
// "NotRequired" (both fields explicitly declared). Plan-time only — no tenant writes.
func TestAccResource_ProSelfServiceMacosSettings_RejectsSamlWithoutLogin(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						login_method        = "NotRequired"
						authentication_type = "Saml"
					}
				`,
				ExpectError: regexp.MustCompile(`Single Sign-On requires user login`),
			},
		},
	})
}

// TestAccResource_ProSelfServiceMacosSettings_BrowseCategory exercises the BROWSE landing
// page with a real category fixture (the wire rejects unknown ids under BROWSE with 400
// INVALID_ID). The final step returns the landing page to HOME / All Items before destroy
// so the category fixture can be deleted while no longer referenced.
func TestAccResource_ProSelfServiceMacosSettings_BrowseCategory(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreSettings(t)

	const addr = "jamfplatform_pro_self_service_macos_settings.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_category" "test" {
						name     = "tf-acc-sss-browse-category"
						priority = 9
					}

					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						default_landing_page     = "BROWSE"
						default_home_category_id = tonumber(jamfplatform_pro_category.test.id)
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "default_landing_page", "BROWSE"),
					resource.TestCheckResourceAttrPair(addr, "default_home_category_id", "jamfplatform_pro_category.test", "id"),
				),
			},
			{
				// Release the category reference before destroy deletes the fixture.
				Config: `
					resource "jamfplatform_pro_category" "test" {
						name     = "tf-acc-sss-browse-category"
						priority = 9
					}

					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						default_landing_page     = "HOME"
						default_home_category_id = -1
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "default_landing_page", "HOME"),
					resource.TestCheckResourceAttr(addr, "default_home_category_id", "-1"),
				),
			},
		},
	})
}

// TestAccResource_ProSelfServiceMacosSettings_CreateAdopt proves the singleton
// GET-on-create-adopt and full-replace omit=preserve: a field omitted from HCL on the FIRST
// apply re-sends the value set out of band (simulating a UI edit) rather than resetting it
// to the server default on the full-replace PUT. This is the split-ownership test required
// by STYLE_GUIDE §Full-replace endpoints.
func TestAccResource_ProSelfServiceMacosSettings_CreateAdopt(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreSettings(t)

	const addr = "jamfplatform_pro_self_service_macos_settings.test"
	const outOfBandBookmarks = "tf-acc-adopted-bookmarks"

	// Set a recognizable bookmarks name out of band BEFORE Terraform creates the resource,
	// via read-modify-write (the PUT requires the full object).
	setBookmarksOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		current, err := c.GetSelfServiceSettingsV1(context.Background())
		if err != nil {
			t.Fatalf("out-of-band baseline GET: %v", err)
		}
		current.ConfigurationSettings.BookmarksName = outOfBandBookmarks
		if _, err := c.UpdateSelfServiceSettingsV1(context.Background(), current); err != nil {
			t.Fatalf("out-of-band baseline PUT: %v", err)
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				// Create declaring ONE unrelated field; bookmarks_display_name omitted.
				// GET-on-create-adopt must preserve the out-of-band value.
				PreConfig: setBookmarksOutOfBand,
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						notifications_enabled = true
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "notifications_enabled", "true"),
					resource.TestCheckResourceAttr(addr, "bookmarks_display_name", outOfBandBookmarks),
				),
			},
			{
				// Update an unrelated field; the omitted bookmarks name must survive the
				// full-replace PUT via UseStateForUnknown.
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						notifications_enabled = false
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "notifications_enabled", "false"),
					resource.TestCheckResourceAttr(addr, "bookmarks_display_name", outOfBandBookmarks),
				),
			},
		},
	})
}

// TestAccResource_ProSelfServiceMacosSettings_RejectsNonSingletonImport verifies the
// ImportState guard: any identifier other than "singleton" must fail with a clear error
// rather than silently normalizing to the singleton ID.
func TestAccResource_ProSelfServiceMacosSettings_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreSettings(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "test" {}
				`,
			},
			{
				ResourceName:  "jamfplatform_pro_self_service_macos_settings.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_ProSelfServiceMacosSettings_RejectsCategoryWithoutBrowse verifies the
// plan-time ConfigValidator rejects a category id declared alongside a non-BROWSE landing
// page (both fields explicitly declared). Plan-time only — no tenant writes.
func TestAccResource_ProSelfServiceMacosSettings_RejectsCategoryWithoutBrowse(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						default_landing_page     = "HOME"
						default_home_category_id = 42
					}
				`,
				ExpectError: regexp.MustCompile(`Default home category requires the Browse landing page`),
			},
		},
	})
}

// TestAccResource_ProSelfServiceMacosSettings_RejectsBlankLocationWhenAuto verifies the
// plan-time ConfigValidator mirrors the server's install-location requirement. Plan-time
// only — no tenant writes.
func TestAccResource_ProSelfServiceMacosSettings_RejectsBlankLocationWhenAuto(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "test" {
						install_automatically = true
						install_location      = ""
					}
				`,
				ExpectError: regexp.MustCompile(`Install location required`),
			},
		},
	})
}

func TestAccDataSource_ProSelfServiceMacosSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreSettings(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_self_service_macos_settings" "src" {
						notifications_enabled = true
					}

					data "jamfplatform_pro_self_service_macos_settings" "lookup" {
						depends_on = [jamfplatform_pro_self_service_macos_settings.src]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_self_service_macos_settings.lookup", "id", "singleton"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_self_service_macos_settings.lookup", "notifications_enabled", "jamfplatform_pro_self_service_macos_settings.src", "notifications_enabled"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_self_service_macos_settings.lookup", "login_method"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_self_service_macos_settings.lookup", "default_landing_page"),
				),
			},
		},
	})
}
