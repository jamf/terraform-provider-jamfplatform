// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateStaticMobileDeviceGroupV2          POST   /pro/v2/mobile-device-groups/static-groups
//   pro.GetStaticMobileDeviceGroupV2             GET    /pro/v2/mobile-device-groups/static-groups/{id}
//   pro.PatchStaticMobileDeviceGroupV2           PATCH  /pro/v2/mobile-device-groups/static-groups/{id}
//   pro.DeleteStaticMobileDeviceGroupV2          DELETE /pro/v2/mobile-device-groups/static-groups/{id}
//   pro.ListStaticMobileDeviceGroupMembershipV2  GET    /pro/v2/mobile-device-groups/static-group-membership/{id}
//   pro.ListStaticMobileDeviceGroupsV2           GET    /pro/v2/mobile-device-groups/static-groups        (plural data source, list resource)
//   pro.ResolveStaticMobileDeviceGroupV2IDByName GET    /pro/v2/mobile-device-groups/static-groups        (data source name lookup)
//   pro.ListGroupsV2                             GET    /pro/v2/groups                                    (platform identifier bridge, via internal/common/progroups)
//
// Status: current. Last reviewed 2026-09-17.
//
// # One body, two opposite write semantics
//
// This is the most surprising write in the provider's device-group family, and
// nothing about it generalises from the static COMPUTER group, whose equivalent
// field means the opposite. All four facts were probed.
//
//   - Omitting `assignments` answers 500 with an empty errors array and applies
//     nothing, the scalars included. So the key is always emitted.
//   - `assignments: []` leaves membership exactly as it was. It is a no-op, not
//     a clear.
//   - An entry {mobileDeviceId, selected: true} ADDS that device and leaves
//     every other member in place.
//   - An entry {mobileDeviceId, selected: false} REMOVES that device and leaves
//     every other member in place.
//
// So `assignments` is a delta, and the only way to make the configuration
// authoritative is to read the current membership and send the difference:
// planned-not-current as selected true, current-not-planned as selected false.
// An update that manages membership therefore costs two requests, and a failed
// pre-read fails the apply. Falling back to sending additions alone would leave
// devices the practitioner removed still in the group while reporting success,
// and the plan would never settle.
//
// The scalars in the same body do not behave that way at all. `groupName`,
// `groupDescription` and `siteId` full-replace, so omitting the description
// empties it. Every write emits all three.
//
// # Identifiers
//
// The create asks for the platform identifier and gets it in place of the Jamf
// Pro one; the read-after-create then runs against that value, because every
// route here accepts either, and reports the Jamf Pro identifier in `groupId`.
// So the happy path costs no bridging request. Only import starts with a Jamf
// Pro identifier and no platform one, and progroups resolves that.

package static_mobile_device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// membersPath anchors a membership diagnostic to the attribute that caused it.
var membersPath = path.Root("assigned_mobile_device_ids")

// sitePath anchors a site diagnostic to the attribute that caused it.
var sitePath = path.Root("site_id")

// ModifyPlan reports the plan-time impact alert for a membership change. A group
// entering or leaving Terraform management changes what everything scoped to it
// applies to, so it runs on creates and destroys as well as updates.
func (r *StaticMobileDeviceGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportMembershipImpact(ctx, req, resp)
}

// Create creates the group, then reads it back to learn its Jamf Pro identifier.
func (r *StaticMobileDeviceGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config StaticMobileDeviceGroupResourceModel
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

	if plan.AssignedMobileDeviceIDs.IsNull() && helpers.IsConfiguredValue(config.AssignedMobileDeviceIDs) {
		plan.AssignedMobileDeviceIDs = config.AssignedMobileDeviceIDs
	}
	manageMembership := helpers.IsConfiguredValue(plan.AssignedMobileDeviceIDs)

	assignments, assignmentDiags := buildCreateAssignments(createCtx, plan.AssignedMobileDeviceIDs)
	resp.Diagnostics.Append(assignmentDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateStaticMobileDeviceGroupV2(createCtx, buildGroupInput(plan, assignments), true)
	if err != nil {
		resp.Diagnostics.Append(progroups.WriteDiagnostics(err, progroups.Label(deviceType, groupKind), sitePath, membersPath)...)
		return
	}
	if created == nil || created.ID == "" {
		resp.Diagnostics.AddError(
			"Jamf Pro reported no identifier for the group it created",
			"The group was created but Jamf Pro returned no identifier for it, so Terraform cannot record what it made. Find the group in Jamf Pro and import it, or delete it and apply again.",
		)
		return
	}
	plan.PlatformID = types.StringValue(created.ID)

	got, err := r.client.GetStaticMobileDeviceGroupV2(createCtx, created.ID)
	if err != nil {
		recordCreatedGroup(createCtx, r.client, &plan, created.ID, resp)
		resp.Diagnostics.AddError("Error reading the created Jamf Pro static mobile device group", helpers.APIErrorDetail(err))
		return
	}
	if got == nil || got.GroupID == "" {
		resp.Diagnostics.AddError(
			"Jamf Pro reported the created group without its identifier",
			"The group was created but the read that follows returned no identifier for it, so Terraform cannot record what it made. Find the group in Jamf Pro and import it, or delete it and apply again.",
		)
		return
	}
	assignResourceModel(&plan, got)
	if !manageMembership {
		plan.AssignedMobileDeviceIDs = types.SetNull(types.StringType)
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, staticMobileDeviceGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro static mobile device group", map[string]any{
		"id":          plan.ID.ValueString(),
		"platform_id": plan.PlatformID.ValueString(),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes state. Membership costs a second request, so it is fetched only
// when the configuration manages it or when this is a first-time import.
func (r *StaticMobileDeviceGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state StaticMobileDeviceGroupResourceModel
	identityOnly := req.State.Raw.IsNull()

	if identityOnly {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this static mobile device group with neither prior state nor identity data, so the provider cannot tell which group to read.",
			)
			return
		}
		var identity staticMobileDeviceGroupIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing group identifier",
				"The resource identity carried no `id`, so the provider cannot refresh the static mobile device group.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(staticMobileDeviceGroupTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	firstHydration := state.Name.IsNull()

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing group identifier", "Cannot read a Jamf Pro static mobile device group without an identifier.")
		return
	}

	manageMembership := helpers.IsConfiguredValue(state.AssignedMobileDeviceIDs)

	got, err := r.client.GetStaticMobileDeviceGroupV2(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro static mobile device group not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, staticMobileDeviceGroupIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro static mobile device group", helpers.APIErrorDetail(err))
		return
	}
	assignResourceModel(&state, got)

	var memberIDs []string
	if manageMembership || firstHydration {
		devices, memberErr := r.client.ListStaticMobileDeviceGroupMembershipV2(readCtx, state.ID.ValueString(), nil, "")
		if memberErr != nil {
			resp.Diagnostics.AddError("Error reading the membership of a Jamf Pro static mobile device group", helpers.APIErrorDetail(memberErr))
			return
		}
		memberIDs = membershipIDs(devices)
	}
	resp.Diagnostics.Append(assignMembership(readCtx, &state, memberIDs, manageMembership, firstHydration)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.resolvePlatformID(readCtx, &state)...)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, staticMobileDeviceGroupIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the group. Managing membership means reading what the group
// currently holds and sending the difference, because the endpoint merges the
// assignments it is given instead of replacing them.
func (r *StaticMobileDeviceGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config StaticMobileDeviceGroupResourceModel
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

	if plan.ID.IsNull() || plan.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing group identifier", "Cannot update a Jamf Pro static mobile device group without an identifier.")
		return
	}

	if plan.AssignedMobileDeviceIDs.IsNull() && helpers.IsConfiguredValue(config.AssignedMobileDeviceIDs) {
		plan.AssignedMobileDeviceIDs = config.AssignedMobileDeviceIDs
	}
	manageMembership := helpers.IsConfiguredValue(plan.AssignedMobileDeviceIDs)

	assignments := []pro.Assignment{}
	if manageMembership {
		planned, plannedDiags := helpers.SetToStringSlice(updateCtx, plan.AssignedMobileDeviceIDs)
		resp.Diagnostics.Append(plannedDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		devices, memberErr := r.client.ListStaticMobileDeviceGroupMembershipV2(updateCtx, plan.ID.ValueString(), nil, "")
		if memberErr != nil {
			resp.Diagnostics.AddAttributeError(
				membersPath,
				"Cannot change this group's membership without first reading it",
				"Jamf Pro adds and removes the devices a write names and leaves everyone else in the group, so Terraform has to read the current membership to work out what to remove. That read failed, and Terraform stopped rather than send the additions alone, which would have left the devices you removed in the group and reported success."+
					"\n\nJamf Pro's response: "+helpers.APIErrorDetail(memberErr),
			)
			return
		}
		assignments = assignmentsForUpdate(planned, membershipIDs(devices))
	}

	if _, err := r.client.PatchStaticMobileDeviceGroupV2(updateCtx, plan.ID.ValueString(), buildGroupInput(plan, assignments)); err != nil {
		resp.Diagnostics.Append(progroups.WriteDiagnostics(err, progroups.Label(deviceType, groupKind), sitePath, membersPath)...)
		return
	}

	got, err := r.client.GetStaticMobileDeviceGroupV2(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading the updated Jamf Pro static mobile device group", helpers.APIErrorDetail(err))
		return
	}
	assignResourceModel(&plan, got)
	if !manageMembership {
		plan.AssignedMobileDeviceIDs = types.SetNull(types.StringType)
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, staticMobileDeviceGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the group. An already-absent group is a success, and a refusal
// because something is still scoped to the group is permanent, so it is reported
// rather than retried.
func (r *StaticMobileDeviceGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state StaticMobileDeviceGroupResourceModel
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
		resp.Diagnostics.AddError("Missing group identifier", "Cannot delete a Jamf Pro static mobile device group without an identifier.")
		return
	}

	if err := r.client.DeleteStaticMobileDeviceGroupV2(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro static mobile device group already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.Append(progroups.DeleteDiagnostics(err, progroups.Label(deviceType, groupKind))...)
	}
}

// resolvePlatformID fills in platform_id when state has none, which happens only
// on import: the create learns it from Jamf Pro directly, and it never changes
// afterwards, so an ordinary refresh already has it.
//
// The lookup is best-effort. platform_id exists only to reference the group from
// a Jamf Platform construct, so a failure warns and leaves the prior value
// alone rather than discarding a refresh that has already succeeded.
func (r *StaticMobileDeviceGroupResource) resolvePlatformID(ctx context.Context, state *StaticMobileDeviceGroupResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if !state.PlatformID.IsNull() {
		return diags
	}
	byJamfProID, bridgeDiags := progroups.PlatformIDsByJamfProID(ctx, r.client, r.pd, deviceType, groupKind)
	diags.Append(bridgeDiags...)
	if resolved := progroups.PlatformIDValue(byJamfProID, state.ID.ValueString()); !resolved.IsNull() {
		state.PlatformID = resolved
	}
	return diags
}

// recordCreatedGroup records a group the create has already made, when the
// mandatory read-after-create is what failed.
//
// Returning an error and no state is what orphans a group: Terraform keeps no
// record of it, the next apply is refused for a duplicate name, and the group
// has to be found and imported by hand. The create reported the platform
// identifier, so the Jamf Pro identifier the resource keys on is recoverable —
// and recoverable from a DIFFERENT endpoint than the one that just failed, which
// is what makes the attempt worth making rather than a retry of the same call.
//
// When even that fails there is nothing to record, and the error the caller adds
// names the platform identifier so the group can still be found.
func recordCreatedGroup(ctx context.Context, client *pro.Client, plan *StaticMobileDeviceGroupResourceModel, platformID string, resp *resource.CreateResponse) {
	jamfProID, err := progroups.JamfProIDForPlatformID(ctx, client, platformID)
	if err != nil {
		return
	}
	plan.ID = types.StringValue(jamfProID)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}
