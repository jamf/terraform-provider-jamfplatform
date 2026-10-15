// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"slices"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/appleschemas/declarations"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

// TestDeclarationKindWideningIsStillAWidening is the tripwire for the gateway widening recorded on
// the const group in the SDK's appleschemas/declarations: blueprints.DeclarationKindValues()
// declares only CONFIGURATION and ASSET, yet POST /blueprints/v1/blueprints accepted ACTIVATION and
// MANAGEMENT and stored both verbatim (EU gateway, 2026-09-10). input_builders.go sends whatever
// declarations.KindForType derives, so the provider is the consumer that depends on it.
//
// The SDK carries no guard of its own for the day a spec ingest promotes either value into
// DeclarationKindValues(); when this fails, the widening has ended and the spec is the authority,
// so delete the corresponding entry here rather than editing it.
func TestDeclarationKindWideningIsStillAWidening(t *testing.T) {
	widened := map[string]string{
		"com.apple.activation.simple":                            "ACTIVATION",
		"com.apple.management.status-subscriptions-not-declared": "MANAGEMENT",
	}
	declared := blueprints.DeclarationKindValues()
	for declarationType, kind := range widened {
		if got := declarations.KindForType(declarationType); got != kind {
			t.Errorf("KindForType(%q) = %q, want %q: the SDK no longer derives the widened kind", declarationType, got, kind)
		}
		if slices.Contains(declared, kind) {
			t.Errorf("blueprints.DeclarationKindValues() now declares %q: the gateway widening has ended, so remove %q from this test", kind, declarationType)
		}
	}
}
