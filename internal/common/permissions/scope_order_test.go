// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package permissions

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// providerdataScopeKinds maps the provider's own ScopeKind onto the SDK's, which
// is the enum this package orders. It is spelled out rather than derived because
// deriving it would need the very agreement the test below exists to check.
var providerdataScopeKinds = map[providerdata.ScopeKind]jamfplatform.ScopeKind{
	providerdata.ScopeEnvironment:  jamfplatform.ScopeEnvironment,
	providerdata.ScopeTenant:       jamfplatform.ScopeTenant,
	providerdata.ScopeOrganization: jamfplatform.ScopeOrganization,
}

// TestScopeOrderAgreesWithTheAuthorizationPath pins the two orderings together.
//
// One user-facing decision is expressed twice. This package's scopeOrder decides
// which scope a construct's documentation marks preferred; providerdata's decides
// which scope RequireScope's diagnostic offers first when it refuses one. Until
// now each was checked only against itself, so reordering either would silently
// have the published page recommend a first choice the diagnostic does not — and
// an operator following the page would create the integration the provider then
// lists second. Nothing else in the repo compares them.
//
// The import is test-only and runs one way. providerdata does not import this
// package, so nothing here creates a cycle in production code.
func TestScopeOrderAgreesWithTheAuthorizationPath(t *testing.T) {
	gate := providerdata.ScopeOrder()
	if len(gate) != len(scopeOrder) {
		t.Fatalf("providerdata orders %d scope kinds and this package orders %d — one of them grew a "+
			"kind the other has not; a set neither can render is worse than an unordered one",
			len(gate), len(scopeOrder))
	}
	for i, k := range gate {
		want, ok := providerdataScopeKinds[k]
		if !ok {
			t.Errorf("providerdata.ScopeOrder()[%d] is %s, which this test has no SDK kind for — add it "+
				"rather than letting the comparison skip a position", i, k)
			continue
		}
		if scopeOrder[i] != want {
			t.Errorf("position %d: providerdata offers %s and this package renders %s — the diagnostic "+
				"and the documentation would recommend different first choices", i, k, scopeLabels[scopeOrder[i]])
		}
	}
}
