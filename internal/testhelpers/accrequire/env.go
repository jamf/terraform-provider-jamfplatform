// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package accrequire carries the acceptance suite's credential gate: the
// JAMFPLATFORM_ACC_REQUIRE promotion of a skip into a failure, the dual-read of
// renamed acceptance variables, and the report to hand Jamf Support when
// credentials are supplied and refused.
//
// It is a leaf package, importing nothing but the standard library, and that is
// the whole reason it is not part of internal/testhelpers. testhelpers imports
// internal/provider to build the test provider factories, and provider reaches
// internal/common/impact through providerdata — so a package below testhelpers
// in the graph cannot import it, and internal/common/impact's own acceptance
// tests need this gate. Keeping the gate at the bottom of the graph means every
// acceptance test in the repo can route through it, which is the property that
// makes it airtight; testhelpers re-exports the three entry points so ordinary
// call sites still say testhelpers.AccEnv and testhelpers.SkipOrFailUnset.
//
// It also carries no //go:build acceptance tag, unlike everything else in the
// acceptance harness, and that is deliberate: this package IS the gate, and a
// gate needs tests that run in `make test`. Under the tag its own tests would
// be invisible to the untagged suite, and adding an acceptance-tagged test file
// would instead enrol the package into the acceptance suite — making it a lane
// candidate whose "tests" need no credentials, which is a different kind of
// wrong. Dropping the tag costs nothing: only acceptance-tagged files import
// this package, so it reaches no shipped binary either way.
package accrequire

import (
	"log"
	"os"
	"sync"
)

// accLegacyEnvNames maps each current acceptance variable to the name it had
// before the 2026-09-03 rename, so an existing local .env keeps working.
//
// The rename gave every acceptance variable the shape
// JAMFPLATFORM_ACC_<PRODUCT>_<FIELD>, aligning this repo with
// jamfplatform-go-sdk so the two projects' secrets and workflows line up. The
// product token appears on anything product-specific — credentials, fixtures and
// write gates alike — and a bare ACC_ prefix is left for genuinely suite-wide
// knobs. Where the SDK already names the same secret, this repo adopts its name
// VERBATIM rather than merely its shape, so one GitHub secret value serves both
// repos: hence JAMFPLATFORM_ACC_PRO_DEP_TOKEN and
// JAMFPLATFORM_ACC_SECURITYCLOUD_UEM_PRO_TENANT_ID, neither of which is what
// this repo would have called them on its own. Naming is product-first rather than scope-first because an ADE token
// belongs to Jamf Pro and not to a scope: ACC_PRO_ADE_TOKEN is the only order
// that works, and scope-first would have split one scheme into two orders.
//
// # What is deliberately absent from this map
//
// Nothing here renames a variable the PROVIDER itself reads.
// JAMFPLATFORM_BASE_URL, _CLIENT_ID, _CLIENT_SECRET, _TENANT_ID,
// _ENVIRONMENT_ID, _MIN_REQUEST_INTERVAL_MS, _CUSTOM_HEADERS,
// _AUTHORIZATION_HEADER_NAME and _IMPACT_ALERTS are the provider's own
// configuration: the provider schema reads them at Configure and they are
// documented for users, so they are public API and cannot move. The alignment
// with the SDK therefore happens at the SECRET layer rather than the runtime
// layer — the GitHub secret carries the aligned name and
// .github/workflows/acceptance.yml maps it onto the provider's consumer
// variable. That mapping is the one deliberate divergence from the SDK, and
// acceptance.yml says so where it happens.
//
// This is also why the map carries no "deliberately not shimmed" exclusions,
// unlike the SDK's: the SDK had to stop its acceptance suite falling back to the
// bare consumer names it published in doc.go, whereas here those names are what
// the suite is supposed to read.
var accLegacyEnvNames = map[string]string{
	// Jamf Account — organization scope. The declaration keeps its own note in
	// AccPreCheckAccount: nothing can be compared against it.
	"JAMFPLATFORM_ACC_ORGANIZATION_DECLARED_ID":             "JAMFPLATFORM_ACCOUNT_ORGANIZATION_ID",
	"JAMFPLATFORM_ACC_ORGANIZATION_SSO_VERIFIED_DOMAIN":     "JAMFPLATFORM_ACC_SSO_VERIFIED_DOMAIN",
	"JAMFPLATFORM_ACC_ORGANIZATION_SSO_UNVERIFIABLE_DOMAIN": "JAMFPLATFORM_ACC_SSO_UNVERIFIABLE_DOMAIN",

	// Jamf Security Cloud — entitlement declarations and fixtures.
	"JAMFPLATFORM_ACC_SECURITYCLOUD_ENVIRONMENT_ID": "JAMFPLATFORM_SECURITY_CLOUD_ENVIRONMENT_ID",
	"JAMFPLATFORM_ACC_SECURITYCLOUD_TENANT_ID":      "JAMFPLATFORM_SECURITY_CLOUD_TENANT_ID",
	// UEM_PRO_TENANT_ID is the SDK's name for this exact fixture — the Jamf Pro
	// tenant a UEM connector points at — so one secret value serves both repos.
	"JAMFPLATFORM_ACC_SECURITYCLOUD_UEM_PRO_TENANT_ID":                  "JAMFPLATFORM_ACC_UEM_CONNECT_PLATFORM_TENANT_ID",
	"JAMFPLATFORM_ACC_SECURITYCLOUD_UEM_SERVER_URL":                     "JAMFPLATFORM_ACC_UEM_CONNECT_SERVER_URL",
	"JAMFPLATFORM_ACC_SECURITYCLOUD_UEM_CLIENT_ID":                      "JAMFPLATFORM_ACC_UEM_CONNECT_CLIENT_ID",
	"JAMFPLATFORM_ACC_SECURITYCLOUD_UEM_CLIENT_SECRET":                  "JAMFPLATFORM_ACC_UEM_CONNECT_CLIENT_SECRET",
	"JAMFPLATFORM_ACC_SECURITYCLOUD_ACTIVATION_PROFILE_CODE":            "JAMFPLATFORM_ACC_ACTIVATION_PROFILE_CODE",
	"JAMFPLATFORM_ACC_SECURITYCLOUD_ACTIVATION_PROFILE_MOBILE_GROUP_ID": "JAMFPLATFORM_ACC_ACTIVATION_PROFILE_MOBILE_GROUP_ID",

	// Jamf AI Governance — entitlement declaration.
	"JAMFPLATFORM_ACC_AIGOVERNANCE_ENVIRONMENT_ID": "JAMFPLATFORM_AI_GOVERNANCE_ENVIRONMENT_ID",

	// Jamf Pro — Automated Device Enrollment. DEP rather than ADE because
	// JAMFPLATFORM_ACC_PRO_DEP_TOKEN is the name the SDK already uses for the
	// same secret, and the point of the rename is that one secret value serves
	// both repos; the four serials follow the token's product token so the
	// family reads as one. Jamf's current name for the programme is ADE, so the
	// mismatch is deliberate and this comment is the reason it survives.
	"JAMFPLATFORM_ACC_PRO_DEP_TOKEN":          "JAMFPLATFORM_ADE_TOKEN",
	"JAMFPLATFORM_ACC_PRO_DEP_SERIAL":         "JAMFPLATFORM_ADE_SERIAL",
	"JAMFPLATFORM_ACC_PRO_DEP_SERIAL2":        "JAMFPLATFORM_ADE_SERIAL2",
	"JAMFPLATFORM_ACC_PRO_DEP_MOBILE_SERIAL":  "JAMFPLATFORM_ADE_MOBILE_SERIAL",
	"JAMFPLATFORM_ACC_PRO_DEP_MOBILE_SERIALS": "JAMFPLATFORM_ADE_MOBILE_SERIALS",

	// Jamf Pro — Volume Purchasing.
	"JAMFPLATFORM_ACC_PRO_VPP_TOKEN": "JAMFPLATFORM_VPP_TOKEN",

	// Jamf Pro — Jamf Protect deployment. A Protect API credential used BY a
	// Jamf Pro test, which is why it is filed under the pro product and not
	// under a protect lane of its own; see the planned "protect" lane in
	// .github/acceptance-lanes.json.
	"JAMFPLATFORM_ACC_PRO_PROTECT_URL":           "JAMFPLATFORM_PROTECT_URL",
	"JAMFPLATFORM_ACC_PRO_PROTECT_CLIENT_ID":     "JAMFPLATFORM_PROTECT_CLIENT_ID",
	"JAMFPLATFORM_ACC_PRO_PROTECT_PASSWORD":      "JAMFPLATFORM_PROTECT_PASSWORD",
	"JAMFPLATFORM_ACC_PRO_PROTECT_DEPLOYMENT_ID": "JAMFPLATFORM_ACC_PROTECT_DEPLOYMENT_ID",

	// Jamf Pro — Google Secure LDAP cloud identity provider. The _ROTATED pair
	// guards the WriteOnly cert-rotation regression.
	"JAMFPLATFORM_ACC_PRO_GOOGLE_DOMAIN_NAME":               "JAMFPLATFORM_GOOGLE_DOMAIN_NAME",
	"JAMFPLATFORM_ACC_PRO_GOOGLE_KEYSTORE_BASE64":           "JAMFPLATFORM_GOOGLE_KEYSTORE_BASE64",
	"JAMFPLATFORM_ACC_PRO_GOOGLE_KEYSTORE_PASSWORD":         "JAMFPLATFORM_GOOGLE_KEYSTORE_PASSWORD",
	"JAMFPLATFORM_ACC_PRO_GOOGLE_KEYSTORE_BASE64_ROTATED":   "JAMFPLATFORM_GOOGLE_KEYSTORE_BASE64_ROTATED",
	"JAMFPLATFORM_ACC_PRO_GOOGLE_KEYSTORE_PASSWORD_ROTATED": "JAMFPLATFORM_GOOGLE_KEYSTORE_PASSWORD_ROTATED",

	// Jamf Pro — existing tenant objects the suite references rather than creates.
	"JAMFPLATFORM_ACC_PRO_COMPUTER_SERIAL":    "JAMFPLATFORM_ACC_COMPUTER_SERIAL",
	"JAMFPLATFORM_ACC_PRO_COMPUTER_SERIAL_2":  "JAMFPLATFORM_ACC_COMPUTER_SERIAL_2",
	"JAMFPLATFORM_ACC_PRO_LAPS_USERNAME":      "JAMFPLATFORM_ACC_LAPS_USERNAME",
	"JAMFPLATFORM_ACC_PRO_MDPP_UUID_LOOKUP":   "JAMFPLATFORM_ACC_MDPP_UUID_LOOKUP",
	"JAMFPLATFORM_ACC_PRO_ADCS_API_CLIENT_ID": "JAMFPLATFORM_ACC_ADCS_API_CLIENT_ID",

	// Jamf Pro — real directory-service fixture.
	"JAMFPLATFORM_ACC_PRO_LDAP_GROUP_NAME": "JAMFPLATFORM_ACC_LDAP_GROUP_NAME",
	"JAMFPLATFORM_ACC_PRO_LDAP_USERNAME":   "JAMFPLATFORM_ACC_LDAP_USERNAME",
	"JAMFPLATFORM_ACC_PRO_LDAP_PASSWORD":   "JAMFPLATFORM_ACC_LDAP_PASSWORD",

	// Jamf Pro — SSO and Self Service material. These two are JAMF PRO's SSO
	// settings, not Jamf Account's: they are read by pro/sso_settings and
	// pro/enrollment_customization. Jamf Account's SSO fixtures are the
	// ACC_ORGANIZATION_SSO_* pair above, and the two families must not be
	// conflated — they name different objects on different scopes.
	"JAMFPLATFORM_ACC_PRO_SSO_IDP_URL":              "JAMFPLATFORM_ACC_SSO_IDP_URL",
	"JAMFPLATFORM_ACC_PRO_SSO_METADATA_BASE64":      "JAMFPLATFORM_ACC_SSO_METADATA_BASE64",
	"JAMFPLATFORM_ACC_PRO_SELF_SERVICE_SAML":        "JAMFPLATFORM_ACC_SELF_SERVICE_SAML",
	"JAMFPLATFORM_ACC_PRO_SMTP_GOOGLE":              "JAMFPLATFORM_ACC_SMTP_GOOGLE",
	"JAMFPLATFORM_ACC_PRO_SUPERVISION_P12_BASE64":   "JAMFPLATFORM_ACC_SUPERVISION_P12_BASE64",
	"JAMFPLATFORM_ACC_PRO_SUPERVISION_P12_PASSWORD": "JAMFPLATFORM_ACC_SUPERVISION_P12_PASSWORD",

	// Jamf Pro — Global Service Exchange.
	"JAMFPLATFORM_ACC_PRO_GSX_KEYSTORE_BASE64":   "JAMFPLATFORM_ACC_GSX_KEYSTORE_BASE64",
	"JAMFPLATFORM_ACC_PRO_GSX_KEYSTORE_PASSWORD": "JAMFPLATFORM_ACC_GSX_KEYSTORE_PASSWORD",
	"JAMFPLATFORM_ACC_PRO_GSX_SERVICE_ACCOUNT":   "JAMFPLATFORM_ACC_GSX_SERVICE_ACCOUNT",
	"JAMFPLATFORM_ACC_PRO_GSX_SHIP_TO":           "JAMFPLATFORM_ACC_GSX_SHIP_TO",
	"JAMFPLATFORM_ACC_PRO_GSX_TOKEN":             "JAMFPLATFORM_ACC_GSX_TOKEN",
	"JAMFPLATFORM_ACC_PRO_GSX_USERNAME":          "JAMFPLATFORM_ACC_GSX_USERNAME",

	// Write gate. Renamed from _DESTRUCTIVE to the SDK's _WRITE_OK suffix, and
	// moved from a GitHub secret to a repository VARIABLE: a boolean gate has
	// nothing to hide, and a secret masks its value in the logs, turning "1"
	// into "***" everywhere it appears.
	"JAMFPLATFORM_ACC_PRO_CDP_WRITE_OK": "JAMFPLATFORM_ACC_CDP_DESTRUCTIVE",
}

// accLegacyEnvUsed remembers which legacy names this run fell back to, so the
// warning is emitted once per name rather than once per lookup.
var accLegacyEnvUsed sync.Map

// AccEnv reads an acceptance variable, falling back to its pre-rename name.
//
// Dual-read rather than a hard cutover, deliberately: a stale .env carrying only
// the old names would otherwise not error but silently drop a fixture or a scope
// declaration, and the affected tests would self-skip green — which is the whole
// class of bug JAMFPLATFORM_ACC_REQUIRE exists to close. Remove this shim once
// every local .env has moved.
//
// It cannot rescue CI, and that is worth knowing before relying on it: a GitHub
// secret is not an ambient environment variable, so an unmapped old-scheme
// secret never reaches the process at all. The workflow's own name mapping is
// the only migration path there, which is why acceptance.yml maps the new names
// and leaves the old secrets unreferenced and inert.
func AccEnv(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	old, ok := accLegacyEnvNames[name]
	if !ok {
		return ""
	}
	v := os.Getenv(old)
	if v != "" {
		if _, seen := accLegacyEnvUsed.LoadOrStore(old, true); !seen {
			log.Printf("acceptance: %s is deprecated, rename it to %s", old, name)
		}
	}
	return v
}
