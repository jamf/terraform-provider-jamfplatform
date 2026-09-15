// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package service_discovery_enrollment_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const addr = "jamfplatform_pro_service_discovery_enrollment.test"
const adeAddr = "jamfplatform_pro_automated_device_enrollment.ade"

// adeTokenEnvVar supplies a base64-encoded `.p7m` ADE server token downloaded from Apple
// Business Manager / Apple School Manager (the same env var the ADE resource's own
// acceptance tests use). Service discovery well-known settings are keyed by a synced AxM
// org's Server UUID, so the test creates a jamfplatform_pro_automated_device_enrollment
// instance from this token and manages its server_uuid. Without the token the test skips.
// Never commit token material to fixtures.
const adeTokenEnvVar = "JAMFPLATFORM_ACC_PRO_DEP_TOKEN"

// adeFixture returns an ADE resource block created from the env-supplied token. Its
// server_uuid (the Apple-recorded MDM Server UUID) is the key the well-known setting rows
// reference.
func adeFixture(name, token string, woVersion int) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_automated_device_enrollment" "ade" {
			name                    = %q
			server_token            = %q
			server_token_wo_version = %d
		}
	`, name, token, woVersion)
}

// serviceDiscoveryConfig points one well_known_setting row at the ADE fixture's
// server_uuid, at the given enrollment type.
func serviceDiscoveryConfig(adeName, token string, enrollmentType string) string {
	return adeFixture(adeName, token, 1) + fmt.Sprintf(`
		resource "jamfplatform_pro_service_discovery_enrollment" "test" {
			well_known_setting = [
				{
					server_uuid     = jamfplatform_pro_automated_device_enrollment.ade.server_uuid
					enrollment_type = %q
				},
			]
		}
	`, enrollmentType)
}

// checkSingletonRecordStillExists verifies the well-known settings record persists on the
// tenant after Terraform destroys the resources from state (singleton — no remote delete).
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "service discovery well-known settings", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetServiceDiscoveryEnrollmentWellKnownSettingsV1(ctx)
	})
}

// TestAccResource_ProServiceDiscoveryEnrollment_Basic creates an ADE server object from
// the env-supplied token, manages its Server UUID's service-discovery enrollment type
// across mdm-byod → mdm-adde → none, imports, and verifies the Computed org_name echo is
// populated from the post-write GET. Singleton resources have no remote Delete, so
// CheckDestroy verifies the record PERSISTS on the tenant.
//
// Two probe notes (adjust-on-run, not broken build — mirrors access_management_settings):
//
//   - org_name is populated only once the new ADE token's AxM org is reflected in the
//     well-known GET. If the org sync is not immediate, the provider emits a "server_uuid
//     not recognized" warning and org_name is null — re-run after the sync settles.
//   - The import step round-trips cleanly only when the managed rows equal ALL synced AxM
//     orgs on the tenant: import adopts every row the GET returns. On a tenant with other
//     pre-synced orgs beyond this fixture, ImportStateVerify will diff (more rows imported
//     than managed) — expected for a subset-by-key resource, not a regression.
//
// NOTE: this mutates the tenant's live service-discovery configuration. The final managed
// state before destroy is "none"; re-apply your real values afterward if the tenant is in
// use.
func TestAccResource_ProServiceDiscoveryEnrollment_Basic(t *testing.T) {
	token := testhelpers.AccEnv(adeTokenEnvVar)
	if token == "" {
		t.Skipf("%s not set; skipping service discovery enrollment acceptance test", adeTokenEnvVar)
	}
	testhelpers.AccPreCheck(t)

	suffix := testhelpers.RunSuffix()
	adeName := "tf-acc-pro-ade-svcdisc-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: serviceDiscoveryConfig(adeName, token, "mdm-byod"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "singleton"),
					resource.TestCheckResourceAttr(addr, "well_known_setting.#", "1"),
					resource.TestCheckResourceAttr(addr, "well_known_setting.0.enrollment_type", "mdm-byod"),
					resource.TestCheckResourceAttrPair(addr, "well_known_setting.0.server_uuid", adeAddr, "server_uuid"),
					// org_name is the server echo, populated from the GET-after-write.
					resource.TestCheckResourceAttrSet(addr, "well_known_setting.0.org_name"),
				),
			},
			{
				Config: serviceDiscoveryConfig(adeName, token, "mdm-adde"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "well_known_setting.0.enrollment_type", "mdm-adde"),
					resource.TestCheckResourceAttrSet(addr, "well_known_setting.0.org_name"),
				),
			},
			{
				// Import while a real Server UUID is managed. Strict ImportStateVerify is
				// NOT usable here: import adopts EVERY synced-org row the GET returns (a
				// superset of the managed rows), so on any tenant with more than one synced
				// AxM org the imported list has more entries than the config manages — an
				// expected consequence of the subset-by-key model, not a regression. Instead
				// ImportStateCheck verifies the import succeeds, the singleton id is set, and
				// the managed enrollment type (mdm-adde) round-trips into the imported state.
				ResourceName:  addr,
				ImportState:   true,
				ImportStateId: "singleton",
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported state, got %d", len(states))
					}
					s := states[0]
					if s.Attributes["id"] != "singleton" {
						return fmt.Errorf("imported id = %q, want singleton", s.Attributes["id"])
					}
					n, err := strconv.Atoi(s.Attributes["well_known_setting.#"])
					if err != nil || n < 1 {
						return fmt.Errorf("imported state has no well_known_setting rows (#=%q)", s.Attributes["well_known_setting.#"])
					}
					for i := range n {
						if s.Attributes[fmt.Sprintf("well_known_setting.%d.enrollment_type", i)] == "mdm-adde" {
							return nil
						}
					}
					return fmt.Errorf("imported state missing the managed mdm-adde row: %v", s.Attributes)
				},
			},
			{
				// Disable via "none" (the supported way to turn off hosting — not removal).
				Config: serviceDiscoveryConfig(adeName, token, "none"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "well_known_setting.0.enrollment_type", "none"),
				),
			},
		},
	})
}

// TestAccResource_ProServiceDiscoveryEnrollment_RejectsInvalidEnrollmentType verifies the
// enrollment_type OneOf validator. Plan-time validation fails before any API call, so this
// needs neither a token nor a real server_uuid.
func TestAccResource_ProServiceDiscoveryEnrollment_RejectsInvalidEnrollmentType(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_service_discovery_enrollment" "test" {
						well_known_setting = [
							{ server_uuid = "00000000000000000000000000000000", enrollment_type = "not-a-type" },
						]
					}
				`,
				// Match the OneOf summary (a short, single line) — the detail wraps at
				// ~80 cols around "value must be one of", per the ExpectError line-wrap lesson.
				ExpectError: regexp.MustCompile(`Invalid Attribute Value Match`),
			},
		},
	})
}

// TestAccResource_ProServiceDiscoveryEnrollment_RejectsNonSingletonImport verifies the
// ImportState guard rejects any id other than "singleton".
func TestAccResource_ProServiceDiscoveryEnrollment_RejectsNonSingletonImport(t *testing.T) {
	token := testhelpers.AccEnv(adeTokenEnvVar)
	if token == "" {
		t.Skipf("%s not set; skipping service discovery enrollment acceptance test", adeTokenEnvVar)
	}
	testhelpers.AccPreCheck(t)

	suffix := testhelpers.RunSuffix()
	adeName := "tf-acc-pro-ade-svcdisc-imp-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: serviceDiscoveryConfig(adeName, token, "none"),
			},
			{
				ResourceName:  addr,
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccDataSource_ProServiceDiscoveryEnrollment_Basic verifies the data source surfaces
// the managed org row, including the Computed org_name echo.
func TestAccDataSource_ProServiceDiscoveryEnrollment_Basic(t *testing.T) {
	token := testhelpers.AccEnv(adeTokenEnvVar)
	if token == "" {
		t.Skipf("%s not set; skipping service discovery enrollment acceptance test", adeTokenEnvVar)
	}
	testhelpers.AccPreCheck(t)

	suffix := testhelpers.RunSuffix()
	adeName := "tf-acc-pro-ade-svcdisc-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: serviceDiscoveryConfig(adeName, token, "none") + `
					data "jamfplatform_pro_service_discovery_enrollment" "lookup" {
						depends_on = [jamfplatform_pro_service_discovery_enrollment.test]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_service_discovery_enrollment.lookup", "id", "singleton"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_service_discovery_enrollment.lookup", "well_known_setting.#"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_service_discovery_enrollment.lookup", "well_known_setting.0.org_name"),
				),
			},
		},
	})
}
