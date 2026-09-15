// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetParentAppSettingsV1
//   pro.UpdateParentAppSettingsV1
//
// Not adopted:
//   pro.ListParentAppHistoryV1, pro.CreateParentAppHistoryNoteV1 — object
//     history endpoints (convention: history is not modeled).
//
// The PUT is FULL-REPLACE and echoes the body (wire-probed 2026-06-10):
// omitted optional fields are reset (safelistedApps → [], the bools → false
// except allowTemplates → true); an omitted restrictedTimes or timezoneId is
// rejected with HTTP 500, and an omitted deviceGroupId decodes to 0 and 400s
// — all three are mandatory on every PUT. State is sourced from a
// GET-after-write rather than the echo (singleton convention).
//
// allowTemplates is intentionally NOT modeled (maintainer decision: it does
// not appear in the Jamf Pro UI). Because the full-replace PUT would otherwise
// reset it to its server default (true), the field is ROUND-TRIPPED verbatim
// per STYLE_GUIDE §Full-replace endpoints item 3 "round-trip non-owned
// fields": BOTH Create and Update read the live settings first and pass the
// stored AllowTemplates pointer through to the PUT unchanged. This is why this
// resource — unlike jamf_teacher_settings, which owns every request field and
// only GETs on create — performs the GET → overlay owned fields → PUT sequence
// on every write. No provider mutex is needed: only this resource writes
// /v1/parent-app, so there is no same-process read-merge-write race. The
// residual cross-process race (two concurrent applies from different
// processes) is N/A beyond the single-field carry — last-write-wins, as for
// any singleton without optimistic concurrency.
//
// Status: current. Last reviewed 2026-06-10.

package jamf_parent_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions the singleton. The Jamf Parent settings object always
// exists on the tenant, so Create is really adoption: applyAndRefresh reads
// the live settings and passes them as the merge base so fields the user did
// not declare keep their current value instead of being reset by the
// full-replace write — and so the unmodeled allowTemplates field is carried
// through unchanged.
func (r *JamfParentSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan JamfParentSettingsResourceModel
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

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfParentSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro Jamf Parent settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest Jamf Parent settings.
func (r *JamfParentSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state JamfParentSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = helpers.InitialSingletonID()
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(jamfParentSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetParentAppSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Jamf Parent settings", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignJamfParentSettingsResourceModel(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = helpers.InitialSingletonID()

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfParentSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update reconciles Jamf Parent settings via the full-replace write. Unlike
// the teacher sibling, Update also fetches the live settings first: by now
// UseStateForUnknown has already carried every omitted Optional+Computed field
// into the plan as a known prior value, so the merge base effectively only
// supplies the unmodeled allowTemplates field, round-tripped verbatim per
// STYLE_GUIDE §768.3 (see the annotation block at the top of this file).
func (r *JamfParentSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan JamfParentSettingsResourceModel
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

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfParentSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is state-only — `terraform destroy` removes the resource from
// Terraform state and leaves the Jamf Parent settings on the tenant intact.
//
// The Jamf Parent settings object is a tenant-wide singleton that always
// exists and cannot be deleted; there is no remote delete to perform. No SDK
// call is made and no diagnostics are emitted; Terraform removes the resource
// from state on its own after the handler returns.
func (r *JamfParentSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro Jamf Parent settings from Terraform state (singleton — no remote delete)")
}

// applyAndRefresh performs the GET → overlay owned fields → full-replace PUT
// sequence, then reads the stored settings back into plan via a second GET
// (authoritative state — singleton convention). The initial GET runs on every
// write — Create AND Update — because the unmodeled allowTemplates field must
// be round-tripped verbatim through the full-replace PUT (§768.3); on create
// the same read also serves as the adopt merge base for undeclared
// Optional+Computed fields. Returns false if a diagnostic was emitted.
func applyAndRefresh(ctx context.Context, client *pro.Client, plan *JamfParentSettingsResourceModel, diags *diag.Diagnostics) bool {
	current, err := client.GetParentAppSettingsV1(ctx)
	if err != nil {
		diags.AddError("Error reading existing Jamf Pro Jamf Parent settings", helpers.APIErrorDetail(err))
		return false
	}

	body, buildDiags := buildParentSettingsInput(ctx, *plan, current)
	diags.Append(buildDiags...)
	if diags.HasError() {
		return false
	}

	if _, err := client.UpdateParentAppSettingsV1(ctx, body); err != nil {
		diags.AddError("Error updating Jamf Pro Jamf Parent settings", helpers.APIErrorDetail(err))
		return false
	}

	got, err := client.GetParentAppSettingsV1(ctx)
	if err != nil {
		diags.AddError("Error reading Jamf Pro Jamf Parent settings after write", helpers.APIErrorDetail(err))
		return false
	}

	diags.Append(assignJamfParentSettingsResourceModel(ctx, plan, got)...)
	return !diags.HasError()
}
