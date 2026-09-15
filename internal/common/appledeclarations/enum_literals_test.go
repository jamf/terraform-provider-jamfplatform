// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package appledeclarations

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error codes come from the
// SDK, not from literals" for this package. See internal/common/enumguard for what the walker
// covers.
//
// The one vocabulary here is the declaration kind KindForType derives from a type's reverse-domain
// prefix. The guard sees it only because the four values are declared as consts rather than
// returned as literals from the switch: enumguard deliberately does not reach a literal that is
// merely returned, so a switch arm spelling "ASSET" inline would leave this guard unable to protect
// its own source.
//
// ACTIVATION and MANAGEMENT sit in Absent, which is what makes this test the tripwire for the
// gateway widening recorded on the const group: the day a spec ingest promotes either into
// DeclarationKindValues(), the promotion fails here rather than going unnoticed.
//
// The Kind constants (boolean, string, dictionary, …) are Apple's own value-type vocabulary from
// the generated schema table, not a Jamf enum, so no SDK helper covers them and none should.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(blueprints.DeclarationKindValues()),
		Absent: map[string]string{
			"ACTIVATION": "blueprints.DeclarationKindValues() declares only CONFIGURATION and ASSET; the gateway accepted and stored ACTIVATION verbatim on 2026-09-10",
			"MANAGEMENT": "blueprints.DeclarationKindValues() declares only CONFIGURATION and ASSET; the gateway accepted and stored MANAGEMENT verbatim on 2026-09-10",
		},
		Ignore: map[string]string{
			"any":        "Apple's own value-type vocabulary in the generated schema table, not a Jamf enum",
			"array":      "Apple's own value-type vocabulary in the generated schema table, not a Jamf enum",
			"boolean":    "Apple's own value-type vocabulary in the generated schema table, not a Jamf enum",
			"data":       "Apple's own value-type vocabulary in the generated schema table, not a Jamf enum",
			"date":       "Apple's own value-type vocabulary in the generated schema table, not a Jamf enum",
			"dictionary": "Apple's own value-type vocabulary in the generated schema table, not a Jamf enum",
			"integer":    "Apple's own value-type vocabulary in the generated schema table, not a Jamf enum",
			"real":       "Apple's own value-type vocabulary in the generated schema table, not a Jamf enum",
			"string":     "Apple's own value-type vocabulary in the generated schema table, not a Jamf enum",
			"com.apple.configuration.management.status-subscriptions": "a declaration type Apple publishes, not an enum value",
		},
	})
	if err != nil {
		t.Fatalf("enumguard.Check: %v", err)
	}
	for _, problem := range got.Problems() {
		t.Error(problem)
	}
	if got.Examined == 0 {
		t.Fatal("no string literals parsed — the guard found nothing to check")
	}
}
