// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateUserGroupByID
//   proclassic.GetUserGroupByID
//   proclassic.UpdateUserGroupByID
//   proclassic.DeleteUserGroupByID
//   proclassic.ListUserGroups            (data source / list resource / name collision check in collision.go)
//   proclassic.GetUserGroupByName        (data source name lookup)
//   proclassic.ResolveUserGroupIDByName  (data source name → ID)
//
// Uniqueness: name, enforced (409 Duplicate name), case-sensitive, surrounding spaces trimmed, accent-sensitive.
//
// Status: current. Last reviewed 2026-10-25.

package user_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// dsGroupObjectType is the object class this resource targets, used to dispatch
// the per-class directory-service group criterion allowlist. User groups accept
// only the Username directory-service group criterion.
const dsGroupObjectType = criteria.ObjectTypeUser

// toCriterionModels / fromCriterionModels bridge the user-group criterion model
// to the shared criteria.CriterionModel (field-identical) so the directory-service
// group resolve/restore/readback/suppress helpers can be reused. The shared
// helpers only ever mutate Value; every other field round-trips unchanged.
func toCriterionModels(in []UserGroupCriterionModel) []criteria.CriterionModel {
	if in == nil {
		return nil // preserve nil (no-criteria static group) — never flip nil -> []
	}
	out := make([]criteria.CriterionModel, len(in))
	for i, c := range in {
		out[i] = criteria.CriterionModel{
			Priority:              c.Priority,
			Name:                  c.Name,
			SearchType:            c.SearchType,
			Value:                 c.Value,
			AndOr:                 c.AndOr,
			HasOpeningParenthesis: c.HasOpeningParenthesis,
			HasClosingParenthesis: c.HasClosingParenthesis,
		}
	}
	return out
}

func fromCriterionModels(in []criteria.CriterionModel) []UserGroupCriterionModel {
	if in == nil {
		return nil // preserve nil — never flip nil -> []
	}
	out := make([]UserGroupCriterionModel, len(in))
	for i, c := range in {
		out[i] = UserGroupCriterionModel{
			Priority:              c.Priority,
			Name:                  c.Name,
			SearchType:            c.SearchType,
			Value:                 c.Value,
			AndOr:                 c.AndOr,
			HasOpeningParenthesis: c.HasOpeningParenthesis,
			HasClosingParenthesis: c.HasClosingParenthesis,
		}
	}
	return out
}

// groupRefWorkaroundApplies reports whether the Jamf-group member-of name<->id
// workaround should engage for this tenant. The classic /usergroups write is a pure
// pass-through (the server accepts the group name and stores the id itself), so the
// only workaround activity on the write path is building the id->name restore map
// used by the post-create/update read-back; it is only needed inside the 11.29
// regressed window [11.29.0, 11.30.1), where the server echoes the id back. 11.30.1+
// restored the name round-trip (wire-probed live), so the map is skipped and the
// authored name flows through untouched — no needless name->id lookups. Soft: an
// unavailable/unparseable version keeps the workaround engaged (fail-open, matching
// device_group). Version is read from the cached providerdata (no network after the
// first lookup in a run); r.pd is nil only in unit tests that never set it.
func (r *UserGroupResource) groupRefWorkaroundApplies(ctx context.Context) bool {
	if r.pd == nil {
		return true
	}
	v, err := r.pd.GetJamfProVersion(ctx)
	if err != nil {
		return true
	}
	return criteria.GroupRefWorkaroundApplies(v)
}

// ModifyPlan suppresses a no-op diff when a directory-service group criterion's
// planned value is a different REPRESENTATION of the same group already in state
// (a raw base64 value swapped for the equivalent group name, or vice versa).
// Skips create (no prior state) and destroy (no plan); soft — any resolve
// failure leaves the diff intact.
func (r *UserGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)

	// Runs ahead of the create/destroy guard below: a group entering or leaving
	// management changes what everything scoped to it applies to.
	r.reportMembershipImpact(ctx, req, resp)

	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() || (r.ldap == nil && r.groupRef == nil) {
		return
	}
	var plan, state UserGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || len(plan.Criteria) == 0 {
		return
	}
	planModels := toCriterionModels(plan.Criteria)
	stateModels := toCriterionModels(state.Criteria)
	// Suppress no-op representation swaps for both criterion families: a
	// directory-service group base64<->name swap, and a Jamf-group name<->id swap
	// (11.29 reads a member-of value back as the group id). Disjoint criterion
	// names, so the two passes never touch the same element.
	suppressed := criteria.SuppressEquivalentDSGroupValues(ctx, r.ldap, planModels, stateModels)
	if r.groupRef != nil {
		suppressed = criteria.SuppressEquivalentGroupRefValues(ctx, r.groupRef, dsGroupObjectType, suppressed, stateModels)
	}
	changed := false
	for i := range suppressed {
		if !suppressed[i].Value.Equal(planModels[i].Value) {
			changed = true
			break
		}
	}
	if !changed {
		return // additive: only touch the plan when a representation actually collapsed
	}
	plan.Criteria = fromCriterionModels(suppressed)
	// member_count and site_name are both Computed without UseStateForUnknown, so
	// core flipped them to unknown for the (now-reverted) update. When suppression
	// reconciled the criteria back to state, restore them so a representation-only
	// swap is an empty plan. Safe: member_count depends only on membership, which is
	// unchanged when the criteria are equal; site_name is derived from site_id (it
	// dropped UseStateForUnknown per §886) and a criteria-only swap cannot change
	// site_id, so the prior name still holds.
	if criteria.CriteriaModelsEqual(suppressed, toCriterionModels(state.Criteria)) {
		plan.MemberCount = state.MemberCount
		if plan.SiteID.Equal(state.SiteID) {
			plan.SiteName = state.SiteName
		}
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// Create creates a new Jamf Pro user group. Classic POSTs to id="0"; the
// server allocates the real integer ID and returns it in the response body.
func (r *UserGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan UserGroupResourceModel
	var config UserGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
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

	// Preserve the user's intent to manage members across the Create+refresh
	// hop. Members is Optional+UseStateForUnknown — if the user supplied it
	// the plan carries it, otherwise it's null.
	if plan.Members.IsNull() && helpers.IsConfiguredValue(config.Members) {
		plan.Members = config.Members
	}
	manageMembers := helpers.IsConfiguredValue(plan.Members)

	if err := validateUserGroupPlan(&plan); err != nil {
		resp.Diagnostics.AddError("Invalid user group configuration", helpers.APIErrorDetail(err))
		return
	}

	resolved, authored, dsDiags := criteria.ResolveDSGroupCriteria(createCtx, r.ldap, dsGroupObjectType, toCriterionModels(plan.Criteria))
	resp.Diagnostics.Append(dsDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Criteria = fromCriterionModels(resolved)
	var groupRefAuthored map[string]string
	if r.groupRefWorkaroundApplies(createCtx) {
		groupRefAuthored = criteria.ResolveAuthoredGroupRefMap(createCtx, r.groupRef, dsGroupObjectType, resolved)
	}

	input, inputDiags := buildUserGroupInput(createCtx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateUserGroupByID(createCtx, "0", input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro user group", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing user group ID",
			"Jamf Pro returned 201 Created with no user group ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	got, err := r.client.GetUserGroupByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro user group", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignUserGroupResourceModel(createCtx, &plan, got, manageMembers, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	restored := criteria.RestoreAuthoredDSGroupCriteria(toCriterionModels(plan.Criteria), authored)
	restored = criteria.RestoreAuthoredGroupRefCriteria(restored, groupRefAuthored, dsGroupObjectType)
	plan.Criteria = fromCriterionModels(restored)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, userGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro user group", map[string]any{"id": plan.ID.ValueString(), "group_type": plan.GroupType.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest user group representation.
func (r *UserGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state UserGroupResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this user group without existing state or identity data, so the provider cannot determine which user group to read.",
			)
			return
		}
		var identity userGroupIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing user group ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the user group.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(userGroupTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro user group without ID.")
		return
	}

	got, err := r.client.GetUserGroupByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro user group not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, userGroupIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro user group", helpers.APIErrorDetail(err))
		return
	}

	manageMembers := helpers.IsConfiguredValue(state.Members)
	hydrating := importHydration(isImport, state.Name)
	priorCriteria := toCriterionModels(state.Criteria)
	resp.Diagnostics.Append(assignUserGroupResourceModel(readCtx, &state, got, manageMembers, hydrating)...)
	if resp.Diagnostics.HasError() {
		return
	}
	readback := criteria.ReadbackDSGroupCriteria(readCtx, r.ldap, dsGroupObjectType, toCriterionModels(state.Criteria), priorCriteria)
	readback = criteria.ReadbackGroupRefCriteria(readCtx, r.groupRef, dsGroupObjectType, readback, priorCriteria)
	state.Criteria = fromCriterionModels(readback)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, userGroupIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro user group. Classic UpdateUserGroupByID returns
// 201 with an empty body — we must GET to refresh state.
func (r *UserGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan UserGroupResourceModel
	var config UserGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
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

	if plan.Members.IsNull() && helpers.IsConfiguredValue(config.Members) {
		plan.Members = config.Members
	}
	manageMembers := helpers.IsConfiguredValue(plan.Members)

	if err := validateUserGroupPlan(&plan); err != nil {
		resp.Diagnostics.AddError("Invalid user group configuration", helpers.APIErrorDetail(err))
		return
	}

	resolved, authored, dsDiags := criteria.ResolveDSGroupCriteria(updateCtx, r.ldap, dsGroupObjectType, toCriterionModels(plan.Criteria))
	resp.Diagnostics.Append(dsDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Criteria = fromCriterionModels(resolved)
	var groupRefAuthored map[string]string
	if r.groupRefWorkaroundApplies(updateCtx) {
		groupRefAuthored = criteria.ResolveAuthoredGroupRefMap(updateCtx, r.groupRef, dsGroupObjectType, resolved)
	}

	input, inputDiags := buildUserGroupInput(updateCtx, plan)
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateUserGroupByID(updateCtx, plan.ID.ValueString(), input); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro user group", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetUserGroupByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro user group", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignUserGroupResourceModel(updateCtx, &plan, got, manageMembers, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	restored := criteria.RestoreAuthoredDSGroupCriteria(toCriterionModels(plan.Criteria), authored)
	restored = criteria.RestoreAuthoredGroupRefCriteria(restored, groupRefAuthored, dsGroupObjectType)
	plan.Criteria = fromCriterionModels(restored)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, userGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro user group.
func (r *UserGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state UserGroupResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro user group without ID.")
		return
	}

	if err := r.client.DeleteUserGroupByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro user group already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro user group", helpers.APIErrorDetail(err))
	}
}
