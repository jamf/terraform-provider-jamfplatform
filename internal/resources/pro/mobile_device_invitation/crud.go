// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateMobileDeviceInvitationByID      (POST /mobiledeviceinvitations/id/0)
//   proclassic.GetMobileDeviceInvitationByID         (GET-after-create, Read)
//   proclassic.DeleteMobileDeviceInvitationByID
//   proclassic.GetMobileDeviceInvitationByInvitation (data source: lookup by code)
//   proclassic.ListMobileDeviceInvitations           (list resource)
//
// There is no usable update endpoint on /mobiledeviceinvitations. The SDK
// exposes UpdateMobileDeviceInvitationByInvitation, but the server rejects PUT
// (`409 "Put is not supported"`), so it is never called. The resource is
// create + delete only; every user-settable attribute is RequiresReplace. The
// Update method below is required by the framework but never performs a
// mutation.
//
// Status: current.

package mobile_device_invitation

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro mobile device invitation. Classic POSTs to
// id="0"; the server mints the numeric ID and the `invitation` code and returns
// them in the (otherwise sparse) response body, so we GET-after-create to
// populate the full state.
func (r *MobileDeviceInvitationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MobileDeviceInvitationResourceModel
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

	// enroll_into_site_id is the user's site ID (Optional+Computed). Omitted →
	// Unknown → no site block, so the server defaults to `-1`/`NONE`.
	var siteID string
	if !plan.EnrollIntoSiteID.IsNull() && !plan.EnrollIntoSiteID.IsUnknown() {
		siteID = plan.EnrollIntoSiteID.ValueString()
	}

	created, err := r.client.CreateMobileDeviceInvitationByID(createCtx, "0", buildMobileDeviceInvitationInput(plan, siteID))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro mobile device invitation", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing mobile device invitation ID",
			"Jamf Pro returned success with no mobile device invitation ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	// The POST body carries only <id> + <invitation>; GET to capture the full
	// representation (expiration echo, site, server-defaulted bools, target_ios).
	got, err := r.client.GetMobileDeviceInvitationByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro mobile device invitation", helpers.APIErrorDetail(err))
		return
	}
	assignMobileDeviceInvitationResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileDeviceInvitationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro mobile device invitation", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest mobile device invitation
// representation. Reconcile uses GET-by-id (never the LIST endpoint, which lags
// newly created invitations). Import-time refresh sources the ID from the
// identity object so users can `terraform import` by the numeric ID.
func (r *MobileDeviceInvitationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MobileDeviceInvitationResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this mobile device invitation without existing state or identity data, so the provider cannot determine which invitation to read.",
			)
			return
		}
		var identity mobileDeviceInvitationIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing mobile device invitation ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the mobile device invitation.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(mobileDeviceInvitationTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro mobile device invitation without ID.")
		return
	}

	got, err := r.client.GetMobileDeviceInvitationByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro mobile device invitation not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileDeviceInvitationIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro mobile device invitation", helpers.APIErrorDetail(err))
		return
	}

	assignMobileDeviceInvitationResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileDeviceInvitationIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is dead code in practice: /mobiledeviceinvitations has no usable update
// operation (the server rejects PUT) and every user-settable attribute is
// RequiresReplace, so Terraform always destroys + recreates on a content change
// and never calls Update for a real diff. The method is still required by the
// framework, and a diff confined to the non-RequiresReplace `timeouts` block can
// route here — so rather than erroring (which would break a timeouts-only apply)
// we GET-refresh the server-derived fields and persist the plan.
func (r *MobileDeviceInvitationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MobileDeviceInvitationResourceModel
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

	got, err := r.client.GetMobileDeviceInvitationByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro mobile device invitation", helpers.APIErrorDetail(err))
		return
	}
	assignMobileDeviceInvitationResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileDeviceInvitationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro mobile device invitation. Delete is synchronous and
// a subsequent GET returns a clean 404.
func (r *MobileDeviceInvitationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MobileDeviceInvitationResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro mobile device invitation without ID.")
		return
	}

	if err := r.client.DeleteMobileDeviceInvitationByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro mobile device invitation already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro mobile device invitation", helpers.APIErrorDetail(err))
	}
}
