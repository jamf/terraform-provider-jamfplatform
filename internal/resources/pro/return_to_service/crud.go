// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateReturnToServiceConfigurationV1     (POST — returns Href + ID only; full record read back via GET)
//   pro.GetReturnToServiceConfigurationV1        (Read)
//   pro.UpdateReturnToServiceConfigurationV1     (PUT — full replace; both fields required; GET after for canonical state)
//   pro.DeleteReturnToServiceConfigurationV1     (DELETE)
//   pro.ListReturnToServiceConfigurationsV1      (list resource)
//   pro.ResolveReturnToServiceConfigurationV1ByName (data source name lookup; *AmbiguousMatchError on duplicate names)
//
// Not adopted: pro.ApplyReturnToServiceConfigurationV1 (create-or-update-by-name
// convenience) — the resource owns the id lifecycle directly.
//
// Related but separate surface: the inline ReturnToService payload
// (enabled/bootstrapToken/mdmProfileData/wifiProfileData) is a field on the
// erase-device command (EraseDeviceComputerRequest.returnToService) under
// internal/actions/device/, not this configuration endpoint.
//
// Status: current.

package return_to_service

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Return to Service configuration. The Pro POST returns
// only an href + id (not the created object), so the full representation is read
// back via GET; the endpoint is immediately consistent, so no polling is needed.
func (r *ReturnToServiceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ReturnToServiceResourceModel
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

	created, err := r.client.CreateReturnToServiceConfigurationV1(createCtx, buildReturnToServiceInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro Return to Service configuration", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing Return to Service configuration ID",
			"Jamf Pro returned 201 Created with no configuration ID; cannot persist state.",
		)
		return
	}
	plan.ID = types.StringValue(created.ID)

	got, err := r.client.GetReturnToServiceConfigurationV1(createCtx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro Return to Service configuration", helpers.APIErrorDetail(err))
		return
	}
	assignReturnToServiceResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, returnToServiceIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro Return to Service configuration", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest configuration representation.
func (r *ReturnToServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ReturnToServiceResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this Return to Service configuration without existing state or identity data, so the provider cannot determine which configuration to read.",
			)
			return
		}
		var identity returnToServiceIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing Return to Service configuration ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the configuration.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(returnToServiceTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro Return to Service configuration without ID.")
		return
	}

	got, err := r.client.GetReturnToServiceConfigurationV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro Return to Service configuration not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, returnToServiceIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro Return to Service configuration", helpers.APIErrorDetail(err))
		return
	}

	assignReturnToServiceResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, returnToServiceIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Return to Service configuration. The Pro PUT is a full
// replace requiring both writable fields; a GET afterwards refreshes state from
// the canonical representation.
func (r *ReturnToServiceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ReturnToServiceResourceModel
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

	if _, err := r.client.UpdateReturnToServiceConfigurationV1(updateCtx, plan.ID.ValueString(), buildReturnToServiceInput(plan)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro Return to Service configuration", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetReturnToServiceConfigurationV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro Return to Service configuration", helpers.APIErrorDetail(err))
		return
	}
	assignReturnToServiceResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, returnToServiceIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Return to Service configuration.
func (r *ReturnToServiceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ReturnToServiceResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro Return to Service configuration without ID.")
		return
	}

	if err := r.client.DeleteReturnToServiceConfigurationV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro Return to Service configuration already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro Return to Service configuration", helpers.APIErrorDetail(err))
	}
}
