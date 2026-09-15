// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package.
//
// Nothing here needs aliasing, and the two exemptions are the reason to have
// the test at all: both values collide with an SDK vocabulary that is not the
// one this resource writes. A single member matching while the rest of the set
// does not is the signature of a coincidental collision, and taking the
// constant anyway is how a wrong-vocabulary spelling reaches the wire.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			pro.PackageManifestHashTypeValues(),
			pro.CloudDistributionPointInventoryFileInfoStatusValues(),
		),
		Ignore: map[string]string{
			"MD5":   "hash_type is the package hash vocabulary (MD5 / SHA_256 / SHA_512 / SHA3_512), wire-probed; pro.PackageManifestHashType is the *manifest* hash type and spells its SHA member SHA256, so only MD5 collides and borrowing it would tie this set to an unrelated vocabulary",
			"READY": "cloudTransferStatus has no generated enum; pro.CloudDistributionPointInventoryFileInfoStatus is the CDP inventory file status, a different field",
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
