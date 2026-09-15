// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package benchmark_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	cbSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	dgSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckBenchmarkResourcesDestroy verifies that benchmarks and device groups created
// during the test have been destroyed. Benchmark deletion is async, so this polls briefly.
func testAccCheckBenchmarkResourcesDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testhelpers.NewAcceptanceClient(t)
		cbClient := cbSDK.New(c)
		dgClient := dgSDK.New(c)
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			switch rs.Type {
			case "jamfplatform_cbengine_benchmark":
				deadline := time.Now().Add(30 * time.Second)
				for time.Now().Before(deadline) {
					_, err := cbClient.GetBenchmark(ctx, rs.Primary.ID)
					if err != nil {
						if helpers.IsNotFoundError(err) {
							break
						}
						return fmt.Errorf("error checking benchmark %s: %s", rs.Primary.ID, err)
					}
					time.Sleep(2 * time.Second)
				}
			case "jamfplatform_device_group":
				deadline := time.Now().Add(60 * time.Second)
				for time.Now().Before(deadline) {
					_, err := dgClient.GetDeviceGroup(ctx, rs.Primary.ID)
					if err != nil {
						if helpers.IsNotFoundError(err) {
							break
						}
						return fmt.Errorf("error checking device group %s: %s", rs.Primary.ID, err)
					}
					time.Sleep(2 * time.Second)
				}
			}
		}
		return nil
	}
}

// ensureBenchmarkCleanup deletes a benchmark by title and waits for async deletion to complete.
func ensureBenchmarkCleanup(t *testing.T, title string) {
	t.Helper()
	ctx := context.Background()
	c := testhelpers.NewAcceptanceClient(t)
	testhelpers.EnsureBenchmarkDeleted(t, c, ctx, title)
}

func TestAccResource_Benchmark_AllRules_Monitor(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	ctx := context.Background()
	c := testhelpers.NewAcceptanceClient(t)
	cbClient := cbSDK.New(c)
	baselines, err := cbClient.ListBaselines(ctx)
	if err != nil {
		t.Fatalf("Failed to check baselines: %v", err)
	}
	if len(baselines.Baselines) == 0 {
		t.Skip("No baselines available — CB Engine may not be enabled")
	}

	baselineID := baselines.Baselines[0].BaselineID
	rules, err := cbClient.GetBaselineRules(ctx, baselineID)
	if err != nil {
		t.Fatalf("Failed to get rules: %v", err)
	}
	if len(rules.Rules) == 0 {
		t.Skip("No rules found for baseline")
	}

	var ruleBlocks []string
	for _, r := range rules.Rules {
		block := fmt.Sprintf(`{ id = %q, enabled = %t`, r.ID, r.Enabled)
		if r.ODV != nil {
			block += fmt.Sprintf(`, odv_value = %q`, r.ODV.Value)
		}
		block += " }"
		ruleBlocks = append(ruleBlocks, block)
	}

	benchmarkTitle := "tf-acc-benchmark-all-rules-" + suffix
	scopeNameA := "tf-acc-benchmark-scope-a-" + suffix
	scopeNameB := "tf-acc-benchmark-scope-b-" + suffix

	ensureBenchmarkCleanup(t, benchmarkTitle)

	config := fmt.Sprintf(`
		resource "jamfplatform_device_group" "scope_a" {
			name        = %q
			group_type  = "smart"
			device_type = "computer"
			criteria = [{
				criteria = "Serial Number"
				operator = "like"
				value    = ""
			}]
		}

		resource "jamfplatform_device_group" "scope_b" {
			name        = %q
			group_type  = "smart"
			device_type = "computer"
			criteria = [{
				criteria = "Serial Number"
				operator = "like"
				value    = ""
			}]
		}

		resource "jamfplatform_cbengine_benchmark" "test_all_rules" {
			title              = %q
			description        = "Acceptance test — safe to delete"
			source_baseline_id = %q

			rules = [%s]

			target_device_groups = [
				jamfplatform_device_group.scope_a.id,
				jamfplatform_device_group.scope_b.id,
			]
			enforcement_mode    = "MONITOR"
		}
	`, scopeNameA, scopeNameB, benchmarkTitle, baselineID, strings.Join(ruleBlocks, ",\n"))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBenchmarkResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_cbengine_benchmark.test_all_rules", "id"),
					resource.TestCheckResourceAttr("jamfplatform_cbengine_benchmark.test_all_rules", "title", benchmarkTitle),
					resource.TestCheckResourceAttr("jamfplatform_cbengine_benchmark.test_all_rules", "enforcement_mode", "MONITOR"),
					resource.TestCheckResourceAttr("jamfplatform_cbengine_benchmark.test_all_rules", "target_device_groups.#", "2"),
					// Removed at schema v1 once its 90-day deprecation window closed; assert
					// it stays gone rather than silently reappearing in state.
					resource.TestCheckNoResourceAttr("jamfplatform_cbengine_benchmark.test_all_rules", "target_device_group"),
					// selected_os_versions omitted → computed to the full available set (omit == all).
					resource.TestCheckResourceAttr("jamfplatform_cbengine_benchmark.test_all_rules", "available_os_versions.#", fmt.Sprintf("%d", len(rules.AvailableOsVersions))),
					resource.TestCheckResourceAttr("jamfplatform_cbengine_benchmark.test_all_rules", "selected_os_versions.#", fmt.Sprintf("%d", len(rules.AvailableOsVersions))),
					resource.TestCheckResourceAttrSet("jamfplatform_cbengine_benchmark.test_all_rules", "sources.#"),
				),
			},
		},
	})
}

func TestAccResource_Benchmark_CustomRules_MonitorAndEnforce(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	ctx := context.Background()
	c := testhelpers.NewAcceptanceClient(t)
	cbClient := cbSDK.New(c)
	baselines, err := cbClient.ListBaselines(ctx)
	if err != nil {
		t.Fatalf("Failed to check baselines: %v", err)
	}
	if len(baselines.Baselines) == 0 {
		t.Skip("No baselines available — CB Engine may not be enabled")
	}

	baselineID := baselines.Baselines[0].BaselineID
	rules, err := cbClient.GetBaselineRules(ctx, baselineID)
	if err != nil {
		t.Fatalf("Failed to get rules: %v", err)
	}
	if len(rules.Rules) < 2 {
		t.Skip("Need at least 2 rules to create custom benchmark")
	}

	var ruleBlocks []string
	for i := range 2 {
		r := rules.Rules[i]
		block := fmt.Sprintf(`{ id = %q, enabled = true`, r.ID)
		if r.ODV != nil {
			block += fmt.Sprintf(`, odv_value = %q`, r.ODV.Value)
		}
		block += " }"
		ruleBlocks = append(ruleBlocks, block)
	}

	// Exercise selected_os_versions by scoping the benchmark to a single
	// available OS version (the highest one the baseline offers).
	if len(rules.AvailableOsVersions) == 0 {
		t.Skip("baseline exposes no available OS versions")
	}
	selectedOs := rules.AvailableOsVersions[0]
	for _, v := range rules.AvailableOsVersions {
		if v.OsVersion > selectedOs.OsVersion {
			selectedOs = v
		}
	}
	selectedOsBlock := fmt.Sprintf(`{ os_type = %q, os_version = %d }`, selectedOs.OsType, selectedOs.OsVersion)

	benchmarkTitle := "tf-acc-benchmark-custom-rules-" + suffix
	scopeNameA := "tf-acc-benchmark-scope-custom-a-" + suffix
	scopeNameB := "tf-acc-benchmark-scope-custom-b-" + suffix

	ensureBenchmarkCleanup(t, benchmarkTitle)

	config := fmt.Sprintf(`
		resource "jamfplatform_device_group" "scope_a" {
			name        = %q
			group_type  = "smart"
			device_type = "computer"
			criteria = [{
				criteria = "Serial Number"
				operator = "like"
				value    = ""
			}]
		}

		resource "jamfplatform_device_group" "scope_b" {
			name        = %q
			group_type  = "smart"
			device_type = "computer"
			criteria = [{
				criteria = "Serial Number"
				operator = "like"
				value    = ""
			}]
		}

		resource "jamfplatform_cbengine_benchmark" "test_custom" {
			title              = %q
			description        = "Acceptance test custom rules — safe to delete"
			source_baseline_id = %q

			rules = [%s]

			selected_os_versions = [%s]

			target_device_groups = [
				jamfplatform_device_group.scope_a.id,
				jamfplatform_device_group.scope_b.id,
			]
			enforcement_mode    = "MONITOR_AND_ENFORCE"
		}
	`, scopeNameA, scopeNameB, benchmarkTitle, baselineID, strings.Join(ruleBlocks, ",\n"), selectedOsBlock)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBenchmarkResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_cbengine_benchmark.test_custom", "id"),
					resource.TestCheckResourceAttr("jamfplatform_cbengine_benchmark.test_custom", "title", benchmarkTitle),
					resource.TestCheckResourceAttr("jamfplatform_cbengine_benchmark.test_custom", "enforcement_mode", "MONITOR_AND_ENFORCE"),
					resource.TestCheckResourceAttr("jamfplatform_cbengine_benchmark.test_custom", "target_device_groups.#", "2"),
					// selected_os_versions scoped to a single version; sources remain the full computed set.
					resource.TestCheckResourceAttr("jamfplatform_cbengine_benchmark.test_custom", "selected_os_versions.#", "1"),
					resource.TestCheckResourceAttrSet("jamfplatform_cbengine_benchmark.test_custom", "sources.#"),
					resource.TestCheckResourceAttrSet("jamfplatform_cbengine_benchmark.test_custom", "available_os_versions.#"),
				),
			},
		},
	})
}

// TestAccResource_Benchmark_SelectedOsVersionsOrderIndependent guards the choice
// of a Set (not a List) for selected_os_versions. The server canonicalises the
// ordering of the versions it echoes back, so the same versions supplied in a
// different order must NOT produce a diff (and therefore must not trigger the
// RequiresReplace on this attribute). Step 2 re-plans the same set in reverse
// order with PlanOnly, which fails if the plan is non-empty.
func TestAccResource_Benchmark_SelectedOsVersionsOrderIndependent(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	ctx := context.Background()
	c := testhelpers.NewAcceptanceClient(t)
	cbClient := cbSDK.New(c)
	baselines, err := cbClient.ListBaselines(ctx)
	if err != nil {
		t.Fatalf("Failed to check baselines: %v", err)
	}
	if len(baselines.Baselines) == 0 {
		t.Skip("No baselines available — CB Engine may not be enabled")
	}
	baselineID := baselines.Baselines[0].BaselineID
	rules, err := cbClient.GetBaselineRules(ctx, baselineID)
	if err != nil {
		t.Fatalf("Failed to get rules: %v", err)
	}
	if len(rules.Rules) == 0 {
		t.Skip("No rules found for baseline")
	}
	if len(rules.AvailableOsVersions) < 2 {
		t.Skip("Need at least 2 available OS versions to test order independence")
	}

	r := rules.Rules[0]
	ruleBlock := fmt.Sprintf(`{ id = %q, enabled = true }`, r.ID)

	a, b := rules.AvailableOsVersions[0], rules.AvailableOsVersions[1]
	osAB := fmt.Sprintf(`{ os_type = %q, os_version = %d }, { os_type = %q, os_version = %d }`, a.OsType, a.OsVersion, b.OsType, b.OsVersion)
	osBA := fmt.Sprintf(`{ os_type = %q, os_version = %d }, { os_type = %q, os_version = %d }`, b.OsType, b.OsVersion, a.OsType, a.OsVersion)

	benchmarkTitle := "tf-acc-benchmark-osorder-" + suffix
	scopeName := "tf-acc-benchmark-scope-osorder-" + suffix
	ensureBenchmarkCleanup(t, benchmarkTitle)

	config := func(osBlock string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_device_group" "scope" {
				name        = %q
				group_type  = "smart"
				device_type = "computer"
				criteria = [{
					criteria = "Serial Number"
					operator = "like"
					value    = ""
				}]
			}

			resource "jamfplatform_cbengine_benchmark" "test_osorder" {
				title                = %q
				description          = "Acceptance test OS-version order independence — safe to delete"
				source_baseline_id   = %q
				rules                = [%s]
				selected_os_versions = [%s]
				target_device_groups = [jamfplatform_device_group.scope.id]
				enforcement_mode     = "MONITOR"
			}
		`, scopeName, benchmarkTitle, baselineID, ruleBlock, osBlock)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBenchmarkResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(osAB),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_cbengine_benchmark.test_osorder", "id"),
					resource.TestCheckResourceAttr("jamfplatform_cbengine_benchmark.test_osorder", "selected_os_versions.#", "2"),
				),
			},
			{
				// Same set, reversed order. A Set treats this as no change; a List
				// would plan a replace here, failing the empty-plan assertion.
				Config:   config(osBA),
				PlanOnly: true,
			},
		},
	})
}

func TestAccDataSource_Baselines(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBenchmarkResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_cbengine_baselines" "all" {}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.jamfplatform_cbengine_baselines.all", "baselines.#"),
				),
			},
		},
	})
}

func TestAccDataSource_Benchmarks(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBenchmarkResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_cbengine_benchmarks" "all" {}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.jamfplatform_cbengine_benchmarks.all", "benchmarks.#"),
				),
			},
		},
	})
}

func TestAccDataSource_Rules(t *testing.T) {
	testhelpers.AccPreCheck(t)

	ctx := context.Background()
	c := testhelpers.NewAcceptanceClient(t)
	cbClient := cbSDK.New(c)
	baselines, err := cbClient.ListBaselines(ctx)
	if err != nil {
		t.Fatalf("Failed to check baselines: %v", err)
	}
	if len(baselines.Baselines) == 0 {
		t.Skip("No baselines available")
	}

	config := fmt.Sprintf(`
		data "jamfplatform_cbengine_rules" "test" {
			baseline_id = %q
		}
	`, baselines.Baselines[0].BaselineID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBenchmarkResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.jamfplatform_cbengine_rules.test", "rules.#"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_cbengine_rules.test", "sources.#"),
				),
			},
		},
	})
}
