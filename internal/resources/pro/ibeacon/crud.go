// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateIBeaconByID
//   proclassic.GetIBeaconByID
//   proclassic.UpdateIBeaconByID
//   proclassic.DeleteIBeaconByID
//   proclassic.ListIBeacons            (data source / list resource)
//   proclassic.GetIBeaconByName        (data source name lookup)
//   proclassic.ResolveIBeaconIDByName  (data source name → ID)
//
// Uniqueness: name, enforced (409), case-, leading-space-, trailing-space- and accent-insensitive. The plan-time collision check lists with proclassic.ListIBeacons (collision.go).
//
// Status: current. Last reviewed 2026-05-22.

package ibeacon

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro iBeacon. Classic POSTs to id="0"; the server
// allocates the real integer ID and returns it in the response body.
func (r *IbeaconResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IbeaconResourceModel
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

	if err := validateIbeaconPlan(&plan); err != nil {
		resp.Diagnostics.AddError("Invalid iBeacon configuration", helpers.APIErrorDetail(err))
		return
	}

	created, err := r.client.CreateIBeaconByID(createCtx, "0", buildIbeaconInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro iBeacon", helpers.APIErrorDetail(err))
		return
	}
	// Defensive: the classic SDK trusts the server and would deref a nil ID via
	// ApplyIBeacon; we explicitly guard so a null ID never lands in state.
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing iBeacon ID",
			"Jamf Pro returned 201 Created with no iBeacon ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	got, err := r.client.GetIBeaconByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro iBeacon", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignIbeaconResourceModel(&plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ibeaconIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro iBeacon", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest iBeacon representation.
func (r *IbeaconResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IbeaconResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this iBeacon without existing state or identity data, so the provider cannot determine which iBeacon to read.",
			)
			return
		}
		var identity ibeaconIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing iBeacon ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the iBeacon.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(ibeaconTimeoutAttributeTypes)
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

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro iBeacon without ID.")
		return
	}

	got, err := r.client.GetIBeaconByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro iBeacon not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ibeaconIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro iBeacon", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignIbeaconResourceModel(&state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ibeaconIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro iBeacon. Classic UpdateIBeaconByID returns 201
// with an empty body — we must GET to refresh state.
func (r *IbeaconResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan IbeaconResourceModel
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

	if err := validateIbeaconPlan(&plan); err != nil {
		resp.Diagnostics.AddError("Invalid iBeacon configuration", helpers.APIErrorDetail(err))
		return
	}

	if err := r.client.UpdateIBeaconByID(updateCtx, plan.ID.ValueString(), buildIbeaconInput(plan)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro iBeacon", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetIBeaconByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro iBeacon", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignIbeaconResourceModel(&plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ibeaconIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro iBeacon.
func (r *IbeaconResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IbeaconResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultDeleteTimeout, state.Timeouts.Delete)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro iBeacon without ID.")
		return
	}

	if err := r.client.DeleteIBeaconByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro iBeacon already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro iBeacon", helpers.APIErrorDetail(err))
	}
}
