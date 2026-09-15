// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error codes
// come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// The three value vocabularies are keyed on SDK constants in mappings.go, so nothing
// restates them. The error codes are a different matter: the spec documents every one
// of them in prose on the `409` response, but only NOT_ENTITLED reaches the generated
// `ApiErrorItemCode` enum — that enum is declared from the DNS schema and carries
// none of the ZTNA app codes. Each is therefore exempted by name below rather than as
// a set, because "the SDK has none of these" is a claim that has been wrong before
// when a generated enum carried the generic codes and none of a construct's own.
//
// Absent rather than Ignore, deliberately: these are values the SDK does not yet
// generate, not members of a different vocabulary that happen to share a spelling. So
// an SDK release that starts generating one fails this test and the literal gets
// replaced with the constant, which is exactly the drift this guard is for.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			securitycloud.ApiErrorItemCodeValues(),
			securitycloud.RoutingTypeValues(),
			securitycloud.RoutingDnsIpResolutionTypeValues(),
			securitycloud.RiskControlsLevelThresholdValues(),
		),
		Absent: map[string]string{
			"HOSTNAME_CONFLICT":        "documented in the ZTNA spec's 409 prose but absent from the generated ApiErrorItemCode enum, which is declared from the DNS schema",
			"BARE_IPS_CONFLICT":        "documented in the ZTNA spec's 409 prose; not in the generated enum",
			"MISSING_CATEGORY_NAME":    "documented in the ZTNA spec's 409 prose; not in the generated enum",
			"MISSING_USER_GROUPS":      "documented in the ZTNA spec's 409 prose; not in the generated enum",
			"PREDEFINED_APP_NOT_FOUND": "documented in the ZTNA spec's 409 prose; not in the generated enum",
			"CONFLICT":                 "the spec's generic 409 fallback code; not in the generated enum",
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
