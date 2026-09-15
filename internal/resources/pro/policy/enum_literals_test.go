// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package.
//
// Two vocabularies the audit flagged here stay literals and are worth naming,
// because both look like they have a constant and do not:
//
//   - self_service.notification_location ("Self Service", "Self Service and
//     Notification Center") collides with the patch-policy notification
//     location, a different construct. The classic /policies spec models the
//     field as a plain string.
//   - deferral_type ("none", "date", "duration") is a provider-side
//     discriminator over three wire fields, not a wire enum at all.
//
// Neither appears in Covered, so neither needs an exemption — they are listed
// here so a reader does not have to re-derive why.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			proclassic.PolicyGeneralDateTimeLimitationsNoExecuteOnDayValues(),
			proclassic.PolicyAccountMaintenanceAccountsAccountItemActionValues(),
			proclassic.PolicyPrintersPrinterItemActionValues(),
			proclassic.PolicyGeneralNetworkRequirementsValues(),
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
