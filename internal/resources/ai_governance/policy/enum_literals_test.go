// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/enumguard"
)

// TestEnumLiteralsComeFromTheSDK pins STYLE_GUIDE.md §"Enum values and error codes come from the
// SDK, not from literals" for this package. See internal/common/enumguard for what the walker
// covers.
//
// Every error code this package restates is checked individually against the SDK's generated sets,
// not asserted as a group: the recurring defect this guard exists to catch is a comment claiming the
// SDK carries none of a set of codes when it carries some of them.
func TestEnumLiteralsComeFromTheSDK(t *testing.T) {
	got, err := enumguard.Check(enumguard.Params{
		Covered: enumguard.Union(
			aigovernance.PolicyDetailStatusValues(),
			aigovernance.PolicySummaryStatusValues(),
			aigovernance.BlueprintDeploymentStateValues(),
			aigovernance.DeploymentRunStateValues(),
		),
		Absent: map[string]string{
			"TOOL_ID_UNKNOWN":              "aigovernance.ApiErrorItem.Code is a plain string; the SDK generates no error-code enum for this namespace.",
			"SCHEMA_VERSION_UNKNOWN":       "as above — no generated error-code vocabulary exists to alias.",
			"SCHEMA_VALIDATION_FAILED":     "as above — no generated error-code vocabulary exists to alias.",
			"VALIDATION_FAILED":            "as above — no generated error-code vocabulary exists to alias.",
			"POLICY_NOT_FOUND":             "as above — no generated error-code vocabulary exists to alias.",
			"NO_DRAFT_TO_PUBLISH":          "as above — no generated error-code vocabulary exists to alias.",
			"POLICY_VERSION_CONFLICT":      "as above. v2121 added the code with the If-Match precondition, but names it only in the parameter, operation and 409 response descriptions — never in an enum, so the generator emits nothing for it.",
			"REQUEST_CONTEXT_NOT_PROVIDED": "the platform gateway's own pre-routing code; it belongs to no service spec, so no SDK package generates it.",
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
