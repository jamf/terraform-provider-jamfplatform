// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetSelfServicePlusSettingsV1
//   pro.UpdateSelfServicePlusSettingsV1
//
// Status: current.

package self_service_plus_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create handles initial provisioning of the Self Service Plus settings singleton.
// The Jamf Pro API has no Create endpoint for this object — one record per tenant
// already exists — so this funnels into Update against the plan, then reads back
// to capture authoritative state.
func (r *SelfServicePlusSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan SelfServicePlusSettingsResourceModel
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

	if err := r.client.UpdateSelfServicePlusSettingsV1(createCtx, buildSelfServicePlusSettingsInput(plan)); err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro Self Service Plus settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetSelfServicePlusSettingsV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Self Service Plus settings after write", helpers.APIErrorDetail(err))
		return
	}
	assignSelfServicePlusSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServicePlusSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro Self Service Plus settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API.
func (r *SelfServicePlusSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state SelfServicePlusSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(selfServicePlusSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetSelfServicePlusSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Self Service Plus settings", helpers.APIErrorDetail(err))
		return
	}

	assignSelfServicePlusSettingsResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServicePlusSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings to the Jamf Pro API. Same SDK call as Create.
func (r *SelfServicePlusSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan SelfServicePlusSettingsResourceModel
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

	if err := r.client.UpdateSelfServicePlusSettingsV1(updateCtx, buildSelfServicePlusSettingsInput(plan)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro Self Service Plus settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetSelfServicePlusSettingsV1(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Self Service Plus settings after update", helpers.APIErrorDetail(err))
		return
	}
	assignSelfServicePlusSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServicePlusSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. Singleton objects cannot be deleted — the
// record persists on the tenant. Terraform removes the resource from state on its
// own after this handler returns. No SDK call is made and no diagnostics are emitted.
//
// Canonical singleton template: every singleton Delete should look exactly like this
// — a single tflog.Trace explaining the no-op, with `_` markers on the unused
// request/response so future maintainers immediately see the omission is intentional.
func (r *SelfServicePlusSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro Self Service Plus settings from Terraform state (singleton — no remote delete)")
}
