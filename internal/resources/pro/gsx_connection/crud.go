// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetGSXConnectionV1
//   pro.UpdateGSXConnectionV1
//
// Status: current.
//
// Notes:
//   - PatchGSXConnectionV1 is intentionally not used. The GSX PUT mandates token +
//     keystore on every write (wire-probed 2026-06-09 — FIELD_REQUIRED), so the full
//     replace via UpdateGSXConnectionV1 is the safe path; it does not depend on the
//     un-probeable PATCH-preserve-on-omit behaviour (no valid certificate could be stored
//     against the live tenant to probe it).
//   - TestGSXConnectionV1 and the GSX history endpoints are out of scope (connectivity
//     test verb / audit history — not state). Use jamf-cli to test connectivity.

package gsx_connection

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions the GSX Connection settings singleton. The Jamf Pro API has no Create
// endpoint for this object — one record per tenant always exists — so this funnels into a
// full-replace PUT, then reads back authoritative state. The live settings read first are
// passed as the merge base so any Optional field the user omitted keeps its current value
// rather than being reset by the full-replace write.
func (r *GsxConnectionSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, cfg GsxConnectionSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
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

	current, err := r.client.GetGSXConnectionV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro GSX Connection settings", helpers.APIErrorDetail(err))
		return
	}

	input, err := buildGsxConnectionInput(plan, cfg, current)
	if err != nil {
		resp.Diagnostics.AddError("Invalid GSX Connection configuration", helpers.APIErrorDetail(err))
		return
	}

	if _, err := r.client.UpdateGSXConnectionV1(createCtx, input); err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro GSX Connection settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetGSXConnectionV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro GSX Connection settings after write", helpers.APIErrorDetail(err))
		return
	}
	assignGsxConnectionSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, gsxConnectionSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro GSX Connection settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API. The
// WriteOnly secrets are not present in the response and are not carried in state.
func (r *GsxConnectionSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state GsxConnectionSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(gsxConnectionSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetGSXConnectionV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro GSX Connection settings", helpers.APIErrorDetail(err))
		return
	}

	assignGsxConnectionSettingsResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, gsxConnectionSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings via the full-replace PUT, re-sending the three secrets
// from config (the GSX API mandates them on every write — Design B). `current` is nil:
// UseStateForUnknown has already carried the Optional non-secret fields into the plan.
func (r *GsxConnectionSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, cfg GsxConnectionSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
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

	input, err := buildGsxConnectionInput(plan, cfg, nil)
	if err != nil {
		resp.Diagnostics.AddError("Invalid GSX Connection configuration", helpers.APIErrorDetail(err))
		return
	}

	if _, err := r.client.UpdateGSXConnectionV1(updateCtx, input); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro GSX Connection settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetGSXConnectionV1(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro GSX Connection settings after update", helpers.APIErrorDetail(err))
		return
	}
	assignGsxConnectionSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, gsxConnectionSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. Singleton objects cannot be deleted — the record
// persists on the tenant. Terraform removes the resource from state on its own after this
// handler returns.
func (r *GsxConnectionSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro GSX Connection settings from Terraform state (singleton — no remote delete)")
}
