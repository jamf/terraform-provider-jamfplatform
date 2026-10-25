// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateAllowedFileExtensionByID    (POST /id/0 — server mints the ID, 201)
//   proclassic.GetAllowedFileExtensionByID
//   proclassic.GetAllowedFileExtensionByExtension (data source extension lookup)
//   proclassic.DeleteAllowedFileExtensionByID     (returns 200)
//   proclassic.ListAllowedFileExtensions          (data source / list resource / collision check in collision.go)
//
// This is a create-and-delete-only record: the classic /allowedfileextensions endpoint
// exposes GET, POST .../id/{id}, and DELETE .../id/{id} only — there is no PUT (wire-
// probed: PUT /id/{id} → 403). The single user attribute `extension` is therefore
// RequiresReplace and Update never performs a write — it refreshes state from the server
// (reachable only on a timeouts-only change). Classic create POSTs to id="0"; the server
// allocates the integer ID and returns a body carrying the ID only (no extension echo) —
// Create must GET to populate `extension`. The server stores the extension with case and
// any leading dot preserved but trims surrounding whitespace (handled by a schema
// validator), and rejects a duplicate value with 409.
//
// Uniqueness: extension, enforced (409 "Duplicate extension"), case-, surrounding-space- and accent-insensitive, stored exactly as entered.
//
// Status: current. Last reviewed 2026-06-13.

package allowed_file_extension

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro allowed file extension. Classic POSTs to id="0"; the
// server allocates the real integer ID and returns it in the response body.
func (r *AllowedFileExtensionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AllowedFileExtensionResourceModel
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

	created, err := r.client.CreateAllowedFileExtensionByID(createCtx, "0", buildAllowedFileExtensionInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro allowed file extension", helpers.APIErrorDetail(err))
		return
	}
	// Defensive: the classic SDK trusts the server and would deref a nil ID; we explicitly
	// guard so a null ID never lands in state.
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing allowed file extension ID",
			"Jamf Pro returned 201 Created with no allowed file extension ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	// The create response carries the ID only (no extension echo); GET to populate extension.
	got, err := r.client.GetAllowedFileExtensionByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro allowed file extension", helpers.APIErrorDetail(err))
		return
	}
	assignAllowedFileExtensionResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, allowedFileExtensionIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro allowed file extension", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest allowed file extension representation.
func (r *AllowedFileExtensionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AllowedFileExtensionResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this allowed file extension without existing state or identity data, so the provider cannot determine which record to read.",
			)
			return
		}
		var identity allowedFileExtensionIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing allowed file extension ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the allowed file extension.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(allowedFileExtensionTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro allowed file extension without ID.")
		return
	}

	got, err := r.client.GetAllowedFileExtensionByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro allowed file extension not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, allowedFileExtensionIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro allowed file extension", helpers.APIErrorDetail(err))
		return
	}

	assignAllowedFileExtensionResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, allowedFileExtensionIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update refreshes the Terraform state for a Jamf Pro allowed file extension. The classic
// endpoint exposes no PUT, and `extension` is RequiresReplace, so this method never
// performs a write: it is reachable only when a non-replacing attribute changes (the
// timeouts block). It GETs the current record and re-sets state so the new timeout value
// persists without ever silently dropping a change. Mirrors mobile_device_provisioning_profile.
func (r *AllowedFileExtensionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AllowedFileExtensionResourceModel
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

	got, err := r.client.GetAllowedFileExtensionByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro allowed file extension", helpers.APIErrorDetail(err))
		return
	}
	assignAllowedFileExtensionResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, allowedFileExtensionIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro allowed file extension.
func (r *AllowedFileExtensionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AllowedFileExtensionResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro allowed file extension without ID.")
		return
	}

	if err := r.client.DeleteAllowedFileExtensionByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro allowed file extension already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro allowed file extension", helpers.APIErrorDetail(err))
	}
}
