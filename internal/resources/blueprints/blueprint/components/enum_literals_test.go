// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// The last two sets are the discriminators of the bookmark and software-update
// unions, and the guard cannot see the sites that matter: every use here is a
// struct field, a map assignment or a switch case, all of which are uses rather
// than declarations. They are listed anyway so a literal declared here later — a
// OneOf vocabulary, a builder naming the one value it sends — is caught, and the
// use sites are keyed on the constants for the reason the guard exists: a literal
// keeps compiling after the spec renames the value it names, so the case simply
// stops matching and the branch behind it silently never runs.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			blueprints.UnpairingTimePolicyValues(),
			blueprints.StorageModeValueValues(),
			blueprints.AcceptCookiesValueValues(),
			blueprints.NewTabStartPagePageTypeValues(),
			blueprints.AutomaticActionValueValues(),
			blueprints.RecommendedCadenceValueValues(),
			blueprints.BookmarkItemTypeValues(),
			blueprints.SwUpdateAutomaticConfigurationStrategyValues(),
			blueprints.SwUpdateConfigurationEnforcementTypeValues(),
		),
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
