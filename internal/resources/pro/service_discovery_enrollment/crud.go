// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetServiceDiscoveryEnrollmentWellKnownSettingsV1
//   pro.UpdateServiceDiscoveryEnrollmentWellKnownSettingsV1
//
// Status: current.

package service_discovery_enrollment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions the service-discovery well-known settings. The API has no Create
// endpoint — the record always exists (one per tenant) — so this PUTs the declared
// rows. The PUT is a by-key merge that returns 204 No Content with no echo, so state
// is read back via GET.
func (r *ServiceDiscoveryEnrollmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ServiceDiscoveryEnrollmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultCreateTimeout, plan.Timeouts.Create)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	createCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	items, diags := wellKnownSettingsFromList(createCtx, plan.WellKnownSetting)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateServiceDiscoveryEnrollmentWellKnownSettingsV1(createCtx, buildServiceDiscoveryEnrollmentInput(items)); err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro service discovery well-known settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetServiceDiscoveryEnrollmentWellKnownSettingsV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro service discovery well-known settings after write", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignServiceDiscoveryEnrollmentResourceModel(createCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, serviceDiscoveryEnrollmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro service discovery well-known settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API.
func (r *ServiceDiscoveryEnrollmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state ServiceDiscoveryEnrollmentResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(serviceDiscoveryEnrollmentTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	got, err := r.client.GetServiceDiscoveryEnrollmentWellKnownSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro service discovery well-known settings", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignServiceDiscoveryEnrollmentResourceModel(readCtx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, serviceDiscoveryEnrollmentIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the declared rows to the Jamf Pro API. Same SDK call as Create; the
// PUT merges by server_uuid (undeclared orgs are untouched). The PUT returns 204 with
// no echo, so authoritative state comes from a follow-up GET.
func (r *ServiceDiscoveryEnrollmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ServiceDiscoveryEnrollmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultUpdateTimeout, plan.Timeouts.Update)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	items, diags := wellKnownSettingsFromList(updateCtx, plan.WellKnownSetting)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateServiceDiscoveryEnrollmentWellKnownSettingsV1(updateCtx, buildServiceDiscoveryEnrollmentInput(items)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro service discovery well-known settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetServiceDiscoveryEnrollmentWellKnownSettingsV1(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro service discovery well-known settings after update", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignServiceDiscoveryEnrollmentResourceModel(updateCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, serviceDiscoveryEnrollmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. The singleton cannot be deleted — the record
// persists on the tenant, and the rows the resource managed keep their last value
// (the PUT is a merge; there is no remove). To turn off Jamf-hosted service discovery
// for an org, set its enrollment_type to "none" before removing the resource.
// Terraform removes the resource from state on its own after this handler returns.
func (r *ServiceDiscoveryEnrollmentResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro service discovery well-known settings from Terraform state (singleton — no remote delete)")
}
