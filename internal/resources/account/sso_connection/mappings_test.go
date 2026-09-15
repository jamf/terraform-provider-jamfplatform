// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// TestRenameTablesCoverEveryJamfValue is what makes a rename safe to ship.
//
// The accepted values, the documented list and the read-side translation are all
// derived from the SDK's own vocabularies, so a value Jamf adds is accepted
// rather than refused — but it would be accepted under Jamf's spelling while the
// rest of the set uses the console's, which is a worse outcome than a build
// failure. This is the build failure.
func TestRenameTablesCoverEveryJamfValue(t *testing.T) {
	for name, table := range map[string]struct {
		wire   []string
		toWire map[string]string
	}{
		"connection_type": {account.ConnectionTypeValues(), connectionTypeToWire},
		"auth_method":     {account.TokenEndpointAuthMethodValues(), authMethodToWire},
		"pkce":            {account.PkceAuthTypeValues(), pkceToWire},
	} {
		covered := make(map[string]bool, len(table.toWire))
		for _, v := range table.toWire {
			covered[v] = true
		}
		for _, wire := range table.wire {
			if !covered[wire] {
				t.Errorf("%s: Jamf accepts %q and this provider has no name for it — add one to mappings.go", name, wire)
			}
		}
		if len(table.toWire) != len(table.wire) {
			t.Errorf("%s: %d names map onto %d Jamf values — one of them names something Jamf no longer accepts",
				name, len(table.toWire), len(table.wire))
		}
	}
}

// TestRenameTablesRoundTrip pins that the two directions agree. A rename applied
// on the way out and not undone on the way in reads back as Jamf's spelling and
// gives a difference on every plan.
func TestRenameTablesRoundTrip(t *testing.T) {
	for name, pair := range map[string]struct {
		toWire   map[string]string
		fromWire map[string]string
	}{
		"connection_type": {connectionTypeToWire, connectionTypeFromWire},
		"auth_method":     {authMethodToWire, authMethodFromWire},
		"pkce":            {pkceToWire, pkceFromWire},
		"filter operator": {filterOperatorToWire, filterOperatorFromWire},
	} {
		if len(pair.toWire) != len(pair.fromWire) {
			t.Errorf("%s: the two directions have different sizes, so two names share a Jamf value", name)
		}
		for terraform, wire := range pair.toWire {
			if back := pair.fromWire[wire]; back != terraform {
				t.Errorf("%s: %q maps to %q, which maps back to %q", name, terraform, wire, back)
			}
		}
	}
}

// TestConnectionTypeValuesUseTheConsoleVocabulary pins the one rename that
// exists for a reason a reader has to know: WAAD is a product Microsoft renamed
// to Entra years ago, and the console says Entra.
func TestConnectionTypeValuesUseTheConsoleVocabulary(t *testing.T) {
	if got := connectionTypeToWire[connectionTypeEntra]; got != account.ConnectionTypeWaad {
		t.Errorf("entra maps to %q, want Jamf's own spelling for it", got)
	}
	for _, value := range connectionTypeValues() {
		if value != strings.ToLower(value) {
			t.Errorf("accepted value %q is not in the console's lower-case vocabulary", value)
		}
	}
}

// TestHostingRegionKeepsJamfsValues pins the deliberate divergence from the
// rename rule. One of the five regions has no console label at all, and the other
// four are conventional codes rather than jargon, so inventing labels would be
// worse than passing them through.
func TestHostingRegionKeepsJamfsValues(t *testing.T) {
	for _, region := range account.RegionValues() {
		if strings.ToLower(region) == region {
			t.Errorf("region %q looks renamed; hosting_region deliberately keeps Jamf's own values", region)
		}
	}
	if _, renamed := connectionTypeToWire[account.RegionUs]; renamed {
		t.Error("a region has been added to the connection type rename table")
	}
}

// TestProductDocsNameTheCrypticIdentifiers pins that the two product identifiers
// a reader cannot resolve on sight are documented, since the values themselves
// keep Jamf's spelling.
func TestProductDocsNameTheCrypticIdentifiers(t *testing.T) {
	docs := productDocs()
	for _, want := range []string{"Jamf Executive Threat Protection", "Jamf Security Cloud"} {
		if !strings.Contains(docs, want) {
			t.Errorf("the product vocabulary does not explain %q:\n%s", want, docs)
		}
	}
	for _, product := range productValues() {
		if !strings.Contains(docs, "`"+product+"`") {
			t.Errorf("the product vocabulary omits %q:\n%s", product, docs)
		}
	}
}

// TestMarkdownValueListIsSortedAndQuoted pins the documented rendering, so a
// change in the order Jamf declares a vocabulary does not churn the published
// documentation.
func TestMarkdownValueListIsSortedAndQuoted(t *testing.T) {
	got := markdownValueList([]string{"zebra", "apple"})
	if got != "`apple`, `zebra`" {
		t.Errorf("markdownValueList = %q, want a sorted, quoted list", got)
	}
}

// TestRenamedValuesFallsBackToJamfsSpelling pins the fallback. A value Jamf adds
// before this provider names it stays usable rather than being silently
// unaccepted; the coverage test above is what reports the gap.
func TestRenamedValuesFallsBackToJamfsSpelling(t *testing.T) {
	got := renamedValues([]string{account.ConnectionTypeOidc, "SOMETHING_NEW"}, connectionTypeFromWire)
	if len(got) != 2 || got[0] != connectionTypeGenericOIDC || got[1] != "SOMETHING_NEW" {
		t.Errorf("renamedValues = %v, want the known value renamed and the unknown one carried through", got)
	}
}
