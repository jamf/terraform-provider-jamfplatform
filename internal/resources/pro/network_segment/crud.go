// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateNetworkSegmentByID
//   proclassic.GetNetworkSegmentByID
//   proclassic.UpdateNetworkSegmentByID
//   proclassic.DeleteNetworkSegmentByID
//   proclassic.ListNetworkSegments            (data source / list resource / collision check in collision.go)
//   proclassic.GetNetworkSegmentByName        (data source name lookup)
//   proclassic.ResolveNetworkSegmentIDByName  (data source name → ID)
//
// Write semantics: the PUT merges field by field; an omitted element keeps
// the stored value, an empty <building>/<department> clears it and an explicit
// false clears an override flag (wire-probed 2026-09-06). See
// buildNetworkSegmentInput.
//
// Uniqueness: name, enforced (409), case-, leading-space- and trailing-space-insensitive; accents are significant.
//
// Status: current. Last reviewed 2026-09-06.

package network_segment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro network segment. Classic POSTs to id="0"; the server
// allocates the real integer ID and returns it in the response body.
func (r *NetworkSegmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan NetworkSegmentResourceModel
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

	created, err := r.client.CreateNetworkSegmentByID(createCtx, "0", buildNetworkSegmentInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro network segment", helpers.APIErrorDetail(err))
		return
	}
	// Defensive: the classic SDK trusts the server and would deref a nil ID via
	// ApplyNetworkSegment; we explicitly guard so a null ID never lands in state.
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing network segment ID",
			"Jamf Pro returned 201 Created with no network segment ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	got, err := r.client.GetNetworkSegmentByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro network segment", helpers.APIErrorDetail(err))
		return
	}
	assignNetworkSegmentResourceModel(&plan, got, false)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, networkSegmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro network segment", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest network segment representation.
func (r *NetworkSegmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NetworkSegmentResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this network segment without existing state or identity data, so the provider cannot determine which network segment to read.",
			)
			return
		}
		var identity networkSegmentIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing network segment ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the network segment.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(networkSegmentTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro network segment without ID.")
		return
	}

	got, err := r.client.GetNetworkSegmentByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro network segment not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, networkSegmentIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro network segment", helpers.APIErrorDetail(err))
		return
	}

	assignNetworkSegmentResourceModel(&state, got, false)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, networkSegmentIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro network segment. Classic UpdateNetworkSegmentByID returns
// 201 with an empty body — we must GET to refresh state.
func (r *NetworkSegmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan NetworkSegmentResourceModel
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

	if err := r.client.UpdateNetworkSegmentByID(updateCtx, plan.ID.ValueString(), buildNetworkSegmentInput(plan)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro network segment", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetNetworkSegmentByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro network segment", helpers.APIErrorDetail(err))
		return
	}
	assignNetworkSegmentResourceModel(&plan, got, false)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, networkSegmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro network segment.
func (r *NetworkSegmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NetworkSegmentResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro network segment without ID.")
		return
	}

	if err := r.client.DeleteNetworkSegmentByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro network segment already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro network segment", helpers.APIErrorDetail(err))
	}
}
