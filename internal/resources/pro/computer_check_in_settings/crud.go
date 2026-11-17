// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetCheckInSettingsV3
//   pro.UpdateCheckInSettingsV3
//
// Status: current.

package computer_check_in_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create handles initial provisioning of the Client Check-In settings singleton.
// The Jamf Pro API has no Create endpoint for this object — one record per tenant
// already exists — so this funnels into Update against the plan, then reads back
// to capture authoritative state.
func (r *ComputerCheckInSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ComputerCheckInSettingsResourceModel
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

	// The Client Check-In settings singleton always exists on the tenant, so
	// "create" is really adoption. Read the live settings and pass them as the
	// merge base so a toggle the user did not declare keeps its current value
	// rather than being reset to false by the full-replace write (wire-probed
	// full-replace 2026-06-09). On update the merge base is nil — UseStateForUnknown
	// has already carried omitted toggles into the plan as known prior values.
	current, err := r.client.GetCheckInSettingsV3(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro Client Check-In settings", helpers.APIErrorDetail(err))
		return
	}

	if _, err := r.client.UpdateCheckInSettingsV3(createCtx, buildComputerCheckInSettingsInput(plan, current)); err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro Client Check-In settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetCheckInSettingsV3(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Client Check-In settings after write", helpers.APIErrorDetail(err))
		return
	}
	assignComputerCheckInSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, computerCheckInSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro Client Check-In settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API.
func (r *ComputerCheckInSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state ComputerCheckInSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(computerCheckInSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetCheckInSettingsV3(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Client Check-In settings", helpers.APIErrorDetail(err))
		return
	}

	assignComputerCheckInSettingsResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, computerCheckInSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings to the Jamf Pro API. Same SDK call as Create.
func (r *ComputerCheckInSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ComputerCheckInSettingsResourceModel
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

	if _, err := r.client.UpdateCheckInSettingsV3(updateCtx, buildComputerCheckInSettingsInput(plan, nil)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro Client Check-In settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetCheckInSettingsV3(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Client Check-In settings after update", helpers.APIErrorDetail(err))
		return
	}
	assignComputerCheckInSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, computerCheckInSettingsIdentityModel{ID: plan.ID})...)
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
func (r *ComputerCheckInSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro Client Check-In settings from Terraform state (singleton — no remote delete)")
}
