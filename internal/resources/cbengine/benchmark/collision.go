// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_cbengine_benchmark"

// benchmarkNamespace is the uniqueness rule for benchmark titles. Jamf refuses a
// duplicate with 409 DuplicateFieldException. The match ignores case and nothing
// else: leading spaces, trailing spaces and accents all make a distinct title
// (wire-probed 2026-10-25).
func benchmarkNamespace(client *compliancebenchmarks.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "compliancebenchmarks.benchmark-titles",
		Product:   "Jamf Compliance Benchmarks",
		KeyLabel:  "title",
		Enforced:  true,
		Collation: collision.Collation{IgnoreCase: true},
		Sources: []collision.Source{{
			Noun:         "benchmark",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				benchmarks, err := client.ListBenchmarks(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(benchmarks.Benchmarks))
				for _, b := range benchmarks.Benchmarks {
					entries = append(entries, collision.Entry{ID: b.ID, Key: b.Title})
				}
				return entries, nil
			},
		}},
	}
}

func (r *BenchmarkResource) reportTitleCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[BenchmarkResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[BenchmarkResourceModel]{{
			Namespace: benchmarkNamespace(r.client),
			Path:      path.Root("title"),
			Keys:      func(m *BenchmarkResourceModel) []types.String { return []types.String{m.Title} },
		}},
	})
}
