// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error codes
// come from the SDK, not from literals" for this package. See
// internal/common/enumguard for what the walker covers.
//
// This package was the only Security Cloud package with a write path and no such
// guard, which matters here more than anywhere: it carries seven error-code literals,
// the most of any construct in the namespace, and every one of them is a literal for
// the same structural reason. The generated `ApiErrorItemCode` enum is declared from
// the DNS schema, so it carries the DNS vocabulary and NOT_ENTITLED and nothing else;
// the ZTNA codes appear in the spec only as response examples, which the generator
// does not emit.
//
// Each is exempted by name rather than as a set, because "the SDK has none of these"
// is a claim that has been wrong before when a generated enum carried the generic
// codes and none of a construct's own.
//
// Absent rather than Ignore, deliberately: these are values the SDK does not yet
// generate, not members of a different vocabulary that happen to share a spelling. So
// an SDK release that starts generating one fails this test and the literal gets
// replaced with the constant, which is exactly the drift this guard is for.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			securitycloud.ApiErrorItemCodeValues(),
			securitycloud.GatewayCreateRequestDatacenterValues(),
			securitycloud.GatewayStatusStateValues(),
			securitycloud.RoutingStrategyValues(),
		),
		Absent: map[string]string{
			"GATEWAY_TYPE_CHANGE_NOT_SUPPORTED": "documented in the ZTNA spec's 4xx prose but absent from the generated ApiErrorItemCode enum, which is declared from the DNS schema",
			"IPSEC_SECRET_CLEAR_NOT_SUPPORTED":  "documented in the ZTNA spec's 4xx prose; not in the generated enum",
			"DEDICATED_IPS_LIMIT":               "taken from the spec's shared 409 catalogue; not in the generated enum",
			"BAD_REQUEST":                       "the spec's generic 400 fallback code; not in the generated enum",

			"GATEWAY_REFERENCED_BY_ACCESS_POLICIES":  "wire-probed 2026-08-30 on the gateway delete path; documented in the spec's 409 prose but not in the generated enum",
			"GATEWAY_REFERENCED_BY_DNS_ZONES":        "wire-probed 2026-08-30 on the gateway delete path; not in the generated enum",
			"GATEWAY_REFERENCED_BY_GROUPED_GATEWAYS": "wire-probed 2026-08-30 on the gateway delete path; not in the generated enum",
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
