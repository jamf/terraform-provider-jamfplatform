// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetAppRequestSettingsV1
//   pro.UpdateAppRequestSettingsV1   (PUT — full-replace, returns the echo)
//
// GET/PUT-only singleton: there is no Create or Delete endpoint (one record per tenant).
// Create adopts the existing settings — it reads the live settings as a merge base so any
// Optional+Computed field the user omits keeps its current value, then PUTs and seeds state
// from the returned echo. Delete is a no-op (state-only removal). The PUT is a full replace
// (wire-probed 2026-06-13: omitting appStoreLocale → HTTP 500; omitting approverEmails →
// cleared), so buildAppRequestSettingsInput always emits every field.
//
// Enabling App Requests has a tenant prerequisite the settings payload does not carry: at
// least one App Request form field must already exist (wire-probed 2026-09-04 — a tenant
// with none refuses an enabling PUT with "[INVALID_SIZE] formInputFields: At least one form
// field is required", and the identical write succeeds once one field exists). The fields
// are a separate collection (jamfplatform_pro_app_request_form_field), so nothing in the
// plan can prove their presence and the check cannot move to plan time: a configuration
// creating the field and enabling the settings in one apply has no field on the tenant while
// the plan is built. Hence the refusal is translated at apply time by
// appRequestWriteErrorDiagnostic. Jamf Pro enforces the rule only on the settings write:
// deleting the last remaining form field while App Requests are enabled is accepted, leaving
// the tenant enabled with no fields (same probe).
//
// Status: current.

package app_request_settings

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create adopts and writes the App Request settings singleton. There is no Create endpoint,
// so this reads the live settings as a merge base, PUTs the plan, and seeds state from the
// returned echo.
func (r *AppRequestSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan AppRequestSettingsResourceModel
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

	current, err := r.client.GetAppRequestSettingsV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading existing Jamf Pro App Request settings", helpers.APIErrorDetail(err))
		return
	}

	input, diags := buildAppRequestSettingsInput(ctx, plan, current)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateAppRequestSettingsV1(createCtx, input)
	if err != nil {
		resp.Diagnostics.AddError(appRequestWriteErrorDiagnostic("Error setting Jamf Pro App Request settings", err))
		return
	}
	resp.Diagnostics.Append(assignAppRequestSettingsResourceModel(ctx, &plan, updated)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appRequestSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro App Request settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API.
func (r *AppRequestSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state AppRequestSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(appRequestSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetAppRequestSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro App Request settings", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignAppRequestSettingsResourceModel(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appRequestSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings to the Jamf Pro API. The merge base is nil — UseStateForUnknown
// has already carried omitted Optional+Computed fields into the plan as known prior values.
func (r *AppRequestSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan AppRequestSettingsResourceModel
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

	input, diags := buildAppRequestSettingsInput(ctx, plan, nil)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateAppRequestSettingsV1(updateCtx, input)
	if err != nil {
		resp.Diagnostics.AddError(appRequestWriteErrorDiagnostic("Error updating Jamf Pro App Request settings", err))
		return
	}
	resp.Diagnostics.Append(assignAppRequestSettingsResourceModel(ctx, &plan, updated)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appRequestSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. The settings singleton cannot be deleted — the
// record persists on the tenant. Terraform removes the resource from state on its own after
// this handler returns.
func (r *AppRequestSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro App Request settings from Terraform state (singleton — no remote delete)")
}

// appRequestWriteErrorDiagnostic names the tenant prerequisite behind Jamf Pro's refusal of
// an enabling write, and otherwise passes the error through under the caller's summary.
//
// Jamf Pro reports the missing form fields as a validation failure on a field the settings
// payload does not even have, so the raw message sends an operator looking for an attribute
// of this resource that does not exist. The match is on the field name plus the shortfall
// the message states, not on the INVALID_SIZE code the same endpoint uses for other size
// violations, and not on the field name alone: a violation of some other bound on
// formInputFields would otherwise be answered with "add a form field", which is the
// opposite of the fix.
func appRequestWriteErrorDiagnostic(summary string, err error) (string, string) {
	msg := err.Error()
	if strings.Contains(msg, "formInputFields") && strings.Contains(strings.ToLower(msg), "at least one") {
		return "App Requests cannot be enabled without a form field",
			"Jamf Pro requires at least one App Request form field on the tenant before App Requests can be enabled. " +
				"Declare a jamfplatform_pro_app_request_form_field resource and add a depends_on, so the field " +
				"exists before these settings are written.\n\nJamf Pro reported: " + msg
	}
	return summary, msg
}
