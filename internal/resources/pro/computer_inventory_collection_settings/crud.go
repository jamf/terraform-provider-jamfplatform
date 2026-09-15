// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetComputerInventoryCollectionSettingsV2
//   pro.UpdateComputerInventoryCollectionSettingsV2            (PATCH merge-patch, 204 No Content)
//   pro.CreateComputerInventoryCollectionCustomPathV2
//   pro.DeleteComputerInventoryCollectionCustomPathV2
//
// The V1 functions are deprecated (api-spec deprecation-date 2025-06-30) and are not used.
//
// Status: current. Last reviewed 2026-06-03.

package computer_inventory_collection_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// applicationPathScope is the only scope value the V2 custom-path create endpoint
// accepts. Re-probed on Jamf Pro 11.31.1 (2026-09-01): FONT, PLUGIN, an unknown
// BOGUS and the lower-case "app" are each refused with 400 INVALID_FIELD on field
// "scope", the description naming the accepted set as "[APP]". So the vocabulary is
// exactly one value, matched case-sensitively, and this resource manages application
// search paths exclusively — the Fonts/Plug-ins custom paths in the admin UI are
// reachable only through the deprecated V1 endpoint, which still answers but is not
// adopted here.
//
// The narrowing to one value is a real V2 change and not a publish artifact: the
// same instance served APP, FONT and PLUGIN on V1 minutes either side of the V2
// refusals.
const applicationPathScope = pro.CreatePathV2ScopeApp

// reconcileApplicationPaths brings the tenant's user-created application search paths in
// line with the planned set. It is a no-op when the attribute is unmanaged (null/unknown),
// leaving the tenant's paths untouched. The collection has no update endpoint, so changes
// are expressed as create (path in plan, absent on tenant) and delete-by-id (path on
// tenant, absent from plan). Diffing is by path string — a newly declared path has no id
// until the server mints one on create.
func (r *ComputerInventoryCollectionSettingsResource) reconcileApplicationPaths(ctx context.Context, planned types.Set) error {
	if planned.IsNull() || planned.IsUnknown() {
		return nil
	}

	desired, diags := helpers.SetToStringSlice(ctx, planned)
	if diags.HasError() {
		return diagsToError(diags)
	}

	got, err := r.client.GetComputerInventoryCollectionSettingsV2(ctx)
	if err != nil {
		return err
	}

	// path -> server id for user-created paths (built-in id == "-1" excluded).
	current := map[string]string{}
	if got.ApplicationPaths != nil {
		for _, p := range *got.ApplicationPaths {
			if p.ID == builtInPathID {
				continue
			}
			current[p.Path] = p.ID
		}
	}

	desiredSet := make(map[string]struct{}, len(desired))
	for _, p := range desired {
		desiredSet[p] = struct{}{}
	}

	// Create paths declared but not present.
	for _, p := range desired {
		if _, ok := current[p]; ok {
			continue
		}
		if _, err := r.client.CreateComputerInventoryCollectionCustomPathV2(ctx, &pro.CreatePathV2{Path: p, Scope: applicationPathScope}); err != nil {
			return err
		}
	}

	// Delete user-created paths present but no longer declared.
	for p, id := range current {
		if _, ok := desiredSet[p]; ok {
			continue
		}
		if err := r.client.DeleteComputerInventoryCollectionCustomPathV2(ctx, id); err != nil {
			return err
		}
	}

	return nil
}

// applySettings runs the full write sequence shared by Create and Update: PATCH the
// collection preferences (merge-patch, 204 — no echoed body), reconcile the custom
// application paths, then GET to capture authoritative state.
func (r *ComputerInventoryCollectionSettingsResource) applySettings(ctx context.Context, plan *ComputerInventoryCollectionSettingsResourceModel) (*pro.ComputerInventoryCollectionSettingsV2, error) {
	if err := r.client.UpdateComputerInventoryCollectionSettingsV2(ctx, buildComputerInventoryCollectionSettingsInput(*plan)); err != nil {
		return nil, err
	}
	if err := r.reconcileApplicationPaths(ctx, plan.ApplicationSearchPaths); err != nil {
		return nil, err
	}
	return r.client.GetComputerInventoryCollectionSettingsV2(ctx)
}

// Create handles initial provisioning of the settings singleton. The Jamf Pro API has no
// Create endpoint for this object, so this funnels into the shared apply sequence.
func (r *ComputerInventoryCollectionSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ComputerInventoryCollectionSettingsResourceModel
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

	got, err := r.applySettings(createCtx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error setting Jamf Pro computer inventory collection settings", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignComputerInventoryCollectionSettingsResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, computerInventoryCollectionSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Pro computer inventory collection settings")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest settings from the Jamf Pro API.
func (r *ComputerInventoryCollectionSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state ComputerInventoryCollectionSettingsResourceModel
	isImport := helpers.IsSingletonImport(ctx, req, resp)

	if isImport {
		state.ID = types.StringValue(helpers.SingletonID)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(computerInventoryCollectionSettingsTimeoutAttributeTypes)
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

	got, err := r.client.GetComputerInventoryCollectionSettingsV2(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro computer inventory collection settings", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignComputerInventoryCollectionSettingsResourceModel(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, computerInventoryCollectionSettingsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the new settings to the Jamf Pro API. Same apply sequence as Create.
func (r *ComputerInventoryCollectionSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan ComputerInventoryCollectionSettingsResourceModel
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

	got, err := r.applySettings(updateCtx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro computer inventory collection settings", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignComputerInventoryCollectionSettingsResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = types.StringValue(helpers.SingletonID)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, computerInventoryCollectionSettingsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op on the Jamf Pro API. Singleton settings cannot be deleted — the
// record persists on the tenant, and any custom application paths are left intact
// (they are tenant settings, not Terraform-owned objects). Terraform removes the
// resource from state on its own after this handler returns.
func (r *ComputerInventoryCollectionSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Trace(ctx, "removing Jamf Pro computer inventory collection settings from Terraform state (singleton — no remote delete)")
}
