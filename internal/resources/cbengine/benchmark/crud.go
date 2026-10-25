// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Compliance Benchmark resource in Terraform.
//
// Uniqueness: title, enforced (409 DuplicateFieldException), case-insensitive; spaces and accents are significant.
func (r *BenchmarkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data BenchmarkResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.SourceBaselineID.IsNull() || data.SourceBaselineID.IsUnknown() || data.SourceBaselineID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing source baseline", "Attribute source_baseline_id must be provided when creating a benchmark.")
		return
	}

	createTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultCreateTimeout, data.Timeouts.Create)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	release, err := r.lock.Acquire(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error creating benchmark", "Terraform canceled the create while it waited for another benchmark to finish creating or deleting: "+helpers.APIErrorDetail(err))
		return
	}
	defer release()

	createCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	reqBody := buildBenchmarkRequest(&data)

	tflog.Debug(ctx, "creating cbengine benchmark", map[string]any{
		"title": data.Title.ValueString(),
	})

	bench, err := r.client.CreateBenchmark(createCtx, reqBody)
	if err != nil {
		resp.Diagnostics.AddError("Error creating benchmark", helpers.APIErrorDetail(err))
		return
	}

	tflog.Debug(ctx, "created benchmark (async)", map[string]any{
		"benchmark_id": bench.BenchmarkID,
		"tenant_id":    bench.TenantID,
	})

	pollInterval := 5 * time.Second
	tflog.Debug(ctx, "waiting for benchmark to reach SYNCED state", map[string]any{
		"benchmark_id":  bench.BenchmarkID,
		"poll_interval": pollInterval.String(),
	})

	syncedBench, err := waitForBenchmarkSync(createCtx, r.client, bench.BenchmarkID, pollInterval)
	if err != nil {
		tflog.Warn(ctx, "wait for benchmark sync failed", map[string]any{"error": err.Error(), "benchmark_id": bench.BenchmarkID})
		resp.Diagnostics.AddWarning(
			"Benchmark deployment failed.",
			"The benchmark was successfully created but did not deploy successfully: "+err.Error()+
				". Check your Jamf instance to verify the benchmark status.",
		)
	} else {
		tflog.Debug(ctx, "benchmark synced", map[string]any{"benchmark_id": syncedBench.ID})
	}

	assignBenchmarkModelFromResponse(&data, bench)
	if syncedBench != nil {
		data.ID = types.StringValue(syncedBench.ID)
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, benchmarkIdentityModel{ID: data.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created a resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read reads the current state of a Jamf Compliance Benchmark resource from the API and updates the Terraform state.
func (r *BenchmarkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data BenchmarkResourceModel

	if req.State.Raw.IsNull() {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this benchmark without existing state or identity data, so the provider cannot determine which benchmark to read.",
			)
			return
		}

		var identity benchmarkIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}

		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing benchmark ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the benchmark.",
			)
			return
		}

		data.ID = identity.ID
		data.Timeouts = helpers.NewResourceTimeoutsNullValue(benchmarkTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot read benchmark without ID.")
		return
	}

	bench, err := r.client.GetBenchmark(readCtx, data.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Benchmark not found, removing from state", map[string]any{
				"benchmark_id": data.ID.ValueString(),
			})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, benchmarkIdentityModel{ID: data.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError("Error reading benchmark", helpers.APIErrorDetail(err))
		return
	}

	assignBenchmarkModelFromResponse(&data, bench)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, benchmarkIdentityModel{ID: data.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update is not supported for Jamf Compliance Benchmark resources. The resource must be recreated to apply changes.
func (r *BenchmarkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "This resource must be destroyed and recreated to apply changes.")
}

// Delete deletes a Jamf Compliance Benchmark resource from the API and removes it from the Terraform state.
func (r *BenchmarkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data BenchmarkResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultDeleteTimeout, data.Timeouts.Delete)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot delete benchmark without ID.")
		return
	}

	release, err := r.lock.Acquire(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting benchmark", "Terraform canceled the delete while it waited for another benchmark to finish creating or deleting: "+helpers.APIErrorDetail(err))
		return
	}
	defer release()

	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	if err := r.client.DeleteBenchmark(deleteCtx, data.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Benchmark already deleted", map[string]any{
				"benchmark_id": data.ID.ValueString(),
			})
			return
		}

		resp.Diagnostics.AddError(
			"Error deleting benchmark",
			"Could not delete benchmark: "+helpers.APIErrorDetail(err),
		)
		return
	}

	pollInterval := 5 * time.Second
	if err := waitForBenchmarkDeletion(deleteCtx, r.client, data.ID.ValueString(), pollInterval, redeleteInterval); err != nil {
		if helpers.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error waiting for benchmark deletion",
			fmt.Sprintf("Jamf did not finish deleting benchmark %s: %s", data.ID.ValueString(), helpers.APIErrorDetail(err)),
		)
		return
	}
}
