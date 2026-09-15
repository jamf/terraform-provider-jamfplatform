// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package providerdata

import (
	"strings"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
)

// TestScopeKindFromClientMapsEveryKnownKind pins the mapping New relies on, at
// the level the client cannot reach: TestNewDerivesScopeFromClient covers the
// same ground through the SDK options, but only the two kinds an option can
// select, so the organization kind arriving with an ID is only checkable here.
func TestScopeKindFromClientMapsEveryKnownKind(t *testing.T) {
	for sdk, want := range map[jamfplatform.ScopeKind]ScopeKind{
		jamfplatform.ScopeOrganization: ScopeOrganization,
		jamfplatform.ScopeEnvironment:  ScopeEnvironment,
		jamfplatform.ScopeTenant:       ScopeTenant,
	} {
		if got := scopeKindFromClient(sdk, "scope-id"); got != want {
			t.Errorf("scopeKindFromClient(%s) = %s, want %s", sdk, got, want)
		}
	}
}

// TestScopeKindFromClientPanicsOnAnUnrecognisedKind is the client-side twin of
// TestScopeKindFromSDKPanicsOnAnUnknownKind, and the direction is the point: a
// silent default would resolve to ScopeOrganization, the sole member of
// AccountScopes and so the one scope that passes the jamfplatform_account_*
// gate. Every other family would reject it, which makes an unrecognised kind
// fail closed at 26 call sites and fail open at the one where it counts.
func TestScopeKindFromClientPanicsOnAnUnrecognisedKind(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("scopeKindFromClient returned for an unrecognised SDK kind instead of panicking")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "no ScopeKind for") {
			t.Errorf("panic value %v does not name the missing mapping", r)
		}
	}()
	scopeKindFromClient(jamfplatform.ScopeKind(99), "scope-id")
}

// TestScopeKindFromClientTreatsAnEmptyIDAsOrganization bounds what the panic
// above actually guards. Client.Scope() returns an empty ID for every kind that
// carries no request header, so a fourth kind the SDK grows without a header
// resolves to organization rather than reaching the panic — which is correct,
// because a client sending no scope header is organization-scoped whatever its
// author intended.
func TestScopeKindFromClientTreatsAnEmptyIDAsOrganization(t *testing.T) {
	for _, sdk := range []jamfplatform.ScopeKind{
		jamfplatform.ScopeOrganization,
		jamfplatform.ScopeEnvironment,
		jamfplatform.ScopeTenant,
		jamfplatform.ScopeKind(99),
	} {
		if got := scopeKindFromClient(sdk, ""); got != ScopeOrganization {
			t.Errorf("scopeKindFromClient(%d, \"\") = %s, want %s", sdk, got, ScopeOrganization)
		}
	}
}
