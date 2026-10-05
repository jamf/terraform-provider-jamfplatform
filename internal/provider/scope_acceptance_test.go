// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package provider_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// scopeProbeConfig is a provider block plus the cheapest data source that takes
// no required arguments, so Terraform has a reason to configure the provider.
// Neither test reaches the read: one fails in provider Configure and the other in
// the data source's Configure.
const scopeProbeConfig = `
provider "jamfplatform" {
%s
}

data "jamfplatform_device_groups" "scope_probe" {}
`

// TestAccProviderScope_ConflictingScopesRejected pins the mutual exclusion of
// environment_id and tenant_id against a real Terraform run.
//
// It needs no environment-scoped integration, so it is the one scope test that
// earns its place in CI before the Platform API GA: the IDs never reach the wire.
// resolveScope runs before ValidateCredentials in provider Configure, so the
// conflict is caught with whatever credentials the run already has, under
// whatever scope those credentials carry.
//
// The regex matches only the diagnostic summary. Terraform hard-wraps detail text
// at roughly 80 columns, which breaks any regex spanning more than a few words;
// the detail's wording is asserted by the unit tests in scope_test.go instead.
func TestAccProviderScope_ConflictingScopesRejected(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(scopeProbeConfig, `
  environment_id = "11111111-1111-4111-8111-111111111111"
  tenant_id      = "22222222-2222-4222-8222-222222222222"`),
				ExpectError: regexp.MustCompile(`Conflicting API Integration Scope`),
			},
		},
	})
}

// TestAccProviderScope_OrganizationScopeRejectedPerConstruct verifies the
// per-construct gate end-to-end: an integration carrying no scope at all
// configures the provider successfully, then fails on the first construct that
// needs a scope.
//
// This is the only test that exercises the organization-scope rejection path,
// and it will keep being the only one until the organization-level constructs
// exist. It runs in CI today because it needs credentials but deliberately no
// scope — both scope variables are cleared for the duration, which is also why
// it cannot use testhelpers.AccPreCheck (that helper skips when neither is set).
func TestAccProviderScope_OrganizationScopeRejectedPerConstruct(t *testing.T) {
	accPreCheckCredentialsOnly(t)

	// Cleared before the provider is configured, so resolveScope sees neither
	// variable and the client is built with no scope header at all.
	t.Setenv("JAMFPLATFORM_ENVIRONMENT_ID", "")
	t.Setenv("JAMFPLATFORM_TENANT_ID", "")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(scopeProbeConfig, ""),
				ExpectError: regexp.MustCompile(`Unsupported API Integration Scope`),
			},
		},
	})
}

// accPreCheckCredentialsOnly gates on the credentials alone, without the scope
// requirement testhelpers.AccPreCheck adds. Only the organization-scope test
// wants this: every other acceptance test needs a scope to reach anything.
//
// It routes its skip through testhelpers.SkipOrFailUnset under AccPreCheck's
// require tokens, and must keep doing so. A package-local precheck that skips
// directly is a hole in the require mechanism: in the pro-tenant lane, with
// JAMFPLATFORM_ACC_REQUIRE set, a missing credential would skip this test green
// while every test routed through the shared helper failed loudly.
// internal/conformance/acc_lanes_test.go allow-lists this helper by name, which
// documents the exemption rather than closing it, so the routing here is what
// actually closes it.
func accPreCheckCredentialsOnly(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"JAMFPLATFORM_BASE_URL",
		"JAMFPLATFORM_CLIENT_ID",
		"JAMFPLATFORM_CLIENT_SECRET",
	} {
		if os.Getenv(key) == "" {
			testhelpers.SkipOrFailUnset(t, "AccPreCheck", key+" is unset")
		}
	}

	t.Setenv("TF_ACC", "1")
}

// TestAccProviderBetaGateway_Rejected pins the beta gateway guard
// against a real Terraform run.
//
// Like the scope conflict above, it earns its place in CI without an estate:
// betaGatewayError runs before ValidateCredentials, so nothing reaches the wire
// and the run's own credentials are never assessed. The host it configures is
// the one that made this guard necessary — it still answers the token exchange,
// so without the guard the run authenticates and then reports every managed
// object as deleted from Jamf.
//
// The regex matches only the diagnostic summary, for the line-wrapping reason
// given above; the detail's wording is asserted in auth_diagnostics_test.go.
func TestAccProviderBetaGateway_Rejected(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(scopeProbeConfig, `  base_url = "https://us.apigw.jamf.com"`),
				ExpectError: regexp.MustCompile(`Base URL Names the Beta Gateway`),
			},
		},
	})
}

// TestAccProviderID_NonUUID4InBlockRejected pins the schema validators on
// client_id, tenant_id and environment_id against a real Terraform run. The
// rejection happens at validate time, so nothing reaches the wire.
//
// The regex matches only the framework's summary, since Terraform hard-wraps the
// detail; the pattern itself is asserted by TestUUID4Regexp.
func TestAccProviderID_NonUUID4InBlockRejected(t *testing.T) {
	for _, attr := range []string{"client_id", "tenant_id", "environment_id"} {
		t.Run(attr, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testhelpers.AccPreCheck(t) },
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      fmt.Sprintf(scopeProbeConfig, fmt.Sprintf("  %s = \"not-a-uuid\"", attr)),
						ExpectError: regexp.MustCompile(`Invalid Attribute Value Match`),
					},
				},
			})
		})
	}
}

// TestAccProviderID_NonUUID4FromEnvironmentRejected covers the path no schema
// validator sees: a malformed JAMFPLATFORM_CLIENT_ID is caught in Configure.
func TestAccProviderID_NonUUID4FromEnvironmentRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	t.Setenv("JAMFPLATFORM_CLIENT_ID", "not-a-uuid")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(scopeProbeConfig, ""),
				ExpectError: regexp.MustCompile(`Invalid client_id`),
			},
		},
	})
}

// TestAccProviderID_NonUUID4ScopeFromEnvironmentRejected covers the scope half
// of the environment path: a malformed JAMFPLATFORM_ENVIRONMENT_ID or
// JAMFPLATFORM_TENANT_ID is caught in Configure. The other scope variable is
// cleared so resolveScope reaches the ID check instead of the conflict error.
func TestAccProviderID_NonUUID4ScopeFromEnvironmentRejected(t *testing.T) {
	for _, tc := range []struct{ set, clear string }{
		{"JAMFPLATFORM_ENVIRONMENT_ID", "JAMFPLATFORM_TENANT_ID"},
		{"JAMFPLATFORM_TENANT_ID", "JAMFPLATFORM_ENVIRONMENT_ID"},
	} {
		t.Run(tc.set, func(t *testing.T) {
			testhelpers.AccPreCheck(t)
			t.Setenv(tc.clear, "")
			t.Setenv(tc.set, "not-a-uuid")

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      fmt.Sprintf(scopeProbeConfig, ""),
						ExpectError: regexp.MustCompile(`Invalid API Integration Scope ID`),
					},
				},
			})
		})
	}
}
