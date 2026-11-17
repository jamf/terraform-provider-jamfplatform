// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetSelfServiceSettingsV1
//   pro.UpdateSelfServiceSettingsV1
//
// Not adopted: pro.ListSelfServiceSettingsHistoryV1, pro.CreateSelfServiceSettingsHistoryNoteV1
// (object history — convention-wide exclusion).
//
// Status: current.

package self_service_macos_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create handles initial provisioning of the Self Service macOS settings singleton. The Jamf
// Pro API has no Create endpoint for this object — one record per tenant already exists — so
// this funnels into Update against the plan, then reads back to capture authoritative state.
//
// The PUT is full-replace and the wire requires all three nested setting groups on every
// write (omitting one returns HTTP 500 — wire-probed 2026-06-10), so the live GET read before
// the write is passed as the merge base: a field the user did not declare adopts its current
// value rather than resetting to the server default. The PUT echo matched a follow-up GET in
// every probe, but the post-write GET stays per the singleton convention (future-proofing).
func (r *SelfServiceMacosSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan SelfServiceMacosSettingsResourceModel
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

	// The Self Service settings singleton always exists on the tenant, so "create" is really
	// adoption. Read the live settings and pass them as the merge base so a field the user
	// did not declare keeps its current value on the full-replace PUT.
	current, err := r.client.GetSelfServiceSettingsV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro Self Service macOS settings", helpers.APIErrorDetail(err))
		return
	}

	if _, err := r.client.UpdateSelfServiceSettingsV1(createCtx, buildSelfServiceMacosSettingsInput(plan, current)); err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro Self Service macOS settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetSelfServiceSettingsV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Self Service macOS settings after write", helpers.APIErrorDetail(err))
		return
	}
	assignSelfServiceMacosSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServiceMacosSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro Self Service macOS settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API.
func (r *SelfServiceMacosSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state SelfServiceMacosSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(selfServiceMacosSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetSelfServiceSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Self Service macOS settings", helpers.APIErrorDetail(err))
		return
	}

	assignSelfServiceMacosSettingsResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServiceMacosSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings to the Jamf Pro API. Same SDK call as Create, including the
// live GET merge base: UseStateForUnknown carries most omitted fields into the plan as known
// prior values, but ModifyPlan re-marks authentication_type Unknown on a login_method →
// NotRequired transition, and an Unknown must round-trip the live server value (re-sending a
// stored "Saml" is accepted; only a Basic→Saml *change* trips the server's prerequisite
// check). Authoritative state comes from a follow-up GET per the singleton convention.
func (r *SelfServiceMacosSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan SelfServiceMacosSettingsResourceModel
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

	current, err := r.client.GetSelfServiceSettingsV1(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro Self Service macOS settings", helpers.APIErrorDetail(err))
		return
	}

	if _, err := r.client.UpdateSelfServiceSettingsV1(updateCtx, buildSelfServiceMacosSettingsInput(plan, current)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro Self Service macOS settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetSelfServiceSettingsV1(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Self Service macOS settings after update", helpers.APIErrorDetail(err))
		return
	}
	assignSelfServiceMacosSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServiceMacosSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. Singleton objects cannot be deleted — the record
// persists on the tenant. Terraform removes the resource from state on its own after this
// handler returns. No SDK call is made and no diagnostics are emitted.
func (r *SelfServiceMacosSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro Self Service macOS settings from Terraform state (singleton — no remote delete)")
}
