// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /macapplications endpoint.
// Classic has known concurrency issues when multiple writes hit the same
// resource type — keep these tests serial with any future classic acceptance
// work in this package.
//
// Scope happy-paths use all_computers = true so they need no pre-existing
// tenant target objects, and general.site is left unset to avoid the
// site/scope "not site-enabled" 409 invariant. The ungated tests never set
// vpp.assign_vpp_device_based_licenses = true (a non-VPP title 409s "App is
// not available for device assignment"); the gated full-metadata test
// (TestAccResource_ProMacApp_VPPFullMetadata) does, behind JAMFPLATFORM_ACC_PRO_VPP_TOKEN.

package mac_app_store_app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const macAppResourceAddr = "jamfplatform_pro_mac_app_store_app.test"

// vppTokenEnvVar holds the base64 `.vpptoken` contents (same gate as the
// volume_purchasing_location acceptance test). The full-metadata VPP test
// below is skipped unless it is set, because device-based VPP assignment
// requires a real Apple Business Manager / Apple School Manager token whose
// location owns device-assignable licenses for the test title (Jamf Parent).
// Tokens MUST come from env — never commit token material.
const vppTokenEnvVar = "JAMFPLATFORM_ACC_PRO_VPP_TOKEN"

// testAccCheckMacAppDestroy verifies apps created during the test were destroyed.
func testAccCheckMacAppDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_mac_app_store_app" {
				continue
			}
			_, err := c.GetMacApplicationByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro Mac App Store app %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro Mac App Store app %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// macAppGeneralOnlyConfig is the import-stable shape: only the required general
// fields. The importer populates general post-Read but leaves optional blocks
// null, so ImportStateVerify must run against a general-only config.
func macAppGeneralOnlyConfig(name, version, deploymentType string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name            = %q
				version         = %q
				bundle_id       = "com.example.tfacc.macapp"
				url             = "https://apps.apple.com/app/id000000001"
				deployment_type = %q
			}
		}
	`, name, version, deploymentType)
}

// macAppFullConfig adds self_service and an all_computers scope on top of general.
func macAppFullConfig(name, version, buttonText string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name            = %q
				version         = %q
				bundle_id       = "com.example.tfacc.macapp"
				url             = "https://apps.apple.com/app/id000000001"
				deployment_type = "Make Available in Self Service"
			}
			scope = {
				targets = {
					all_computers = true
				}
			}
			self_service = {
				install_button_text            = %q
				self_service_description        = "Managed by Terraform acceptance test."
				force_users_to_view_description = false
				feature_on_main_page            = true
			}
		}
	`, name, version, buttonText)
}

// TestAccResource_ProMacApp_Basic exercises create, in-place update, and import
// for the general-only shape. The version/deployment_type change verifies the
// GET-after-Update path (classic UpdateMacApplicationByID returns 201 empty).
func TestAccResource_ProMacApp_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: macAppGeneralOnlyConfig(name, "1.0", "Make Available in Self Service"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(macAppResourceAddr, "id"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.name", name),
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.version", "1.0"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.deployment_type", "Make Available in Self Service"),
				),
			},
			{
				ResourceName:      macAppResourceAddr,
				ImportState:       true,
				ImportStateVerify: true,
				// timeouts: not returned by the API. scope / self_service /
				// vpp: Optional state-gated blocks this general-only config
				// never declares. Import hydrates them from the server's
				// echoed defaults (correct — see the import-hydration fix),
				// which legitimately differs from this config's null. Not
				// verified here.
				ImportStateVerifyIgnore: []string{"timeouts", "scope", "self_service", "vpp"},
			},
			{
				// In-place update: bump version + flip the install method.
				Config: macAppGeneralOnlyConfig(name, "2.0", "Install Automatically/Prompt Users to Install"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.version", "2.0"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.deployment_type", "Install Automatically/Prompt Users to Install"),
				),
			},
		},
	})
}

// TestAccResource_ProMacApp_ScopeAndSelfService exercises the scope + self_service
// blocks and an in-place self_service mutation (not a block deletion — the
// classic PUT is partial-merge and will not clear an omitted block).
func TestAccResource_ProMacApp_ScopeAndSelfService(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-ss-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: macAppFullConfig(name, "1.0", "Install"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(macAppResourceAddr, "id"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.targets.all_computers", "true"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.install_button_text", "Install"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.feature_on_main_page", "true"),
				),
			},
			{
				Config: macAppFullConfig(name, "1.0", "Get"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.install_button_text", "Get"),
				),
			},
		},
	})
}

// TestAccResource_ProMacApp_InvalidDeploymentType asserts the deployment_type
// OneOf validator rejects an out-of-set value at plan time.
func TestAccResource_ProMacApp_InvalidDeploymentType(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-bad-dt-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      macAppGeneralOnlyConfig(name, "1.0", "Force Install Right Now"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("value must be one of"),
			},
		},
	})
}

// TestAccResource_ProMacApp_AllComputersConflict asserts the shared scope
// validator rejects all_computers = true alongside a computer target.
func TestAccResource_ProMacApp_AllComputersConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-allc-" + suffix
	config := fmt.Sprintf(`
		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name      = %q
				version   = "1.0"
				bundle_id = "com.example.tfacc.macapp"
				url       = "https://apps.apple.com/app/id000000001"
			}
			scope = {
				targets = {
					all_computers = true
					computer_ids  = ["1"]
				}
			}
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("Conflicts with all-flag"),
			},
		},
	})
}

// TestAccResource_ProMacApp_AllJssUsersConflict asserts the shared scope
// validator rejects all_jss_users = true alongside a user target.
func TestAccResource_ProMacApp_AllJssUsersConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-allu-" + suffix
	config := fmt.Sprintf(`
		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name      = %q
				version   = "1.0"
				bundle_id = "com.example.tfacc.macapp"
				url       = "https://apps.apple.com/app/id000000001"
			}
			scope = {
				targets = {
					all_jss_users = true
					user_ids      = ["1"]
				}
			}
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("Conflicts with all-flag"),
			},
		},
	})
}

// macAppCategoryFlipConfig builds an app whose category points at one of two
// jamfplatform_pro_category fixtures (selectedCat is "one" or "two").
func macAppCategoryFlipConfig(name, suffix, selectedCat string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_category" "one" {
			name     = "tf-acc-macapp-cat1-%[2]s"
			priority = 9
		}

		resource "jamfplatform_pro_category" "two" {
			name     = "tf-acc-macapp-cat2-%[2]s"
			priority = 9
		}

		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name        = %[1]q
				version     = "1.0"
				bundle_id   = "com.example.tfacc.macapp"
				url         = "https://apps.apple.com/app/id000000001"
				category_id = jamfplatform_pro_category.%[3]s.id
			}
		}
	`, name, suffix, selectedCat)
}

// TestAccResource_ProMacApp_CategoryFlip flips category_id between two category
// fixtures. category_name is server-derived from category_id; the second step
// asserts it re-resolves to the new category, guarding against the derived-name
// plan-modifier regression (UseStateForUnknown would pin the stale name and
// trip "produced inconsistent result after apply").
func TestAccResource_ProMacApp_CategoryFlip(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-cat-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: macAppCategoryFlipConfig(name, suffix, "one"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(macAppResourceAddr, "general.category_id", "jamfplatform_pro_category.one", "id"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.category_name", "tf-acc-macapp-cat1-"+suffix),
				),
			},
			{
				Config: macAppCategoryFlipConfig(name, suffix, "two"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(macAppResourceAddr, "general.category_id", "jamfplatform_pro_category.two", "id"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.category_name", "tf-acc-macapp-cat2-"+suffix),
				),
			},
		},
	})
}

// itunesMacAppMeta is the subset of the iTunes Lookup API response the
// /macapplications endpoint stores verbatim (no App Store resolution happens
// server-side, so the test must supply real metadata). Field tags mirror the
// public API (see the iTunes Search API docs / the itunessearchapi provider).
type itunesMacAppMeta struct {
	TrackName    string `json:"trackName"`
	BundleID     string `json:"bundleId"`
	Version      string `json:"version"`
	TrackViewURL string `json:"trackViewUrl"`
	Description  string `json:"description"`
	PrimaryGenre string `json:"primaryGenreName"`
	ArtworkURL   string `json:"artworkUrl512"`
}

// lookupITunesMacApp resolves an adam_id to its App Store metadata via the public
// iTunes Lookup API (https://itunes.apple.com/lookup?id=<adamId>). It returns
// ok=false (not an error) when the id has no public listing — VPP/ABM accounts
// can own custom B2B or unlisted apps that the public catalog cannot resolve, so
// the caller tries each owned adam_id until one resolves. A non-nil error is a
// transport/decoding failure, distinct from "not found".
func lookupITunesMacApp(adamID string) (itunesMacAppMeta, bool, error) {
	endpoint := fmt.Sprintf("https://itunes.apple.com/lookup?id=%s&country=us", url.QueryEscape(adamID))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return itunesMacAppMeta{}, false, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return itunesMacAppMeta{}, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return itunesMacAppMeta{}, false, fmt.Errorf("status %d", resp.StatusCode)
	}
	var env struct {
		ResultCount int                `json:"resultCount"`
		Results     []itunesMacAppMeta `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return itunesMacAppMeta{}, false, fmt.Errorf("decode: %w", err)
	}
	if env.ResultCount == 0 || len(env.Results) == 0 {
		return itunesMacAppMeta{}, false, nil // no public listing
	}
	m := env.Results[0]
	if m.BundleID == "" || m.Version == "" {
		return itunesMacAppMeta{}, false, nil // listing too sparse to drive /macapplications
	}
	return m, true, nil
}

// setMacAppTFVars publishes the resolved metadata as TF_VAR_* so the dynamic
// config below reads it through `variable` blocks. Routing the values through
// env (rather than baking them into the HCL string) sidesteps escaping the
// multi-line App Store description and lets the same static config serve every
// step. Registered cleanups unset the vars after the test.
func setMacAppTFVars(t *testing.T, m itunesMacAppMeta) {
	t.Helper()
	for k, v := range map[string]string{
		"TF_VAR_mac_name":        m.TrackName,
		"TF_VAR_mac_version":     m.Version,
		"TF_VAR_mac_bundle_id":   m.BundleID,
		"TF_VAR_mac_url":         m.TrackViewURL,
		"TF_VAR_mac_description": m.Description,
		"TF_VAR_mac_artwork_url": m.ArtworkURL,
	} {
		key := k
		if err := os.Setenv(key, v); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
		t.Cleanup(func() { _ = os.Unsetenv(key) })
	}
}

// macAppVPPLocationFixture is the env-token VPP location both VPP-test steps
// share. Both step configs must include it so Terraform does not destroy the
// directory between Create and Update (the token can only register one location).
func macAppVPPLocationFixture(token, suffix string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_volume_purchasing_location" "vpp" {
			name                                     = "tf-acc-macapp-vpp-%[2]s"
			service_token                            = %[1]q
			service_token_wo_version                 = 1
			automatically_populate_purchased_content = true
		}
	`, token, suffix)
}

// macAppVPPDiscoverConfig stands up the VPP location and surfaces, via an HCL
// output, the adam_ids of every MAC_APP in its synced content that still has a
// free device-assignable license. The discovery step's Check reads the output
// and resolves the first one's App Store metadata from iTunes.
func macAppVPPDiscoverConfig(token, suffix string) string {
	return macAppVPPLocationFixture(token, suffix) + `
		output "mac_adam_ids" {
			value = [
				for c in jamfplatform_pro_volume_purchasing_location.vpp.content :
				c.adam_id
				if c.content_type == "MAC_APP" && c.license_count_total > c.license_count_in_use
			]
		}
	`
}

// macAppVPPDynamicConfig builds the Mac App Store app from TF_VAR_* metadata
// (resolved live by the discovery step) and assigns device-based VPP licenses
// from the shared location. buttonText and assignDeviceLicenses vary per step.
func macAppVPPDynamicConfig(token, suffix, buttonText string, assignDeviceLicenses bool) string {
	return macAppVPPLocationFixture(token, suffix) + fmt.Sprintf(`
		variable "mac_name" { type = string }
		variable "mac_version" { type = string }
		variable "mac_bundle_id" { type = string }
		variable "mac_url" { type = string }
		variable "mac_description" { type = string }
		variable "mac_artwork_url" { type = string }

		resource "jamfplatform_pro_category" "edu" {
			name     = "tf-acc-macapp-edu-%[1]s"
			priority = 9
		}

		resource "jamfplatform_pro_icon" "app" {
			icon_file_source = var.mac_artwork_url
		}

		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name            = "${var.mac_name} tf-acc %[1]s"
				version         = var.mac_version
				bundle_id       = var.mac_bundle_id
				url             = var.mac_url
				is_free         = true
				deployment_type = "Make Available in Self Service"
				category_id     = jamfplatform_pro_category.edu.id
			}

			scope = {
				targets = {
					all_computers = true
				}
			}

			self_service = {
				install_button_text             = %[2]q
				self_service_description         = var.mac_description
				force_users_to_view_description  = false
				feature_on_main_page             = true
				notification_enabled             = true
				notification_method              = "Self Service"
				notification_subject             = "${var.mac_name} is available"
				notification_message             = "Install ${var.mac_name} from Self Service."

				self_service_icon = {
					id = jamfplatform_pro_icon.app.id
				}

				self_service_categories = [
					{
						id         = jamfplatform_pro_category.edu.id
						display_in = true
						feature_in = false
					},
				]
			}

			vpp = {
				assign_vpp_device_based_licenses = %[3]t
				vpp_admin_account_id             = jamfplatform_pro_volume_purchasing_location.vpp.id
			}
		}
	`, suffix, buttonText, assignDeviceLicenses)
}

// TestAccResource_ProMacApp_VPPFullMetadata is the full-coverage gated test: it
// exercises every embeddable field at once — full general metadata (incl.
// is_free + category), an all_computers scope, the complete self_service block
// (description, notification, the SetNested self_service_categories diff-by-id
// path, and a self_service_icon referencing a jamfplatform_pro_icon resource
// that uploads the app's real App Store artwork), and the top-level vpp block
// with device-based assignment.
//
// The app is sourced dynamically rather than hard-coded: device-based VPP
// assignment 409s ("App is not available for device assignment") unless the
// app's bundle_id matches a device-assignable title the token's location owns.
// Step 1 stands up the location and reads a device-assignable MAC_APP adam_id
// from its synced content via an HCL output; the Check resolves that adam_id's
// App Store metadata (bundle_id / version / url / description / artwork) from
// the public iTunes Lookup API; later steps inject it via TF_VAR_*. The update
// step flips install_button_text and assign_vpp_device_based_licenses
// true->false (wire-probed to round-trip), verifying the GET-after-Update path
// on the vpp block.
//
// Gated on JAMFPLATFORM_ACC_PRO_VPP_TOKEN. Skipped if the token's location owns no
// device-assignable Mac app (nothing to assign).
func TestAccResource_ProMacApp_VPPFullMetadata(t *testing.T) {
	token := testhelpers.AccEnv(vppTokenEnvVar)
	if token == "" {
		t.Skipf("%s not set; skipping full-metadata VPP acceptance test", vppTokenEnvVar)
	}
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	// Populated by the discovery step's Check, consumed by later steps' PreConfig.
	var meta itunesMacAppMeta

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			// Step 1 (discovery): stand up the location, pick a device-assignable
			// MAC_APP adam_id from its content output, resolve its App Store metadata.
			{
				Config: macAppVPPDiscoverConfig(token, suffix),
				Check: func(s *terraform.State) error {
					out, ok := s.RootModule().Outputs["mac_adam_ids"]
					if !ok {
						return fmt.Errorf("output mac_adam_ids not found in state")
					}
					ids, ok := out.Value.([]any)
					if !ok {
						return fmt.Errorf("output mac_adam_ids is %T, want a list", out.Value)
					}
					if len(ids) == 0 {
						t.Skipf("VPP token's location owns no device-assignable Mac app; nothing to assign")
					}
					// The location may own custom/B2B apps absent from the public
					// catalog. Try each owned adam_id until one resolves.
					for _, raw := range ids {
						id := fmt.Sprintf("%v", raw)
						m, found, err := lookupITunesMacApp(id)
						if err != nil {
							t.Logf("iTunes lookup for adam_id %s failed: %v; trying next", id, err)
							continue
						}
						if !found {
							t.Logf("adam_id %s has no public iTunes listing; trying next", id)
							continue
						}
						meta = m
						t.Logf("resolved adam_id %s -> %q (%s)", id, m.TrackName, m.BundleID)
						return nil
					}
					t.Skipf("none of the %d device-assignable Mac apps the token owns resolve in the public iTunes catalog (likely custom/B2B titles)", len(ids))
					return nil
				},
			},
			// Step 2 (create): inject resolved metadata and create the app with
			// device-based VPP assignment against the same location.
			{
				PreConfig: func() { setMacAppTFVars(t, meta) },
				Config:    macAppVPPDynamicConfig(token, suffix, "Get", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(macAppResourceAddr, "id"),
					resource.TestCheckResourceAttrWith(macAppResourceAddr, "general.bundle_id", func(v string) error {
						if v != meta.BundleID {
							return fmt.Errorf("general.bundle_id = %q, want resolved %q", v, meta.BundleID)
						}
						return nil
					}),
					resource.TestCheckResourceAttrWith(macAppResourceAddr, "general.version", func(v string) error {
						if v != meta.Version {
							return fmt.Errorf("general.version = %q, want resolved %q", v, meta.Version)
						}
						return nil
					}),
					resource.TestCheckResourceAttrSet(macAppResourceAddr, "general.url"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.is_free", "true"),
					resource.TestCheckResourceAttrSet(macAppResourceAddr, "general.category_id"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.targets.all_computers", "true"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.install_button_text", "Get"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.notification_enabled", "true"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.self_service_categories.#", "1"),
					resource.TestCheckResourceAttrPair(macAppResourceAddr, "self_service.self_service_icon.id", "jamfplatform_pro_icon.app", "id"),
					resource.TestCheckResourceAttrSet(macAppResourceAddr, "self_service.self_service_icon.uri"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "vpp.assign_vpp_device_based_licenses", "true"),
					resource.TestCheckResourceAttrSet(macAppResourceAddr, "vpp.vpp_admin_account_id"),
					resource.TestCheckResourceAttrSet(macAppResourceAddr, "vpp.total_vpp_licenses"),
				),
			},
			// Step 3 (update): flip the Self Service button text and turn
			// device-based assignment off.
			{
				PreConfig: func() { setMacAppTFVars(t, meta) },
				Config:    macAppVPPDynamicConfig(token, suffix, "Install", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.install_button_text", "Install"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "vpp.assign_vpp_device_based_licenses", "false"),
				),
			},
		},
	})
}

// macAppScopeTargetsConfig builds sibling building/department/network_segment
// resources and references their IDs from the app's scope — exercising the
// scope target/limitation/exclusion ID round-trips that the all_computers
// happy-paths never touch. directoryNames is interpolated into the
// directory-service name sets (plain strings, no sibling needed). The two
// computer/user target categories are omitted because they need real tenant
// inventory; everything else is hermetic.
func macAppScopeTargetsConfig(suffix string, limitationUserNames string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_building" "b1" {
			name = "tf-acc-macapp-bldg-%[1]s"
		}

		resource "jamfplatform_pro_department" "d1" {
			name = "tf-acc-macapp-dept-%[1]s"
		}

		resource "jamfplatform_pro_network_segment" "n1" {
			name             = "tf-acc-macapp-ns-%[1]s"
			starting_address = "10.123.0.1"
			ending_address   = "10.123.0.254"
		}

		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name      = "tf-acc-pro-macapp-scope-%[1]s"
				version   = "1.0"
				bundle_id = "com.example.tfacc.macapp.scope"
				url       = "https://apps.apple.com/app/id000000002"
			}

			scope = {
				targets = {
					building_ids   = [jamfplatform_pro_building.b1.id]
					department_ids = [jamfplatform_pro_department.d1.id]
				}

				limitations = {
					network_segment_ids                   = [jamfplatform_pro_network_segment.n1.id]
					directory_service_or_local_user_names  = %[2]s
					# directory_service_user_group_names omitted: the server matches
					# it against real LDAP groups ("Problem matching limitation user
					# group" 409), so it can't be exercised hermetically.
				}

				exclusions = {
					network_segment_ids                    = [jamfplatform_pro_network_segment.n1.id]
					directory_service_or_local_user_names   = ["excluded-user"]
				}
			}
		}
	`, suffix, limitationUserNames)
}

// TestAccResource_ProMacApp_ScopeTargets exercises the scope target / limitation
// / exclusion ID round-trips against real sibling resources, and (step 2) a
// set-shrink: the limitation directory-user name set goes from two entries to
// one, verifying the nested set updates cleanly under the scope
// read-merge-write update.
func TestAccResource_ProMacApp_ScopeTargets(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: macAppScopeTargetsConfig(suffix, `["alice", "bob"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(macAppResourceAddr, "id"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.targets.building_ids.#", "1"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.targets.department_ids.#", "1"),
					resource.TestCheckResourceAttrPair(macAppResourceAddr, "scope.targets.building_ids.0", "jamfplatform_pro_building.b1", "id"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.limitations.network_segment_ids.#", "1"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.limitations.directory_service_or_local_user_names.#", "2"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.exclusions.network_segment_ids.#", "1"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.exclusions.directory_service_or_local_user_names.#", "1"),
				),
			},
			{
				// Set-shrink: drop one directory-user name (2 -> 1).
				Config: macAppScopeTargetsConfig(suffix, `["alice"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.limitations.directory_service_or_local_user_names.#", "1"),
				),
			},
		},
	})
}

// TestAccDataSource_ProMacApp resolves a created app through the singular data
// source by ID and by name, asserting the flat read-only projection.
func TestAccDataSource_ProMacApp(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-ds-" + suffix

	config := fmt.Sprintf(`
		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name      = %q
				version   = "3.1"
				bundle_id = "com.example.tfacc.macapp.ds"
				url       = "https://apps.apple.com/app/id000000003"
				is_free   = true
			}
		}

		data "jamfplatform_pro_mac_app_store_app" "by_id" {
			id = jamfplatform_pro_mac_app_store_app.test.id
		}

		data "jamfplatform_pro_mac_app_store_app" "by_name" {
			name = jamfplatform_pro_mac_app_store_app.test.general.name
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_mac_app_store_app.by_id", "name", macAppResourceAddr, "general.name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mac_app_store_app.by_id", "version", "3.1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mac_app_store_app.by_id", "bundle_id", "com.example.tfacc.macapp.ds"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mac_app_store_app.by_id", "is_free", "true"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_mac_app_store_app.by_name", "id", macAppResourceAddr, "id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mac_app_store_app.by_name", "name", name),
				),
			},
		},
	})
}

// TestAccListResource_ProMacApp exercises the list resource via the
// `terraform query` workflow, filtering to the unique created app by name.
// The list is identity-only, so the check asserts the filtered result count.
func TestAccListResource_ProMacApp(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_mac_app_store_app" "test" {
						general = {
							name      = %q
							version   = "1.0"
							bundle_id = "com.example.tfacc.macapp.list"
							url       = "https://apps.apple.com/app/id000000004"
						}
					}
				`, name),
				Check: resource.TestCheckResourceAttrSet(macAppResourceAddr, "id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_mac_app_store_app" "test" {
						provider = jamfplatform

						config {
							filter = {
								name_substring = %q
							}
						}
					}
				`, name),
				// The list is identity-only (no include_resource), so assert the
				// filtered result count rather than per-resource attribute values.
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_mac_app_store_app.test", 1),
				},
			},
		},
	})
}

// TestAccResource_ProMacApp_ScopeLdapGroup exercises a limitation that
// references a real directory-service user group, which the server validates
// against the configured LDAP / cloud-IdP. Gated on JAMFPLATFORM_ACC_PRO_LDAP_GROUP_NAME
// (a group the Okta directory actually has); also serves as the live check that the
// plan-time DS-group preflight accepts a real group rather than rejecting it. The
// directory must exist before plan, so the LDAP server is pre-created via the SDK.
func TestAccResource_ProMacApp_ScopeLdapGroup(t *testing.T) {
	ldapEnv := testhelpers.RequireOktaLdapEnv(t)
	group := testhelpers.RequireLdapGroupName(t)
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-ldap-" + suffix
	testhelpers.EnsureLdapServerFixture(t, name, ldapEnv)
	// Wait until the fresh fixture's bind is up so the plan-time scope preflight
	// resolves the group instead of failing "not found".
	testhelpers.WaitForLdapGroupResolvable(t, group)

	config := fmt.Sprintf(`
		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name      = %q
				version   = "1.0"
				bundle_id = "com.example.tfacc.macapp.ldap"
				url       = "https://apps.apple.com/app/id000000005"
			}

			scope = {
				targets = {
					all_computers = true
				}

				limitations = {
					directory_service_user_group_names = [%q]
				}
			}
		}
	`, name, group)

	// cleared removes the directory-service group from scope by assigning an
	// empty set `[]` — under granular scope ownership the explicit `[]` is the
	// clear gesture (omitting the category would leave it unmanaged and
	// preserved). Applied as a final step BEFORE the framework destroys the
	// resource: destroying an app while a DS group is still scoped can leave an
	// orphaned app->LDAP association that blocks the LDAP server's deletion (a
	// server-side data-integrity bug). By clearing the reference first,
	// teardown leaves nothing pinning the directory.
	cleared := fmt.Sprintf(`
		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name      = %q
				version   = "1.0"
				bundle_id = "com.example.tfacc.macapp.ldap"
				url       = "https://apps.apple.com/app/id000000005"
			}

			scope = {
				targets = {
					all_computers = true
				}

				limitations = {
					directory_service_user_group_names = []
				}
			}
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(macAppResourceAddr, "id"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.limitations.directory_service_user_group_names.#", "1"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.limitations.directory_service_user_group_names.0", group),
				),
			},
			{
				// Detach the DS group before destroy (see `cleared` above) via an
				// empty set `[]`. Declared `[]` round-trips as `[]` under granular
				// ownership, so the count is asserted directly; the implicit
				// post-step empty-plan check enforces that the clear round-tripped.
				Config: cleared,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(macAppResourceAddr, "id"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.limitations.directory_service_user_group_names.#", "0"),
				),
			},
		},
	})
}

// TestAccResource_ProMacApp_ScopeSplitOwnership proves the granular scope
// ownership contract: a category the config does not declare (departments) is
// left to the admin UI — an out-of-band edit survives an unrelated Terraform
// change instead of being wiped by the scope subtree replace — while a
// declared category (buildings) stays owned, and declaring `[]` afterwards
// clears the co-managed category, proving Terraform can still take over.
func TestAccResource_ProMacApp_ScopeSplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	var appID string
	var deptID string

	// addDepartmentOutOfBand simulates a UI scope edit: GET the app, add the
	// department target, PUT the full object back (like the admin console).
	addDepartmentOutOfBand := func() {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		got, err := c.GetMacApplicationByID(ctx, appID)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}
		id, err := strconv.Atoi(deptID)
		if err != nil {
			t.Fatalf("department id %q: %v", deptID, err)
		}
		got.Scope.Departments = &proclassic.MacApplicationScopeDepartments{
			Department: &[]proclassic.IDName{{ID: &id}},
		}
		if err := c.UpdateMacApplicationByID(ctx, appID, got); err != nil {
			t.Fatalf("out-of-band PUT: %v", err)
		}
	}

	checkServerDepartments := func(want int) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := proclassic.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetMacApplicationByID(context.Background(), appID)
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			n := 0
			if got.Scope != nil && got.Scope.Departments != nil && got.Scope.Departments.Department != nil {
				n = len(*got.Scope.Departments.Department)
			}
			if n != want {
				return fmt.Errorf("server departments = %d, want %d", n, want)
			}
			return nil
		}
	}

	config := func(version, departmentIDs string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_building" "b1" {
				name = "tf-acc-macapp-split-bldg-%[1]s"
			}

			resource "jamfplatform_pro_department" "d1" {
				name = "tf-acc-macapp-split-dept-%[1]s"
			}

			resource "jamfplatform_pro_mac_app_store_app" "test" {
				general = {
					name      = "tf-acc-pro-macapp-split-%[1]s"
					version   = %[2]q
					bundle_id = "com.example.tfacc.macapp.split"
					url       = "https://apps.apple.com/app/id000000006"
				}

				scope = {
					targets = {
						building_ids = [jamfplatform_pro_building.b1.id]
						%[3]s
					}
				}
			}
		`, suffix, version, departmentIDs)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				// department_ids undeclared from the start: unmanaged.
				Config: config("1.0", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.targets.building_ids.#", "1"),
					resource.TestCheckNoResourceAttr(macAppResourceAddr, "scope.targets.department_ids"),
					func(s *terraform.State) error {
						appID = s.RootModule().Resources[macAppResourceAddr].Primary.ID
						deptID = s.RootModule().Resources["jamfplatform_pro_department.d1"].Primary.ID
						return nil
					},
				),
			},
			{
				// Admin adds a department in the UI; config changes only the
				// version. The read-merge-write update must re-emit the
				// department so the subtree replace does not wipe it — and it
				// must never enter Terraform state.
				PreConfig: addDepartmentOutOfBand,
				Config:    config("1.1", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "general.version", "1.1"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.targets.building_ids.#", "1"),
					resource.TestCheckNoResourceAttr(macAppResourceAddr, "scope.targets.department_ids"),
					checkServerDepartments(1),
				),
			},
			{
				// Declaring `[]` takes ownership and clears the category; the
				// declared buildings remain intact.
				Config: config("1.1", "department_ids = []"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.targets.department_ids.#", "0"),
					resource.TestCheckResourceAttr(macAppResourceAddr, "scope.targets.building_ids.#", "1"),
					checkServerDepartments(0),
				),
			},
		},
	})
}

// macAppOmitRetainsConfig is the fully declared shape for the omit-retains
// contract: every state-gated block carries a distinctive value so that a
// server which stopped retaining an omitted element is caught on content, not
// on presence.
func macAppOmitRetainsConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_category" "omit" {
			name     = "%[1]s-cat"
			priority = 9
		}

		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name            = %[1]q
				version         = "1.0"
				bundle_id       = "com.example.tfacc.macapp.omit"
				url             = "https://apps.apple.com/app/id000000001"
				deployment_type = "Install Automatically/Prompt Users to Install"
			}
			scope = {
				targets = {
					all_computers = true
				}
				exclusions = {
					directory_service_or_local_user_names = ["tf-acc-omit-retains-user"]
				}
			}
			self_service = {
				install_button_text             = "Retain me"
				self_service_description        = "Omit-retains contract description."
				force_users_to_view_description = true
				feature_on_main_page            = true
				self_service_categories = [
					{
						id         = jamfplatform_pro_category.omit.id
						display_in = true
						feature_in = true
					},
				]
			}
			vpp = {
				assign_vpp_device_based_licenses = false
				vpp_admin_account_id             = "-1"
			}
		}
	`, name)
}

// macAppOmitRetainsChildrenDroppedConfig keeps every parent block but drops
// their gated children and one Optional+Computed leaf: scope loses
// exclusions (re-emitted from the granular merge), self_service loses its
// categories collection and feature_on_main_page, and vpp goes entirely.
func macAppOmitRetainsChildrenDroppedConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_category" "omit" {
			name     = "%[1]s-cat"
			priority = 9
		}

		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name            = %[1]q
				version         = "1.0"
				bundle_id       = "com.example.tfacc.macapp.omit"
				url             = "https://apps.apple.com/app/id000000001"
				deployment_type = "Install Automatically/Prompt Users to Install"
			}
			scope = {
				targets = {
					all_computers = true
				}
			}
			self_service = {
				install_button_text             = "Retain me"
				self_service_description        = "Omit-retains contract description."
				force_users_to_view_description = true
			}
		}
	`, name)
}

// macAppOmitRetainsGeneralOnlyConfig drops every optional block, so the PUT
// carries <general> alone.
func macAppOmitRetainsGeneralOnlyConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_category" "omit" {
			name     = "%[1]s-cat"
			priority = 9
		}

		resource "jamfplatform_pro_mac_app_store_app" "test" {
			general = {
				name            = %[1]q
				version         = "1.0"
				bundle_id       = "com.example.tfacc.macapp.omit"
				url             = "https://apps.apple.com/app/id000000001"
				deployment_type = "Install Automatically/Prompt Users to Install"
			}
		}
	`, name)
}

// macAppRetainedOnServer asserts the server's copy still carries every value
// the omit-retains config declared in its first step.
func macAppRetainedOnServer(t *testing.T) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return testhelpers.CheckLiveObject(macAppResourceAddr,
		func(ctx context.Context, id string) (*proclassic.MacApplication, error) {
			return c.GetMacApplicationByID(ctx, id)
		},
		func(a *proclassic.MacApplication) error {
			if a.Scope == nil {
				return fmt.Errorf("scope: absent")
			}
			if err := testhelpers.RequireEqual("scope.all_computers", true, testhelpers.Deref(a.Scope.AllComputers)); err != nil {
				return err
			}
			if a.Scope.Exclusions == nil || a.Scope.Exclusions.Users == nil || a.Scope.Exclusions.Users.User == nil || len(*a.Scope.Exclusions.Users.User) != 1 {
				return fmt.Errorf("scope.exclusions.users: want exactly one user, got %+v", a.Scope.Exclusions)
			}
			if err := testhelpers.RequireEqual("scope.exclusions.users[0].name", "tf-acc-omit-retains-user", testhelpers.Deref((*a.Scope.Exclusions.Users.User)[0].Name)); err != nil {
				return err
			}
			if a.SelfService == nil {
				return fmt.Errorf("self_service: absent")
			}
			if err := testhelpers.RequireEqual("self_service.install_button_text", "Retain me", testhelpers.Deref(a.SelfService.InstallButtonText)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("self_service.self_service_description", "Omit-retains contract description.", testhelpers.Deref(a.SelfService.SelfServiceDescription)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("self_service.force_users_to_view_description", true, testhelpers.Deref(a.SelfService.ForceUsersToViewDescription)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("self_service.feature_on_main_page", true, testhelpers.Deref(a.SelfService.FeatureOnMainPage)); err != nil {
				return err
			}
			if a.SelfService.SelfServiceCategories == nil || a.SelfService.SelfServiceCategories.Category == nil || len(*a.SelfService.SelfServiceCategories.Category) != 1 {
				return fmt.Errorf("self_service.self_service_categories: want exactly one category, got %+v", a.SelfService.SelfServiceCategories)
			}
			cat := (*a.SelfService.SelfServiceCategories.Category)[0]
			if err := testhelpers.RequireEqual("self_service.self_service_categories[0].feature_in", true, testhelpers.Deref(cat.FeatureIn)); err != nil {
				return err
			}
			if a.Vpp == nil {
				return fmt.Errorf("vpp: absent")
			}
			return testhelpers.RequireEqual("vpp.vpp_admin_account_id", -1, testhelpers.Deref(a.Vpp.VppAdminAccountID))
		})
}

// TestAccResource_ProMacApp_OmittedBlocksRetained pins the omit-retains
// contract the plan output cannot show: dropping a gated block or
// sub-collection from config plans it as removed, but the classic PUT omits
// the element and the server keeps every value. Step 2 keeps every parent
// block and drops its gated children (scope exclusions through the granular
// merge, self_service categories, one Optional+Computed leaf) plus the vpp
// block; step 3 drops every optional block so the PUT carries <general>
// alone. Each step's implicit
// post-apply plan must be empty, which is what makes the contract usable.
// If this test fails on content, the endpoint no longer merges and nothing
// that suppresses the removal plan may ship for this resource.
func TestAccResource_ProMacApp_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-macapp-omit-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMacAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: macAppOmitRetainsConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.install_button_text", "Retain me"),
					macAppRetainedOnServer(t),
				),
			},
			{
				Config: macAppOmitRetainsChildrenDroppedConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(macAppResourceAddr, "self_service.install_button_text", "Retain me"),
					resource.TestCheckNoResourceAttr(macAppResourceAddr, "self_service.self_service_categories.#"),
					resource.TestCheckNoResourceAttr(macAppResourceAddr, "scope.exclusions.directory_service_or_local_user_names.#"),
					resource.TestCheckNoResourceAttr(macAppResourceAddr, "vpp.vpp_admin_account_id"),
					macAppRetainedOnServer(t),
				),
			},
			{
				Config: macAppOmitRetainsGeneralOnlyConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(macAppResourceAddr, "scope.targets.all_computers"),
					macAppRetainedOnServer(t),
				),
			},
		},
	})
}
