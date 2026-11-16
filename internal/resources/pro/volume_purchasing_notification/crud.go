// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateVolumePurchasingSubscriptionV1 (POST /v1/volume-purchasing-subscriptions → {href,id})
//   pro.GetVolumePurchasingSubscriptionV1    (GET  /v1/volume-purchasing-subscriptions/{id})
//   pro.UpdateVolumePurchasingSubscriptionV1 (PUT  /v1/volume-purchasing-subscriptions/{id})
//   pro.DeleteVolumePurchasingSubscriptionV1 (DELETE)
//   pro.ListVolumePurchasingSubscriptionsV1                (list resource)
//   pro.ResolveVolumePurchasingSubscriptionV1IDByName      (data source name lookup)
//
// Status: current.
//
// The endpoint/SDK name the feature "subscriptions"; the UI (and this provider)
// call it "Notifications". Create returns only {href,id}; state is built from a
// follow-up GET, and Update refreshes via GET too (Pro PUT responses are routinely
// lossy for server-derived fields). The write is FULL-REPLACE — wire-probed: a PUT
// omitting a collection resets it to empty, and omitting `enabled` resets it to the
// server default (true) — so the input builder always emits every collection
// (empty slice clears) and the read path flattens an empty wire array to an empty
// set rather than null.

package volume_purchasing_notification

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a notification, then refreshes state from a follow-up GET (the
// POST returns only {href,id}).
func (r *VolumePurchasingNotificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VolumePurchasingNotificationResourceModel
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

	input, diags := buildVolumePurchasingNotificationInput(createCtx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateVolumePurchasingSubscriptionV1(createCtx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Volume Purchasing notification", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing notification ID",
			"Jamf Pro returned 201 Created with no notification ID; cannot persist state.",
		)
		return
	}

	got, err := r.client.GetVolumePurchasingSubscriptionV1(createCtx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Volume Purchasing notification", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignVolumePurchasingNotificationResourceModel(createCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, volumePurchasingNotificationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Volume Purchasing notification", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest notification representation.
func (r *VolumePurchasingNotificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VolumePurchasingNotificationResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this notification without existing state or identity data, so the provider cannot determine which notification to read.",
			)
			return
		}
		var identity volumePurchasingNotificationIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing notification ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the notification.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(volumePurchasingNotificationTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Volume Purchasing notification without ID.")
		return
	}

	got, err := r.client.GetVolumePurchasingSubscriptionV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Volume Purchasing notification not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, volumePurchasingNotificationIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Volume Purchasing notification", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignVolumePurchasingNotificationResourceModel(readCtx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, volumePurchasingNotificationIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a notification, then refreshes state from a GET (PUT responses on
// Pro endpoints are routinely lossy for server-derived fields).
func (r *VolumePurchasingNotificationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VolumePurchasingNotificationResourceModel
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

	input, diags := buildVolumePurchasingNotificationInput(updateCtx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.UpdateVolumePurchasingSubscriptionV1(updateCtx, plan.ID.ValueString(), input); err != nil {
		resp.Diagnostics.AddError("Error updating Volume Purchasing notification", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetVolumePurchasingSubscriptionV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Volume Purchasing notification", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignVolumePurchasingNotificationResourceModel(updateCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, volumePurchasingNotificationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a notification.
func (r *VolumePurchasingNotificationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VolumePurchasingNotificationResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Volume Purchasing notification without ID.")
		return
	}

	if err := r.client.DeleteVolumePurchasingSubscriptionV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Volume Purchasing notification already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Volume Purchasing notification", helpers.APIErrorDetail(err))
	}
}
