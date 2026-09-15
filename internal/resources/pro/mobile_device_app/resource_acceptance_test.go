// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /mobiledeviceapplications
// endpoint. Classic has known concurrency issues when multiple writes hit the
// same resource type — keep these tests serial with any future classic
// acceptance work in this package.
//
// Scope happy-paths use all_mobile_devices = true so they need no pre-existing
// tenant target objects, and general.site is left unset. os_type is required on
// every config (the server demands it on a PUT to an in-house app, which is the
// common case). The ungated tests never set vpp.assign_vpp_device_based_licenses
// = true on a non-VPP title (that 409s "App is not available for device
// assignment"); TestAccResource_ProMobileApp_VPPDeviceAssignment409 asserts that
// 409 deliberately.

package mobile_device_app_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const mobileAppResourceAddr = "jamfplatform_pro_mobile_device_app.test"

// testAccCheckMobileAppDestroy verifies apps created during the test were
// destroyed. Note the server returns 400 on a successful DELETE (handled in the
// resource), so destroy correctness is itself part of what this asserts.
func testAccCheckMobileAppDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_mobile_device_app" {
				continue
			}
			_, err := c.GetMobileDeviceApplicationByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro mobile device app %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro mobile device app %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// mobileAppGeneralOnlyConfig is the import-stable shape: only the required
// general fields. The importer populates general post-Read but leaves optional
// blocks null, so ImportStateVerify must run against a general-only config.
// deployAutomatically is what actually selects the distribution method:
// general.deployment_type is a read-only mirror of it (false -> "Make Available
// in Self Service", true -> "Install Automatically/Prompt Users to Install"),
// and a write to deployment_type is discarded on create and on update alike.
func mobileAppGeneralOnlyConfig(name, version string, deployAutomatically bool) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name                = %q
				version             = %q
				bundle_id           = "com.example.tfacc.mobileapp"
				os_type             = "iOS"
				deploy_automatically = %t
			}
		}
	`, name, version, deployAutomatically)
}

// mobileAppFullConfig adds self_service and an all_mobile_devices scope on top of general.
func mobileAppFullConfig(name, version, buttonText string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name            = %q
				version         = %q
				bundle_id       = "com.example.tfacc.mobileapp"
				os_type         = "iOS"
				deploy_automatically = false
			}
			scope = {
				targets = {
					all_mobile_devices = true
				}
			}
			self_service = {
				install_button_text        = %q
				after_install_button_text  = "Open"
				self_service_description    = "Managed by Terraform acceptance test."
				feature_on_main_page        = true
				notification_enabled        = true
			}
		}
	`, name, version, buttonText)
}

// TestAccResource_ProMobileApp_Basic exercises create, in-place update, and
// import for the general-only shape. The version/deploy_automatically change
// verifies the GET-after-Update path (classic UpdateMobileDeviceApplicationByID
// returns 201 empty). Update also re-sends os_type — without it the server 409s
// on the PUT.
//
// The distribution method is driven through general.deploy_automatically and
// only asserted on general.deployment_type. This step used to set
// deployment_type directly and passed only because the read was sticky: the
// write has never worked, on create or update, so the state was reporting a
// value the server had thrown away.
func TestAccResource_ProMobileApp_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mobileAppGeneralOnlyConfig(name, "1.0", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(mobileAppResourceAddr, "id"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.name", name),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.version", "1.0"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.os_type", "iOS"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.deploy_automatically", "false"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.deployment_type", "Make Available in Self Service"),
				),
			},
			{
				ResourceName:      mobileAppResourceAddr,
				ImportState:       true,
				ImportStateVerify: true,
				// timeouts: not returned by the API. os_type IS verifiable here:
				// Create issues a follow-up PUT that persists it, so the GET (incl.
				// import's) echoes it for this in-house app. scope /
				// self_service / vpp / app_configuration: Optional state-gated
				// blocks this general-only config never declares. Import
				// hydrates them from the server's echoed defaults (correct —
				// see the import-hydration fix), which legitimately differs
				// from this config's null. Not verified here.
				ImportStateVerifyIgnore: []string{"timeouts", "scope", "self_service", "vpp", "app_configuration"},
			},
			{
				Config: mobileAppGeneralOnlyConfig(name, "2.0", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.version", "2.0"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.deploy_automatically", "true"),
					// The mirror moved with its source, which is the whole point.
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.deployment_type", "Install Automatically/Prompt Users to Install"),
				),
			},
			{
				// Rename: guards that an update changing name applies cleanly. The
				// server-derived echo fields (description / category_name /
				// site_name) do not change on a rename, so their UseStateForUnknown
				// plan values stay consistent through apply.
				Config: mobileAppGeneralOnlyConfig(name+"-renamed", "2.0", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "general.name", name+"-renamed"),
				),
			},
		},
	})
}

// TestAccResource_ProMobileApp_ScopeAndSelfService exercises the scope +
// self_service blocks and an in-place self_service mutation (not a block
// deletion — the classic PUT is partial-merge and will not clear an omitted block).
func TestAccResource_ProMobileApp_ScopeAndSelfService(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-ss-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mobileAppFullConfig(name, "1.0", "Install"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(mobileAppResourceAddr, "id"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.targets.all_mobile_devices", "true"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "self_service.install_button_text", "Install"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "self_service.after_install_button_text", "Open"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "self_service.feature_on_main_page", "true"),
				),
			},
			{
				Config: mobileAppFullConfig(name, "1.0", "Get"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "self_service.install_button_text", "Get"),
				),
			},
		},
	})
}

// TestAccResource_ProMobileApp_AppConfiguration exercises the app_configuration
// block and the CRLF↔LF semantic-equality plan modifier: step 2 reuses the same
// content with CRLF newlines and must produce an empty plan (no diff).
func TestAccResource_ProMobileApp_AppConfiguration(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-appcfg-" + suffix

	cfg := func(prefs string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_mobile_device_app" "test" {
				general = {
					name      = %q
					version   = "1.0"
					bundle_id = "com.example.tfacc.mobileapp.appcfg"
					os_type   = "iOS"
				}
				app_configuration = {
					preferences = %q
				}
			}
		`, name, prefs)
	}

	const lf = "<dict>\n  <key>Server</key>\n  <string>https://example.com</string>\n</dict>"
	const crlf = "<dict>\r\n  <key>Server</key>\r\n  <string>https://example.com</string>\r\n</dict>"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg(lf),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(mobileAppResourceAddr, "id"),
					resource.TestCheckResourceAttrSet(mobileAppResourceAddr, "app_configuration.preferences"),
				),
			},
			{
				// Same content, CRLF newlines: the plan modifier treats it as equal.
				Config:   cfg(crlf),
				PlanOnly: true,
			},
		},
	})
}

// TestAccResource_ProMobileApp_DeploymentTypeIsUnconfigurable asserts that
// general.deployment_type cannot be set at all.
//
// It replaces a test that asserted a OneOf validator on the attribute. The
// validator was not wrong about the vocabulary, but it implied the attribute
// was writable, and it never was: Jamf Pro derives deployment_type from
// general.deploy_automatically and discards a write to it on create and on
// update alike. Even the in-set value the old test's sibling steps sent was
// being thrown away — the sticky read is what made that look like success.
func TestAccResource_ProMobileApp_DeploymentTypeIsUnconfigurable(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-bad-dt-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name            = %q
				version         = "1.0"
				bundle_id       = "com.example.tfacc.mobileapp"
				os_type         = "iOS"
				deployment_type = "Install Automatically/Prompt Users to Install"
			}
		}
	`, name),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid Configuration for Read-Only Attribute`),
			},
		},
	})
}

// TestAccResource_ProMobileApp_InvalidOsType asserts the os_type OneOf validator
// rejects a value outside {iOS, tvOS} at plan time.
func TestAccResource_ProMobileApp_InvalidOsType(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-bad-os-" + suffix
	config := fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name      = %q
				version   = "1.0"
				bundle_id = "com.example.tfacc.mobileapp"
				os_type   = "watchOS"
			}
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("value must be one of"),
			},
		},
	})
}

// TestAccResource_ProMobileApp_AllMobileDevicesConflict asserts the shared scope
// validator rejects all_mobile_devices = true alongside a device target.
func TestAccResource_ProMobileApp_AllMobileDevicesConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-allmd-" + suffix
	config := fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name      = %q
				version   = "1.0"
				bundle_id = "com.example.tfacc.mobileapp"
				os_type   = "iOS"
			}
			scope = {
				targets = {
					all_mobile_devices = true
					mobile_device_ids  = ["1"]
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

// TestAccResource_ProMobileApp_AllJssUsersConflict asserts the shared scope
// validator rejects all_jss_users = true alongside a user target.
func TestAccResource_ProMobileApp_AllJssUsersConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-allu-" + suffix
	config := fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name      = %q
				version   = "1.0"
				bundle_id = "com.example.tfacc.mobileapp"
				os_type   = "iOS"
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

// TestAccResource_ProMobileApp_VPPDeviceAssignment409 asserts the documented VPP
// invariant: assigning device-based licenses to a non-VPP-backed title returns
// HTTP 409 "App is not available for device assignment". Apply-time error.
func TestAccResource_ProMobileApp_VPPDeviceAssignment409(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-vpp409-" + suffix
	config := fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name      = %q
				version   = "1.0"
				bundle_id = "com.example.tfacc.mobileapp.vpp"
				os_type   = "iOS"
				is_free   = true
			}
			vpp = {
				assign_vpp_device_based_licenses = true
			}
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile("(?i)not available for device assignment"),
			},
		},
	})
}

// mobileAppScopeTargetsConfig builds sibling building/department/network_segment
// resources and references their IDs from the app's scope — exercising the scope
// target/limitation/exclusion ID round-trips that the all_mobile_devices
// happy-paths never touch. The mobile-device/user target categories are omitted
// because they need real tenant inventory; everything else is hermetic.
func mobileAppScopeTargetsConfig(suffix string, limitationUserNames string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_building" "b1" {
			name = "tf-acc-mobileapp-bldg-%[1]s"
		}

		resource "jamfplatform_pro_department" "d1" {
			name = "tf-acc-mobileapp-dept-%[1]s"
		}

		resource "jamfplatform_pro_network_segment" "n1" {
			name             = "tf-acc-mobileapp-ns-%[1]s"
			starting_address = "10.124.0.1"
			ending_address   = "10.124.0.254"
		}

		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name      = "tf-acc-pro-mobileapp-scope-%[1]s"
				version   = "1.0"
				bundle_id = "com.example.tfacc.mobileapp.scope"
				os_type   = "iOS"
			}

			scope = {
				targets = {
					building_ids   = [jamfplatform_pro_building.b1.id]
					department_ids = [jamfplatform_pro_department.d1.id]
				}

				limitations = {
					network_segment_ids                    = [jamfplatform_pro_network_segment.n1.id]
					directory_service_or_local_user_names   = %[2]s
				}

				exclusions = {
					network_segment_ids                    = [jamfplatform_pro_network_segment.n1.id]
					directory_service_or_local_user_names   = ["excluded-user"]
				}
			}
		}
	`, suffix, limitationUserNames)
}

// TestAccResource_ProMobileApp_ScopeTargets exercises the scope target /
// limitation / exclusion ID round-trips against real sibling resources, and
// (step 2) a set-shrink on the limitation directory-user names.
func TestAccResource_ProMobileApp_ScopeTargets(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mobileAppScopeTargetsConfig(suffix, `["alice", "bob"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(mobileAppResourceAddr, "id"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.targets.building_ids.#", "1"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.targets.department_ids.#", "1"),
					resource.TestCheckResourceAttrPair(mobileAppResourceAddr, "scope.targets.building_ids.0", "jamfplatform_pro_building.b1", "id"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.limitations.network_segment_ids.#", "1"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.limitations.directory_service_or_local_user_names.#", "2"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.exclusions.network_segment_ids.#", "1"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.exclusions.directory_service_or_local_user_names.#", "1"),
				),
			},
			{
				Config: mobileAppScopeTargetsConfig(suffix, `["alice"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.limitations.directory_service_or_local_user_names.#", "1"),
				),
			},
		},
	})
}

// TestAccDataSource_ProMobileApp resolves a created app through the singular data
// source by ID and by name, asserting the flat read-only projection.
func TestAccDataSource_ProMobileApp(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-ds-" + suffix

	config := fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name      = %q
				version   = "3.1"
				bundle_id = "com.example.tfacc.mobileapp.ds"
				os_type   = "iOS"
				is_free   = true
			}
		}

		data "jamfplatform_pro_mobile_device_app" "by_id" {
			id = jamfplatform_pro_mobile_device_app.test.id
		}

		data "jamfplatform_pro_mobile_device_app" "by_name" {
			name = jamfplatform_pro_mobile_device_app.test.general.name
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_mobile_device_app.by_id", "name", mobileAppResourceAddr, "general.name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mobile_device_app.by_id", "version", "3.1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mobile_device_app.by_id", "bundle_id", "com.example.tfacc.mobileapp.ds"),
					// os_type is recoverable on read because Create persists it via
					// the follow-up PUT (this is an in-house app).
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mobile_device_app.by_id", "os_type", "iOS"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mobile_device_app.by_id", "is_free", "true"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_mobile_device_app.by_name", "id", mobileAppResourceAddr, "id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mobile_device_app.by_name", "name", name),
				),
			},
		},
	})
}

// TestAccListResource_ProMobileApp exercises the list resource via the
// `terraform query` workflow, filtering to the unique created app by name.
func TestAccListResource_ProMobileApp(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_mobile_device_app" "test" {
						general = {
							name      = %q
							version   = "1.0"
							bundle_id = "com.example.tfacc.mobileapp.list"
							os_type   = "iOS"
						}
					}
				`, name),
				Check: resource.TestCheckResourceAttrSet(mobileAppResourceAddr, "id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_mobile_device_app" "test" {
						provider = jamfplatform

						config {
							filter = {
								name_substring = %q
							}
						}
					}
				`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_mobile_device_app.test", 1),
				},
			},
		},
	})
}

// TestAccResource_ProMobileApp_ScopeLdapGroup exercises a limitation that
// references a real directory-service user group, which the server validates
// against the configured LDAP / cloud-IdP. Gated on JAMFPLATFORM_ACC_PRO_LDAP_GROUP_NAME;
// also serves as the live check that the plan-time DS-group preflight accepts a real
// group rather than rejecting it. The directory must exist before plan, so the LDAP
// server is pre-created via the SDK.
func TestAccResource_ProMobileApp_ScopeLdapGroup(t *testing.T) {
	ldapEnv := testhelpers.RequireOktaLdapEnv(t)
	group := testhelpers.RequireLdapGroupName(t)
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-ldap-" + suffix
	testhelpers.EnsureLdapServerFixture(t, name, ldapEnv)
	// Wait until the fresh fixture's bind is up so the plan-time scope preflight
	// resolves the group instead of failing "not found".
	testhelpers.WaitForLdapGroupResolvable(t, group)

	config := fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name      = %q
				version   = "1.0"
				bundle_id = "com.example.tfacc.mobileapp.ldap"
				os_type   = "iOS"
			}

			scope = {
				targets = {
					all_mobile_devices = true
				}

				limitations = {
					directory_service_user_group_names = [%q]
				}
			}
		}
	`, name, group)

	// cleared removes the directory-service group from scope by declaring an
	// empty set `[]` — the granular-ownership clear gesture (a declared empty
	// category is Terraform-owned and emptied; omitting it would preserve the
	// group). Applied as a final step BEFORE the framework destroys the
	// resource: destroying an app while a DS group is still scoped can leave an
	// orphaned app->LDAP association that blocks the LDAP server's deletion (a
	// server-side data-integrity bug). By clearing the reference first,
	// teardown leaves nothing pinning the directory.
	cleared := fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name      = %q
				version   = "1.0"
				bundle_id = "com.example.tfacc.mobileapp.ldap"
				os_type   = "iOS"
			}

			scope = {
				targets = {
					all_mobile_devices = true
				}

				limitations = {
					directory_service_user_group_names = []
				}
			}
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(mobileAppResourceAddr, "id"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.limitations.directory_service_user_group_names.#", "1"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.limitations.directory_service_user_group_names.0", group),
				),
			},
			{
				// Detach the DS group before destroy (see `cleared` above) via a
				// declared empty set `[]`, which round-trips as `[]` in state. The
				// implicit post-step empty-plan check enforces that the clear
				// round-tripped server-side.
				Config: cleared,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(mobileAppResourceAddr, "id"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.limitations.directory_service_user_group_names.#", "0"),
				),
			},
		},
	})
}

// mobileAppOmitRetainsPreferences is the app_configuration plist the
// omit-retains contract declares. The server stores it re-serialised inside a
// full plist envelope (XML prolog, DOCTYPE, tab indentation — wire-observed
// 2026-09-06), so the wire assertion looks for the distinctive value inside
// the stored document rather than comparing the strings byte for byte.
const (
	mobileAppOmitRetainsPreferences      = "<dict><key>Server</key><string>https://example.com/omit-retains</string></dict>"
	mobileAppOmitRetainsPreferencesValue = "<string>https://example.com/omit-retains</string>"
)

// mobileAppOmitRetainsFixtures returns the sibling objects every omit-retains
// step references: two buildings (one targeted, one excluded), a department, a
// network segment (limited and excluded) and a Self Service category.
func mobileAppOmitRetainsFixtures(suffix string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_building" "b1" {
			name = "tf-acc-mobileapp-omit-bldg1-%[1]s"
		}

		resource "jamfplatform_pro_building" "b2" {
			name = "tf-acc-mobileapp-omit-bldg2-%[1]s"
		}

		resource "jamfplatform_pro_department" "d1" {
			name = "tf-acc-mobileapp-omit-dept-%[1]s"
		}

		resource "jamfplatform_pro_network_segment" "n1" {
			name             = "tf-acc-mobileapp-omit-ns-%[1]s"
			starting_address = "10.213.0.1"
			ending_address   = "10.213.0.254"
		}

		resource "jamfplatform_pro_category" "c1" {
			name     = "tf-acc-mobileapp-omit-cat-%[1]s"
			priority = 7
		}
	`, suffix)
}

// mobileAppOmitRetainsConfig is the fully declared shape for the omit-retains
// contract: every state-gated block — scope targets / limitations /
// exclusions, self_service with its categories, vpp and app_configuration —
// carries a distinctive value so that a server which stopped retaining an
// omitted element is caught on content, not on presence.
func mobileAppOmitRetainsConfig(suffix, name string) string {
	return mobileAppOmitRetainsFixtures(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name            = %[1]q
				version         = "1.0"
				bundle_id       = "com.example.tfacc.mobileapp.omit"
				os_type         = "iOS"
			}
			scope = {
				targets = {
					building_ids   = [jamfplatform_pro_building.b1.id]
					department_ids = [jamfplatform_pro_department.d1.id]
				}
				limitations = {
					network_segment_ids                   = [jamfplatform_pro_network_segment.n1.id]
					directory_service_or_local_user_names = ["tf-acc-omit-retains-limit-user"]
				}
				exclusions = {
					building_ids                          = [jamfplatform_pro_building.b2.id]
					network_segment_ids                   = [jamfplatform_pro_network_segment.n1.id]
					directory_service_or_local_user_names = ["tf-acc-omit-retains-excl-user"]
				}
			}
			self_service = {
				install_button_text      = "Retain me"
				self_service_description = "Omit-retains contract description."
				feature_on_main_page     = true
				self_service_categories = [
					{
						id         = jamfplatform_pro_category.c1.id
						display_in = true
					},
				]
			}
			vpp = {
				assign_vpp_device_based_licenses = false
				vpp_admin_account_id             = "-1"
			}
			app_configuration = {
				preferences = %[2]q
			}
		}
	`, name, mobileAppOmitRetainsPreferences)
}

// mobileAppOmitRetainsParentsOnlyConfig keeps the two blocks that have gated
// children but drops the children: scope keeps its targets and loses
// limitations and exclusions (so the scope goes through the granular merge),
// self_service loses self_service_categories and the Optional+Computed
// feature_on_main_page leaf. vpp and app_configuration are dropped whole.
func mobileAppOmitRetainsParentsOnlyConfig(suffix, name string) string {
	return mobileAppOmitRetainsFixtures(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name            = %q
				version         = "1.0"
				bundle_id       = "com.example.tfacc.mobileapp.omit"
				os_type         = "iOS"
			}
			scope = {
				targets = {
					building_ids   = [jamfplatform_pro_building.b1.id]
					department_ids = [jamfplatform_pro_department.d1.id]
				}
			}
			self_service = {
				install_button_text      = "Retain me"
				self_service_description = "Omit-retains contract description."
			}
		}
	`, name)
}

// mobileAppOmitRetainsGeneralOnlyConfig drops every optional block, so the PUT
// carries <general> alone. The fixtures stay so the server-side references
// they back remain valid while the app still holds them.
func mobileAppOmitRetainsGeneralOnlyConfig(suffix, name string) string {
	return mobileAppOmitRetainsFixtures(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_mobile_device_app" "test" {
			general = {
				name            = %q
				version         = "1.0"
				bundle_id       = "com.example.tfacc.mobileapp.omit"
				os_type         = "iOS"
			}
		}
	`, name)
}

// mobileAppStateID reads a sibling fixture's Jamf Pro id out of Terraform state
// so the wire assertion can compare scope members by value rather than count.
func mobileAppStateID(s *terraform.State, addr string) (int, error) {
	rs, ok := s.RootModule().Resources[addr]
	if !ok {
		return 0, fmt.Errorf("fixture %s not found in state", addr)
	}
	id, err := strconv.Atoi(rs.Primary.ID)
	if err != nil {
		return 0, fmt.Errorf("fixture %s: id %q is not an integer: %w", addr, rs.Primary.ID, err)
	}
	return id, nil
}

// mobileAppRetainedOnServer asserts the server's copy still carries every value
// the omit-retains config declared in its first step. It covers only what the
// GET echoes: self_service_after_install_button_text, notification,
// notification_subject and notification_message are accepted on the PUT but
// never returned for a mobile device app (wire-observed 2026-09-06), so the
// config leaves them out rather than declare a value nothing can verify.
func mobileAppRetainedOnServer(t *testing.T) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return func(s *terraform.State) error {
		b1, err := mobileAppStateID(s, "jamfplatform_pro_building.b1")
		if err != nil {
			return err
		}
		b2, err := mobileAppStateID(s, "jamfplatform_pro_building.b2")
		if err != nil {
			return err
		}
		d1, err := mobileAppStateID(s, "jamfplatform_pro_department.d1")
		if err != nil {
			return err
		}
		n1, err := mobileAppStateID(s, "jamfplatform_pro_network_segment.n1")
		if err != nil {
			return err
		}
		c1, err := mobileAppStateID(s, "jamfplatform_pro_category.c1")
		if err != nil {
			return err
		}
		return testhelpers.CheckLiveObject(mobileAppResourceAddr,
			func(ctx context.Context, id string) (*proclassic.MobileDeviceApplication, error) {
				return c.GetMobileDeviceApplicationByID(ctx, id)
			},
			func(a *proclassic.MobileDeviceApplication) error {
				if a.Scope == nil {
					return fmt.Errorf("scope: absent")
				}
				if err := requireOneIDName("scope.targets.buildings", buildingSliceOf(a.Scope.Buildings), b1); err != nil {
					return err
				}
				if err := requireOneIDName("scope.targets.departments", departmentSliceOf(a.Scope.Departments), d1); err != nil {
					return err
				}
				if a.Scope.Limitations == nil {
					return fmt.Errorf("scope.limitations: absent")
				}
				if a.Scope.Limitations.NetworkSegments == nil {
					return fmt.Errorf("scope.limitations.network_segments: absent")
				}
				if err := requireOneIDName("scope.limitations.network_segments", a.Scope.Limitations.NetworkSegments.NetworkSegment, n1); err != nil {
					return err
				}
				if a.Scope.Limitations.Users == nil || a.Scope.Limitations.Users.User == nil || len(*a.Scope.Limitations.Users.User) != 1 {
					return fmt.Errorf("scope.limitations.users: want exactly one user, got %+v", a.Scope.Limitations.Users)
				}
				if err := testhelpers.RequireEqual("scope.limitations.users[0].name", "tf-acc-omit-retains-limit-user", testhelpers.Deref((*a.Scope.Limitations.Users.User)[0].Name)); err != nil {
					return err
				}
				if a.Scope.Exclusions == nil {
					return fmt.Errorf("scope.exclusions: absent")
				}
				if a.Scope.Exclusions.Buildings == nil {
					return fmt.Errorf("scope.exclusions.buildings: absent")
				}
				if err := requireOneIDName("scope.exclusions.buildings", a.Scope.Exclusions.Buildings.Building, b2); err != nil {
					return err
				}
				if a.Scope.Exclusions.NetworkSegments == nil || a.Scope.Exclusions.NetworkSegments.NetworkSegment == nil || len(*a.Scope.Exclusions.NetworkSegments.NetworkSegment) != 1 {
					return fmt.Errorf("scope.exclusions.network_segments: want exactly one segment, got %+v", a.Scope.Exclusions.NetworkSegments)
				}
				if err := testhelpers.RequireEqual("scope.exclusions.network_segments[0].id", n1, testhelpers.Deref((*a.Scope.Exclusions.NetworkSegments.NetworkSegment)[0].ID)); err != nil {
					return err
				}
				if a.Scope.Exclusions.Users == nil || a.Scope.Exclusions.Users.User == nil || len(*a.Scope.Exclusions.Users.User) != 1 {
					return fmt.Errorf("scope.exclusions.users: want exactly one user, got %+v", a.Scope.Exclusions.Users)
				}
				if err := testhelpers.RequireEqual("scope.exclusions.users[0].name", "tf-acc-omit-retains-excl-user", testhelpers.Deref((*a.Scope.Exclusions.Users.User)[0].Name)); err != nil {
					return err
				}

				ss := a.SelfService
				if ss == nil {
					return fmt.Errorf("self_service: absent")
				}
				if err := testhelpers.RequireEqual("self_service.install_button_text", "Retain me", testhelpers.Deref(ss.SelfServiceInstallButtonText)); err != nil {
					return err
				}
				if err := testhelpers.RequireEqual("self_service.self_service_description", "Omit-retains contract description.", testhelpers.Deref(ss.SelfServiceDescription)); err != nil {
					return err
				}
				if err := testhelpers.RequireEqual("self_service.feature_on_main_page", true, testhelpers.Deref(ss.FeatureOnMainPage)); err != nil {
					return err
				}
				if ss.SelfServiceCategories == nil || ss.SelfServiceCategories.Category == nil || len(*ss.SelfServiceCategories.Category) != 1 {
					return fmt.Errorf("self_service.self_service_categories: want exactly one category, got %+v", ss.SelfServiceCategories)
				}
				cat := (*ss.SelfServiceCategories.Category)[0]
				if err := testhelpers.RequireEqual("self_service.self_service_categories[0].id", c1, testhelpers.Deref(cat.ID)); err != nil {
					return err
				}
				if err := testhelpers.RequireEqual("self_service.self_service_categories[0].display_in", true, testhelpers.Deref(cat.DisplayIn)); err != nil {
					return err
				}

				if a.Vpp == nil {
					return fmt.Errorf("vpp: absent")
				}
				if err := testhelpers.RequireEqual("vpp.assign_vpp_device_based_licenses", false, testhelpers.Deref(a.Vpp.AssignVppDeviceBasedLicenses)); err != nil {
					return err
				}
				if err := testhelpers.RequireEqual("vpp.vpp_admin_account_id", -1, testhelpers.Deref(a.Vpp.VppAdminAccountID)); err != nil {
					return err
				}

				if a.AppConfiguration == nil {
					return fmt.Errorf("app_configuration: absent")
				}
				if prefs := testhelpers.Deref(a.AppConfiguration.Preferences); !strings.Contains(prefs, mobileAppOmitRetainsPreferencesValue) {
					return fmt.Errorf("app_configuration.preferences: want a plist containing %s, got %q", mobileAppOmitRetainsPreferencesValue, prefs)
				}
				return nil
			})(s)
	}
}

// requireOneIDName asserts an id/name scope category holds exactly the one
// member with the given id.
func requireOneIDName(field string, items *[]proclassic.IDName, wantID int) error {
	if items == nil || len(*items) != 1 {
		return fmt.Errorf("%s: want exactly one member, got %+v", field, items)
	}
	return testhelpers.RequireEqual(field+"[0].id", wantID, testhelpers.Deref((*items)[0].ID))
}

func buildingSliceOf(b *proclassic.MobileDeviceApplicationScopeBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func departmentSliceOf(d *proclassic.MobileDeviceApplicationScopeDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

// TestAccResource_ProMobileApp_OmittedBlocksRetained pins the omit-retains
// contract the plan output cannot show: dropping scope limitations and
// exclusions, self_service categories, vpp and app_configuration from config
// plans them as removed, but the classic PUT omits the elements and the
// server keeps every value. Step 2 keeps scope.targets and a trimmed
// self_service so the scope goes through the granular merge and the
// self_service PUT omits its categories; step 3 drops every optional block so
// the PUT carries <general> alone. Each step's implicit post-apply plan must
// be empty, which is what makes the contract usable. If this test fails on
// content, the endpoint no longer merges and nothing that suppresses the
// removal plan may ship for this resource.
func TestAccResource_ProMobileApp_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mobileapp-omit-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMobileAppDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mobileAppOmitRetainsConfig(suffix, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "self_service.install_button_text", "Retain me"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "self_service.self_service_categories.#", "1"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.exclusions.building_ids.#", "1"),
					mobileAppRetainedOnServer(t),
				),
			},
			{
				Config: mobileAppOmitRetainsParentsOnlyConfig(suffix, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "scope.targets.building_ids.#", "1"),
					resource.TestCheckResourceAttr(mobileAppResourceAddr, "self_service.feature_on_main_page", "true"),
					resource.TestCheckNoResourceAttr(mobileAppResourceAddr, "scope.limitations.network_segment_ids.#"),
					resource.TestCheckNoResourceAttr(mobileAppResourceAddr, "scope.exclusions.building_ids.#"),
					resource.TestCheckNoResourceAttr(mobileAppResourceAddr, "self_service.self_service_categories.#"),
					resource.TestCheckNoResourceAttr(mobileAppResourceAddr, "vpp.vpp_admin_account_id"),
					resource.TestCheckNoResourceAttr(mobileAppResourceAddr, "app_configuration.preferences"),
					mobileAppRetainedOnServer(t),
				),
			},
			{
				Config: mobileAppOmitRetainsGeneralOnlyConfig(suffix, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(mobileAppResourceAddr, "scope.targets.building_ids.#"),
					resource.TestCheckNoResourceAttr(mobileAppResourceAddr, "self_service.install_button_text"),
					mobileAppRetainedOnServer(t),
				),
			},
		},
	})
}
