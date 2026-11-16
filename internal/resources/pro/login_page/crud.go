// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetLoginCustomizationV1
//   pro.UpdateLoginCustomizationV1
//
// Status: current.

package login_page

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create handles initial provisioning of the login page settings singleton. The Jamf Pro API
// has no Create endpoint for this object — one record per tenant already exists — so this
// funnels into Update against the plan, then reads back to capture authoritative state.
//
// The PUT returns 200 OK with an echo body, but the echo is the LoginContentPut type (the
// four editable fields only); the resource re-GETs for full authoritative state via a single
// assigner shared with Read. The live GET read before the write is passed as the merge base
// so a field the user did not declare adopts its current value rather than being rejected by
// the all-fields-required write (wire-probed 2026-06-09).
func (r *LoginPageSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan LoginPageSettingsResourceModel
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

	// The login page settings singleton always exists on the tenant, so "create" is really
	// adoption. Read the live settings and pass them as the merge base so a field the user
	// did not declare keeps its current value. The server requires all four fields on every
	// write, so the live read always supplies a valid non-empty value for omitted fields.
	current, err := r.client.GetLoginCustomizationV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro login page settings", helpers.APIErrorDetail(err))
		return
	}

	if _, err := r.client.UpdateLoginCustomizationV1(createCtx, buildLoginPageSettingsInput(plan, current)); err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro login page settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetLoginCustomizationV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro login page settings after write", helpers.APIErrorDetail(err))
		return
	}
	assignLoginPageSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, loginPageSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro login page settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API.
func (r *LoginPageSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state LoginPageSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(loginPageSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetLoginCustomizationV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro login page settings", helpers.APIErrorDetail(err))
		return
	}

	assignLoginPageSettingsResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, loginPageSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings to the Jamf Pro API. Same SDK call as Create. The merge base
// is nil — UseStateForUnknown has already carried omitted fields into the plan as known prior
// values. The PUT returns a partial echo (LoginContentPut), so authoritative state comes from
// a follow-up GET.
func (r *LoginPageSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan LoginPageSettingsResourceModel
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

	if _, err := r.client.UpdateLoginCustomizationV1(updateCtx, buildLoginPageSettingsInput(plan, nil)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro login page settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetLoginCustomizationV1(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro login page settings after update", helpers.APIErrorDetail(err))
		return
	}
	assignLoginPageSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, loginPageSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. Singleton objects cannot be deleted — the record
// persists on the tenant. Terraform removes the resource from state on its own after this
// handler returns. No SDK call is made and no diagnostics are emitted.
func (r *LoginPageSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro login page settings from Terraform state (singleton — no remote delete)")
}
