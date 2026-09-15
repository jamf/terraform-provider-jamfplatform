// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package self_service_plus_settings_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// checkSingletonRecordStillExists verifies the Jamf Pro Self Service Plus settings
// record persists on the tenant after Terraform destroys the resource from state.
// Canonical singleton acceptance check: the remote Delete is a no-op, so the API
// must still return the record (with whatever value was last applied) post-destroy.
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "Self Service Plus settings", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetSelfServicePlusSettingsV1(ctx)
	})
}

// TestAccResource_ProSelfServicePlusSettings_Basic toggles Self Service Plus on, then
// off, exercising both Update paths against a real tenant. Singleton resources have
// no remote Delete, so CheckDestroy is wired to verify the record PERSISTS on the
// tenant after Terraform stops managing it (the opposite of the standard pattern).
func TestAccResource_ProSelfServicePlusSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_self_service_plus_settings" "test" {
						enabled = true
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_self_service_plus_settings.test", "id", "singleton"),
					resource.TestCheckResourceAttr("jamfplatform_pro_self_service_plus_settings.test", "enabled", "true"),
				),
			},
			{
				Config: `
					resource "jamfplatform_pro_self_service_plus_settings" "test" {
						enabled = false
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_self_service_plus_settings.test", "enabled", "false"),
				),
			},
			{
				ResourceName:      "jamfplatform_pro_self_service_plus_settings.test",
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

// TestAccResource_ProSelfServicePlusSettings_RejectsNonSingletonImport verifies the
// ImportState guard added in resource.go: any identifier other than "singleton" must
// fail with a clear error rather than silently normalizing to the singleton ID.
func TestAccResource_ProSelfServicePlusSettings_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_self_service_plus_settings" "test" {
						enabled = false
					}
				`,
			},
			{
				ResourceName:  "jamfplatform_pro_self_service_plus_settings.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

func TestAccDataSource_ProSelfServicePlusSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_self_service_plus_settings" "src" {
						enabled = false
					}

					data "jamfplatform_pro_self_service_plus_settings" "lookup" {
						depends_on = [jamfplatform_pro_self_service_plus_settings.src]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_self_service_plus_settings.lookup", "id", "singleton"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_self_service_plus_settings.lookup", "enabled", "jamfplatform_pro_self_service_plus_settings.src", "enabled"),
				),
			},
		},
	})
}
