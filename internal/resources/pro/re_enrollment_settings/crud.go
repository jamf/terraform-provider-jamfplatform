// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetReenrollmentSettingsV1
//   pro.UpdateReenrollmentSettingsV1
//
// Status: current. Last reviewed 2026-05-29.

package re_enrollment_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions the singleton. The Re-enrollment settings object always
// exists on the tenant, so Create is a full-replace write followed by a
// read-back. The write is guarded by the shared enrollment lock because the
// Re-enrollment object shares a backing store with the User-Initiated
// Enrollment settings object.
func (r *ReEnrollmentSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ReEnrollmentSettingsResourceModel
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

	// Critical section: serialize against concurrent enrollment-settings writes.
	if r.enrollmentMu != nil {
		r.enrollmentMu.Lock()
		defer r.enrollmentMu.Unlock()
	}

	// The Re-enrollment settings singleton always exists on the tenant, so "create"
	// is really adoption. Read the live settings (inside the lock) and pass them as
	// the merge base: toggles the user did not declare keep their current value
	// instead of being reset to the server default by the full-replace write. (On
	// update the merge base is nil — UseStateForUnknown has already carried omitted
	// toggles into the plan as known prior values.)
	current, err := r.client.GetReenrollmentSettingsV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro Re-enrollment settings", helpers.APIErrorDetail(err))
		return
	}

	if !applyAndRefresh(createCtx, r.client, &plan, current, &resp.Diagnostics) {
		return
	}

	plan.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, reEnrollmentSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro Re-enrollment settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest Re-enrollment settings. Read
// is GET-only, so it does not take the enrollment write lock.
func (r *ReEnrollmentSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state ReEnrollmentSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = helpers.InitialSingletonID()
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(reEnrollmentSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetReenrollmentSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Re-enrollment settings", helpers.APIErrorDetail(err))
		return
	}
	assignReEnrollmentSettingsResourceModel(&state, got)

	state.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, reEnrollmentSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update reconciles Re-enrollment settings state via a full-replace write,
// guarded by the shared enrollment lock.
func (r *ReEnrollmentSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ReEnrollmentSettingsResourceModel
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

	// Critical section: serialize against concurrent enrollment-settings writes.
	if r.enrollmentMu != nil {
		r.enrollmentMu.Lock()
		defer r.enrollmentMu.Unlock()
	}

	if !applyAndRefresh(updateCtx, r.client, &plan, nil, &resp.Diagnostics) {
		return
	}

	plan.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, reEnrollmentSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is state-only — `terraform destroy` removes the resource from
// Terraform state and leaves the Re-enrollment settings on the tenant intact.
//
// The Re-enrollment settings object is a tenant-wide singleton that always
// exists and cannot be deleted; there is no remote delete to perform. Users who
// want to reset the options should set them explicitly and apply before
// destroy.
func (r *ReEnrollmentSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro Re-enrollment settings from Terraform state (no remote delete; settings retained on tenant)")
}

// applyAndRefresh performs the full-replace PUT built from plan (merged over
// current for any toggle the user did not declare), then reads the stored
// settings back into plan. current may be nil (update path). Returns false if a
// diagnostic was emitted. Callers must hold the enrollment write lock.
func applyAndRefresh(ctx context.Context, client *pro.Client, plan *ReEnrollmentSettingsResourceModel, current *pro.Reenrollment, diags *diag.Diagnostics) bool {
	body := buildReenrollmentInput(*plan, current)
	if _, err := client.UpdateReenrollmentSettingsV1(ctx, body); err != nil {
		diags.AddError("Error updating Jamf Pro Re-enrollment settings", helpers.APIErrorDetail(err))
		return false
	}

	got, err := client.GetReenrollmentSettingsV1(ctx)
	if err != nil {
		diags.AddError("Error reading Jamf Pro Re-enrollment settings", helpers.APIErrorDetail(err))
		return false
	}
	assignReEnrollmentSettingsResourceModel(plan, got)
	return true
}
