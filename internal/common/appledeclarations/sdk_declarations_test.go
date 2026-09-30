// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package appledeclarations

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

// sdkDeclarationVariants pairs each declaration type the SDK's com.jamf.ddm-strict union declares
// with the kind that variant declares, both taken from the variant's own generated helpers so the
// pairing is the spec's rather than one restated here.
func sdkDeclarationVariants() map[string][]string {
	return map[string][]string{
		blueprints.AppSettingsDeclarationTypeValues()[0]:                  blueprints.AppSettingsDeclarationKindValues(),
		blueprints.ContentCachingDeclarationTypeValues()[0]:               blueprints.ContentCachingDeclarationKindValues(),
		blueprints.ExternalIntelligenceSettingsDeclarationTypeValues()[0]: blueprints.ExternalIntelligenceSettingsDeclarationKindValues(),
		blueprints.IntelligenceSettingsDeclarationTypeValues()[0]:         blueprints.IntelligenceSettingsDeclarationKindValues(),
		blueprints.KeyboardSettingsDeclarationTypeValues()[0]:             blueprints.KeyboardSettingsDeclarationKindValues(),
		blueprints.ManagedAppDeclarationTypeValues()[0]:                   blueprints.ManagedAppDeclarationKindValues(),
		blueprints.PackageDeclarationTypeValues()[0]:                      blueprints.PackageDeclarationKindValues(),
		blueprints.ScreenSharingConnectionDeclarationTypeValues()[0]:      blueprints.ScreenSharingConnectionDeclarationKindValues(),
		blueprints.ScreenSharingConnectionGroupDeclarationTypeValues()[0]: blueprints.ScreenSharingConnectionGroupDeclarationKindValues(),
		blueprints.ScreenSharingHostSettingsDeclarationTypeValues()[0]:    blueprints.ScreenSharingHostSettingsDeclarationKindValues(),
		blueprints.SiriSettingsDeclarationTypeValues()[0]:                 blueprints.SiriSettingsDeclarationKindValues(),
	}
}

// TestTableCoversEverySDKDeclarationType asserts that every declaration type the Blueprints API
// specification lists for com.jamf.ddm-strict is one the embedded Apple table knows.
//
// The spec's list is not the vocabulary the provider validates against, and must not become it:
// the service accepts and stores declaration types the list omits (the SDK's own wire facts,
// 2026-09-30, record `com.apple.configuration.bogus` answering 201), so restricting
// apple_declarations to these eleven would refuse configurations Jamf takes. What the list does
// prove is the other direction. A type Jamf documents for this component and the table has never
// heard of is refused at plan time as an unknown declaration type, so this failing means the table
// is behind the spec and `make apple-schemas` is the fix.
//
// sdkDeclarationVariants must enumerate the union exactly, or a type added to it by a spec ingest
// would never be checked; the length and membership comparison against the union's own Values()
// helper is what makes that list complete rather than a sample.
func TestTableCoversEverySDKDeclarationType(t *testing.T) {
	union := blueprints.DeclarationsComponentConfigurationDeclarationsItemTypeValues()
	variants := sdkDeclarationVariants()
	if len(union) == 0 {
		t.Fatal("the SDK's ddm-strict union declares no declaration types; the check would pass vacuously")
	}
	if len(variants) != len(union) {
		t.Errorf("sdkDeclarationVariants names %d types but the SDK's ddm-strict union declares %d; add the new variant's Type and Kind helpers", len(variants), len(union))
	}
	for _, declarationType := range union {
		if _, ok := variants[declarationType]; !ok {
			t.Errorf("the SDK's ddm-strict union declares %q, which sdkDeclarationVariants does not pair with a kind", declarationType)
		}
		if _, ok := Lookup(declarationType); !ok {
			t.Errorf("the SDK's ddm-strict union declares %q, which the embedded Apple table does not know (%s); run make apple-schemas", declarationType, ProvenanceSummary())
		}
	}
}

// TestKindForTypeAgreesWithSDK asserts that the kind KindForType derives from a declaration type's
// prefix is the kind the Blueprints API specification declares for that type. KindForType is a
// prefix rule precisely so that it needs no table, which also means nothing but a test can notice
// the spec pairing a type with a different kind.
func TestKindForTypeAgreesWithSDK(t *testing.T) {
	for declarationType, kinds := range sdkDeclarationVariants() {
		if len(kinds) != 1 {
			t.Errorf("the SDK declares %d kinds for %q, want exactly one: %v", len(kinds), declarationType, kinds)
			continue
		}
		if got := KindForType(declarationType); got != kinds[0] {
			t.Errorf("KindForType(%q) = %q, but the SDK declares kind %q for it", declarationType, got, kinds[0])
		}
	}
}
