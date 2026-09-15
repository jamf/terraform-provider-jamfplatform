// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"
	"github.com/jamf/terraform-provider-jamfplatform/internal/provider"
)

// AccTestProtoV6ProviderFactories returns the provider factories for Terraform acceptance tests.
var AccTestProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"jamfplatform": providerserver.NewProtocol6WithError(provider.New("test")()),
}

// AccPreCheck validates that the required environment variables are set before running
// acceptance tests and ensures TF_ACC is set so terraform-plugin-testing runs the tests.
//
// Every gate here routes through SkipOrFailUnset rather than t.Skip directly, so
// an unset credential stays a skip locally — a contributor with no estate must
// be able to run `make testacc` — and becomes a FAILURE in a pipeline that wired
// the secret. A skip in CI is invisible: the package prints `ok` and the check
// goes green having asserted nothing. See internal/testhelpers/accrequire/require.go
// for the three incidents that motivated it.
//
// It reads the provider's OWN configuration variables, not ACC_-prefixed ones,
// and that is deliberate: JAMFPLATFORM_BASE_URL and friends are what the
// provider schema reads at Configure, so the suite must exercise the same names
// a user would set. .github/workflows/acceptance.yml maps the aligned
// JAMFPLATFORM_ACC_<SCOPE>_* secrets onto these names per lane.
func AccPreCheck(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"JAMFPLATFORM_BASE_URL",
		"JAMFPLATFORM_CLIENT_ID",
		"JAMFPLATFORM_CLIENT_SECRET",
	} {
		if os.Getenv(name) == "" {
			SkipOrFailUnset(t, "AccPreCheck", name+" is unset")
		}
	}
	if os.Getenv("JAMFPLATFORM_ENVIRONMENT_ID") == "" && os.Getenv("JAMFPLATFORM_TENANT_ID") == "" {
		SkipOrFailUnset(t, "AccPreCheck", "neither JAMFPLATFORM_ENVIRONMENT_ID nor JAMFPLATFORM_TENANT_ID is set, so the provider has no API integration scope")
	}

	if err := os.Setenv("TF_ACC", "1"); err != nil {
		t.Fatalf("setting TF_ACC: %v", err)
	}
}

// AccPreCheckOffline is the precheck for acceptance tests that need no tenant —
// provider-defined functions, which run offline with no API client or provider
// configuration. It sets TF_ACC (so terraform-plugin-testing actually runs the
// test) without gating on the JAMFPLATFORM_* credentials that AccPreCheck
// requires. Use this instead of AccPreCheck in function acceptance tests: it
// keeps them runnable under the raw `go test -tags=acceptance ./...` command
// (see TESTING.md), which does not set TF_ACC itself, so they neither skip
// silently nor demand credentials they don't use.
func AccPreCheckOffline(t *testing.T) {
	t.Helper()
	t.Setenv("TF_ACC", "1")
}

// AccPreCheckSecurityCloud is the precheck for Jamf Security Cloud acceptance
// tests. It runs AccPreCheck, then requires the operator to have *declared* that
// the scope the provider is configured with belongs to a Security Cloud tenant,
// via JAMFPLATFORM_ACC_SECURITYCLOUD_ENVIRONMENT_ID or
// JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_ID. The declared ID must equal the
// JAMFPLATFORM_* scope actually in use.
//
// A mismatch and an ambiguous pair route through SkipOrFailUnset alongside the
// unset case, rather than staying plain skips. Locally either is a legitimate
// state — a stale value from another estate must skip, never green-light a run
// against the wrong one. In the securitycloud lane both mean the secrets
// disagree with each other, which is a wiring fault, and a wiring fault that
// skips is a lane reporting success having asserted nothing.
//
// Why a declaration rather than a probe: Security Cloud is a separate
// entitlement, so a perfectly valid acceptance tenant can hold Jamf Pro and not
// hold this. Pointing the suite at such a tenant must skip these tests, not fail
// them. Requiring the ID to match the configured scope is what makes the
// declaration load-bearing — a stale value left over from a different tenant
// skips instead of green-lighting a run against the wrong estate.
//
// It deliberately does not introduce a second credential set. One integration
// serves every construct in this provider, and a parallel Security Cloud
// credential path would mean every Security Cloud acceptance config had to emit
// its own provider block, diverging from the rest of the suite for no gain.
func AccPreCheckSecurityCloud(t *testing.T) {
	t.Helper()
	AccPreCheck(t)

	declaredEnvironment := AccEnv("JAMFPLATFORM_ACC_SECURITYCLOUD_ENVIRONMENT_ID")
	declaredTenant := AccEnv("JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_ID")
	if declaredEnvironment == "" && declaredTenant == "" {
		SkipOrFailUnset(t, "AccPreCheckSecurityCloud", "neither JAMFPLATFORM_ACC_SECURITYCLOUD_ENVIRONMENT_ID nor JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_ID is set to declare that the configured scope is a Jamf Security Cloud one")
	}
	if declaredEnvironment != "" && declaredTenant != "" {
		SkipOrFailUnset(t, "AccPreCheckSecurityCloud", "JAMFPLATFORM_ACC_SECURITYCLOUD_ENVIRONMENT_ID and JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_ID are both set, so the declared Security Cloud scope is ambiguous; unset one")
	}

	if declaredEnvironment != "" {
		if configured := os.Getenv("JAMFPLATFORM_ENVIRONMENT_ID"); configured != declaredEnvironment {
			SkipOrFailUnset(t, "AccPreCheckSecurityCloud", fmt.Sprintf("JAMFPLATFORM_ACC_SECURITYCLOUD_ENVIRONMENT_ID (%s) does not match the configured JAMFPLATFORM_ENVIRONMENT_ID (%s), so the provider is not scoped to the declared Security Cloud environment", declaredEnvironment, configured))
		}
		return
	}
	if configured := os.Getenv("JAMFPLATFORM_TENANT_ID"); configured != declaredTenant {
		SkipOrFailUnset(t, "AccPreCheckSecurityCloud", fmt.Sprintf("JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_ID (%s) does not match the configured JAMFPLATFORM_TENANT_ID (%s), so the provider is not scoped to the declared Security Cloud tenant", declaredTenant, configured))
	}
}

// AccPreCheckAIGovernance gates the Jamf AI Governance acceptance tests.
//
// Like the Security Cloud gate, it requires the operator to *declare* that the configured scope
// belongs to a tenant holding AI Governance, and skips otherwise: an environment without the surface
// is a legitimate acceptance environment, not a failure, and the alternative — inferring entitlement
// from an empty policy list — would read a working tenant with no policies as an unentitled one.
//
// This is the gate whose declaration was referenced by no workflow at all: all 18 AI Governance
// tests skipped green on every run for the life of the file, which is why the aigovernance lane has
// its own require token rather than sharing the environment lane's. Routing the skips through
// SkipOrFailUnset is the other half of that fix.
//
// Environment scope only, and probed rather than assumed. AI Governance answers a request carrying
// no scope header with REQUEST_CONTEXT_NOT_PROVIDED, and one carrying X-Tenant-Id with
// BAD_PERMISSIONS — while a Jamf Pro request presenting the same header succeeds, so the header
// itself is accepted and the refusal belongs to the namespace. That code is indistinguishable from a
// real privilege gap, so the namespace is presumed unrouted under tenant scope and there is no
// tenant form to declare.
func AccPreCheckAIGovernance(t *testing.T) {
	t.Helper()
	AccPreCheck(t)

	declared := AccEnv("JAMFPLATFORM_ACC_AIGOVERNANCE_ENVIRONMENT_ID")
	if declared == "" {
		SkipOrFailUnset(t, "AccPreCheckAIGovernance", "JAMFPLATFORM_ACC_AIGOVERNANCE_ENVIRONMENT_ID is unset, so nothing declares that the configured environment holds Jamf AI Governance")
	}
	if configured := os.Getenv("JAMFPLATFORM_ENVIRONMENT_ID"); configured != declared {
		SkipOrFailUnset(t, "AccPreCheckAIGovernance", fmt.Sprintf("JAMFPLATFORM_ACC_AIGOVERNANCE_ENVIRONMENT_ID (%s) does not match the configured JAMFPLATFORM_ENVIRONMENT_ID (%s), so the provider is not scoped to the declared AI Governance environment", declared, configured))
	}
}

// RequireAIGovernanceTool returns the named tool from Jamf's AI tool catalogue, carrying its current
// settings schema version and every version it still accepts.
//
// The two ways this can come up short are deliberately not the same outcome.
//
// A tool the catalogue does not list SKIPS. The catalogue is Jamf's, not the tenant's — an
// environment only offers the tools Jamf has shipped to it, and there is nothing an acceptance test
// can create to change that, so a tool that is simply absent is a platform capability gap rather
// than a defect.
//
// A catalogue read that FAILS is a defect and fails the test. AccPreCheckAIGovernance has already
// required the operator to *declare* that this environment holds AI Governance, and the whole point
// of that declaration is that entitlement is declared rather than inferred from a read. Skipping on
// a failed read would put the inference straight back: a regression in the namespace path, in the
// scope header, or in the SDK's own catalogue call would take every AI Governance test with it while
// the run still reported green — the exact shape of a broken environment hiding behind a skip.
func RequireAIGovernanceTool(t *testing.T, toolID string) aigovernance.ToolSummary {
	t.Helper()

	response, err := aigovernance.New(NewAcceptanceClient(t)).ListTools(context.Background())
	if err != nil {
		t.Fatalf("Failed to read the AI tool catalogue: %v; this environment was declared to hold Jamf AI Governance, so a failed catalogue read is a defect rather than an environment condition", err)
	}

	for _, tool := range response.Results {
		if tool.ID == toolID {
			return tool
		}
	}

	t.Skipf("Skipping: this environment does not offer the AI tool %s", toolID)
	return aigovernance.ToolSummary{}
}

// AccTenantIDOrSkip returns the tenant ID the provider is configured with, or
// skips when the run is environment-scoped.
//
// For the handful of assertions that need to compare a value the tenant reports
// against the scope the provider was pointed at. Under an environment scope
// there is nothing local to compare with — the provider sends an environment ID
// and the gateway resolves the tenant server-side — so the honest outcome is a
// skip rather than an assertion built on a value the test cannot know.
//
// In the pro-tenant lane that skip becomes a failure, and that is the point of
// the lane existing: everything else in the suite now runs on the environment
// credential, so this is the assertion that proves the tenant path still works,
// and an assertion that quietly stops running is worse than one that fails.
func AccTenantIDOrSkip(t *testing.T) string {
	t.Helper()

	tenant := os.Getenv("JAMFPLATFORM_TENANT_ID")
	if tenant == "" {
		SkipOrFailUnset(t, "AccTenantIDOrSkip", "JAMFPLATFORM_TENANT_ID is unset, and an environment-scoped run has no locally known tenant ID to compare against")
	}
	return tenant
}

// AccPreCheckAccount is the precheck for Jamf Account acceptance tests.
//
// It cannot call AccPreCheck, and that is the whole point of its existence:
// AccPreCheck skips unless one of JAMFPLATFORM_ENVIRONMENT_ID or
// JAMFPLATFORM_TENANT_ID is set, and a Jamf Account integration is
// organization-scoped — it sets *neither*. Routing these tests through
// AccPreCheck would skip every one of them silently, and a suite that skips
// everywhere still reports green.
//
// So this checks the credentials AccPreCheck checks, then requires the opposite
// of what AccPreCheck requires: both scope variables must be *absent*. A scope
// variable that is set means the provider will send a scope header and
// providerdata's gate will refuse every Jamf Account construct at Configure, so
// the run would fail for a configuration reason rather than a code one — skip
// instead, and say which variable to unset.
//
// JAMFPLATFORM_ACC_ORGANIZATION_DECLARED_ID is the operator's declaration that the
// configured credentials really are organization-scoped, mirroring
// AccPreCheckSecurityCloud's declaration pattern. There is nothing to compare it
// against — an organization-scoped request carries no scope header, so no
// JAMFPLATFORM_* variable holds the organization — which is exactly why the
// declaration is required: without it, a Pro-only credential set with both scope
// variables unset would look identical to a real organization integration and
// fail deep in an apply. jamfplatform-go-sdk has no equivalent — its
// organization client needs no ID by design — so this variable is provider-only
// and its name is the one place the aligned scheme adds a field the SDK lacks.
//
// The namespace is also US-only: /sso/v1 is absent from the EU gateway, where
// even a bogus route under it returns the gateway's own bare 404. A non-US base
// URL therefore skips.
func AccPreCheckAccount(t *testing.T) {
	t.Helper()

	baseURL := os.Getenv("JAMFPLATFORM_BASE_URL")
	for _, name := range []string{
		"JAMFPLATFORM_BASE_URL",
		"JAMFPLATFORM_CLIENT_ID",
		"JAMFPLATFORM_CLIENT_SECRET",
	} {
		if os.Getenv(name) == "" {
			SkipOrFailUnset(t, "AccPreCheckAccount", name+" is unset")
		}
	}
	if !strings.Contains(baseURL, "us.api.jamfcloud.com") {
		SkipOrFailUnset(t, "AccPreCheckAccount", fmt.Sprintf("Jamf Account SSO is served only from the US gateway and JAMFPLATFORM_BASE_URL is %q", baseURL))
	}
	if v := os.Getenv("JAMFPLATFORM_ENVIRONMENT_ID"); v != "" {
		SkipOrFailUnset(t, "AccPreCheckAccount", "JAMFPLATFORM_ENVIRONMENT_ID is set, but Jamf Account requires an organization-scoped integration, which sends no scope header; unset it to run these tests")
	}
	if v := os.Getenv("JAMFPLATFORM_TENANT_ID"); v != "" {
		SkipOrFailUnset(t, "AccPreCheckAccount", "JAMFPLATFORM_TENANT_ID is set, but Jamf Account requires an organization-scoped integration, which sends no scope header; unset it to run these tests")
	}
	if v := AccEnv("JAMFPLATFORM_ACC_ORGANIZATION_DECLARED_ID"); v == "" {
		SkipOrFailUnset(t, "AccPreCheckAccount", "JAMFPLATFORM_ACC_ORGANIZATION_DECLARED_ID is unset, so nothing declares that the configured credentials are organization-scoped")
	}

	if err := os.Setenv("TF_ACC", "1"); err != nil {
		t.Fatalf("setting TF_ACC: %v", err)
	}
}
