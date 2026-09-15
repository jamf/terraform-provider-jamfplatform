// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uemconnectactions

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error
// codes come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// Every code is checked against the SDK enum individually rather than as a set:
// securitycloud.ApiErrorItemCode is the DNS namespace's error schema, so it carries
// the generic NOT_ENTITLED and INVALID_FIELD while carrying none of the UEM Connect
// codes. Reasoning about the set instead of the members is how two other packages
// shipped a restated NOT_ENTITLED under a comment asserting the SDK had no such
// code.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			securitycloud.ApiErrorItemCodeValues(),
			securitycloud.ActivationProfileDeployRequestPlatformValues(),
		),
		Absent: map[string]string{
			"CONNECTOR_DISABLED":           "the refusal to synchronize or deploy through a disabled integration. The UEM Connect spec declares no error-code enum of its own, so no generated constant exists",
			"CONNECTOR_NOT_CONNECTED":      "the refusal to deploy before the integration has connected to Jamf Pro; absent for the same reason",
			"CONNECTOR_MISCONFIGURED":      "the refusal for a group ID that does not exist or is the wrong kind for the chosen os. Undocumented in the spec entirely, let alone generated",
			"ACTIVATION_PROFILE_NOT_FOUND": "the 404 code from the deploy. The spec documents NOT_FOUND and NO_ACTIVATION_PROFILE here and the server sends neither, so there was never a generated constant to key on",
			"MULTIPLE_ACTIVATION_PROFILES": "the ambiguous-target refusal; documented in prose in the spec's 409 description but not as an enum member",
			"VALIDATION_FAILED":            "this service's catch-all body-validation code, which is why the group-ID diagnostic also matches on the description. Not an enum member anywhere in the SDK",
			"JAMF":                         "the only UEM platform the deploy accepts. The spec declares this enum inline on the request body rather than as a named schema, so the generator emits no type and no constant for it",
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
