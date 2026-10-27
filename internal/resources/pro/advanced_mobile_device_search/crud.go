// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateAdvancedMobileDeviceSearchV1            (POST — returns Href; ID parsed, then GET)
//   pro.GetAdvancedMobileDeviceSearchV1
//   pro.UpdateAdvancedMobileDeviceSearchV1            (PUT — full replace; GET after, response body unreliable)
//   pro.DeleteAdvancedMobileDeviceSearchV1
//   pro.ListAdvancedMobileDeviceSearchesV1            (list resource / collision check in collision.go)
//   pro.ResolveAdvancedMobileDeviceSearchV1ByName     (data source name lookup)
//
// Uniqueness: name, enforced (400 DUPLICATE_FIELD), case-insensitive only; leading space, trailing space and accents are distinct.
//
// Status: current. Last reviewed 2026-06-02.

package advanced_mobile_device_search

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// dsGroupObjectType is the object class this resource targets, used to dispatch
// the per-class directory-service group criterion allowlist.
const dsGroupObjectType = criteria.ObjectTypeMobile

// ModifyPlan first reports a name another search already holds, then suppresses a
// no-op diff when a directory-service group criterion's
// planned value is a different REPRESENTATION of the same group already in state
// (a raw base64 value swapped for the equivalent group name, or vice versa). It
// resets such a criterion's planned value to the prior state value so
// `terraform plan` shows nothing to change. Skips create (no prior state) and
// destroy (no plan); soft — any resolve failure leaves the diff intact.
func (r *AdvancedMobileDeviceSearchResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() || r.client == nil {
		return
	}
	var plan, state AdvancedMobileDeviceSearchResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plannedModels, planDiags := criteria.CriteriaModelsFromList(ctx, plan.Criteria)
	resp.Diagnostics.Append(planDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(plannedModels) == 0 {
		// Nothing to suppress (also covers a null/unknown planned list, which
		// CriteriaModelsFromList returns as nil) — return before any reconversion
		// so a null/unknown list is never spuriously flipped to an empty list.
		return
	}

	priorModels, priorDiags := criteria.CriteriaModelsFromList(ctx, state.Criteria)
	resp.Diagnostics.Append(priorDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	suppressed := criteria.SuppressEquivalentDSGroupValues(ctx, r.client, plannedModels, priorModels)
	// Additive: only reconvert + Set when suppression actually collapsed a
	// representation. Otherwise leave the planned types.List exactly as the
	// framework computed it — a non-DS plan must round-trip untouched.
	changed := false
	for i := range suppressed {
		if !suppressed[i].Value.Equal(plannedModels[i].Value) {
			changed = true
			break
		}
	}
	if !changed {
		return
	}

	suppressedList, suppressedDiags := criteria.CriteriaListValue(ctx, suppressed)
	resp.Diagnostics.Append(suppressedDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Criteria = suppressedList

	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// Create creates a new advanced mobile device search. The Pro POST returns only
// an href + id (not the created object), so the ID is parsed from the response
// and the full representation read back via GET.
func (r *AdvancedMobileDeviceSearchResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan AdvancedMobileDeviceSearchResourceModel
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

	planModels, planModelDiags := criteria.CriteriaModelsFromList(createCtx, plan.Criteria)
	resp.Diagnostics.Append(planModelDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resolved, authored, dsDiags := criteria.ResolveDSGroupCriteria(createCtx, r.client, dsGroupObjectType, planModels)
	resp.Diagnostics.Append(dsDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resolvedList, resolvedDiags := criteria.CriteriaListValue(createCtx, resolved)
	resp.Diagnostics.Append(resolvedDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Criteria = resolvedList

	input, inputDiags := buildAdvancedMobileDeviceSearchInput(createCtx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateAdvancedMobileDeviceSearchV1(createCtx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro advanced mobile device search", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	if created == nil || created.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing advanced mobile device search ID",
			"Jamf Pro returned 201 Created with no search ID; cannot persist state.",
		)
		return
	}
	plan.ID = types.StringValue(created.ID)

	got, err := r.client.GetAdvancedMobileDeviceSearchV1(createCtx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro advanced mobile device search", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignAdvancedMobileDeviceSearchResourceModel(createCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(authored) > 0 {
		flattened, flattenDiags := criteria.CriteriaModelsFromList(createCtx, plan.Criteria)
		resp.Diagnostics.Append(flattenDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		restoredList, restoredDiags := criteria.CriteriaListValue(createCtx, criteria.RestoreAuthoredDSGroupCriteria(flattened, authored))
		resp.Diagnostics.Append(restoredDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.Criteria = restoredList
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, advancedMobileDeviceSearchIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro advanced mobile device search", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest search representation.
func (r *AdvancedMobileDeviceSearchResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state AdvancedMobileDeviceSearchResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this advanced mobile device search without existing state or identity data, so the provider cannot determine which search to read.",
			)
			return
		}
		var identity advancedMobileDeviceSearchIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing advanced mobile device search ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the search.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(advancedMobileDeviceSearchTimeoutAttributeTypes)
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

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro advanced mobile device search without ID.")
		return
	}

	got, err := r.client.GetAdvancedMobileDeviceSearchV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro advanced mobile device search not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, advancedMobileDeviceSearchIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro advanced mobile device search", helpers.APIErrorDetail(err))
		return
	}

	priorCriteria := state.Criteria
	resp.Diagnostics.Append(assignAdvancedMobileDeviceSearchResourceModel(readCtx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	priorModels, priorDiags := criteria.CriteriaModelsFromList(readCtx, priorCriteria)
	resp.Diagnostics.Append(priorDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	wireModels, wireDiags := criteria.CriteriaModelsFromList(readCtx, state.Criteria)
	resp.Diagnostics.Append(wireDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readbackList, readbackDiags := criteria.CriteriaListValue(readCtx, criteria.ReadbackDSGroupCriteria(readCtx, r.client, dsGroupObjectType, wireModels, priorModels))
	resp.Diagnostics.Append(readbackDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Criteria = readbackList

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, advancedMobileDeviceSearchIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates an advanced mobile device search. The Pro PUT is a full replace
// and returns the object, but its body echoes the submitted display fields
// (including any the server silently drops), so we GET afterwards to refresh
// state from the canonical representation.
func (r *AdvancedMobileDeviceSearchResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AdvancedMobileDeviceSearchResourceModel
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

	planModels, planModelDiags := criteria.CriteriaModelsFromList(updateCtx, plan.Criteria)
	resp.Diagnostics.Append(planModelDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resolved, authored, dsDiags := criteria.ResolveDSGroupCriteria(updateCtx, r.client, dsGroupObjectType, planModels)
	resp.Diagnostics.Append(dsDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resolvedList, resolvedDiags := criteria.CriteriaListValue(updateCtx, resolved)
	resp.Diagnostics.Append(resolvedDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Criteria = resolvedList

	input, inputDiags := buildAdvancedMobileDeviceSearchInput(updateCtx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.UpdateAdvancedMobileDeviceSearchV1(updateCtx, plan.ID.ValueString(), input); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro advanced mobile device search", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetAdvancedMobileDeviceSearchV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro advanced mobile device search", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignAdvancedMobileDeviceSearchResourceModel(updateCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(authored) > 0 {
		flattened, flattenDiags := criteria.CriteriaModelsFromList(updateCtx, plan.Criteria)
		resp.Diagnostics.Append(flattenDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		restoredList, restoredDiags := criteria.CriteriaListValue(updateCtx, criteria.RestoreAuthoredDSGroupCriteria(flattened, authored))
		resp.Diagnostics.Append(restoredDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.Criteria = restoredList
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, advancedMobileDeviceSearchIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes an advanced mobile device search.
func (r *AdvancedMobileDeviceSearchResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AdvancedMobileDeviceSearchResourceModel
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

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro advanced mobile device search without ID.")
		return
	}

	if err := r.client.DeleteAdvancedMobileDeviceSearchV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro advanced mobile device search already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro advanced mobile device search", helpers.APIErrorDetail(err))
	}
}
