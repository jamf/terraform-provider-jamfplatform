// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	listschema "github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultListTimeout caps how long the list operation waits on the benchmark
// list endpoint.
const defaultListTimeout = 5 * time.Minute

// defaultItemReadTimeout bounds each per-item benchmark read issued when
// IncludeResource is set (config generation), giving every item its own
// deadline independent of the list-fetch budget so one slow benchmark cannot
// exhaust a shared deadline. A read that times out aborts the query with the
// same diagnostic as any other per-item read failure.
const defaultItemReadTimeout = 30 * time.Second

var _ list.ListResource = &BenchmarkListResource{}
var _ list.ListResourceWithConfigure = &BenchmarkListResource{}

// NewBenchmarkListResource creates a new list resource for compliance benchmarks.
func NewBenchmarkListResource() list.ListResource {
	return &BenchmarkListResource{}
}

// BenchmarkListResource implements Terraform list support for compliance benchmarks.
type BenchmarkListResource struct {
	client *compliancebenchmarks.Client
}

// Metadata configures the resource type name for Terraform.
func (r *BenchmarkListResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cbengine_benchmark"
}

// Configure wires the provider client into the list resource.
func (r *BenchmarkListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected List Configure Type",
			"Expected *providerdata.Data. Please report this issue to the provider developers.",
		)
		return
	}

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_cbengine_benchmark", providerdata.ComplianceBenchmarksScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client = compliancebenchmarks.New(pd.Client)
}

// ListResourceConfigSchema declares the supported filter attributes.
func (r *BenchmarkListResource) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = listschema.Schema{
		Description: "Searches for Jamf Compliance Benchmarks with an optional case-insensitive substring filter." + listResourcePrivileges,
		Attributes: map[string]listschema.Attribute{
			"search": listschema.StringAttribute{
				Optional:    true,
				Description: "Optional substring to match (case-insensitive) against benchmark title or description.",
			},
		},
	}
	resp.Schema.Attributes = listtimeouts.Add(ctx, resp.Schema.Attributes)
}

// List fetches Jamf Compliance Benchmarks and streams them back to Terraform.
func (r *BenchmarkListResource) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if r.client == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unconfigured Provider",
				"The provider has not been configured yet. Re-run the command after `terraform init` completes successfully.",
			),
		})
		return
	}

	var config BenchmarkListResourceModel
	diags := req.Config.Get(ctx, &config)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}

	searchTerm, hasSearch := helpers.NormalizedFilterString(config.Search)
	searchLower := strings.ToLower(searchTerm)

	listCtx, cancelList := listtimeouts.Bound(ctx, req.Config, &diags, defaultListTimeout)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}
	resp, err := r.client.ListBenchmarks(listCtx)
	cancelList()
	if err != nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unable to list benchmarks",
				helpers.APIErrorDetail(err),
			),
		})
		return
	}

	if resp == nil {
		stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
			diag.NewErrorDiagnostic(
				"Unable to list benchmarks",
				"The Jamf Platform API returned an empty response.",
			),
		})
		return
	}

	benchmarks := resp.Benchmarks
	if len(benchmarks) == 0 {
		stream.Results = list.NoListResults
		return
	}

	maxResults := req.Limit
	if maxResults <= 0 || maxResults > int64(len(benchmarks)) {
		maxResults = int64(len(benchmarks))
	}

	results := make([]list.ListResult, 0, int(maxResults))
	var emitted int64

	for _, bench := range benchmarks {
		if hasSearch {
			titleMatch := strings.Contains(strings.ToLower(bench.Title), searchLower)
			descMatch := strings.Contains(strings.ToLower(bench.Description), searchLower)
			if !titleMatch && !descMatch {
				continue
			}
		}

		if maxResults > 0 && emitted >= maxResults {
			break
		}

		result := req.NewListResult(ctx)
		result.DisplayName = bench.Title

		identity := benchmarkIdentityModel{ID: types.StringValue(bench.ID)}
		result.Diagnostics.Append(helpers.SetIdentity(ctx, result.Identity, identity)...)
		if result.Diagnostics.HasError() {
			stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
			return
		}

		if req.IncludeResource {
			itemCtx, cancelItem := context.WithTimeout(ctx, defaultItemReadTimeout)
			detail, err := r.client.GetBenchmark(itemCtx, bench.ID)
			cancelItem()
			if err != nil {
				stream.Results = list.ListResultsStreamDiagnostics(diag.Diagnostics{
					diag.NewErrorDiagnostic(
						"Unable to read benchmark",
						helpers.APIErrorDetail(err),
					),
				})
				return
			}

			state := BenchmarkResourceModel{
				Timeouts: helpers.NewResourceTimeoutsNullValue(benchmarkTimeoutAttributeTypes),
			}

			assignBenchmarkModelFromResponse(&state, detail)
			state.Timeouts = helpers.EnsureResourceTimeouts(state.Timeouts, benchmarkTimeoutAttributeTypes)

			result.Diagnostics.Append(result.Resource.Set(ctx, &state)...)
			if result.Diagnostics.HasError() {
				stream.Results = list.ListResultsStreamDiagnostics(result.Diagnostics)
				return
			}
		}

		results = append(results, result)
		emitted++
	}

	tflog.Debug(ctx, "Listed compliance benchmarks", map[string]any{
		"search":   searchTerm,
		"limit":    req.Limit,
		"returned": len(results),
	})

	if len(results) == 0 {
		stream.Results = list.NoListResults
		return
	}

	stream.Results = func(push func(list.ListResult) bool) {
		for _, result := range results {
			if !push(result) {
				return
			}
		}
	}
}
