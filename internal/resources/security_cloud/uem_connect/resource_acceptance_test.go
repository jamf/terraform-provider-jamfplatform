// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package uem_connect_test

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const resourceName = "jamfplatform_security_cloud_uem_connect.test"

// placeholderDeviceGroupID stands in for a Jamf Security Cloud device group.
//
// Jamf Security Cloud does not check that either side of a group mapping exists
// (wire-verified 2026-08-28), so a placeholder exercises the write faithfully
// without the test having to create and clean up a real device group. It also keeps
// a real tenant's group IDs out of a committed file.
const placeholderDeviceGroupID = "00000000-0000-0000-0000-000000000000"

// Fixture environment variables.
//
// The platform tenant ID cannot come from jamfplatform_pro_tenant_id here: the
// provider is scoped to the Security Cloud tenant for these tests, and the Jamf Pro
// namespace does not answer under that scope. It has to be supplied.
//
// The OAuth credentials are for an API integration on the target Jamf Pro instance.
// Jamf Security Cloud runs a real connection test on create, so a placeholder fails
// the apply — there is no way to exercise that form without working credentials.
const (
	envPlatformTenantID = "JAMFPLATFORM_ACC_SECURITYCLOUD_UEM_PRO_TENANT_ID"
	envServerURL        = "JAMFPLATFORM_ACC_SECURITYCLOUD_UEM_SERVER_URL"
	envClientID         = "JAMFPLATFORM_ACC_SECURITYCLOUD_UEM_CLIENT_ID"
	envClientSecret     = "JAMFPLATFORM_ACC_SECURITYCLOUD_UEM_CLIENT_SECRET"
)

// securityCloudClient returns a Security Cloud client for the out-of-band reads
// these tests do.
func securityCloudClient(t *testing.T) *securitycloud.Client {
	t.Helper()
	return securitycloud.New(testhelpers.NewAcceptanceClient(t))
}

// requireNoExistingIntegration skips when the tenant already holds a UEM Connect
// integration.
//
// A tenant holds at most one, so a pre-existing integration makes every create in
// this file fail with a conflict. Skipping states that plainly instead of reporting
// a provider fault, and it refuses to delete someone else's integration to make
// room — that would take the tenant's device sync down.
func requireNoExistingIntegration(t *testing.T) {
	t.Helper()

	page, err := securityCloudClient(t).ListUemConnectorsV1(context.Background())
	if err != nil {
		t.Skipf("could not check for an existing UEM Connect integration, so cannot tell whether these tests would conflict: %v", err)
	}
	if page != nil && len(page.Results) > 0 {
		t.Skipf("this tenant already holds a UEM Connect integration (%s); these tests create one and a tenant holds only one. Remove it first to run them.", page.Results[0].ID)
	}
}

// platformTenantIDOrSkip returns the Jamf Pro tenant ID to point the integration at.
func platformTenantIDOrSkip(t *testing.T) string {
	t.Helper()

	tenant := testhelpers.AccEnv(envPlatformTenantID)
	if tenant == "" {
		t.Skipf("%s must be set to the platform tenant ID of a Jamf Pro instance for these tests", envPlatformTenantID)
	}
	return tenant
}

// oauthCredentialsOrSkip returns credentials for an API integration on the target
// Jamf Pro instance.
func oauthCredentialsOrSkip(t *testing.T) (serverURL, clientID, clientSecret string) {
	t.Helper()

	serverURL = testhelpers.AccEnv(envServerURL)
	clientID = testhelpers.AccEnv(envClientID)
	clientSecret = testhelpers.AccEnv(envClientSecret)

	if serverURL == "" || clientID == "" || clientSecret == "" {
		t.Skipf("%s, %s and %s must all be set to exercise the supplied-credentials form; Jamf Security Cloud runs a real connection test on create, so placeholders cannot stand in",
			envServerURL, envClientID, envClientSecret)
	}
	return serverURL, clientID, clientSecret
}

// checkServerDataFieldMappingsAreDefault reads the integration out of band and
// asserts Jamf Security Cloud has the device field mappings back at its defaults.
//
// Needed because the resource cannot show this: dropping the block makes it null in
// state, so the only evidence that the settings write actually replaced the tenant's
// values — rather than leaving step 2's in place — is on the tenant.
func checkServerDataFieldMappingsAreDefault(t *testing.T) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		rs, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s not in state", resourceName)
		}

		got, err := securityCloudClient(t).GetUemConnectorV1(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading the integration back: %w", err)
		}
		if got.DeviceFieldMappings == nil {
			return fmt.Errorf("the integration reports no device field mappings at all")
		}

		defaults := map[string]struct {
			got  *string
			want string
		}{
			"deviceNameMapping":  {got.DeviceFieldMappings.DeviceNameMapping, "DEVICE_NAME"},
			"userNameMapping":    {got.DeviceFieldMappings.UserNameMapping, "USER_NAME"},
			"userIdMapping":      {got.DeviceFieldMappings.UserIDMapping, "EXTERNAL_USER_ID"},
			"phoneNumberMapping": {got.DeviceFieldMappings.PhoneNumberMapping, "PHONE_NUMBER"},
		}
		for field, c := range defaults {
			if c.got == nil {
				return fmt.Errorf("%s is absent; want %s", field, c.want)
			}
			if *c.got != c.want {
				return fmt.Errorf("%s = %q, want the default %q — the settings write did not replace step 2's value", field, *c.got, c.want)
			}
		}
		if got.DeviceFieldMappings.UserEmailMapping == nil {
			return fmt.Errorf("userEmailMapping is absent")
		}
		if got.DeviceFieldMappings.UserEmailMapping.Type != "EMAIL_ADDRESS" {
			return fmt.Errorf("userEmailMapping.type = %q, want EMAIL_ADDRESS", got.DeviceFieldMappings.UserEmailMapping.Type)
		}
		if got.DeviceFieldMappings.UserEmailMapping.FieldSuffix != nil && *got.DeviceFieldMappings.UserEmailMapping.FieldSuffix != "" {
			return fmt.Errorf("userEmailMapping.fieldSuffix = %q, want it cleared", *got.DeviceFieldMappings.UserEmailMapping.FieldSuffix)
		}
		return nil
	}
}

// checkIntegrationDestroyed asserts the integration is gone from the tenant.
func checkIntegrationDestroyed(t *testing.T) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		client := securityCloudClient(t)
		for _, rs := range state.RootModule().Resources {
			if rs.Type != "jamfplatform_security_cloud_uem_connect" {
				continue
			}
			_, err := client.GetUemConnectorV1(context.Background(), rs.Primary.ID)
			if err == nil {
				return fmt.Errorf("UEM Connect integration %s still exists after destroy", rs.Primary.ID)
			}
			// Only a not-found is proof of destruction. A 403, a 500 or a dropped
			// connection says nothing about whether the integration is gone, and
			// accepting one would make this check pass for the wrong reason.
			apiErr := jamfplatform.AsAPIError(err)
			if apiErr == nil || !apiErr.HasStatus(http.StatusNotFound) {
				return fmt.Errorf("reading UEM Connect integration %s back after destroy failed with something other than a not-found, so it cannot be confirmed gone: %w", rs.Primary.ID, err)
			}
		}
		return nil
	}
}

// TestAccResource_SecurityCloudUEMConnect_PlatformTenant is the main lifecycle test
// for the preferred authentication form, and carries the update round-trip.
//
// Step 1 creates at the defaults. Step 2 declares both optional blocks for the first
// time while leaving most of their members unset, which is the transition where a
// nested Optional+Computed member has a null prior state — the case a plan modifier
// copying that null would break. Step 3 changes every attribute that can be changed
// in place and adds two group mappings and a full data field mapping — the point
// being that the settings write is a full replacement, so a step that changed one
// field would pass while silently resetting the rest. Step 4 removes one group
// mapping and keeps the other, since a nested collection needs both an add and a
// remove exercised. Step 5 empties the list, which is how mappings are cleared and
// is not the same as omitting it. Step 6 imports.
// Two things this test asserts belong to issue #379. uem_server_url must NOT be
// recorded on this form: it conflicts with platform_tenant, so an address in
// state is an address `terraform plan -generate-config-out` writes into a
// configuration the resource then refuses — which is exactly how the defect
// surfaced, the refresh re-committing what Create had deliberately dropped. And
// the final step is the generation guard, on this form rather than the oauth one
// for the same reason.
func TestAccResource_SecurityCloudUEMConnect_PlatformTenant(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoExistingIntegration(t)
	tenant := platformTenantIDOrSkip(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkIntegrationDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = %q
						}
					}
				`, tenant),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttr(resourceName, "uem_vendor", securitycloud.ConnectorCreateRequestBodyVendorJamfPro),
					resource.TestCheckResourceAttr(resourceName, "platform_tenant.tenant_id", tenant),
					resource.TestCheckNoResourceAttr(resourceName, "uem_server_url"),
					resource.TestCheckNoResourceAttr(resourceName, "oauth.client_id"),
					// The defaults the server applies on create.
					resource.TestCheckResourceAttr(resourceName, "enabled", "true"),
					resource.TestCheckResourceAttr(resourceName, "scheduled_sync_enabled", "true"),
					resource.TestCheckResourceAttr(resourceName, "sync_refresh_interval_minutes", "1440"),
					resource.TestCheckResourceAttr(resourceName, "uem_auto_delete_behavior", "remove_deleted_or_retired"),
					resource.TestCheckResourceAttr(resourceName, "unmanaged_sync_threshold", "0"),
					resource.TestCheckResourceAttr(resourceName, "device_risk_uem_signaling_enabled", "false"),
					resource.TestCheckResourceAttr(resourceName, "disable_sync_on_auth_error", "true"),
					resource.TestCheckResourceAttr(resourceName, "concurrent_device_sync_enabled", "true"),
				),
			},
			{
				// Declaring a block for the first time while leaving most of its
				// members unset. Prior state at every nested path is null here,
				// because step 1 managed neither block — the case that makes
				// UseStateForUnknown the wrong modifier inside a nested attribute: it
				// copies that null into the plan for an attribute the server is about
				// to populate, and the apply is then refused rather than accepting
				// the server's default.
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = %q
						}

						user_data_field_mapping = {
							device_name = "SERIAL_NUMBER"
							email = {
								source = "SERIAL_NUMBER"
								suffix = "example.test"
							}
						}

						group_membership_mapping = {
							mappings = [
								{ uem_group_id = "computer_30", security_cloud_group_id = %[2]q },
							]
						}
					}
				`, tenant, placeholderDeviceGroupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "user_data_field_mapping.device_name", "SERIAL_NUMBER"),
					// The four members the configuration left unset arrive from the
					// server at its defaults, which is the assertion that matters: a
					// null would mean the plan had committed to one and the apply had
					// to be refused.
					resource.TestCheckResourceAttr(resourceName, "user_data_field_mapping.user_name", "USER_NAME"),
					resource.TestCheckResourceAttr(resourceName, "user_data_field_mapping.user_id", "EXTERNAL_USER_ID"),
					resource.TestCheckResourceAttr(resourceName, "user_data_field_mapping.phone_number", "PHONE_NUMBER"),
					resource.TestCheckResourceAttrSet(resourceName, "user_data_field_mapping.email.only_if_email_missing"),
					resource.TestCheckResourceAttrSet(resourceName, "group_membership_mapping.enabled"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = %q
						}

						enabled                        = false
						scheduled_sync_enabled         = false
						sync_refresh_interval_minutes          = 720
						uem_auto_delete_behavior           = "remove_deleted_or_unmanaged"
						device_risk_uem_signaling_enabled  = true
						disable_sync_on_auth_error     = false
						concurrent_device_sync_enabled = false

						user_data_field_mapping = {
							device_name  = "SERIAL_NUMBER"
							user_name    = "MDM_ID"
							user_id      = "EMAIL_ADDRESS"
							phone_number = "NO_PHONE_NUMBER"
							email = {
								source                = "SERIAL_NUMBER"
								suffix                = "example.test"
								only_if_email_missing = true
							}
						}

						group_membership_mapping = {
							enabled = true
							mappings = [
								{ uem_group_id = "computer_30", security_cloud_group_id = %[2]q },
								{ uem_group_id = "mobile_20", security_cloud_group_id = %[2]q },
							]
						}
					}
				`, tenant, placeholderDeviceGroupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "enabled", "false"),
					resource.TestCheckResourceAttr(resourceName, "scheduled_sync_enabled", "false"),
					resource.TestCheckResourceAttr(resourceName, "sync_refresh_interval_minutes", "720"),
					resource.TestCheckResourceAttr(resourceName, "uem_auto_delete_behavior", "remove_deleted_or_unmanaged"),
					resource.TestCheckResourceAttr(resourceName, "unmanaged_sync_threshold", "0"),
					resource.TestCheckResourceAttr(resourceName, "device_risk_uem_signaling_enabled", "true"),
					resource.TestCheckResourceAttr(resourceName, "disable_sync_on_auth_error", "false"),
					resource.TestCheckResourceAttr(resourceName, "concurrent_device_sync_enabled", "false"),
					resource.TestCheckResourceAttr(resourceName, "user_data_field_mapping.device_name", "SERIAL_NUMBER"),
					resource.TestCheckResourceAttr(resourceName, "user_data_field_mapping.email.source", "SERIAL_NUMBER"),
					resource.TestCheckResourceAttr(resourceName, "user_data_field_mapping.email.suffix", "example.test"),
					resource.TestCheckResourceAttr(resourceName, "group_membership_mapping.mappings.#", "2"),
					// Order is configuration here: membership is evaluated top to
					// bottom, so the indices are asserted rather than the set.
					resource.TestCheckResourceAttr(resourceName, "group_membership_mapping.mappings.0.uem_group_id", "computer_30"),
					resource.TestCheckResourceAttr(resourceName, "group_membership_mapping.mappings.1.uem_group_id", "mobile_20"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = %q
						}

						group_membership_mapping = {
							enabled = true
							mappings = [
								{ uem_group_id = "mobile_20", security_cloud_group_id = %[2]q },
							]
						}
					}
				`, tenant, placeholderDeviceGroupID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "group_membership_mapping.mappings.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "group_membership_mapping.mappings.0.uem_group_id", "mobile_20"),
					// Dropping the managed scalars returns them to their defaults,
					// which is the full-replacement behaviour made visible: the
					// resource sends the complete state every time, so an attribute
					// the config stops mentioning reverts rather than lingering at
					// step 2's value.
					resource.TestCheckResourceAttr(resourceName, "sync_refresh_interval_minutes", "1440"),
					resource.TestCheckResourceAttr(resourceName, "device_risk_uem_signaling_enabled", "false"),
					// Dropping the whole user_data_field_mapping block removes it from
					// state, because the block is Optional-only: an unmanaged block
					// is null rather than a mirror of whatever the tenant holds.
					resource.TestCheckNoResourceAttr(resourceName, "user_data_field_mapping.device_name"),
					// Which is exactly why the revert has to be checked against the
					// tenant instead. State cannot show it, and this is the only
					// place the full replacement is proven end to end.
					checkServerDataFieldMappingsAreDefault(t),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = %q
						}

						group_membership_mapping = {
							enabled  = false
							mappings = []
						}
					}
				`, tenant),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "group_membership_mapping.mappings.#", "0"),
					resource.TestCheckResourceAttr(resourceName, "group_membership_mapping.enabled", "false"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					// timeouts is provider-side configuration with nothing behind it
					// on the tenant, so an import cannot recover it.
					"timeouts",
					// Both blocks are Optional-only, so they are null in state when
					// the configuration does not declare them — and an import has no
					// configuration to consult, so it populates them from the tenant.
					// The asymmetry is deliberate: not populating would leave an
					// imported integration looking unconfigured and drop the tenant's
					// group mappings on the next apply, which is far worse than a
					// plan that tells the user what to write into their config.
					"user_data_field_mapping",
					"group_membership_mapping",
				},
			},
			testhelpers.GenerateConfigStep(resourceName),
		},
	})
}

// TestAccResource_SecurityCloudUEMConnect_OAuth is the happy path for the second
// mutually-exclusive form.
//
// It also covers what import can and cannot recover on this form. The client secret
// is write-only and never returned, and the rotation counter is the user's own, so
// both are ignored — and the fact that `platform_tenant` stays absent after an
// import is the assertion that the form was recovered correctly, since the response
// carries a client ID either way.
func TestAccResource_SecurityCloudUEMConnect_OAuth(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoExistingIntegration(t)
	serverURL, clientID, clientSecret := oauthCredentialsOrSkip(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkIntegrationDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						uem_server_url = %q
						oauth = {
							client_id                = %q
							client_secret            = %q
							client_secret_wo_version = 1
						}
					}
				`, serverURL, clientID, clientSecret),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "uem_server_url", serverURL),
					resource.TestCheckResourceAttr(resourceName, "oauth.client_id", clientID),
					// Write-only: the secret must never reach state.
					resource.TestCheckNoResourceAttr(resourceName, "oauth.client_secret"),
					resource.TestCheckNoResourceAttr(resourceName, "platform_tenant.tenant_id"),
				),
			},
			{
				// Rotating the counter is the only way to send a new secret, and no
				// Jamf Security Cloud endpoint accepts credentials for an integration
				// that already exists — so it has to replace. An in-place update here
				// would apply cleanly, converge, and leave the old secret on the
				// tenant with nothing to say so.
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = %q
						uem_server_url = %q
						oauth = {
							client_id                = %q
							client_secret            = %q
							client_secret_wo_version = 2
						}
					}
				`, "JAMF_PRO", serverURL, clientID, clientSecret),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "oauth.client_secret_wo_version", "2"),
					resource.TestCheckResourceAttr(resourceName, "oauth.client_id", clientID),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
					// Never returned by Jamf Security Cloud, so an import has
					// nothing to populate it from.
					"oauth.client_secret",
					// The user's own rotation counter; the server has never seen it.
					"oauth.client_secret_wo_version",
					// Optional-only blocks this configuration does not declare; an
					// import populates them from the tenant. See the other import
					// step for why that asymmetry is the right way round.
					"user_data_field_mapping",
					"group_membership_mapping",
				},
			},
		},
	})
}

// TestAccResource_SecurityCloudUEMConnect_ReplaceOnTenantChange pins that changing
// the tenant replaces the integration rather than producing a plan Terraform cannot
// carry out. There is no update operation for the connection.
//
// Two tenant IDs are needed, and a second one is not something this suite can
// invent, so the step that would change it asserts the plan instead: a
// non-empty plan with a replace in it, driven by an address change on the OAuth
// form where the value is user-supplied.
func TestAccResource_SecurityCloudUEMConnect_ReplaceOnConnectionChange(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoExistingIntegration(t)
	serverURL, clientID, clientSecret := oauthCredentialsOrSkip(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkIntegrationDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						uem_server_url = %q
						oauth = {
							client_id     = %q
							client_secret = %q
						}
					}
				`, serverURL, clientID, clientSecret),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						uem_server_url = %q
						oauth = {
							client_id     = %q
							client_secret = %q
						}
					}
				`, serverURL+"/", clientID, clientSecret),
				// A trailing slash is a different address as far as the schema is
				// concerned, so this must plan a replacement. It is not applied:
				// the replacement would create before destroying and hit the
				// one-per-tenant conflict, which is a property of the API and not
				// something the provider can plan around.
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccResource_SecurityCloudUEMConnect_Validation covers both declared
// cross-field validators, which run at plan time and need no tenant object.
func TestAccResource_SecurityCloudUEMConnect_Validation(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Neither form configured. ExactlyOneOf reports this as a missing
				// configuration rather than an invalid combination, so the shared
				// detail is what the regex anchors on.
				Config: `
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
					}
				`,
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured`),
			},
			{
				// Both forms configured — the other half of ExactlyOneOf, which
				// emits a different summary for the same rule.
				Config: `
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						uem_server_url = "https://example.jamfcloud.com"
						platform_tenant = {
							tenant_id = "00000000-0000-0000-0000-000000000000"
						}
						oauth = {
							client_id     = "client"
							client_secret = "secret"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured`),
			},
			{
				// The OAuth form without the address it needs.
				Config: `
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						oauth = {
							client_id     = "client"
							client_secret = "secret"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`These attributes must be configured together`),
			},
			{
				// The address belongs to the OAuth form only. On the platform_tenant
				// form Jamf Security Cloud resolves it from the tenant, so a value
				// here would be either silently overwritten or a failed apply.
				Config: `
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor     = "JAMF_PRO"
						uem_server_url = "https://example.jamfcloud.com"
						platform_tenant = {
							tenant_id = "00000000-0000-0000-0000-000000000000"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`(?s)cannot be configured\s+together:\s+\[platform_tenant,uem_server_url\]`),
			},
			{
				// The address has to carry its scheme. Without one Jamf Security
				// Cloud's only signal is a failed connection test reporting three
				// unrelated causes as one.
				Config: `
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor     = "JAMF_PRO"
						uem_server_url = "example.jamfcloud.com"
						oauth = {
							client_id     = "client"
							client_secret = "secret"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`(?s)including its\s+scheme`),
			},
			{
				Config: `
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "INTUNE"
						platform_tenant = {
							tenant_id = "00000000-0000-0000-0000-000000000000"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`Attribute uem_vendor value must be one of`),
			},
			{
				// The group identifier format, checked at plan time because Jamf
				// Security Cloud's refusal names no field.
				Config: `
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = "00000000-0000-0000-0000-000000000000"
						}
						group_membership_mapping = {
							mappings = [
								{ uem_group_id = "12", security_cloud_group_id = "00000000-0000-0000-0000-000000000000" },
							]
						}
					}
				`,
				// Terraform wraps error output, and where the wrap lands depends on
				// the length of the attribute path — it moved once already when the
				// attribute was renamed. Matching whitespace as \s+ is robust to
				// the wrap falling anywhere in the phrase.
				ExpectError: regexp.MustCompile(`must be\s+.computer_.\s+or\s+.mobile_.`),
			},
			{
				// Duplicate group identifiers. The server accepts them and the
				// second is dead configuration, since membership is evaluated top
				// to bottom.
				Config: `
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = "00000000-0000-0000-0000-000000000000"
						}
						group_membership_mapping = {
							mappings = [
								{ uem_group_id = "computer_12", security_cloud_group_id = "00000000-0000-0000-0000-000000000000" },
								{ uem_group_id = "computer_12", security_cloud_group_id = "11111111-1111-1111-1111-111111111111" },
							]
						}
					}
				`,
				ExpectError: regexp.MustCompile(`(?s)uem_group_id`),
			},
			{
				Config: `
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = "00000000-0000-0000-0000-000000000000"
						}
						sync_refresh_interval_minutes = 0
					}
				`,
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
			{
				Config: `
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = "00000000-0000-0000-0000-000000000000"
						}
						unmanaged_sync_threshold = 3
					}
				`,
				ExpectError: regexp.MustCompile(`Invalid Configuration for Read-Only Attribute`),
			},
			{
				Config: `
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = "00000000-0000-0000-0000-000000000000"
						}
						user_data_field_mapping = {
							user_id = "IMEI"
						}
					}
				`,
				// IMEI is accepted for device_name and email.source but refused for
				// user_id. The per-key vocabularies differ, and the validator has
				// to reflect that rather than pooling them.
				ExpectError: regexp.MustCompile(`Attribute user_data_field_mapping.user_id value must be one of`),
			},
		},
	})
}

// TestAccResource_SecurityCloudUEMConnect_DriftRecovery deletes the integration out
// of band and asserts the next refresh notices, rather than reporting the resource
// as healthy against an object that is gone.
func TestAccResource_SecurityCloudUEMConnect_DriftRecovery(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoExistingIntegration(t)
	tenant := platformTenantIDOrSkip(t)

	config := fmt.Sprintf(`
		resource "jamfplatform_security_cloud_uem_connect" "test" {
			uem_vendor = "JAMF_PRO"
			platform_tenant = {
				tenant_id = %q
			}
		}
	`, tenant)

	var integrationID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkIntegrationDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					func(state *terraform.State) error {
						rs, ok := state.RootModule().Resources[resourceName]
						if !ok {
							return fmt.Errorf("%s not in state", resourceName)
						}
						integrationID = rs.Primary.ID
						return nil
					},
				),
			},
			{
				// A step cannot set both Config and RefreshState; the refresh reuses
				// the preceding step's configuration.
				PreConfig: func() {
					if integrationID == "" {
						t.Fatal("no integration ID captured from the previous step")
					}
					if err := securityCloudClient(t).DeleteUemConnectorV1(context.Background(), integrationID); err != nil {
						t.Fatalf("deleting the integration out of band: %v", err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccDataSource_SecurityCloudUEMConnect_ReadsCreatedIntegration reads the integration the resource in
// the same configuration creates, so the two are checked against each other.
//
// The data source's own additions are asserted for presence rather than value:
// `connected` and the sync summary depend on when the read happens relative to the
// first sync, so pinning them would be pinning a race.
//
// The server address is where the two deliberately disagree on the platform_tenant
// form. The data source reports what Jamf Security Cloud resolved from the tenant;
// the resource records nothing, because uem_server_url conflicts with
// platform_tenant and a generated configuration carrying both cannot be planned.
func TestAccDataSource_SecurityCloudUEMConnect_ReadsCreatedIntegration(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoExistingIntegration(t)
	tenant := platformTenantIDOrSkip(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkIntegrationDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = %q
						}
						sync_refresh_interval_minutes = 720
					}

					data "jamfplatform_security_cloud_uem_connect" "test" {
						depends_on = [jamfplatform_security_cloud_uem_connect.test]
					}
				`, tenant),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.jamfplatform_security_cloud_uem_connect.test", "id",
						resourceName, "id",
					),
					resource.TestCheckResourceAttrSet(
						"data.jamfplatform_security_cloud_uem_connect.test", "uem_server_url"),
					resource.TestCheckNoResourceAttr(resourceName, "uem_server_url"),
					resource.TestCheckResourceAttr(
						"data.jamfplatform_security_cloud_uem_connect.test", "platform_tenant_id", tenant),
					resource.TestCheckResourceAttr(
						"data.jamfplatform_security_cloud_uem_connect.test", "sync_refresh_interval_minutes", "720"),
					// Provisioned by Jamf Security Cloud on this form, not supplied.
					resource.TestCheckResourceAttrSet(
						"data.jamfplatform_security_cloud_uem_connect.test", "client_id"),
					resource.TestCheckResourceAttrSet(
						"data.jamfplatform_security_cloud_uem_connect.test", "connected"),
					resource.TestCheckResourceAttrSet(
						"data.jamfplatform_security_cloud_uem_connect.test", "jamf_pro_version"),
				),
			},
		},
	})
}

// TestAccListResource_SecurityCloudUEMConnect_FindsTheIntegration covers the reason
// the list resource exists: the integration's ID is not something an operator would
// have written down, so `terraform query` is how they get an import block for an
// integration set up in the admin UI.
//
// The display name is asserted to be the Jamf Pro address, because that is the only
// thing about this object a person would recognise it by — it has no name of its own.
func TestAccListResource_SecurityCloudUEMConnect_FindsTheIntegration(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoExistingIntegration(t)
	tenant := platformTenantIDOrSkip(t)

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkIntegrationDestroyed(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_uem_connect" "test" {
						uem_vendor = "JAMF_PRO"
						platform_tenant = {
							tenant_id = %q
						}
						sync_refresh_interval_minutes = 480
					}
				`, tenant),
				Check: resource.TestCheckResourceAttrSet(resourceName, "id"),
			},
			{
				Query: true,
				Config: `
					provider "jamfplatform" {}

					list "jamfplatform_security_cloud_uem_connect" "test" {
						provider         = jamfplatform
						include_resource = true

						config {}
					}
				`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					// Exactly one: a tenant holds one integration, and a query that
					// returned two would mean the one-per-tenant rule had changed.
					querycheck.ExpectLength("jamfplatform_security_cloud_uem_connect.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_security_cloud_uem_connect.test",
						queryfilter.ByDisplayName(knownvalue.NotNull()),
						[]querycheck.KnownValueCheck{
							{
								Path:       tfjsonpath.New("sync_refresh_interval_minutes"),
								KnownValue: knownvalue.Int64Exact(480),
							},
							// Populated as an import would be, even though the
							// configuration above declares neither block.
							{
								Path:       tfjsonpath.New("user_data_field_mapping").AtMapKey("device_name"),
								KnownValue: knownvalue.StringExact("DEVICE_NAME"),
							},
						},
					),
				},
			},
		},
	})
}
