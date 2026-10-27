// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateRemovableMacAddressByID    (POST /id/0 — server mints the ID, 201)
//   proclassic.GetRemovableMacAddressByID
//   proclassic.UpdateRemovableMacAddressByID     (PUT — renames in place, returns 201)
//   proclassic.DeleteRemovableMacAddressByID     (returns 200)
//   proclassic.ListRemovableMacAddresses         (data source / list resource / collision check in collision.go)
//   proclassic.GetRemovableMacAddressByName      (data source name lookup)
//   proclassic.ResolveRemovableMacAddressIDByName (data source name → ID)
//
// Classic create POSTs to id="0"; the server allocates the integer ID and returns a
// body carrying the ID only (no name echo) — Create must GET to populate mac_address.
// The server stores the MAC verbatim (no case/separator canonicalisation) and rejects
// a duplicate value with 409, so mac_address is updatable in place and never drifts.
//
// Uniqueness: mac_address (the record name), enforced (409), case-, leading- and trailing-space-insensitive, accent-sensitive.
//
// Status: current. Last reviewed 2026-06-11.

package removable_mac_address

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro removable MAC address. Classic POSTs to id="0"; the
// server allocates the real integer ID and returns it in the response body.
func (r *RemovableMacAddressResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan RemovableMacAddressResourceModel
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

	created, err := r.client.CreateRemovableMacAddressByID(createCtx, "0", buildRemovableMacAddressInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro removable MAC address", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	// Defensive: the classic SDK trusts the server and would deref a nil ID via
	// ApplyRemovableMacAddress; we explicitly guard so a null ID never lands in state.
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing removable MAC address ID",
			"Jamf Pro returned 201 Created with no removable MAC address ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	// The create response carries the ID only (no name echo); GET to populate mac_address.
	got, err := r.client.GetRemovableMacAddressByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro removable MAC address", helpers.APIErrorDetail(err))
		return
	}
	assignRemovableMacAddressResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, removableMacAddressIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro removable MAC address", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest removable MAC address representation.
func (r *RemovableMacAddressResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state RemovableMacAddressResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this removable MAC address without existing state or identity data, so the provider cannot determine which record to read.",
			)
			return
		}
		var identity removableMacAddressIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing removable MAC address ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the removable MAC address.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(removableMacAddressTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro removable MAC address without ID.")
		return
	}

	got, err := r.client.GetRemovableMacAddressByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro removable MAC address not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, removableMacAddressIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro removable MAC address", helpers.APIErrorDetail(err))
		return
	}

	assignRemovableMacAddressResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, removableMacAddressIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro removable MAC address. Classic UpdateRemovableMacAddressByID
// renames the record in place and returns 201 with an ID-only body — we GET to refresh state.
func (r *RemovableMacAddressResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan RemovableMacAddressResourceModel
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

	if err := r.client.UpdateRemovableMacAddressByID(updateCtx, plan.ID.ValueString(), buildRemovableMacAddressInput(plan)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro removable MAC address", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetRemovableMacAddressByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro removable MAC address", helpers.APIErrorDetail(err))
		return
	}
	assignRemovableMacAddressResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, removableMacAddressIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro removable MAC address.
func (r *RemovableMacAddressResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RemovableMacAddressResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro removable MAC address without ID.")
		return
	}

	if err := r.client.DeleteRemovableMacAddressByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro removable MAC address already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro removable MAC address", helpers.APIErrorDetail(err))
	}
}
