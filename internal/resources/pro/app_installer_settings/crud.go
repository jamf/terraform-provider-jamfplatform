// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetAppInstallerGlobalSettingsV1
//   pro.UpdateAppInstallerGlobalSettingsV1
//
// Status: current.
//
// The end-user-experience block is a required member of the request body, so it
// has no omitted form: the "omit block = leave the server untouched" contract is
// kept by passing the current server block through, not by dropping the key.
// deploymentProcessControls is a pointer and does drop out when omitted.

package app_installer_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create adopts the App Installer global settings singleton. The API has no separate
// Create — PUT updates the existing record. GET-merge preserves any block the user
// did not declare so that adoption does not clobber out-of-band configuration.
func (r *AppInstallerSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan AppInstallerSettingsResourceModel
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

	current, err := r.client.GetAppInstallerGlobalSettingsV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro App Installer global settings before write", helpers.APIErrorDetail(err))
		return
	}

	if _, err := r.client.UpdateAppInstallerGlobalSettingsV1(createCtx, buildMergedInput(current, plan)); err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro App Installer global settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetAppInstallerGlobalSettingsV1(createCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro App Installer global settings after write", helpers.APIErrorDetail(err))
		return
	}
	assignAppInstallerSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appInstallerSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro App Installer global settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API.
func (r *AppInstallerSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state AppInstallerSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(appInstallerSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetAppInstallerGlobalSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro App Installer global settings", helpers.APIErrorDetail(err))
		return
	}

	assignAppInstallerSettingsResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appInstallerSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings to the Jamf Pro API. GET-merge preserves any block
// the user did not declare in their config.
func (r *AppInstallerSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan AppInstallerSettingsResourceModel
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

	current, err := r.client.GetAppInstallerGlobalSettingsV1(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro App Installer global settings before update", helpers.APIErrorDetail(err))
		return
	}

	if _, err := r.client.UpdateAppInstallerGlobalSettingsV1(updateCtx, buildMergedInput(current, plan)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro App Installer global settings", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetAppInstallerGlobalSettingsV1(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro App Installer global settings after update", helpers.APIErrorDetail(err))
		return
	}
	assignAppInstallerSettingsResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appInstallerSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op. Singleton settings cannot be deleted — the record persists on
// the tenant. Terraform removes the resource from state on its own after this returns.
func (r *AppInstallerSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro App Installer global settings from Terraform state (singleton — no remote delete)")
}
