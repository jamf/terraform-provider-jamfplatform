// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.ListMacOSBrandingConfigurationsV1
//   pro.CreateMacOSBrandingConfigurationV1
//   pro.GetMacOSBrandingConfigurationV1
//   pro.UpdateMacOSBrandingConfigurationV1
//   pro.DeleteMacOSBrandingConfigurationV1
//
// Not adopted: pro.ResolveMacOSBrandingConfigurationV1{,ID}ByName,
// pro.ApplyMacOSBrandingConfigurationV1 (client-side convenience helpers — the
// singleton is discovered via List, not resolved by name).
//
// Status: current. Last reviewed 2026-06-10.

package self_service_branding_macos

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// findExisting returns the single macOS branding configuration on the tenant,
// or nil if none exists. The endpoint caps the tenant at one configuration, so
// the List result has zero or one element.
func (r *SelfServiceBrandingMacosResource) findExisting(ctx context.Context) (*pro.MacOsBrandingConfiguration, error) {
	configs, err := r.client.ListMacOSBrandingConfigurationsV1(ctx, nil)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, nil
	}
	return &configs[0], nil
}

// isAlreadyExistsError reports whether a Create error is the server's
// "Cannot create another MacOs branding configuration, one already exists"
// (HTTP 409, CREATE_FAILED) rejection, signalling the singleton already exists
// and should be imported.
func isAlreadyExistsError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}

// Create POSTs a new macOS branding configuration. The endpoint rejects a POST
// when one already exists (409); that case is mapped to import guidance.
func (r *SelfServiceBrandingMacosResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan SelfServiceBrandingMacosResourceModel
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

	href, err := r.client.CreateMacOSBrandingConfigurationV1(createCtx, buildSelfServiceBrandingMacosInput(plan))
	if err != nil {
		if isAlreadyExistsError(err) {
			resp.Diagnostics.AddError(
				"Self Service macOS branding already configured",
				"A Self Service macOS branding configuration already exists on this Jamf Pro tenant, so it cannot be created. "+
					"Import the existing object instead:\n\n"+
					"  terraform import jamfplatform_pro_self_service_branding_macos.<name> singleton\n\n"+
					"Original error: "+helpers.APIErrorDetail(err),
			)
			return
		}
		resp.Diagnostics.AddError("Error creating Jamf Pro Self Service macOS branding", helpers.APIErrorDetail(err))
		return
	}

	// POST returns only an href + id; GET-after for authoritative state.
	got, err := r.client.GetMacOSBrandingConfigurationV1(createCtx, href.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Self Service macOS branding after create", helpers.APIErrorDetail(err))
		return
	}

	assignSelfServiceBrandingMacosResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServiceBrandingMacosIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro Self Service macOS branding")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes state by Listing the singleton. An empty list means the
// configuration was removed (out-of-band or via destroy) — the resource is
// removed from state on a normal refresh, or reported as a failed import.
func (r *SelfServiceBrandingMacosResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state SelfServiceBrandingMacosResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)
	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(selfServiceBrandingMacosTimeoutAttributeTypes)
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

	got, err := r.findExisting(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Self Service macOS branding", helpers.APIErrorDetail(err))
		return
	}
	if got == nil {
		if isImport {
			resp.Diagnostics.AddError(
				"No Self Service macOS branding to import",
				"No Self Service macOS branding configuration exists on this Jamf Pro tenant, so there is nothing to import.",
			)
			return
		}
		tflog.Info(ctx, "Self Service macOS branding not found, removing from state")
		resp.State.RemoveResource(ctx)
		return
	}

	assignSelfServiceBrandingMacosResourceModel(&state, got)
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServiceBrandingMacosIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies a full-replace PUT to the existing configuration: a field
// omitted from the plan (removed from configuration) is cleared on the tenant.
func (r *SelfServiceBrandingMacosResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan SelfServiceBrandingMacosResourceModel
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

	existing, err := r.findExisting(updateCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Self Service macOS branding before update", helpers.APIErrorDetail(err))
		return
	}
	if existing == nil || existing.ID == nil {
		resp.Diagnostics.AddError(
			"Self Service macOS branding no longer exists",
			"The Self Service macOS branding configuration could not be found for update — it may have been removed out-of-band. Re-run apply to recreate it.",
		)
		return
	}

	got, err := r.client.UpdateMacOSBrandingConfigurationV1(updateCtx, *existing.ID, buildSelfServiceBrandingMacosInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro Self Service macOS branding", helpers.APIErrorDetail(err))
		return
	}

	assignSelfServiceBrandingMacosResourceModel(&plan, got)
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, selfServiceBrandingMacosIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated Jamf Pro Self Service macOS branding")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the macOS branding configuration from the tenant (real
// DELETE). An already-absent configuration is the delete's objective.
func (r *SelfServiceBrandingMacosResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state SelfServiceBrandingMacosResourceModel
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

	existing, err := r.findExisting(deleteCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro Self Service macOS branding before delete", helpers.APIErrorDetail(err))
		return
	}
	if existing == nil || existing.ID == nil {
		tflog.Info(ctx, "Jamf Pro Self Service macOS branding already removed")
		return
	}

	if err := r.client.DeleteMacOSBrandingConfigurationV1(deleteCtx, *existing.ID); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro Self Service macOS branding already removed")
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro Self Service macOS branding", helpers.APIErrorDetail(err))
		return
	}
	tflog.Trace(ctx, "deleted Jamf Pro Self Service macOS branding")
}
