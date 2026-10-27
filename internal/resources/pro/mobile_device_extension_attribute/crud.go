// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateMobileDeviceExtensionAttributeV1            (POST — returns Href; ID parsed, then GET)
//   pro.GetMobileDeviceExtensionAttributeV1
//   pro.UpdateMobileDeviceExtensionAttributeV1            (PUT — full replace; GET after)
//   pro.DeleteMobileDeviceExtensionAttributeV1
//   pro.ListMobileDeviceExtensionAttributesV1             (list resource, collision check in collision.go)
//   pro.ResolveMobileDeviceExtensionAttributeV1ByName     (data source name lookup)
//
// Uniqueness: name, enforced (400 DUPLICATE_FIELD), trimmed on save, case- and accent-insensitive.
//
// Status: current. Last reviewed 2026-10-25.

package mobile_device_extension_attribute

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new mobile device extension attribute. The Pro POST returns
// only an href + id, so the ID is parsed and the full representation read back.
func (r *MobileDeviceExtensionAttributeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan MobileDeviceExtensionAttributeResourceModel
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

	input, inputDiags := buildMobileDeviceExtensionAttributeInput(createCtx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateMobileDeviceExtensionAttributeV1(createCtx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro mobile device extension attribute", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	if created == nil || created.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing mobile device extension attribute ID",
			"Jamf Pro returned 201 Created with no extension attribute ID; cannot persist state.",
		)
		return
	}
	plan.ID = types.StringValue(created.ID)

	got, err := r.client.GetMobileDeviceExtensionAttributeV1(createCtx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro mobile device extension attribute", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignMobileDeviceExtensionAttributeResourceModel(createCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileDeviceExtensionAttributeIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro mobile device extension attribute", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest EA representation.
func (r *MobileDeviceExtensionAttributeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state MobileDeviceExtensionAttributeResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this mobile device extension attribute without existing state or identity data, so the provider cannot determine which EA to read.",
			)
			return
		}
		var identity mobileDeviceExtensionAttributeIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing mobile device extension attribute ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the EA.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(mobileDeviceExtensionAttributeTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro mobile device extension attribute without ID.")
		return
	}

	got, err := r.client.GetMobileDeviceExtensionAttributeV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro mobile device extension attribute not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileDeviceExtensionAttributeIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro mobile device extension attribute", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignMobileDeviceExtensionAttributeResourceModel(readCtx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileDeviceExtensionAttributeIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a mobile device extension attribute. The Pro PUT is a full
// replace; a fresh GET supplies the canonical representation.
func (r *MobileDeviceExtensionAttributeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MobileDeviceExtensionAttributeResourceModel
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

	input, inputDiags := buildMobileDeviceExtensionAttributeInput(updateCtx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.UpdateMobileDeviceExtensionAttributeV1(updateCtx, plan.ID.ValueString(), input); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro mobile device extension attribute", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetMobileDeviceExtensionAttributeV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro mobile device extension attribute", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignMobileDeviceExtensionAttributeResourceModel(updateCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileDeviceExtensionAttributeIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a mobile device extension attribute.
func (r *MobileDeviceExtensionAttributeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MobileDeviceExtensionAttributeResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro mobile device extension attribute without ID.")
		return
	}

	if err := r.client.DeleteMobileDeviceExtensionAttributeV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro mobile device extension attribute already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro mobile device extension attribute", helpers.APIErrorDetail(err))
	}
}
