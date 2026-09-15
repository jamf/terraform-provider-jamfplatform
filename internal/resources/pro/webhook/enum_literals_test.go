// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package webhook

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package.
//
// This is the package the rule's exemption clause was written for. Three of the
// four vocabularies here are supersets of what the spec models, because the
// values came out of a live wire probe rather than the spec — so a blanket
// "the SDK has none of these" comment would be false of 20 of the 28 values.
// Every remaining literal is exempted individually below, and the guard fails
// if any of them turns out to be generated after all.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			proclassic.WebhookEventValues(),
			proclassic.WebhookAuthenticationTypeValues(),
			proclassic.WebhookContentTypeValues(),
		),
		Absent: map[string]string{
			"ComputerPatchPolicyCompleted":   "event accepted by a live tenant; absent from the classic spec",
			"DeviceAddedToDEP":               "event accepted by a live tenant; absent from the classic spec",
			"DeviceRateLimited":              "event accepted by a live tenant; absent from the classic spec",
			"MobileDeviceInventoryCompleted": "event accepted by a live tenant; absent from the classic spec",
			"SmartGroupUserMembershipChange": "event accepted by a live tenant; absent from the classic spec",
			"HEADER":                         "authentication type accepted by a live tenant; the spec models only NONE and BASIC",
			"HASH_SIGNATURE":                 "authentication type accepted by a live tenant; the spec models only NONE and BASIC",
			"MTLS":                           "authentication type accepted by a live tenant; the spec models only NONE and BASIC",
			"SHA256":                         "hash_algorithm is not modelled by the spec at all",
			"SHA512":                         "hash_algorithm is not modelled by the spec at all",
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
