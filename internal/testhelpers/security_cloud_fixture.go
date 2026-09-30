// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"regexp"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// catalogueCodeNotEntitled is Jamf Security Cloud's machine-readable "the tenant
// does not have this surface" code, taken from the SDK's generated enum rather than
// restated as a literal (STYLE_GUIDE §Enum values and error codes come from the SDK).
const catalogueCodeNotEntitled = securitycloud.ApiErrorItemCodeNotEntitled

// isMissingEntitlement reports whether err is Jamf Security Cloud refusing the read
// because the tenant does not have the surface, rather than any other failure.
//
// Two forms count. The named one is a NOT_ENTITLED detail code in the error body.
// The unnamed one is a bare 403 with nothing useful in it — Jamf Security Cloud
// answers an unmapped route with BAD_PERMISSIONS, indistinguishable from a real
// privilege gap, and a tenant without the entitlement can present either way. Both
// mean "this tenant cannot see this catalogue", which is a legitimate acceptance
// environment.
//
// Everything else — expired credentials, a wrong base URL, a DNS failure, a 500 —
// is a broken run, not an unentitled tenant, and must fail loudly.
//
// The bare-403 reading is therefore narrowed to a 403 a Jamf service produced:
// an edge error page carries whatever status the edge chose and a WAF or IP
// allowlist serves exactly a 403, so reading one as "not entitled" would skip a
// whole lane whose egress is blocked and report it green — the silent skip
// JAMFPLATFORM_ACC_REQUIRE exists to prevent. Excluded through
// helpers.IsEdgeBlocked, as STYLE_GUIDE requires of anything classifying a
// status itself.
func isMissingEntitlement(err error) bool {
	if err == nil {
		return false
	}
	if apiErr := jamfplatform.AsAPIError(err); apiErr != nil {
		for _, detail := range apiErr.Details() {
			if detail.Code == catalogueCodeNotEntitled {
				return true
			}
		}
	}
	return helpers.IsForbiddenError(err) && !helpers.IsEdgeBlocked(err)
}

// requireCatalogueRead decides the test outcome for a failed Jamf-managed catalogue
// read: skip when the tenant is not entitled, fail otherwise.
//
// The distinction is the whole point. Collapsing every read failure into a skip
// makes a run with expired credentials or a wrong base URL come back green with a
// message blaming entitlement — and because each fixture caches its result in a
// sync.Once, one transient failure would silence every later consumer in the run.
func requireCatalogueRead(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if isMissingEntitlement(err) {
		t.Skipf("Skipping: this tenant is not entitled to %s: %v", what, err)
	}
	t.Fatalf("Failed to read %s; this is a broken acceptance environment, not a missing entitlement: %v", what, err)
}

// sharedGatewaysOnce caches the shared-gateway read so the acceptance suite hits
// the endpoint at most once per run.
var sharedGatewaysOnce sync.Once

// sharedGateways holds the cached shared-gateway list.
var sharedGateways []securitycloud.SharedGateway

// sharedGatewaysErr captures the read failure, if any.
var sharedGatewaysErr error

// RequireSecurityCloudSharedGatewayIDs returns at least count Jamf-managed shared
// ZTNA gateway IDs for tests that need real gateway references.
//
// Shared gateways are a Jamf-managed catalog — every entitled tenant has the same
// set ("Nearest Data Center" plus a shared IP pool per region), so this needs no
// fixture creation and nothing to clean up. Custom DNS zone name servers reference
// them by ID, and the reference is server-enforced: a zone naming a gateway that
// does not exist is refused, so a test cannot invent an ID.
//
// A tenant that is not entitled, or one exposing fewer than count gateways, SKIPS
// rather than fails: a tenant without the Security Cloud entitlement is a legitimate
// environment for the rest of the suite, and inferring "the resource is broken" from
// absent tenant data is exactly the wrong conclusion — the resource's own behaviour
// is guarded by unit tests instead. Any *other* read failure FAILS, because a green
// run blamed on entitlement is how a broken environment hides.
func RequireSecurityCloudSharedGatewayIDs(t *testing.T, count int) []string {
	t.Helper()

	sharedGatewaysOnce.Do(func() {
		c, err := initAcceptanceClient()
		if err != nil {
			sharedGatewaysErr = err
			return
		}
		list, listErr := securitycloud.New(c).ListZtnaSharedGatewaysV1(context.Background())
		if listErr != nil {
			sharedGatewaysErr = listErr
			return
		}
		sharedGateways = list.Results
	})

	requireCatalogueRead(t, "the Jamf Security Cloud shared ZTNA gateway catalogue", sharedGatewaysErr)
	if len(sharedGateways) < count {
		t.Skipf("Skipping: tenant exposes %d shared ZTNA gateway(s), test needs %d", len(sharedGateways), count)
	}

	ids := make([]string, 0, count)
	for _, g := range sharedGateways[:count] {
		ids = append(ids, g.ID)
	}
	return ids
}

// contentCategoriesOnce caches the content-category read so the acceptance suite
// hits the endpoint at most once per run.
var contentCategoriesOnce sync.Once

// contentCategories holds the cached content-category list.
var contentCategories []securitycloud.Category

// contentCategoriesErr captures the read failure, if any.
var contentCategoriesErr error

// RequireSecurityCloudContentCategories returns the Jamf-curated content category
// catalogue for tests that need a real category reference.
//
// The catalogue is Jamf's, not the tenant's — identical for every entitled tenant
// and not writable, so there is nothing to create and nothing to clean up. A missing
// entitlement or an empty catalogue SKIPS rather than fails, for the same reason
// RequireSecurityCloudSharedGatewayIDs does: a tenant without the entitlement is a
// legitimate environment, and concluding "the data source is broken" from absent
// tenant data is the wrong inference. Any other read failure FAILS — see
// requireCatalogueRead.
func RequireSecurityCloudContentCategories(t *testing.T) []securitycloud.Category {
	t.Helper()

	contentCategoriesOnce.Do(func() {
		c, err := initAcceptanceClient()
		if err != nil {
			contentCategoriesErr = err
			return
		}
		list, listErr := securitycloud.New(c).ListContentCategoriesV1(context.Background())
		if listErr != nil {
			contentCategoriesErr = listErr
			return
		}
		contentCategories = list.Results
	})

	requireCatalogueRead(t, "the Jamf Security Cloud content category catalogue", contentCategoriesErr)
	if len(contentCategories) == 0 {
		t.Skip("Skipping: tenant exposes no Jamf Security Cloud content categories")
	}
	return contentCategories
}

// predefinedAppsOnce caches the predefined-app read so the acceptance suite hits
// the endpoint at most once per run.
var predefinedAppsOnce sync.Once

// predefinedApps holds the cached predefined-app list.
var predefinedApps []securitycloud.PredefinedApp

// predefinedAppsErr captures the read failure, if any.
var predefinedAppsErr error

// RequireSecurityCloudPredefinedApps returns the Jamf-curated Zero Trust Network
// Access app templates for tests that need a real template reference.
//
// Same contract as RequireSecurityCloudContentCategories: a Jamf-managed catalogue,
// nothing to create, a missing entitlement or an empty catalogue skips, and any
// other read failure fails.
func RequireSecurityCloudPredefinedApps(t *testing.T) []securitycloud.PredefinedApp {
	t.Helper()

	predefinedAppsOnce.Do(func() {
		c, err := initAcceptanceClient()
		if err != nil {
			predefinedAppsErr = err
			return
		}
		list, listErr := securitycloud.New(c).ListZtnaPredefinedAppsV1(context.Background())
		if listErr != nil {
			predefinedAppsErr = listErr
			return
		}
		predefinedApps = list.Results
	})

	requireCatalogueRead(t, "the Jamf Security Cloud predefined ZTNA app catalogue", predefinedAppsErr)
	if len(predefinedApps) == 0 {
		t.Skip("Skipping: tenant exposes no Jamf Security Cloud predefined ZTNA apps")
	}
	return predefinedApps
}

// noDedicatedIPsEntitlement matches the provider's "Dedicated IP address limit reached"
// diagnostic when the account's allotment is zero. Terraform wraps a diagnostic's detail at
// about 80 columns, hence the \s+ between the words of Jamf Security Cloud's "(0 of 0)".
var noDedicatedIPsEntitlement = regexp.MustCompile(`Dedicated IP address limit reached[\s\S]*\(0\s+of\s+0\)`)

// SkipWithoutDedicatedIPs is a TestCase ErrorCheck for tests that create a dedicated internet
// gateway. It skips the test when the account holds no dedicated IP addresses at all, and passes
// every other error through.
//
// A dedicated internet gateway takes a pair of addresses from a paid allotment, and an account
// without one answers every create with `409 DEDICATED_IPS_LIMIT` "(0 of 0)" (observed on the EU
// acceptance environment, 2026-09-30). No API grants the allotment, so a test cannot provision it
// as a fixture, and like NOT_ENTITLED the refusal states a fact about the account rather than a
// fault in the provider. The skip is deliberately narrowed to a zero allotment: a spent one, such
// as "(2 of 2)", can mean earlier runs leaked gateways, and that has to fail.
//
// The framework does not consult ErrorCheck on a step that sets ExpectError, so a test whose
// refused step needs an internet gateway must create it in an earlier step.
func SkipWithoutDedicatedIPs(t *testing.T) resource.ErrorCheckFunc {
	t.Helper()
	return func(err error) error {
		if err != nil && noDedicatedIPsEntitlement.MatchString(err.Error()) {
			t.Skipf("Skipping: this Jamf Security Cloud account has no dedicated IP addresses, which a dedicated internet gateway needs: %v", err)
		}
		return err
	}
}
