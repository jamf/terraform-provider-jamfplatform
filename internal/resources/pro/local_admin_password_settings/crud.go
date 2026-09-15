// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetLocalAdminPasswordSettingsV2
//   pro.UpdateLocalAdminPasswordSettingsV2
//
// Not adopted (per-device / operational LAPS endpoints with no Terraform
// analogue — read/command surface, managed elsewhere):
//   /v2/local-admin-password/{clientManagementId}/accounts
//   /v2/local-admin-password/{clientManagementId}/account/{username}/password
//   /v2/local-admin-password/{clientManagementId}/account/{username}/audit
//   /v2/local-admin-password/{clientManagementId}/account/{username}/history
//   /v2/local-admin-password/pending-rotations
//   /v2/local-admin-password/{clientManagementId}/account/{username}/set-password
//
// Status: current. Last reviewed 2026-06-11.

package local_admin_password_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions the singleton. The LAPS settings object always exists on the
// tenant, so Create is really adoption: read the live settings, merge the
// declared controls over them, then full-replace write and adopt the echoed
// result as state.
func (r *LocalAdminPasswordSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan LocalAdminPasswordSettingsResourceModel
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

	if !applyAndRefresh(createCtx, r.client, &plan, &resp.Diagnostics) {
		return
	}

	plan.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, localAdminPasswordSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro local administrator password settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest LAPS settings.
func (r *LocalAdminPasswordSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state LocalAdminPasswordSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = helpers.InitialSingletonID()
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(localAdminPasswordSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetLocalAdminPasswordSettingsV2(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro local administrator password settings", helpers.APIErrorDetail(err))
		return
	}
	assignLocalAdminPasswordSettingsResourceModel(&state, got, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	state.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, localAdminPasswordSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update reconciles LAPS settings via a full-replace write.
func (r *LocalAdminPasswordSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan LocalAdminPasswordSettingsResourceModel
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

	if !applyAndRefresh(updateCtx, r.client, &plan, &resp.Diagnostics) {
		return
	}

	plan.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, localAdminPasswordSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is state-only — `terraform destroy` removes the resource from Terraform
// state and leaves the LAPS settings on the tenant intact.
//
// The LAPS settings object is a tenant-wide singleton that always exists and
// cannot be deleted; there is no remote delete to perform. Users who want to
// reset the controls should set them explicitly and apply before destroy.
func (r *LocalAdminPasswordSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro local administrator password settings from Terraform state (no remote delete; settings retained on tenant)")
}

// applyAndRefresh reads the live settings (the merge base for undeclared
// controls), performs the full-replace write built from plan, then adopts the
// echoed result into plan. Returns false if a diagnostic was emitted.
func applyAndRefresh(ctx context.Context, client *pro.Client, plan *LocalAdminPasswordSettingsResourceModel, diags *diag.Diagnostics) bool {
	current, err := client.GetLocalAdminPasswordSettingsV2(ctx)
	if err != nil {
		diags.AddError("Error reading existing Jamf Pro local administrator password settings", helpers.APIErrorDetail(err))
		return false
	}

	body := buildLocalAdminPasswordSettingsInput(*plan, current)

	if _, err := client.UpdateLocalAdminPasswordSettingsV2(ctx, body); err != nil {
		diags.AddError("Error updating Jamf Pro local administrator password settings", helpers.APIErrorDetail(err))
		return false
	}

	// Mandatory post-write read-back (singleton convention): capture authoritative
	// state rather than trusting the write echo, so any server-side transformation
	// or future field addition is picked up without code changes.
	got, err := client.GetLocalAdminPasswordSettingsV2(ctx)
	if err != nil {
		diags.AddError("Error reading Jamf Pro local administrator password settings", helpers.APIErrorDetail(err))
		return false
	}

	assignLocalAdminPasswordSettingsResourceModel(plan, got, diags)
	return !diags.HasError()
}
