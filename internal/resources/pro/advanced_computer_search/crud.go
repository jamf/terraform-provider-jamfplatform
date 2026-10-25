// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateAdvancedComputerSearchByID    (POST id="0" — server mints the ID)
//   proclassic.GetAdvancedComputerSearchByID
//   proclassic.UpdateAdvancedComputerSearchByID    (PUT — 201 with empty body; GET after)
//   proclassic.DeleteAdvancedComputerSearchByID
//   proclassic.ListAdvancedComputerSearches        (list resource / collision check in collision.go)
//   proclassic.GetAdvancedComputerSearchByName     (data source name lookup)
//
// Uniqueness: name, not enforced; duplicates stored; checked only with include_unenforced.
//
// Status: current. Last reviewed 2026-10-25.

package advanced_computer_search

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// dsGroupObjectType is the object class this resource targets, used to dispatch
// the per-class directory-service group criterion allowlist.
const dsGroupObjectType = criteria.ObjectTypeComputer

// ModifyPlan suppresses a no-op diff when a directory-service group criterion's
// planned value is a different REPRESENTATION of the same group already in state
// (a raw base64 value swapped for the equivalent group name, or vice versa). It
// resets such a criterion's planned value to the prior state value so
// `terraform plan` shows nothing to change. Skips create (no prior state) and
// destroy (no plan); soft — any resolve failure leaves the diff intact.
func (r *AdvancedComputerSearchResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() || r.ldap == nil {
		return
	}
	var plan, state AdvancedComputerSearchResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || len(plan.Criteria) == 0 {
		return
	}
	suppressed := criteria.SuppressEquivalentDSGroupValues(ctx, r.ldap, plan.Criteria, state.Criteria)
	if !dsGroupCriteriaChanged(suppressed, plan.Criteria) {
		return // additive: only touch the plan when a representation actually collapsed
	}
	plan.Criteria = suppressed
	// site_name is Computed without UseStateForUnknown, so core flipped it to
	// unknown for the (now-reverted) update. When suppression reconciled the
	// criteria back to state, restore it so a representation-only swap is an empty
	// plan. Safe: site_name derives from the unchanged site_id.
	if criteria.CriteriaModelsEqual(plan.Criteria, state.Criteria) {
		plan.SiteName = state.SiteName
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// dsGroupCriteriaChanged reports whether suppression rewrote any criterion value.
func dsGroupCriteriaChanged(suppressed, planned []criteria.CriterionModel) bool {
	for i := range suppressed {
		if !suppressed[i].Value.Equal(planned[i].Value) {
			return true
		}
	}
	return false
}

// Create creates a new advanced computer search. Classic POSTs to id="0"; the
// server allocates the real integer ID and returns it in the response body
// (which carries only <id> — every other field must be read back via GET).
func (r *AdvancedComputerSearchResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AdvancedComputerSearchResourceModel
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

	resolved, authored, dsDiags := criteria.ResolveDSGroupCriteria(createCtx, r.ldap, dsGroupObjectType, plan.Criteria)
	resp.Diagnostics.Append(dsDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Criteria = resolved

	input, inputDiags := buildAdvancedComputerSearchInput(createCtx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateAdvancedComputerSearchByID(createCtx, "0", input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro advanced computer search", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing advanced computer search ID",
			"Jamf Pro returned 201 Created with no search ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	got, err := r.client.GetAdvancedComputerSearchByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro advanced computer search", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignAdvancedComputerSearchResourceModel(createCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Criteria = criteria.RestoreAuthoredDSGroupCriteria(plan.Criteria, authored)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, advancedComputerSearchIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro advanced computer search", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest search representation.
func (r *AdvancedComputerSearchResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AdvancedComputerSearchResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this advanced computer search without existing state or identity data, so the provider cannot determine which search to read.",
			)
			return
		}
		var identity advancedComputerSearchIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing advanced computer search ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the search.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(advancedComputerSearchTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro advanced computer search without ID.")
		return
	}

	got, err := r.client.GetAdvancedComputerSearchByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro advanced computer search not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, advancedComputerSearchIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro advanced computer search", helpers.APIErrorDetail(err))
		return
	}

	priorCriteria := state.Criteria
	resp.Diagnostics.Append(assignAdvancedComputerSearchResourceModel(readCtx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Criteria = criteria.ReadbackDSGroupCriteria(readCtx, r.ldap, dsGroupObjectType, state.Criteria, priorCriteria)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, advancedComputerSearchIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates an advanced computer search. Classic Update returns 201 with an
// empty body — we GET afterwards to refresh state.
func (r *AdvancedComputerSearchResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AdvancedComputerSearchResourceModel
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

	resolved, authored, dsDiags := criteria.ResolveDSGroupCriteria(updateCtx, r.ldap, dsGroupObjectType, plan.Criteria)
	resp.Diagnostics.Append(dsDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Criteria = resolved

	input, inputDiags := buildAdvancedComputerSearchInput(updateCtx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateAdvancedComputerSearchByID(updateCtx, plan.ID.ValueString(), input); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro advanced computer search", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetAdvancedComputerSearchByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro advanced computer search", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignAdvancedComputerSearchResourceModel(updateCtx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Criteria = criteria.RestoreAuthoredDSGroupCriteria(plan.Criteria, authored)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, advancedComputerSearchIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes an advanced computer search.
func (r *AdvancedComputerSearchResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AdvancedComputerSearchResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro advanced computer search without ID.")
		return
	}

	if err := r.client.DeleteAdvancedComputerSearchByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro advanced computer search already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro advanced computer search", helpers.APIErrorDetail(err))
	}
}
