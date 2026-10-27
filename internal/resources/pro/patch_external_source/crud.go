// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreatePatchExternalSourceByID
//   proclassic.GetPatchExternalSourceByID
//   proclassic.UpdatePatchExternalSourceByID
//   proclassic.DeletePatchExternalSourceByID
//   proclassic.ListPatchExternalSources              (data source / list resource / collision check in collision.go)
//   proclassic.GetPatchExternalSourceByName          (data source name lookup)
//   proclassic.ResolvePatchExternalSourceIDByName    (data source name → ID)
//
// Write semantics: the PUT merges field by field; an omitted <port> keeps the
// stored value and <port>0</port> clears it (wire-probed 2026-09-06; -1 is
// refused). See portForWrite.
//
// Uniqueness: name, not enforced; duplicates stored; checked only with
// include_unenforced. Classic trims the name on save, so the strict comparison also
// ignores leading and trailing spaces, case and accents.
//
// Status: current. Last reviewed 2026-09-06.

package patch_external_source

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro patch external source. Classic POSTs to id="0";
// the server allocates the real integer ID and returns it in the response body.
func (r *PatchExternalSourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan PatchExternalSourceResourceModel
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

	created, err := r.client.CreatePatchExternalSourceByID(createCtx, "0", buildPatchExternalSourceInput(plan, types.Int64Null()))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro patch external source", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	// Defensive: the classic SDK trusts the server and would deref a nil ID; we
	// explicitly guard so a null ID never lands in state.
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing patch external source ID",
			"Jamf Pro returned 201 Created with no patch external source ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	got, err := r.client.GetPatchExternalSourceByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro patch external source", helpers.APIErrorDetail(err))
		return
	}
	assignPatchExternalSourceResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, patchExternalSourceIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro patch external source", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest patch external source
// representation.
func (r *PatchExternalSourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state PatchExternalSourceResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this patch external source without existing state or identity data, so the provider cannot determine which source to read.",
			)
			return
		}
		var identity patchExternalSourceIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing patch external source ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the patch external source.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(patchExternalSourceTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro patch external source without ID.")
		return
	}

	got, err := r.client.GetPatchExternalSourceByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro patch external source not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, patchExternalSourceIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro patch external source", helpers.APIErrorDetail(err))
		return
	}

	assignPatchExternalSourceResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, patchExternalSourceIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro patch external source. Classic
// UpdatePatchExternalSourceByID returns 201 with an empty body — we must GET to
// refresh state. The prior state's port is read alongside the plan so a port
// the config dropped is cleared on the wire (see portForWrite).
func (r *PatchExternalSourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior PatchExternalSourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
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

	if err := r.client.UpdatePatchExternalSourceByID(updateCtx, plan.ID.ValueString(), buildPatchExternalSourceInput(plan, prior.Port)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro patch external source", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetPatchExternalSourceByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro patch external source", helpers.APIErrorDetail(err))
		return
	}
	assignPatchExternalSourceResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, patchExternalSourceIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro patch external source.
func (r *PatchExternalSourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PatchExternalSourceResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro patch external source without ID.")
		return
	}

	if err := r.client.DeletePatchExternalSourceByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro patch external source already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro patch external source", helpers.APIErrorDetail(err))
	}
}
