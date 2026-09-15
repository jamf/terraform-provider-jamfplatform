// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"sort"
	"strings"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// Machine-readable error codes Jamf Account returns on the SSO connection
// operations. Each is translated into a diagnostic that says what to do, because
// the raw message names a code and not a fix.
//
// These are restated rather than aliased because the Jamf Account SDK package
// generates no error-code vocabulary at all: ApiErrorItem.Code is a plain string,
// and the six values its doc comment names are prose with no constant and no
// *Values() helper behind them. That is the genuinely-absent exemption in
// STYLE_GUIDE §"Enum values and error codes come from the SDK, not from
// literals", and enum_literals_test.go records it per value so an SDK release
// that starts generating them fails the test rather than passing silently.
//
// FIELD_VALIDATION is not in the SDK's documented list at all — it was observed
// only on the wire, refusing an absent top-level field — so no future alias can
// be assumed for it either.
const (
	codeUpstreamError   = "UPSTREAM_ERROR"
	codeBadRequest      = "BAD_REQUEST"
	codeFieldValidation = "FIELD_VALIDATION"
	codeNotFound        = "NOT_FOUND"
)

// Terraform's connection_type vocabulary. The values are renamed from Jamf's
// because one of them is unguessable: WAAD stands for "Windows Azure Active
// Directory", a product Microsoft renamed to Entra years ago, and the Jamf
// Account console says "Entra". Renaming one member and keeping three would be
// worse than renaming the set, so the whole vocabulary follows the console.
const (
	connectionTypeGenericOIDC = "generic_oidc"
	connectionTypeEntra       = "entra"
	connectionTypeOkta        = "okta"
	connectionTypeGoogle      = "google_workspace"
)

// Terraform's auth_method vocabulary. Jamf's spelling describes a protocol
// detail; the console names the choice an administrator makes.
const (
	authMethodClientSecret  = "client_secret"
	authMethodPrivateKeyJWT = "private_key_jwt"
)

// Terraform's pkce vocabulary — Jamf's values lower-cased, because the console
// renders them as sentence-case labels rather than as constants.
const (
	pkceDisabled = "disabled"
	pkceAuto     = "auto"
	pkceS256     = "s256"
	pkcePlain    = "plain"
)

// Terraform's group_name_filter operator vocabulary, and the values Jamf stores
// for it. The console renders this as an AND/OR toggle beside the group list.
const (
	filterOperatorOr  = "or"
	filterOperatorAnd = "and"

	filterOpOr  = "OR"
	filterOpAnd = "AND"
)

// Attribute-map modes observed across every readable connection. Jamf serves no
// schema for the attribute map and documents no vocabulary for it, so these are
// a survey finding rather than a declared set — which is why an unrecognised
// mode is a warning and not an error. enum_literals_test.go records each as
// absent from the SDK.
const (
	mappingModeBindAll      = "bind_all"
	mappingModeBasicProfile = "basic_profile"
	mappingModeUseMap       = "use_map"
)

// mappingModeKey is the property the attribute map carries its mode in.
const mappingModeKey = "mapping_mode"

// filterOpKey and filterGroupsKey are the two properties the group filter
// document carries.
const (
	filterOpKey     = "op"
	filterGroupsKey = "groups"
)

// connectionTypeToWire maps Terraform's connection_type onto Jamf's, keyed on
// the SDK's generated constants so a renamed value fails the build rather than
// drifting.
var connectionTypeToWire = map[string]string{
	connectionTypeGenericOIDC: account.ConnectionTypeOidc,
	connectionTypeEntra:       account.ConnectionTypeWaad,
	connectionTypeOkta:        account.ConnectionTypeOkta,
	connectionTypeGoogle:      account.ConnectionTypeGoogleApps,
}

// authMethodToWire maps Terraform's auth_method onto Jamf's.
var authMethodToWire = map[string]string{
	authMethodClientSecret:  account.TokenEndpointAuthMethodClientSecretPost,
	authMethodPrivateKeyJWT: account.TokenEndpointAuthMethodPrivateKeyJwt,
}

// pkceToWire maps Terraform's pkce onto Jamf's.
var pkceToWire = map[string]string{
	pkceDisabled: account.PkceAuthTypeDisabled,
	pkceAuto:     account.PkceAuthTypeAuto,
	pkceS256:     account.PkceAuthTypeS256,
	pkcePlain:    account.PkceAuthTypePlain,
}

// filterOperatorToWire maps Terraform's group filter operator onto the value
// Jamf stores. Jamf declares no vocabulary for it — the operator lives inside an
// opaque document — so both sides of this table are this package's own.
var filterOperatorToWire = map[string]string{
	filterOperatorOr:  filterOpOr,
	filterOperatorAnd: filterOpAnd,
}

// connectionTypeFromWire, authMethodFromWire, pkceFromWire and
// filterOperatorFromWire are the read-side inverses, built once at init so the
// two directions cannot disagree.
var (
	connectionTypeFromWire = invert(connectionTypeToWire)
	authMethodFromWire     = invert(authMethodToWire)
	pkceFromWire           = invert(pkceToWire)
	filterOperatorFromWire = invert(filterOperatorToWire)
)

// invert reverses a rename table.
func invert(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[v] = k
	}
	return out
}

// connectionTypeValues returns Terraform's accepted connection_type values, in
// the order Jamf declares the underlying vocabulary.
//
// Derived from the SDK's own ConnectionTypeValues() rather than restated, so a
// value Jamf adds shows up as a missing rename in mappings_test.go instead of
// silently going unaccepted.
func connectionTypeValues() []string {
	return renamedValues(account.ConnectionTypeValues(), connectionTypeFromWire)
}

// authMethodValues returns Terraform's accepted auth_method values.
func authMethodValues() []string {
	return renamedValues(account.TokenEndpointAuthMethodValues(), authMethodFromWire)
}

// pkceValues returns Terraform's accepted pkce values.
func pkceValues() []string {
	return renamedValues(account.PkceAuthTypeValues(), pkceFromWire)
}

// filterOperatorValues returns Terraform's accepted group filter operators.
func filterOperatorValues() []string {
	return []string{filterOperatorOr, filterOperatorAnd}
}

// renamedValues projects a Jamf vocabulary through a rename table, falling back
// to Jamf's own spelling for a value the table does not cover. The fallback
// keeps a newly-added value usable rather than unaccepted; mappings_test.go is
// what fails, so the gap is reported rather than shipped.
func renamedValues(wire []string, fromWire map[string]string) []string {
	out := make([]string, 0, len(wire))
	for _, v := range wire {
		if renamed, ok := fromWire[v]; ok {
			out = append(out, renamed)
			continue
		}
		out = append(out, v)
	}
	return out
}

// markdownValueList renders a vocabulary for an attribute description, sorted so
// the documented order is stable across releases.
func markdownValueList(values []string) string {
	sorted := make([]string, len(values))
	copy(sorted, values)
	sort.Strings(sorted)

	parts := make([]string, 0, len(sorted))
	for _, v := range sorted {
		parts = append(parts, "`"+v+"`")
	}
	return strings.Join(parts, ", ")
}

// productUILabels names the Jamf Account console label for each product whose
// identifier does not say plainly what it is. The rest are self-describing and
// documenting them would be noise.
var productUILabels = map[string]string{
	account.ProductJetp:          "Jamf Executive Threat Protection",
	account.ProductSecurityCloud: "Jamf Security Cloud",
}

// productValues returns the accepted product identifiers.
//
// These keep Jamf's own spelling, which diverges from the general rule that a
// value follows the console. Seven of the nine are the product's own name in
// upper case and inventing a label for them would add a translation with nothing
// to translate; the two that are not are documented instead. hosting_region is
// kept for the same reason and recorded in resource.go.
func productValues() []string {
	return account.ProductValues()
}

// productDocs renders the product vocabulary, annotating the two identifiers a
// reader cannot resolve on sight.
func productDocs() string {
	values := productValues()
	sorted := make([]string, len(values))
	copy(sorted, values)
	sort.Strings(sorted)

	parts := make([]string, 0, len(sorted))
	for _, v := range sorted {
		if label, ok := productUILabels[v]; ok {
			parts = append(parts, "`"+v+"` ("+label+")")
			continue
		}
		parts = append(parts, "`"+v+"`")
	}
	return strings.Join(parts, ", ")
}
