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

func TestAccResource_Benchmark_TitleCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ctx := context.Background()
	c := testhelpers.NewAcceptanceClient(t)
	cbClient := cbSDK.New(c)

	baselines, err := cbClient.ListBaselines(ctx)
	if err != nil {
		t.Fatalf("Failed to list baselines: %v", err)
	}
	if len(baselines.Baselines) == 0 {
		t.Skip("No baselines available — CB Engine may not be enabled")
	}
	baselineID := baselines.Baselines[0].BaselineID
	rules, err := cbClient.GetBaselineRules(ctx, baselineID)
	if err != nil {
		t.Fatalf("Failed to get rules: %v", err)
	}
	groups, err := dgSDK.New(c).ListDeviceGroups(ctx, nil, "")
	if err != nil || len(groups) == 0 {
		t.Skip("No device group available to target")
	}

	title := "tf-acc-benchmark-collide-" + testhelpers.RunSuffix()
	ruleBlocks := make([]string, 0, len(rules.Rules))
	request := &cbSDK.BenchmarkRequestV2{
		Title:            title,
		SourceBaselineID: baselineID,
		Rules:            make([]cbSDK.RuleRequest, len(rules.Rules)),
		Target:           cbSDK.TargetV2{DeviceGroups: []string{groups[0].ID}},
		EnforcementMode:  "MONITOR",
	}
	for i, r := range rules.Rules {
		block := fmt.Sprintf(`{ id = %q, enabled = %t`, r.ID, r.Enabled)
		request.Rules[i] = cbSDK.RuleRequest{ID: r.ID, Enabled: r.Enabled}
		if r.ODV != nil {
			block += fmt.Sprintf(`, odv_value = %q`, r.ODV.Value)
			request.Rules[i].ODV = &cbSDK.ODVRequest{Value: r.ODV.Value}
		}
		ruleBlocks = append(ruleBlocks, block+" }")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					created, err := cbClient.CreateBenchmark(ctx, request)
					if err != nil {
						t.Fatalf("creating benchmark %q outside Terraform: %v", title, err)
					}
					t.Cleanup(func() { testhelpers.EnsureBenchmarkDeletedByID(t, c, ctx, created.BenchmarkID) })
				},
				Config: fmt.Sprintf(`
					resource "jamfplatform_cbengine_benchmark" "test" {
						title                = %q
						source_baseline_id   = %q
						enforcement_mode     = "MONITOR"
						target_device_groups = [%q]
						rules                = [%s]
					}
				`, strings.ToUpper(title), baselineID, groups[0].ID, strings.Join(ruleBlocks, ", ")),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Title\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Compliance\s+Benchmarks`),
			},
		},
	})
}
