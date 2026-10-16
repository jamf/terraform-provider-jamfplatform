// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_store_app

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/appstore"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error codes come from the
// SDK, not from literals" for this package. The SDK's Platform constants carry the wire values;
// the target labels are this package's own vocabulary and must never restate one.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(appstore.PlatformValues()),
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
