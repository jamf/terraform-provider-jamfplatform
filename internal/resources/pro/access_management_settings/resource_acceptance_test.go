// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package access_management_settings_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// adeTokenEnvVar supplies a base64-encoded `.p7m` ADE server token downloaded from Apple
// Business Manager / Apple School Manager (the same env var the ADE resource's own
// acceptance tests use). The Access Management setting references a real ADE server
// object's Server UUID, so the test creates a `jamfplatform_pro_automated_device_enrollment`
// instance from this token and points the setting at its `server_uuid`. Without the token
// the test skips. Never commit token material to fixtures.
const adeTokenEnvVar = "JAMFPLATFORM_ACC_PRO_DEP_TOKEN"

// adeFixture returns an ADE resource block created from the env-supplied token. Its
// `server_uuid` (the Apple-recorded MDM Server UUID) is what the Access Management setting
// consumes — not the ADE instance `id`.
func adeFixture(name, token string, woVersion int) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_automated_device_enrollment" "ade" {
			name                    = %q
			server_token            = %q
			server_token_wo_version = %d
		}
	`, name, token, woVersion)
}

// checkSingletonRecordStillExists verifies the Access Management settings record persists
// on the tenant after Terraform destroys the resources from state. The remote Delete is a
// no-op, so the API must still return the record post-destroy.
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "Access Management settings", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetEnrollmentAccessManagementV4(ctx)
	})
}

// TestAccResource_ProAccessManagementSettings_Basic creates an ADE server object from the
// env-supplied token, points the Access Management setting at its Server UUID, imports,
// exercises omit=preserve, then clears via "". Singleton resources have no remote Delete,
// so CheckDestroy verifies the record PERSISTS on the tenant.
//
// Step ordering matters: import runs while the field holds a real UUID. Importing after a
// clear-to-"" would fail ImportStateVerify — a server-side empty value is unrecoverably
// null on a fresh Read (the explicit-"" distinction is config-only, by design), so an
// imported null would diff against the prior "".
//
// The CLEAR step (final apply) is the live wire-probe for clear semantics: it assumes
// POSTing {"automatedDeviceEnrollmentServerUuid":""} clears the setting server-side. If
// the server instead preserves/no-ops on empty, GET echoes the old UUID and this step
// fails with "inconsistent result after apply" — that is an expected probe outcome to
// adjust for, not a broken build (the clear/empty mapping itself is unit-tested).
//
// NOTE: this mutates the tenant's live Access Management configuration and clears it on
// destroy — re-apply your real value afterwards if the tenant is in use.
func TestAccResource_ProAccessManagementSettings_Basic(t *testing.T) {
	token := testhelpers.AccEnv(adeTokenEnvVar)
	if token == "" {
		t.Skipf("%s not set; skipping Access Management settings acceptance test", adeTokenEnvVar)
	}
	testhelpers.AccPreCheck(t)

	suffix := testhelpers.RunSuffix()
	adeName := "tf-acc-pro-ade-accessmgmt-" + suffix

	const addr = "jamfplatform_pro_access_management_settings.test"
	const adeAddr = "jamfplatform_pro_automated_device_enrollment.ade"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				// Set: point the setting at the ADE instance's Server UUID.
				Config: adeFixture(adeName, token, 1) + `
					resource "jamfplatform_pro_access_management_settings" "test" {
						automated_device_enrollment_server_uuid = jamfplatform_pro_automated_device_enrollment.ade.server_uuid
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "singleton"),
					resource.TestCheckResourceAttrSet(addr, "automated_device_enrollment_server_uuid"),
					resource.TestCheckResourceAttrPair(addr, "automated_device_enrollment_server_uuid", adeAddr, "server_uuid"),
				),
			},
			{
				// Import while the field holds a real UUID — round-trips cleanly.
				ResourceName:      addr,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
				},
			},
			{
				// Omit = preserve: drop the attribute, keep the ADE resource. UseStateForUnknown
				// must carry the prior Server UUID forward unchanged.
				Config: adeFixture(adeName, token, 1) + `
					resource "jamfplatform_pro_access_management_settings" "test" {}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(addr, "automated_device_enrollment_server_uuid", adeAddr, "server_uuid"),
				),
			},
			{
				// Clear via explicit "" — live clear-semantics wire-probe (see doc comment).
				Config: adeFixture(adeName, token, 1) + `
					resource "jamfplatform_pro_access_management_settings" "test" {
						automated_device_enrollment_server_uuid = ""
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "automated_device_enrollment_server_uuid", ""),
				),
			},
		},
	})
}

// TestAccResource_ProAccessManagementSettings_RejectsNonSingletonImport verifies the
// ImportState guard rejects any identifier other than "singleton".
func TestAccResource_ProAccessManagementSettings_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_access_management_settings" "test" {}
				`,
			},
			{
				ResourceName:  "jamfplatform_pro_access_management_settings.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccDataSource_ProAccessManagementSettings_Basic reads the setting back through the
// data source after the resource points it at the ADE instance's Server UUID.
func TestAccDataSource_ProAccessManagementSettings_Basic(t *testing.T) {
	token := testhelpers.AccEnv(adeTokenEnvVar)
	if token == "" {
		t.Skipf("%s not set; skipping Access Management settings data source test", adeTokenEnvVar)
	}
	testhelpers.AccPreCheck(t)

	suffix := testhelpers.RunSuffix()
	adeName := "tf-acc-pro-ade-accessmgmt-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: adeFixture(adeName, token, 1) + `
					resource "jamfplatform_pro_access_management_settings" "src" {
						automated_device_enrollment_server_uuid = jamfplatform_pro_automated_device_enrollment.ade.server_uuid
					}

					data "jamfplatform_pro_access_management_settings" "lookup" {
						depends_on = [jamfplatform_pro_access_management_settings.src]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_access_management_settings.lookup", "id", "singleton"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_access_management_settings.lookup", "automated_device_enrollment_server_uuid", "jamfplatform_pro_access_management_settings.src", "automated_device_enrollment_server_uuid"),
				),
			},
		},
	})
}
