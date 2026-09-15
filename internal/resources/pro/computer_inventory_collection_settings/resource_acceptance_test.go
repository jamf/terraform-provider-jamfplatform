// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package computer_inventory_collection_settings_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const resName = "jamfplatform_pro_computer_inventory_collection_settings.test"

// checkSingletonRecordStillExists verifies the settings record persists on the tenant
// after Terraform destroys the resource from state. The remote Delete is a no-op.
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "computer inventory collection settings", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetComputerInventoryCollectionSettingsV2(ctx)
	})
}

// TestAccResource_ProComputerInventoryCollectionSettings_Basic exercises a full Update
// round-trip: every preference toggle is flipped between step 1 and step 2, and the
// custom application search-path set is grown (add) then shrunk (remove) across the
// steps to drive the create/delete custom-path endpoints.
func TestAccResource_ProComputerInventoryCollectionSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				// Step 1: baseline — account collection on with both sub-options on.
				Config: `
					resource "jamfplatform_pro_computer_inventory_collection_settings" "test" {
						collect_local_user_accounts                      = true
						include_home_directory_sizes                     = true
						include_hidden_accounts                          = true
						collect_printers                                 = true
						collect_active_services                          = true
						collect_synced_mobile_device_backup_dates        = false
						collect_user_and_location_from_directory_service = true
						collect_package_receipts                         = true
						collect_available_software_updates               = false
						collect_unmanaged_certificates                   = true
						monitor_beacon_regions                           = false
						allow_jamf_binary_user_and_location_changes      = true
						collect_application_usage_information            = false
						use_unix_user_paths                              = true

						application_search_paths = ["/Library/AccTest1/"]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resName, "id", "singleton"),
					resource.TestCheckResourceAttr(resName, "collect_local_user_accounts", "true"),
					resource.TestCheckResourceAttr(resName, "include_home_directory_sizes", "true"),
					resource.TestCheckResourceAttr(resName, "include_hidden_accounts", "true"),
					resource.TestCheckResourceAttr(resName, "monitor_beacon_regions", "false"),
					resource.TestCheckResourceAttr(resName, "use_unix_user_paths", "true"),
					resource.TestCheckResourceAttr(resName, "application_search_paths.#", "1"),
					resource.TestCheckTypeSetElemAttr(resName, "application_search_paths.*", "/Library/AccTest1/"),
				),
			},
			{
				// Step 2: account collection still on, but flip its sub-options off and
				// flip every other toggle; GROW the path set (add a second path).
				Config: `
					resource "jamfplatform_pro_computer_inventory_collection_settings" "test" {
						collect_local_user_accounts                      = true
						include_home_directory_sizes                     = false
						include_hidden_accounts                          = false
						collect_printers                                 = false
						collect_active_services                          = false
						collect_synced_mobile_device_backup_dates        = true
						collect_user_and_location_from_directory_service = false
						collect_package_receipts                         = false
						collect_available_software_updates               = true
						collect_unmanaged_certificates                   = false
						monitor_beacon_regions                           = true
						allow_jamf_binary_user_and_location_changes      = false
						collect_application_usage_information            = true
						use_unix_user_paths                              = false

						application_search_paths = ["/Library/AccTest1/", "/Library/AccTest2/"]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resName, "collect_local_user_accounts", "true"),
					resource.TestCheckResourceAttr(resName, "include_home_directory_sizes", "false"),
					resource.TestCheckResourceAttr(resName, "include_hidden_accounts", "false"),
					resource.TestCheckResourceAttr(resName, "monitor_beacon_regions", "true"),
					resource.TestCheckResourceAttr(resName, "use_unix_user_paths", "false"),
					resource.TestCheckResourceAttr(resName, "application_search_paths.#", "2"),
					resource.TestCheckTypeSetElemAttr(resName, "application_search_paths.*", "/Library/AccTest2/"),
				),
			},
			{
				// Step 3: turn account collection OFF (sub-options must be off too) and
				// SHRINK the path set (remove the first path).
				Config: `
					resource "jamfplatform_pro_computer_inventory_collection_settings" "test" {
						collect_local_user_accounts                      = false
						include_home_directory_sizes                     = false
						include_hidden_accounts                          = false
						collect_printers                                 = false
						collect_active_services                          = false
						collect_synced_mobile_device_backup_dates        = true
						collect_user_and_location_from_directory_service = false
						collect_package_receipts                         = false
						collect_available_software_updates               = true
						collect_unmanaged_certificates                   = false
						monitor_beacon_regions                           = true
						allow_jamf_binary_user_and_location_changes      = false
						collect_application_usage_information            = true
						use_unix_user_paths                              = false

						application_search_paths = ["/Library/AccTest2/"]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resName, "application_search_paths.#", "1"),
					resource.TestCheckTypeSetElemAttr(resName, "application_search_paths.*", "/Library/AccTest2/"),
				),
			},
			{
				ResourceName:      resName,
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

// TestAccResource_ProComputerInventoryCollectionSettings_AccountSubOptionConflict
// verifies the ValidateConfig guard: a sub-option of account collection cannot be true
// while collect_local_user_accounts is false (the combination the admin UI greys out and
// the server refuses). Config validation must reject it before any apply.
func TestAccResource_ProComputerInventoryCollectionSettings_AccountSubOptionConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_computer_inventory_collection_settings" "test" {
						collect_local_user_accounts  = false
						include_home_directory_sizes = true
					}
				`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}

// TestAccResource_ProComputerInventoryCollectionSettings_RejectsNonSingletonImport
// verifies the ImportState guard: any identifier other than "singleton" must fail.
func TestAccResource_ProComputerInventoryCollectionSettings_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `resource "jamfplatform_pro_computer_inventory_collection_settings" "test" {}`,
			},
			{
				ResourceName:  resName,
				ImportState:   true,
				ImportStateId: "not-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}
