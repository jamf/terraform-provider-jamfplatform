// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package benchmark_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	cbSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	dgSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// baselineValidationConfig declares one benchmark over baselineID with the given rule blocks and an
// optional selected_os_versions fragment.
func baselineValidationConfig(baselineID, groupID, ruleBlocks, selectedOSVersions string) string {
	return fmt.Sprintf(`
resource "jamfplatform_cbengine_benchmark" "test" {
  title                = %q
  source_baseline_id   = %q
  enforcement_mode     = "MONITOR"
  target_device_groups = [%q]
  rules                = [%s]
  %s
}
`, "tf-acc-baseline-validation-"+testhelpers.RunSuffix(), baselineID, groupID, ruleBlocks, selectedOSVersions)
}

func baselineValidationGroup(t *testing.T, ctx context.Context) string {
	t.Helper()
	groups, err := dgSDK.New(testhelpers.NewAcceptanceClient(t)).ListDeviceGroups(ctx, nil, "")
	if err != nil || len(groups) == 0 {
		t.Skip("No device group available to target")
	}
	return groups[0].ID
}

// TestAccResource_Benchmark_PlanRejectsWhatTheBaselineRefuses plans configurations the API refuses at
// create, and expects each to fail at plan with a diagnostic naming the problem. Nothing is created.
func TestAccResource_Benchmark_PlanRejectsWhatTheBaselineRefuses(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ctx := context.Background()
	cb := cbSDK.New(testhelpers.NewAcceptanceClient(t))
	groupID := baselineValidationGroup(t, ctx)

	const baselineID = "cis_lvl1"
	baseline, err := cb.GetBaselineRules(ctx, baselineID)
	if err != nil {
		t.Skipf("baseline %s unavailable: %v", baselineID, err)
	}

	var withoutODV, boundedInteger string
	var maximum int
	for _, r := range baseline.Rules {
		if r.ODV == nil && withoutODV == "" {
			withoutODV = r.ID
		}
		if r.ODV != nil && r.ODV.Type == cbSDK.OrganizationDefinedValueTypeInteger && r.ODV.Validation != nil && r.ODV.Validation.Max != 0 && boundedInteger == "" {
			boundedInteger, maximum = r.ID, r.ODV.Validation.Max
		}
	}
	if withoutODV == "" || boundedInteger == "" {
		t.Skip("baseline has no rule without an ODV, or no bounded INTEGER ODV")
	}

	cases := []struct {
		name    string
		rules   string
		osFrag  string
		pattern string
	}{
		{
			name:    "unknown rule",
			rules:   `{ id = "no_such_rule_xyz", enabled = true }`,
			pattern: `Rule\s+not\s+in\s+the\s+baseline`,
		},
		{
			name:    "OS version the baseline does not offer",
			rules:   fmt.Sprintf(`{ id = %q, enabled = true }`, withoutODV),
			osFrag:  `selected_os_versions = [{ os_type = "MAC_OS", os_version = 1 }]`,
			pattern: `OS\s+version\s+not\s+available\s+for\s+the\s+baseline`,
		},
		{
			name:    "ODV on a rule that takes none",
			rules:   fmt.Sprintf(`{ id = %q, enabled = true, odv_value = "5" }`, withoutODV),
			pattern: `Rule\s+takes\s+no\s+organization-defined\s+value`,
		},
		{
			name:    "integer ODV above its maximum",
			rules:   fmt.Sprintf(`{ id = %q, enabled = true, odv_value = %q }`, boundedInteger, fmt.Sprint(maximum+1)),
			pattern: `Invalid\s+organization-defined\s+value`,
		},
		{
			name:    "integer ODV that is not a number",
			rules:   fmt.Sprintf(`{ id = %q, enabled = true, odv_value = "abc" }`, boundedInteger),
			pattern: `Invalid\s+organization-defined\s+value`,
		},
	}

	steps := make([]resource.TestStep, 0, len(cases))
	for _, tc := range cases {
		steps = append(steps, resource.TestStep{
			Config:      baselineValidationConfig(baselineID, groupID, tc.rules, tc.osFrag),
			PlanOnly:    true,
			ExpectError: regexp.MustCompile(tc.pattern),
		})
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

// TestAccResource_Benchmark_PlanAcceptsEveryBaselineDefault plans each baseline with every rule at
// its own default ODV value and every available OS version selected. A validator that refuses any of
// that has a false positive, since the platform itself serves those values.
func TestAccResource_Benchmark_PlanAcceptsEveryBaselineDefault(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ctx := context.Background()
	cb := cbSDK.New(testhelpers.NewAcceptanceClient(t))
	groupID := baselineValidationGroup(t, ctx)

	baselines, err := cb.ListBaselines(ctx)
	if err != nil {
		t.Fatalf("Failed to list baselines: %v", err)
	}
	if len(baselines.Baselines) == 0 {
		t.Skip("No baselines available — CB Engine may not be enabled")
	}

	var steps []resource.TestStep
	for _, b := range baselines.Baselines {
		rules, err := cb.GetBaselineRules(ctx, b.BaselineID)
		if err != nil {
			t.Fatalf("Failed to get rules for %s: %v", b.BaselineID, err)
		}
		blocks := make([]string, 0, len(rules.Rules))
		for _, r := range rules.Rules {
			block := fmt.Sprintf(`{ id = %q, enabled = true`, r.ID)
			if r.ODV != nil {
				block += fmt.Sprintf(`, odv_value = %q`, r.ODV.Value)
			}
			blocks = append(blocks, block+" }")
		}
		versions := make([]string, 0, len(rules.AvailableOsVersions))
		for _, v := range rules.AvailableOsVersions {
			versions = append(versions, fmt.Sprintf(`{ os_type = %q, os_version = %d }`, v.OsType, v.OsVersion))
		}
		steps = append(steps, resource.TestStep{
			Config:             baselineValidationConfig(b.BaselineID, groupID, strings.Join(blocks, ", "), "selected_os_versions = ["+strings.Join(versions, ", ")+"]"),
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		})
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps:                    steps,
	})
}
