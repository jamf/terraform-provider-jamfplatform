// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateDirectoryBindingByID
//   proclassic.GetDirectoryBindingByID
//   proclassic.UpdateDirectoryBindingByID
//   proclassic.DeleteDirectoryBindingByID
//   proclassic.ListDirectoryBindings        (data source / list resource)
//   proclassic.GetDirectoryBindingByName    (data source name lookup)
//
// Write semantics: the PUT merges field by field; an omitted element keeps
// the stored value and an empty <domain>/<username>/<computer_ou> clears it
// (wire-probed 2026-09-06 on an Active Directory binding). See
// buildDirectoryBindingInput.
//
// Uniqueness: name, enforced (409), case-, leading-space-, trailing-space- and accent-insensitive. The plan-time collision check lists with proclassic.ListDirectoryBindings (collision.go).
//
// Status: current. Last reviewed 2026-09-06.

package directory_binding

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro directory binding. Classic POSTs to id="0";
// the server allocates the real integer ID and returns it in the response
// body. We then GET to capture server-populated fields (priority default,
// etc.).
func (r *DirectoryBindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DirectoryBindingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg DirectoryBindingResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
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

	created, err := r.client.CreateDirectoryBindingByID(createCtx, "0", buildDirectoryBindingInput(plan, helpers.OptionalStringPointer(cfg.Password)))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro directory binding", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing directory binding ID",
			"Jamf Pro returned 201 Created with no directory binding ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	got, err := r.client.GetDirectoryBindingByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro directory binding", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignDirectoryBindingResourceModel(&plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, directoryBindingIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro directory binding", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest directory binding
// representation. Import-time refresh (req.State.Raw is null) sources the
// ID from the identity object so users can `terraform import` by the
// binding's integer ID.
func (r *DirectoryBindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DirectoryBindingResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this directory binding without existing state or identity data, so the provider cannot determine which binding to read.",
			)
			return
		}
		var identity directoryBindingIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing directory binding ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the directory binding.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(directoryBindingTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro directory binding without ID.")
		return
	}

	got, err := r.client.GetDirectoryBindingByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro directory binding not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, directoryBindingIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro directory binding", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignDirectoryBindingResourceModel(&state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, directoryBindingIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro directory binding. Classic /directorybindings
// applies PUT as a partial merge: omitted tags preserve, empty tags clear,
// set tags overwrite. The input builder emits unset nested blocks as nil
// pointers (omitted on the wire) so the server preserves stored values for
// untouched type-specific fields. After the write we GET to refresh
// server-computed fields such as the server-allocated priority.
func (r *DirectoryBindingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DirectoryBindingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state DirectoryBindingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg DirectoryBindingResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
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

	// Only include the plaintext `<password>` element on the wire when the
	// user bumped `password_wo_version`. Otherwise omit so the server retains
	// the existing stored value under Classic's partial-merge semantics.
	var password *string
	if !plan.PasswordWoVersion.Equal(state.PasswordWoVersion) {
		password = helpers.OptionalStringPointer(cfg.Password)
	}

	if err := r.client.UpdateDirectoryBindingByID(updateCtx, plan.ID.ValueString(), buildDirectoryBindingInput(plan, password)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro directory binding", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetDirectoryBindingByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro directory binding", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignDirectoryBindingResourceModel(&plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, directoryBindingIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro directory binding.
func (r *DirectoryBindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DirectoryBindingResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro directory binding without ID.")
		return
	}

	if err := r.client.DeleteDirectoryBindingByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro directory binding already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro directory binding", helpers.APIErrorDetail(err))
	}
}
