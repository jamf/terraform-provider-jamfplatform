// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetTeacherAppSettingsV1
//   pro.UpdateTeacherAppSettingsV1
//
// Not adopted:
//   pro.ListTeacherAppHistoryV1, pro.CreateTeacherAppHistoryNoteV1 — object
//     history endpoints (convention: history is not modeled).
//
// The PUT is FULL-REPLACE and echoes the body (wire-probed 2026-06-10): omitted
// optional fields are reset (autoClear / maxRestrictionLengthSeconds → null,
// safelistedApps → [], isEnabled → false) and an omitted timezoneId is rejected
// with HTTP 500 — mandatory on every PUT. State is sourced from a GET-after-write
// rather than the echo (singleton convention).
//
// The response-only fields displayNameType and features are intentionally NOT
// modeled: both are wire-proven ignored on PUT (2026-06-10 — sent values are
// dropped, the stored values echo back) and absent from TeacherSettingsRequest;
// they are managed outside this endpoint.
//
// Status: current. Last reviewed 2026-06-10.

package jamf_teacher_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions the singleton. The Jamf Teacher settings object always
// exists on the tenant, so Create is really adoption: read the live settings
// and pass them as the merge base so fields the user did not declare keep their
// current value instead of being reset by the full-replace write. (On update
// the merge base is nil — UseStateForUnknown has already carried omitted fields
// into the plan as known prior values.)
func (r *JamfTeacherSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan JamfTeacherSettingsResourceModel
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

	current, err := r.client.GetTeacherAppSettingsV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro Jamf Teacher settings", helpers.APIErrorDetail(err))
		return
	}

	if !applyAndRefresh(createCtx, r.client, &plan, current, &resp.Diagnostics) {
		return
	}

	plan.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfTeacherSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro Jamf Teacher settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest Jamf Teacher settings.
func (r *JamfTeacherSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state JamfTeacherSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = helpers.InitialSingletonID()
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(jamfTeacherSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetTeacherAppSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Jamf Teacher settings", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignJamfTeacherSettingsResourceModel(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfTeacherSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update reconciles Jamf Teacher settings via the full-replace write. The merge
// base is nil — UseStateForUnknown has already carried omitted fields into the
// plan as known prior values.
func (r *JamfTeacherSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan JamfTeacherSettingsResourceModel
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

	if !applyAndRefresh(updateCtx, r.client, &plan, nil, &resp.Diagnostics) {
		return
	}

	plan.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfTeacherSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is state-only — `terraform destroy` removes the resource from
// Terraform state and leaves the Jamf Teacher settings on the tenant intact.
//
// The Jamf Teacher settings object is a tenant-wide singleton that always
// exists and cannot be deleted; there is no remote delete to perform. No SDK
// call is made and no diagnostics are emitted; Terraform removes the resource
// from state on its own after the handler returns.
func (r *JamfTeacherSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro Jamf Teacher settings from Terraform state (singleton — no remote delete)")
}

// applyAndRefresh performs the full-replace PUT built from plan (merged over
// current for any field the user did not declare), then reads the stored
// settings back into plan via a GET (authoritative state — singleton
// convention). current may be nil (update path). Returns false if a diagnostic
// was emitted.
func applyAndRefresh(ctx context.Context, client *pro.Client, plan *JamfTeacherSettingsResourceModel, current *pro.TeacherSettingsResponse, diags *diag.Diagnostics) bool {
	body, buildDiags := buildTeacherSettingsInput(ctx, *plan, current)
	diags.Append(buildDiags...)
	if diags.HasError() {
		return false
	}

	if _, err := client.UpdateTeacherAppSettingsV1(ctx, body); err != nil {
		diags.AddError("Error updating Jamf Pro Jamf Teacher settings", helpers.APIErrorDetail(err))
		return false
	}

	got, err := client.GetTeacherAppSettingsV1(ctx)
	if err != nil {
		diags.AddError("Error reading Jamf Pro Jamf Teacher settings after write", helpers.APIErrorDetail(err))
		return false
	}

	diags.Append(assignJamfTeacherSettingsResourceModel(ctx, plan, got)...)
	return !diags.HasError()
}
