// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Blueprint resource in Terraform.
func (r *BlueprintResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data BlueprintResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultCreateTimeout, data.Timeouts.Create)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	createCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	deviceGroups, diags := helpers.SetToStringSlice(createCtx, data.DeviceGroups)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	steps, diags := r.buildSteps(ctx, &data)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	reqBody := &blueprints.CreateBlueprintRequest{
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueStringPointer(),
		Scope: blueprints.CreateScope{
			DeviceGroups: deviceGroups,
		},
		Steps: steps,
	}

	createResp, err := r.client.CreateBlueprint(createCtx, reqBody)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating blueprint",
			"Could not create blueprint: "+helpers.APIErrorDetail(err),
		)
		return
	}

	desiredDeployed := desiredDeployedValue(data.Deployed)
	blueprint, err := r.reconcileBlueprintDeployment(createCtx, createResp.ID, desiredDeployed)
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Blueprint deployment reconciliation failed",
			"Blueprint was created successfully but deployment could not be fully reconciled: "+err.Error()+
				". Check your Jamf instance to verify the blueprint deployment status.",
		)
		blueprint, _ = r.client.GetBlueprint(createCtx, createResp.ID)
		if blueprint == nil {
			resp.Diagnostics.AddError(
				"Error reading created blueprint",
				"Could not read created blueprint after deployment reconciliation failure: "+helpers.APIErrorDetail(err),
			)
			return
		}
	}

	resp.Diagnostics.Append(checkLegacyPayloadDiscards(&data, blueprint)...)
	resp.Diagnostics.Append(updateModelFromAPIResponse(ctx, &data, blueprint)...)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, blueprintIdentityModel{ID: data.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created a resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read reads the Blueprint resource state from the API.
func (r *BlueprintResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data BlueprintResourceModel

	if req.State.Raw.IsNull() {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this blueprint without existing state or identity data, so the provider cannot determine which blueprint to read.",
			)
			return
		}

		var identity blueprintIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}

		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing blueprint ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the blueprint.",
			)
			return
		}

		data.ID = identity.ID
		data.Timeouts = helpers.NewResourceTimeoutsNullValue(blueprintTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	blueprint, err := r.client.GetBlueprint(readCtx, data.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Blueprint not found, removing from state", map[string]any{
				"blueprint_id": data.ID.ValueString(),
			})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, blueprintIdentityModel{ID: data.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error reading blueprint",
			"Could not read blueprint: "+helpers.APIErrorDetail(err),
		)
		return
	}

	resp.Diagnostics.Append(updateModelFromAPIResponse(ctx, &data, blueprint)...)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, blueprintIdentityModel{ID: data.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update updates the Blueprint resource.
func (r *BlueprintResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data BlueprintResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultUpdateTimeout, data.Timeouts.Update)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	deviceGroups, diags := helpers.SetToStringSlice(updateCtx, data.DeviceGroups)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	steps, diags := r.buildSteps(ctx, &data)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	updateReq := &blueprints.UpdateBlueprintRequest{
		Name:        data.Name.ValueStringPointer(),
		Description: data.Description.ValueStringPointer(),
		Scope: &blueprints.BlueprintScope{
			DeviceGroups: deviceGroups,
		},
		Steps: &steps,
	}

	if err := r.client.UpdateBlueprint(updateCtx, data.ID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError(
			"Error updating blueprint",
			"Could not update blueprint: "+helpers.APIErrorDetail(err),
		)
		return
	}

	desiredDeployed := desiredDeployedValue(data.Deployed)
	blueprint, err := r.reconcileBlueprintDeployment(updateCtx, data.ID.ValueString(), desiredDeployed)
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Blueprint deployment reconciliation failed",
			"Blueprint was updated successfully but deployment could not be fully reconciled: "+err.Error()+
				". Check your Jamf instance to verify the blueprint deployment status.",
		)
		blueprint, _ = r.client.GetBlueprint(updateCtx, data.ID.ValueString())
		if blueprint == nil {
			resp.Diagnostics.AddError(
				"Error reading updated blueprint",
				"Could not read updated blueprint after deployment reconciliation failure: "+helpers.APIErrorDetail(err),
			)
			return
		}
	}

	resp.Diagnostics.Append(checkLegacyPayloadDiscards(&data, blueprint)...)
	resp.Diagnostics.Append(updateModelFromAPIResponse(ctx, &data, blueprint)...)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, blueprintIdentityModel{ID: data.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete deletes the Blueprint resource.
//
// The warning branch returns without an error diagnostic, which makes the
// framework drop the resource from state, so which failures may take it is
// isDeleteMaybeComplete's decision rather than a bare 500 check.
func (r *BlueprintResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data BlueprintResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultDeleteTimeout, data.Timeouts.Delete)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	if !helpers.IsConfiguredValue(data.ID) || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot delete blueprint without ID.")
		return
	}

	err := r.client.DeleteBlueprint(deleteCtx, data.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Blueprint already deleted", map[string]any{
				"blueprint_id": data.ID.ValueString(),
			})
			return
		}

		if isDeleteMaybeComplete(err) {
			resp.Diagnostics.AddWarning(
				"Blueprint deletion encountered server error",
				"Delete operation encountered a server error: "+helpers.APIErrorDetail(err)+
					". The blueprint may have been deleted despite the error. Check your Jamf instance to verify the blueprint status.",
			)
			return
		}

		resp.Diagnostics.AddError(
			"Error deleting blueprint",
			"Could not delete blueprint: "+helpers.APIErrorDetail(err),
		)
		return
	}
}
