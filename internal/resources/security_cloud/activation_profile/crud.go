// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   securitycloud.CreateActivationProfileV1
//   securitycloud.GetActivationProfileV1
//   securitycloud.PauseActivationProfileV1
//   securitycloud.ResumeActivationProfileV1
//   securitycloud.DeleteActivationProfilesV1
//
// Status: current. Last reviewed 2026-09-01.

package activation_profile

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create mints a new activation profile and, when the plan asks for a paused
// profile, pauses it.
//
// The create response carries the new activation code and nothing else, so there
// is no read-back: a read would return exactly what the create already gave us.
//
// State is committed before the pause is attempted. The profile exists on the
// tenant the moment the create succeeds, and returning without state would orphan
// it — permanently, because a soft-deleted profile cannot be cleaned up and stays
// in the tenant's list forever.
//
// It is committed with `paused` false, because that is the state the server is
// actually in at that moment: the create endpoint mints a running profile, and
// only a successful pause promotes state to true. So a refused pause — a
// credential holding activation-profiles:create but not :update, say — leaves
// state matching reality, and the next plan sees a real diff and retries the
// pause through Update. Committing the desired value up front would instead leave
// Terraform asserting a closed profile that is quietly accepting enrollments,
// with no plan that ever corrects it, because Read cannot refresh `paused`.
func (r *ActivationProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ActivationProfileResourceModel
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

	request, buildDiags := buildCreateRequest(createCtx, &plan)
	resp.Diagnostics.Append(buildDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateActivationProfileV1(createCtx, request)
	if err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Unable to create activation profile", helpers.APIErrorDetail(err))
		}
		return
	}

	if created.Code == "" {
		resp.Diagnostics.AddError(
			"Jamf Security Cloud did not return an activation code",
			"The activation profile was created, but Jamf Security Cloud returned no activation code for it, so "+
				"Terraform cannot manage it. Find the new profile in Jamf Security Cloud and delete it, then try "+
				"again.",
		)
		return
	}

	applyReadState(&plan, created)
	wantPaused := plan.Paused.ValueBool()
	plan.Paused = types.BoolValue(false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !wantPaused {
		return
	}

	r.assertPaused(createCtx, &resp.Diagnostics, created.Code, true)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Paused = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read confirms the activation profile still exists.
//
// It cannot do more than that. Jamf Security Cloud returns only the activation
// code, so no configured attribute is refreshed and drift on one is undetectable.
// Worse, a deleted profile reads back as a healthy one — deletion is a soft delete
// the read surface does not reflect — so a 404 here means the code never existed
// rather than that it was removed.
//
// Re-asserting the profile's pause state would distinguish live from deleted (204
// against 409), but Read runs during refresh and that would make `terraform plan`
// issue writes, silently resuming a profile someone had paused deliberately.
func (r *ActivationProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ActivationProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	profile, err := r.client.GetActivationProfileV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Unable to read activation profile", helpers.APIErrorDetail(err))
		}
		return
	}

	applyReadState(&state, profile)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies the only change that does not require replacement: pausing or
// resuming the profile.
//
// Every other attribute is RequiresReplace, so by the time Update runs the only
// possible difference is `paused`. There is no update endpoint on this surface at
// all.
func (r *ActivationProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ActivationProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
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

	plan.ID = state.ID
	if plan.Paused.ValueBool() != state.Paused.ValueBool() {
		r.assertPaused(updateCtx, &resp.Diagnostics, plan.ID.ValueString(), plan.Paused.ValueBool())
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete asks Jamf Security Cloud to delete the activation profile.
//
// Destroy-time verification is impossible, which is why nothing here polls: the
// delete is a soft delete — afterwards the item GET still answers 200 and the
// collection still returns the code — and the bulk endpoint answers 204 for a
// code that does not exist, for one already deleted, and for one it actually
// deleted, with no body and no per-code result. A success is therefore not
// evidence anything was deleted, and no amount of re-reading would make it one.
//
// That justifies not polling. It does not justify treating a refusal as success:
// wire probing found no status on this endpoint that is misleadingly negative,
// unlike the classic endpoints STYLE_GUIDE §Delete semantics describes. A refusal
// here — a missing activation-profiles:delete privilege, or a tenant that has lost
// entitlement — means nothing was deleted, so it surfaces as an error and leaves
// the profile in state. Dropping it instead would leave a live activation code
// accepting enrollments with no Terraform record of it.
//
// The profile stays in the tenant's list permanently even on success. Nothing here
// can change that; the resource documentation says so instead.
func (r *ActivationProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ActivationProfileResourceModel
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

	err := r.client.DeleteActivationProfilesV1(deleteCtx, &securitycloud.BulkDeleteActivationProfilesRequest{
		Codes: []string{state.ID.ValueString()},
	})
	if err == nil {
		return
	}
	if helpers.IsNotFoundError(err) {
		return
	}
	if appendWriteDiagnostics(&resp.Diagnostics, err) {
		return
	}
	resp.Diagnostics.AddError("Unable to delete activation profile", helpers.APIErrorDetail(err))
}

// assertPaused pauses or resumes the profile to match the desired state.
//
// Both operations are idempotent — asserting a state the profile is already in
// answers 204 and changes nothing — so this is safe to call whenever the desired
// state is known, without first reading the current one. Which is just as well:
// no endpoint reports it.
func (r *ActivationProfileResource) assertPaused(ctx context.Context, diags *diag.Diagnostics, code string, paused bool) {
	var err error
	if paused {
		err = r.client.PauseActivationProfileV1(ctx, code)
	} else {
		err = r.client.ResumeActivationProfileV1(ctx, code)
	}
	if err == nil {
		return
	}
	if appendWriteDiagnostics(diags, err) {
		return
	}
	action := "resume"
	if paused {
		action = "pause"
	}
	diags.AddError("Unable to "+action+" activation profile", helpers.APIErrorDetail(err))
}
