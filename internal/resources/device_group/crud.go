// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

const (
	deviceGroupCreateRetryDelay = 10 * time.Second
	deviceGroupDeleteRetryDelay = 10 * time.Second
)

// isDeletePropagationConflict reports whether a device-group DELETE error is a
// transient 406/409 raised while a dependent resource that referenced the group
// (e.g. a just-deleted blueprint) is still propagating server-side. The group
// still exists in this case, so re-issuing the DELETE once the reference clears
// is the correct action. (Distinct from the classic apps' accepted-async
// deletes, which return a misleading 400 and must NOT be re-issued.) The
// transport no longer retries 4xx, so this retry is handled here explicitly.
// The 406/409 set is from observed delete-after-blueprint behaviour; widen it if
// acceptance testing surfaces another transient status on this path.
func isDeletePropagationConflict(err error) bool {
	apiErr, ok := errors.AsType[*jamfplatform.APIResponseError](err)
	if !ok {
		return false
	}
	return apiErr.HasStatus(http.StatusNotAcceptable) || apiErr.HasStatus(http.StatusConflict)
}

// ModifyPlan suppresses a no-op diff when a directory-service group criterion's
// planned value is a different REPRESENTATION of the same group already in state
// (a raw base64 value swapped for the equivalent group name, or vice versa).
// Skips create (no prior state) and destroy (no plan); soft — any resolve
// failure leaves the diff intact.
func (r *DeviceGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Runs ahead of the create/destroy guard below: a group entering or leaving
	// management changes what everything scoped to it applies to, so both deserve
	// an impact alert even though neither needs the criteria suppression that
	// follows.
	r.reportMembershipImpact(ctx, req, resp)

	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() || r.proClient == nil {
		return
	}
	var plan, state DeviceGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || len(plan.Criteria) == 0 {
		return
	}
	// Suppress no-op representation swaps for both criterion families: a
	// directory-service group base64<->name swap, and a Jamf-group name<->id swap
	// (11.29 reads a member-of value back as the group id). Disjoint criterion
	// names, so the two passes never touch the same element.
	suppressed := suppressEquivalentDSGroupCriteria(ctx, r.proClient, plan.Criteria, state.Criteria)
	suppressed = suppressEquivalentGroupRefCriteria(ctx, r.groupRef, dsObjectType(plan.DeviceType.ValueString()), suppressed, state.Criteria)
	changed := false
	for i := range suppressed {
		if !suppressed[i].AttributeValue.Equal(plan.Criteria[i].AttributeValue) {
			changed = true
			break
		}
	}
	if !changed {
		return // additive: only touch the plan when a representation actually collapsed
	}
	plan.Criteria = suppressed
	// If suppression reconciled the criteria back to state, the swap is a pure
	// representation change with no membership impact. member_count is Computed
	// WITHOUT UseStateForUnknown, so core has already flipped it to unknown for the
	// (now-reverted) update — leaving a phantom "member_count -> known after apply"
	// diff. Restore it from state so a representation-only swap yields an empty
	// plan. Safe: member_count depends only on membership, which is unchanged when
	// the criteria are equal. (id / jamf_pro_id / members already use
	// UseStateForUnknown and need no such restore.)
	if deviceGroupCriteriaEqual(plan.Criteria, state.Criteria) {
		plan.MemberCount = state.MemberCount
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// deviceGroupCriteriaEqual reports whether two criteria slices are identical
// across every modelled field — used to confirm a directory-service group
// suppression left no real criteria change before restoring Computed siblings.
func deviceGroupCriteriaEqual(a, b []DeviceGroupCriteriaModel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Order.Equal(b[i].Order) ||
			!a[i].AttributeName.Equal(b[i].AttributeName) ||
			!a[i].Operator.Equal(b[i].Operator) ||
			!a[i].AttributeValue.Equal(b[i].AttributeValue) ||
			!a[i].JoinType.Equal(b[i].JoinType) ||
			!a[i].HasOpeningParenthesis.Equal(b[i].HasOpeningParenthesis) ||
			!a[i].HasClosingParenthesis.Equal(b[i].HasClosingParenthesis) {
			return false
		}
	}
	return true
}

// Create creates a new Jamf Platform device group resource.
func (r *DeviceGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan DeviceGroupResourceModel
	var config DeviceGroupResourceModel

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

	if plan.Members.IsNull() && helpers.IsConfiguredValue(config.Members) {
		plan.Members = config.Members
	}

	manageMembers := helpers.IsConfiguredValue(plan.Members)
	manageDescription := helpers.IsConfiguredValue(plan.Description)

	if err := validateDeviceGroupPlan(&plan); err != nil {
		resp.Diagnostics.AddError("Invalid device group configuration", helpers.APIErrorDetail(err))
		return
	}

	reqBody := &devicegroups.DeviceGroupCreateRepresentationV1{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueStringPointer(),
		DeviceType:  strings.ToUpper(plan.DeviceType.ValueString()),
		GroupType:   strings.ToUpper(plan.GroupType.ValueString()),
	}

	var authoredDSGroups map[string]string
	var authoredGroupRefs map[string]string
	switch strings.ToLower(plan.GroupType.ValueString()) {
	case "smart":
		resolved, authored, dsDiags := resolveDSGroupCriteria(createCtx, r.proClient, dsObjectType(plan.DeviceType.ValueString()), plan.Criteria)
		resp.Diagnostics.Append(dsDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.Criteria = resolved
		authoredDSGroups = authored
		var wireCriteria []DeviceGroupCriteriaModel
		wireCriteria, authoredGroupRefs = resolveGroupRefWireIDs(createCtx, r.groupRef, dsObjectType(plan.DeviceType.ValueString()), plan.Criteria, r.groupRefWriteSendsID(createCtx))
		criteria := expandDeviceGroupCriteria(wireCriteria)
		reqBody.Criteria = &criteria
	case "static":
		if manageMembers {
			members, diags := helpers.SetToStringSlice(createCtx, plan.Members)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}
			reqBody.Members = &members
		}
	}

	created, err := r.client.CreateDeviceGroup(createCtx, reqBody)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating device group",
			helpers.APIErrorDetail(err),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)

	if !r.refreshDeviceGroupState(createCtx, created.ID, &plan, manageMembers, manageDescription, &resp.Diagnostics) {
		return
	}
	plan.Criteria = restoreAuthoredDSGroupCriteria(plan.Criteria, authoredDSGroups)
	plan.Criteria = restoreAuthoredGroupRefCriteria(plan.Criteria, authoredGroupRefs, dsObjectType(plan.DeviceType.ValueString()))

	// resolveJamfProID degrades all Pro bridging failures to warnings so the
	// Platform Create result is never discarded. Append diagnostics without a
	// HasError gate — orphaning a successfully-created group would force a
	// manual `terraform import` to recover.
	jamfProID, jamfProDiags := resolveJamfProID(createCtx, r.proClient, r.pd, plan.ID.ValueString())
	resp.Diagnostics.Append(jamfProDiags...)
	plan.JamfProID = jamfProID

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, deviceGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created device group", map[string]any{
		"id":         created.ID,
		"group_type": plan.GroupType.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read syncs the Terraform state with the latest API representation.
func (r *DeviceGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DeviceGroupResourceModel
	stateAbsent := req.State.Raw.IsNull()

	if stateAbsent {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this device group without existing state or identity data, so the provider cannot determine which device group to read.",
			)
			return
		}

		var identity deviceGroupIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}

		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing device group ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the device group.",
			)
			return
		}

		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(deviceGroupTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read device group without ID.")
		return
	}

	grp, err := r.client.GetDeviceGroup(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "device group not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, deviceGroupIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading device group", helpers.APIErrorDetail(err))
		return
	}

	priorName := types.StringNull()
	if !stateAbsent {
		resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("name"), &priorName)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	hydrating := importHydration(stateAbsent, priorName)

	var members []string
	if strings.EqualFold(grp.GroupType, devicegroups.GroupTypeV1Static) && (hydrating || helpers.IsConfiguredValue(state.Members)) {
		var err error
		members, err = r.client.ListDeviceGroupMembers(readCtx, grp.ID)
		if err != nil {
			resp.Diagnostics.AddError("Error reading device group members", helpers.APIErrorDetail(err))
			return
		}
	}

	manageMembers := manageMembersOnRead(hydrating, state.Members, members)
	manageDescription := hydrating || helpers.IsConfiguredValue(state.Description)

	priorCriteria := state.Criteria
	resp.Diagnostics.Append(assignDeviceGroupModel(readCtx, &state, grp, members, manageMembers, manageDescription)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Criteria = readbackDSGroupCriteria(readCtx, r.proClient, state.Criteria, priorCriteria)
	state.Criteria = readbackGroupRefCriteria(readCtx, r.groupRef, dsObjectType(state.DeviceType.ValueString()), state.Criteria, priorCriteria)

	jamfProID, jamfProDiags := resolveJamfProID(readCtx, r.proClient, r.pd, state.ID.ValueString())
	resp.Diagnostics.Append(jamfProDiags...)
	state.JamfProID = jamfProID

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, deviceGroupIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates name/description/criteria and reconciles membership for static groups.
func (r *DeviceGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DeviceGroupResourceModel

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

	if err := validateDeviceGroupPlan(&plan); err != nil {
		resp.Diagnostics.AddError("Invalid device group configuration", helpers.APIErrorDetail(err))
		return
	}

	manageMembers := helpers.IsConfiguredValue(plan.Members)
	manageDescription := helpers.IsConfiguredValue(plan.Description)

	updateReq := &devicegroups.DeviceGroupUpdateRepresentationV1{
		Name:        plan.Name.ValueStringPointer(),
		Description: plan.Description.ValueStringPointer(),
	}

	var authoredDSGroups map[string]string
	var authoredGroupRefs map[string]string
	if strings.ToLower(plan.GroupType.ValueString()) == "smart" {
		resolved, authored, dsDiags := resolveDSGroupCriteria(updateCtx, r.proClient, dsObjectType(plan.DeviceType.ValueString()), plan.Criteria)
		resp.Diagnostics.Append(dsDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.Criteria = resolved
		authoredDSGroups = authored
		var wireCriteria []DeviceGroupCriteriaModel
		wireCriteria, authoredGroupRefs = resolveGroupRefWireIDs(updateCtx, r.groupRef, dsObjectType(plan.DeviceType.ValueString()), plan.Criteria, r.groupRefWriteSendsID(updateCtx))
		criteria := expandDeviceGroupCriteria(wireCriteria)
		updateReq.Criteria = &criteria
	}

	if err := r.client.UpdateDeviceGroup(updateCtx, plan.ID.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError(
			"Error updating device group",
			helpers.APIErrorDetail(err),
		)
		return
	}

	if strings.ToLower(plan.GroupType.ValueString()) == "static" && manageMembers {
		desired, diags := helpers.SetToStringSlice(updateCtx, plan.Members)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		current, err := r.client.ListDeviceGroupMembers(updateCtx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading device group members", helpers.APIErrorDetail(err))
			return
		}

		added, removed := diffStringSlices(current, desired)
		if len(added) > 0 || len(removed) > 0 {
			patch := &devicegroups.DeviceGroupMemberPatchRepresentationV1{}
			if len(added) > 0 {
				patch.Added = &added
			}
			if len(removed) > 0 {
				patch.Removed = &removed
			}
			if err := r.client.UpdateDeviceGroupMembers(updateCtx, plan.ID.ValueString(), patch); err != nil {
				resp.Diagnostics.AddError("Error updating device group members", helpers.APIErrorDetail(err))
				return
			}
		}
	}

	if !r.refreshDeviceGroupState(updateCtx, plan.ID.ValueString(), &plan, manageMembers, manageDescription, &resp.Diagnostics) {
		return
	}
	plan.Criteria = restoreAuthoredDSGroupCriteria(plan.Criteria, authoredDSGroups)
	plan.Criteria = restoreAuthoredGroupRefCriteria(plan.Criteria, authoredGroupRefs, dsObjectType(plan.DeviceType.ValueString()))

	jamfProID, jamfProDiags := resolveJamfProID(updateCtx, r.proClient, r.pd, plan.ID.ValueString())
	resp.Diagnostics.Append(jamfProDiags...)
	plan.JamfProID = jamfProID

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, deviceGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the device group from Jamf Platform.
func (r *DeviceGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DeviceGroupResourceModel

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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete device group without ID.")
		return
	}

	// Retry the DELETE on a transient 406/409 (a dependent resource that
	// referenced this group is still propagating its own deletion); a clean
	// success or a not-found means the group is gone. Any other error is
	// terminal. Bounded by the delete timeout.
	id := state.ID.ValueString()
	pollErr := jamfplatform.PollUntil(deleteCtx, deviceGroupDeleteRetryDelay, func(c context.Context) (bool, error) {
		err := r.client.DeleteDeviceGroup(c, id)
		switch {
		case err == nil:
			return true, nil
		case helpers.IsNotFoundError(err):
			tflog.Info(c, "device group already removed", map[string]any{"id": id})
			return true, nil
		case isDeletePropagationConflict(err):
			tflog.Info(c, "device group delete blocked by a still-propagating dependency; retrying", map[string]any{"id": id})
			return false, nil
		default:
			return false, err
		}
	})
	if pollErr != nil {
		resp.Diagnostics.AddError("Error deleting device group", helpers.APIErrorDetail(pollErr))
	}
}
