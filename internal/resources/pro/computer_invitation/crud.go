// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateComputerInvitationByID      (POST /computerinvitations/id/0)
//   proclassic.GetComputerInvitationByID         (GET-after-create, Read)
//   proclassic.DeleteComputerInvitationByID
//   proclassic.GetComputerInvitationByInvitation (data source: lookup by code)
//   proclassic.ListComputerInvitations           (list resource)
//
// There is no update endpoint on /computerinvitations (the SDK exposes no
// update function and the server has no PUT route). The resource is create +
// delete only; every user-settable attribute is RequiresReplace. The Update
// method below is required by the framework but never performs a mutation.
//
// Status: current.

package computer_invitation

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro computer invitation. Classic POSTs to id="0";
// the server mints the numeric ID and the `invitation` code and returns them in
// the (otherwise sparse) response body, so we GET-after-create to populate the
// full state.
//
// Operational note: through the API gateway, this POST has been observed to
// occasionally return HTTP 500 yet still commit server-side (no id in the 500
// body). We do not attempt orphan recovery — the LIST endpoint lags newly
// created invitations, so a post-error scan cannot reliably find the orphan.
// The SDK surfaces the 500 as an error and Create fails; a stray invitation may
// be left on the tenant in that rare case.
func (r *ComputerInvitationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ComputerInvitationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// ssh_password is WriteOnly: null in the plan model, so read it from config.
	var cfg ComputerInvitationResourceModel
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

	// enroll_into_site_id is the user's site ID (Optional+Computed). Omitted →
	// Unknown → no site block, so the server defaults to `-1`/`NONE`.
	var siteID string
	if !plan.EnrollIntoSiteID.IsNull() && !plan.EnrollIntoSiteID.IsUnknown() {
		siteID = plan.EnrollIntoSiteID.ValueString()
	}

	created, err := r.client.CreateComputerInvitationByID(createCtx, "0", buildComputerInvitationInput(plan, helpers.OptionalStringPointer(cfg.SSHPassword), siteID))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro computer invitation", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing computer invitation ID",
			"Jamf Pro returned success with no computer invitation ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	// The POST body carries only <id> + <invitation>; GET to capture the full
	// representation (status, expiration echo, site, server-defaulted bools).
	got, err := r.client.GetComputerInvitationByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro computer invitation", helpers.APIErrorDetail(err))
		return
	}
	assignComputerInvitationResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, computerInvitationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro computer invitation", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest computer invitation
// representation. Reconcile uses GET-by-id (never the LIST endpoint, which lags
// newly created invitations). Import-time refresh sources the ID from the
// identity object so users can `terraform import` by the numeric ID.
func (r *ComputerInvitationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ComputerInvitationResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this computer invitation without existing state or identity data, so the provider cannot determine which invitation to read.",
			)
			return
		}
		var identity computerInvitationIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing computer invitation ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the computer invitation.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(computerInvitationTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro computer invitation without ID.")
		return
	}

	got, err := r.client.GetComputerInvitationByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro computer invitation not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, computerInvitationIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro computer invitation", helpers.APIErrorDetail(err))
		return
	}

	assignComputerInvitationResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, computerInvitationIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is dead code in practice: /computerinvitations has no update operation
// and every user-settable attribute is RequiresReplace, so Terraform always
// destroys + recreates on a content change and never calls Update for a real
// diff. The method is still required by the framework, and a diff confined to
// the non-RequiresReplace `timeouts` block can route here — so rather than
// erroring (which would break a timeouts-only apply) we GET-refresh the
// server-derived fields and persist the plan.
func (r *ComputerInvitationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ComputerInvitationResourceModel
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

	got, err := r.client.GetComputerInvitationByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro computer invitation", helpers.APIErrorDetail(err))
		return
	}
	assignComputerInvitationResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, computerInvitationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro computer invitation. Delete is synchronous and a
// subsequent GET returns a clean 404.
func (r *ComputerInvitationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ComputerInvitationResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro computer invitation without ID.")
		return
	}

	if err := r.client.DeleteComputerInvitationByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro computer invitation already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro computer invitation", helpers.APIErrorDetail(err))
	}
}
