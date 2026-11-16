// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetEnrollmentAccessManagementV4
//   pro.UpdateEnrollmentAccessManagementV4
//
// Status: current.

package access_management_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create handles initial provisioning of the Access Management settings singleton. The
// Jamf Pro API has no Create endpoint for this object — one record per tenant already
// exists — so this funnels into the Configure (POST) call against the plan, then reads
// back to capture authoritative state.
//
// The singleton always exists, so "create" is really adoption: read the live setting and
// pass it as the merge base so an omitted field keeps its current value rather than being
// cleared by the full-replace write. On update the merge base is nil — UseStateForUnknown
// has already carried an omitted field into the plan as its known prior value.
func (r *AccessManagementSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan AccessManagementSettingsResourceModel
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

	current, err := r.client.GetEnrollmentAccessManagementV4(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro Access Management settings", helpers.APIErrorDetail(err))
		return
	}

	if _, err := r.client.UpdateEnrollmentAccessManagementV4(createCtx, buildAccessManagementSettingsInput(plan, current)); err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro Access Management settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetEnrollmentAccessManagementV4(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Access Management settings after write", helpers.APIErrorDetail(err))
		return
	}
	assignAccessManagementSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, accessManagementSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro Access Management settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest Access Management settings.
func (r *AccessManagementSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state AccessManagementSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(accessManagementSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetEnrollmentAccessManagementV4(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Access Management settings", helpers.APIErrorDetail(err))
		return
	}

	assignAccessManagementSettingsResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, accessManagementSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings to the Jamf Pro API. Same SDK call as Create.
// Authoritative state comes from a follow-up GET.
func (r *AccessManagementSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan AccessManagementSettingsResourceModel
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

	if _, err := r.client.UpdateEnrollmentAccessManagementV4(updateCtx, buildAccessManagementSettingsInput(plan, nil)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro Access Management settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetEnrollmentAccessManagementV4(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Access Management settings after update", helpers.APIErrorDetail(err))
		return
	}
	assignAccessManagementSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, accessManagementSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. Singleton objects cannot be deleted — the
// record persists on the tenant. Terraform removes the resource from state on its own
// after this handler returns. Users who want to clear the setting should set
// `automated_device_enrollment_server_uuid = ""` and apply before destroy.
func (r *AccessManagementSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro Access Management settings from Terraform state (singleton — no remote delete)")
}
