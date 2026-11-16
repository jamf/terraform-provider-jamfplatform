// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetImpactAlertNotificationSettingsV1
//   pro.UpdateImpactAlertNotificationSettingsV1
//
// Status: current.

package impact_alert_notification_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create handles initial provisioning of the Impact Alert Notification settings singleton.
// The Jamf Pro API has no Create endpoint for this object — one record per tenant already
// exists — so this funnels into Update against the plan, then reads back to capture
// authoritative state (the PUT returns 204 No Content with no echo body).
func (r *ImpactAlertNotificationSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ImpactAlertNotificationSettingsResourceModel
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

	// The Impact Alert Notification settings singleton always exists on the tenant, so
	// "create" is really adoption. Read the live settings and pass them as the merge base
	// so a toggle the user did not declare keeps its current value rather than being reset
	// to false by the full-replace write (wire-probed full-replace 2026-06-09). On update
	// the merge base is nil — UseStateForUnknown has already carried omitted toggles into
	// the plan as known prior values.
	current, err := r.client.GetImpactAlertNotificationSettingsV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro Impact Alert Notification settings", helpers.APIErrorDetail(err))
		return
	}

	if err := r.client.UpdateImpactAlertNotificationSettingsV1(createCtx, buildImpactAlertNotificationSettingsInput(plan, current)); err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro Impact Alert Notification settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetImpactAlertNotificationSettingsV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Impact Alert Notification settings after write", helpers.APIErrorDetail(err))
		return
	}
	assignImpactAlertNotificationSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, impactAlertNotificationSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro Impact Alert Notification settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API.
func (r *ImpactAlertNotificationSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state ImpactAlertNotificationSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(impactAlertNotificationSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetImpactAlertNotificationSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Impact Alert Notification settings", helpers.APIErrorDetail(err))
		return
	}

	assignImpactAlertNotificationSettingsResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, impactAlertNotificationSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings to the Jamf Pro API. Same SDK call as Create. The PUT
// returns 204 No Content, so authoritative state comes from a follow-up GET.
func (r *ImpactAlertNotificationSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ImpactAlertNotificationSettingsResourceModel
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

	if err := r.client.UpdateImpactAlertNotificationSettingsV1(updateCtx, buildImpactAlertNotificationSettingsInput(plan, nil)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro Impact Alert Notification settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetImpactAlertNotificationSettingsV1(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Impact Alert Notification settings after update", helpers.APIErrorDetail(err))
		return
	}
	assignImpactAlertNotificationSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, impactAlertNotificationSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. Singleton objects cannot be deleted — the
// record persists on the tenant. Terraform removes the resource from state on its own
// after this handler returns. No SDK call is made and no diagnostics are emitted.
//
// Canonical singleton template: every singleton Delete should look exactly like this —
// a single tflog.Trace explaining the no-op, with `_` markers on the unused
// request/response so future maintainers immediately see the omission is intentional.
func (r *ImpactAlertNotificationSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro Impact Alert Notification settings from Terraform state (singleton — no remote delete)")
}
