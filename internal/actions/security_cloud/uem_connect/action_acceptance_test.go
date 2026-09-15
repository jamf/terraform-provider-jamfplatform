// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package uemconnectactions_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// The action's fixtures are the resource suite's, since the action operates on the
// integration that suite creates. Kept as literals rather than shared so this
// package does not depend on another test package's unexported names.
const envPlatformTenantID = "JAMFPLATFORM_ACC_SECURITYCLOUD_UEM_PRO_TENANT_ID"

func securityCloudClient(t *testing.T) *securitycloud.Client {
	t.Helper()
	return securitycloud.New(testhelpers.NewAcceptanceClient(t))
}

// requireNoExistingIntegration skips when the tenant already holds a UEM Connect
// integration, because this test creates one and a tenant holds only one. It
// refuses to remove someone else's to make room — that would take their device
// sync down.
func requireNoExistingIntegration(t *testing.T) {
	t.Helper()

	page, err := securityCloudClient(t).ListUemConnectorsV1(context.Background())
	if err != nil {
		t.Skipf("could not check for an existing UEM Connect integration: %v", err)
	}
	if page != nil && len(page.Results) > 0 {
		t.Skipf("this tenant already holds a UEM Connect integration (%s); remove it first to run this test", page.Results[0].ID)
	}
}

func platformTenantIDOrSkip(t *testing.T) string {
	t.Helper()

	tenant := testhelpers.AccEnv(envPlatformTenantID)
	if tenant == "" {
		t.Skipf("%s must be set to the platform tenant ID of a Jamf Pro instance for this test", envPlatformTenantID)
	}
	return tenant
}

// TestAccAction_SecurityCloudUEMConnectSynchronize_Invoke triggers a sync on an
// integration the same configuration creates.
//
// What it can assert is deliberately limited. The action is fire-once over an
// asynchronous operation: Jamf Security Cloud accepts the request and runs the sync
// in the background, so a passing apply means "the request was accepted", which is
// the whole contract. Asserting on the sync's outcome would be asserting on a race,
// and the run history is not surfaced by this provider by design.
//
// Referencing the resource's id in the action config is also the dependency edge
// that makes the action run after the integration exists — the ergonomics the
// optional attribute is there for.
func TestAccAction_SecurityCloudUEMConnectSynchronize_Invoke(t *testing.T) {
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

action "jamfplatform_security_cloud_uem_connect_synchronize" "now" {
  config {
    uem_connect_id = jamfplatform_security_cloud_uem_connect.test.id
  }
}

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.jamfplatform_security_cloud_uem_connect_synchronize.now]
    }
  }
}
`, tenant)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: config,
			Check:  resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_uem_connect.test", "id"),
		}},
	})
}

// TestAccAction_SecurityCloudUEMConnectSynchronize_WithoutID covers the form the
// example leads with: no uem_connect_id at all, resolved from the tenant's only
// integration.
//
// Worth its own test rather than a variation, because the resolution path is a
// second read the explicit form never exercises — and it is the path a caller who
// has not read the docs will take.
func TestAccAction_SecurityCloudUEMConnectSynchronize_WithoutID(t *testing.T) {
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

action "jamfplatform_security_cloud_uem_connect_synchronize" "now" {
  config {}
}

# depends_on rather than a config reference: with no uem_connect_id there is nothing
# to create the edge, so the trigger has to carry it or the sync can run before the
# integration exists.
resource "terraform_data" "trigger" {
  depends_on = [jamfplatform_security_cloud_uem_connect.test]

  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.jamfplatform_security_cloud_uem_connect_synchronize.now]
    }
  }
}
`, tenant)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: config,
			Check:  resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_uem_connect.test", "id"),
		}},
	})
}

// TestAccAction_SecurityCloudUEMConnectSynchronize_DisabledIntegration pins the one
// failure worth a named diagnostic: Jamf Security Cloud refuses to synchronize a
// disabled integration, and its message names the integration's identifier rather
// than the setting that has to change.
func TestAccAction_SecurityCloudUEMConnectSynchronize_DisabledIntegration(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoExistingIntegration(t)
	tenant := platformTenantIDOrSkip(t)

	config := fmt.Sprintf(`
resource "jamfplatform_security_cloud_uem_connect" "test" {
  uem_vendor = "JAMF_PRO"
  enabled    = false

  platform_tenant = {
    tenant_id = %q
  }
}

action "jamfplatform_security_cloud_uem_connect_synchronize" "now" {
  config {
    uem_connect_id = jamfplatform_security_cloud_uem_connect.test.id
  }
}

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.jamfplatform_security_cloud_uem_connect_synchronize.now]
    }
  }
}
`, tenant)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: config,
			// Anchored on no-space tokens and with \s+ for the gaps: Terraform wraps
			// error output at ~80 columns and where the wrap lands shifts with the
			// message around it.
			ExpectError: regexp.MustCompile(`disabled,\s+so\s+it\s+cannot\s+synchronize`),
		}},
	})
}

// Gates for the deploy action's fixtures.
//
// The activation profile code cannot be created by a fixture: no route issues one
// and no route lists them, so it is declared or the test skips. The Jamf Pro group
// is a second declaration rather than a resource because the integration's Jamf Pro
// instance is a different tenant from the Jamf Security Cloud one the provider is
// configured for, so this provider cannot create a group there.
const (
	envActivationProfileCode = "JAMFPLATFORM_ACC_SECURITYCLOUD_ACTIVATION_PROFILE_CODE"
	envMobileDeviceGroupID   = "JAMFPLATFORM_ACC_SECURITYCLOUD_ACTIVATION_PROFILE_MOBILE_GROUP_ID"
)

func activationProfileCodeOrSkip(t *testing.T) string {
	t.Helper()

	code := testhelpers.AccEnv(envActivationProfileCode)
	if code == "" {
		t.Skipf("%s must be set to an activation profile code from the Jamf Security Cloud console — "+
			"nothing can create or list one", envActivationProfileCode)
	}
	return code
}

func mobileDeviceGroupIDOrSkip(t *testing.T) string {
	t.Helper()

	group := testhelpers.AccEnv(envMobileDeviceGroupID)
	if group == "" {
		t.Skipf("%s must be set to a mobile device group ID in the integration's Jamf Pro instance",
			envMobileDeviceGroupID)
	}
	return group
}

// deployConfig builds a configuration that stands the integration up and then
// deploys through it.
//
// The connector fixture is the point: the action is invoked against an integration
// this configuration created, not whatever the tenant happened to be holding, and
// depends_on is what orders the two — the action's arguments name no integration,
// so there is no reference to carry the edge.
func deployConfig(tenant, code, osValue, groups string) string {
	return fmt.Sprintf(`
resource "jamfplatform_security_cloud_uem_connect" "test" {
  uem_vendor = "JAMF_PRO"

  platform_tenant = {
    tenant_id = %q
  }
}

action "jamfplatform_security_cloud_activation_profile_deploy" "test" {
  config {
    activation_profile_code = %q
    os                      = %q
%s
  }
}

resource "terraform_data" "trigger" {
  depends_on = [jamfplatform_security_cloud_uem_connect.test]

  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.jamfplatform_security_cloud_activation_profile_deploy.test]
    }
  }
}
`, tenant, code, osValue, groups)
}

// TestAccAction_SecurityCloudActivationProfileDeploy_Invoke deploys an activation
// profile's configuration profile to the Jamf Pro instance behind an integration
// the same configuration creates, scoped to a named group.
//
// What it asserts is deliberately narrow. The action holds no state, so a passing
// apply means "Jamf Security Cloud accepted and completed the deployment", which is
// the whole contract. What the deployment produced is recorded on the integration,
// but in a field the SDK does not model, so there is nothing to check it against
// here.
//
// It leaves a configuration profile behind in Jamf Pro, and there is nothing this
// test can do about that: no route undeploys, so CheckDestroy has no counterpart to
// call. Destroying the integration does not remove it either. Re-running the test
// is safe — the deploy updates the profile it already created rather than adding a
// second — but a run against a tenant you care about leaves a real profile scoped
// to a real group.
func TestAccAction_SecurityCloudActivationProfileDeploy_Invoke(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoExistingIntegration(t)
	tenant := platformTenantIDOrSkip(t)
	code := activationProfileCodeOrSkip(t)
	group := mobileDeviceGroupIDOrSkip(t)

	config := deployConfig(tenant, code, "ios_supervised",
		fmt.Sprintf("    jamf_pro_group_ids      = [%q]", group))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_uem_connect.test", "id"),
			},
			{
				// Second apply of the identical configuration. The deploy is
				// idempotent on the wire — a repeat updates the configuration
				// profile it already created rather than adding another — and this
				// is the step that would catch that changing.
				Config: config,
				Check:  resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_uem_connect.test", "id"),
			},
		},
	})
}

// TestAccAction_SecurityCloudActivationProfileDeploy_WithoutGroups covers the form
// that succeeds while doing less than it looks like it does: with no groups named,
// Jamf Security Cloud reports success and leaves the configuration profile scoped to
// nothing.
//
// Worth its own test because it is the form a caller who has not read the warning
// will write, and because the apply passing is exactly what makes the behaviour hard
// to notice.
func TestAccAction_SecurityCloudActivationProfileDeploy_WithoutGroups(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoExistingIntegration(t)
	tenant := platformTenantIDOrSkip(t)
	code := activationProfileCodeOrSkip(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: deployConfig(tenant, code, "macos", ""),
			Check:  resource.TestCheckResourceAttrSet("jamfplatform_security_cloud_uem_connect.test", "id"),
		}},
	})
}

// TestAccAction_SecurityCloudActivationProfileDeploy_UnknownCode pins the diagnostic
// for a code Jamf Security Cloud does not recognise.
//
// It needs no activation profile code of its own, which is the point: this is the
// one failure path that runs on any Security Cloud tenant.
func TestAccAction_SecurityCloudActivationProfileDeploy_UnknownCode(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoExistingIntegration(t)
	tenant := platformTenantIDOrSkip(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: deployConfig(tenant, "tfacc-no-such-code", "ios_supervised", ""),
			// Anchored on no-space tokens with \s+ for the gaps: Terraform wraps
			// error output at ~80 columns and where the wrap lands shifts with the
			// message around it.
			ExpectError: regexp.MustCompile(`Activation\s+profile\s+not\s+found`),
		}},
	})
}

// TestAccAction_SecurityCloudActivationProfileDeploy_UnknownGroup pins the
// replacement for the worst message on this surface. Jamf Security Cloud answers a
// group ID that does not exist with "UEM is misconfigured", which blames the
// integration and names neither the field nor the group.
func TestAccAction_SecurityCloudActivationProfileDeploy_UnknownGroup(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	requireNoExistingIntegration(t)
	tenant := platformTenantIDOrSkip(t)
	code := activationProfileCodeOrSkip(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: deployConfig(tenant, code, "ios_supervised",
				`    jamf_pro_group_ids      = ["99999999"]`),
			ExpectError: regexp.MustCompile(`group\s+cannot\s+be\s+used\s+for\s+this\s+deployment`),
		}},
	})
}

// TestAccAction_SecurityCloudActivationProfileDeploy_PlanTimeValidation covers the
// three refusals that never reach Jamf Security Cloud.
//
// No integration fixture and no activation profile code: schema validation runs
// before anything is created, and standing an integration up to reject a plan would
// be a slow way to test a validator. The group cases are the ones that earn their
// place — the server's own refusal for a prefixed group ID is a 422 that names the
// value but not the field, and for an empty list it is no refusal at all: the
// deployment succeeds and silently changes no scope.
func TestAccAction_SecurityCloudActivationProfileDeploy_PlanTimeValidation(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	tests := []struct {
		name        string
		osValue     string
		groups      string
		expectError *regexp.Regexp
	}{
		{
			name:        "unknown os",
			osValue:     "ipados",
			expectError: regexp.MustCompile(`(?s)os.*value must be one of`),
		},
		{
			name:    "group id carries the uem_connect prefix",
			osValue: "ios_supervised",
			groups:  `    jamf_pro_group_ids      = ["mobile_20"]`,
			// The prefixed spelling is valid on the integration's group mapping and
			// invalid here, which is exactly why it is worth refusing by name.
			expectError: regexp.MustCompile(`(?s)digits\s+only`),
		},
		{
			name:        "empty group list",
			osValue:     "ios_supervised",
			groups:      `    jamf_pro_group_ids      = []`,
			expectError: regexp.MustCompile(`(?s)at\s+least\s+1\s+element`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := fmt.Sprintf(`
action "jamfplatform_security_cloud_activation_profile_deploy" "test" {
  config {
    activation_profile_code = "tfacc-unused"
    os                      = %q
%s
  }
}

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.jamfplatform_security_cloud_activation_profile_deploy.test]
    }
  }
}
`, tc.osValue, tc.groups)

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      config,
					ExpectError: tc.expectError,
				}},
			})
		})
	}
}
