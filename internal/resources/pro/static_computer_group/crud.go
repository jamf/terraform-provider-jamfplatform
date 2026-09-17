// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateStaticComputerGroupV3           POST   /pro/v3/computer-groups/static-groups?platform=true
//   pro.GetStaticComputerGroupV3              GET    /pro/v3/computer-groups/static-groups/{id}
//   pro.UpdateStaticComputerGroupV3           PUT    /pro/v3/computer-groups/static-groups/{id}
//   pro.DeleteStaticComputerGroupV3           DELETE /pro/v3/computer-groups/static-groups/{id}
//   proclassic.GetComputerGroupByID           GET    /JSSResource/computergroups/id/{id}    (membership read)
//   pro.ListStaticComputerGroupsV3            GET    /pro/v3/computer-groups/static-groups  (plural data source, list resource)
//   pro.ResolveStaticComputerGroupV3IDByName                                                (data source name lookup)
//   pro.ListGroupsV2                          GET    /pro/v2/groups                         (platform identifier bridge, through internal/common/progroups)
//
// Why one classic call sits in an otherwise pro/ construct: Jamf Pro publishes
// no static-computer-group membership read. /pro/v1, /pro/v2 and /pro/v3
// computer-groups/static-group-membership/{id} all answer 403 BAD_PERMISSIONS on
// a credential whose smart-group equivalent answers 200 on the same path shape,
// which this repository reads as an unrouted path rather than a missing
// privilege, and GET /pro/v3/computer-groups/static-groups/{id} carries no
// membership field at all. GET /JSSResource/computergroups/id/{id} is therefore
// the only way to learn who is in the group. Every write stays on pro/.
//
// Status: current. Last reviewed 2026-09-17.

package static_computer_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// Create writes the group and then reads it back.
//
// The create asks for platform identifiers, so its response carries the
// platform UUID rather than the Jamf Pro id. Every /pro/v3 group route accepts
// that UUID in place of {id}, so the read-after-create runs against it unchanged
// and reports the Jamf Pro id in its body. No bridging request is needed.
func (r *StaticComputerGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config StaticComputerGroupResourceModel
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

	if plan.AssignedComputerIDs.IsNull() && helpers.IsConfiguredValue(config.AssignedComputerIDs) {
		plan.AssignedComputerIDs = config.AssignedComputerIDs
	}
	manageMembership := helpers.IsConfiguredValue(plan.AssignedComputerIDs)

	assignments, assignmentDiags := configuredAssignments(createCtx, plan.AssignedComputerIDs)
	resp.Diagnostics.Append(assignmentDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateStaticComputerGroupV3(createCtx, buildStaticComputerGroupInput(plan, assignments), true)
	if err != nil {
		resp.Diagnostics.Append(progroups.WriteDiagnostics(err, groupLabel, path.Root("site_id"), path.Root("assigned_computer_ids"))...)
		return
	}
	if created == nil || created.ID == "" {
		resp.Diagnostics.AddError(
			"Jamf Pro created the group without reporting an identifier",
			"The group was created but Jamf Pro returned no identifier for it, so Terraform cannot record what it made. Find the group in Jamf Pro and import it.",
		)
		return
	}
	plan.PlatformID = types.StringValue(created.ID)

	got, err := r.client.GetStaticComputerGroupV3(createCtx, created.ID)
	if err != nil {
		recordCreatedGroup(createCtx, r.client, &plan, created.ID, resp)
		resp.Diagnostics.AddError("Error reading the created Jamf Pro "+groupLabel, helpers.APIErrorDetail(err))
		return
	}
	if got == nil || got.ID == "" {
		resp.Diagnostics.AddError(
			"Jamf Pro reported the created group without an identifier",
			"The group was created but reading it back produced no Jamf Pro identifier, so Terraform cannot record what it made. Find the group in Jamf Pro and import it.",
		)
		return
	}
	assignStaticComputerGroupResourceState(&plan, got)

	plan.AssignedComputerIDs = types.SetNull(types.StringType)
	if manageMembership {
		set, setDiags := membershipSetValue(createCtx, assignments)
		resp.Diagnostics.Append(setDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.AssignedComputerIDs = set
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, staticComputerGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro static computer group", map[string]any{
		"id":          plan.ID.ValueString(),
		"platform_id": plan.PlatformID.ValueString(),
		"members":     len(assignments),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the group.
//
// Membership is read only when the configuration manages it or when this is the
// first read after an import. Leaving it alone otherwise is what keeps
// membership maintained in Jamf Pro out of Terraform's state and out of its
// plans.
func (r *StaticComputerGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state StaticComputerGroupResourceModel

	if req.State.Raw.IsNull() {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform asked for a refresh of this group with neither state nor identity data, so the provider cannot tell which group to read.",
			)
			return
		}
		var identity staticComputerGroupIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing group identifier",
				"The resource identity carried no `id`, so the provider cannot refresh the group.",
			)
			return
		}
		state.ID = identity.ID
		state.PlatformID = types.StringNull()
		state.AssignedComputerIDs = types.SetNull(types.StringType)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(staticComputerGroupTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing group identifier", "Cannot read a Jamf Pro "+groupLabel+" without an identifier.")
		return
	}

	// name is schema-Required, so Create, Update and every ordinary refresh
	// populate it before Read runs. Neither import path does, which makes a null
	// name the one signal that this read is hydrating state for the first time.
	// Sampled here, before the wire values overwrite it.
	firstHydration := state.Name.IsNull()
	priorMembership := state.AssignedComputerIDs
	manageMembership := helpers.IsConfiguredValue(priorMembership)

	got, err := r.client.GetStaticComputerGroupV3(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro static computer group not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, staticComputerGroupIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro "+groupLabel, helpers.APIErrorDetail(err))
		return
	}
	assignStaticComputerGroupResourceState(&state, got)

	if state.PlatformID.IsNull() {
		byJamfProID, bridgeDiags := progroups.PlatformIDsByJamfProID(readCtx, r.client, r.pd, progroups.DeviceTypeComputer, progroups.GroupKindStatic)
		resp.Diagnostics.Append(bridgeDiags...)
		if resolved := progroups.PlatformIDValue(byJamfProID, state.ID.ValueString()); !resolved.IsNull() {
			state.PlatformID = resolved
		}
	}

	state.AssignedComputerIDs = types.SetNull(types.StringType)
	if manageMembership || firstHydration {
		group, membershipErr := r.classicClient.GetComputerGroupByID(readCtx, state.ID.ValueString())
		if membershipErr != nil {
			if helpers.IsNotFoundError(membershipErr) {
				tflog.Info(ctx, "Jamf Pro static computer group not found, removing from state", map[string]any{"id": state.ID.ValueString()})
				resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, staticComputerGroupIdentityModel{ID: state.ID})...)
				if resp.Diagnostics.HasError() {
					return
				}
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.AddError(
				"Error reading the membership of this Jamf Pro "+groupLabel,
				"Jamf Pro reports a static group's members separately from the group itself, and that read failed: "+helpers.APIErrorDetail(membershipErr),
			)
			return
		}
		wireIDs := flattenStaticComputerGroupMembership(group)
		wire, setDiags := membershipSetValue(readCtx, wireIDs)
		resp.Diagnostics.Append(setDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		// An import adopts the membership Jamf Pro reports, but only when there
		// is some: adopting an empty list would store `[]` where a create that
		// never declared the attribute stores null, and ImportStateVerify
		// compares the two.
		state.AssignedComputerIDs = scope.RefreshManagedSet(priorMembership, wire, firstHydration && len(wireIDs) > 0)
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, staticComputerGroupIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update writes the group and then reads it back.
//
// A configuration that does not manage membership costs one extra request. The
// assignments key is mandatory and a populated value replaces the whole
// membership, so there is no body that means "leave the members alone"; the
// current membership is read and sent back unchanged instead. A failed pre-read
// fails the apply, because the alternative is sending an empty list and emptying
// a group the operator deliberately left unmanaged.
func (r *StaticComputerGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config StaticComputerGroupResourceModel
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
		resp.Diagnostics.AddError("Missing group identifier", "Cannot update a Jamf Pro "+groupLabel+" without an identifier.")
		return
	}

	if plan.AssignedComputerIDs.IsNull() && helpers.IsConfiguredValue(config.AssignedComputerIDs) {
		plan.AssignedComputerIDs = config.AssignedComputerIDs
	}
	manageMembership := helpers.IsConfiguredValue(plan.AssignedComputerIDs)

	var assignments []string
	if manageMembership {
		configured, assignmentDiags := configuredAssignments(updateCtx, plan.AssignedComputerIDs)
		resp.Diagnostics.Append(assignmentDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		assignments = configured
	} else {
		current, membershipErr := r.classicClient.GetComputerGroupByID(updateCtx, plan.ID.ValueString())
		if membershipErr != nil {
			resp.Diagnostics.AddError(
				"Cannot read the current membership of this "+groupLabel,
				"Jamf Pro requires every update to a static group to carry its whole membership, and this configuration does not manage `assigned_computer_ids`, so the current members have to be read and sent back unchanged. That read failed and the update was not attempted, because sending the update without them would have emptied the group."+
					"\n\nJamf Pro's response: "+helpers.APIErrorDetail(membershipErr),
			)
			return
		}
		assignments = flattenStaticComputerGroupMembership(current)
	}

	if _, err := r.client.UpdateStaticComputerGroupV3(updateCtx, plan.ID.ValueString(), buildStaticComputerGroupInput(plan, assignments)); err != nil {
		resp.Diagnostics.Append(progroups.WriteDiagnostics(err, groupLabel, path.Root("site_id"), path.Root("assigned_computer_ids"))...)
		return
	}

	got, err := r.client.GetStaticComputerGroupV3(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading the updated Jamf Pro "+groupLabel, helpers.APIErrorDetail(err))
		return
	}
	assignStaticComputerGroupResourceState(&plan, got)

	plan.AssignedComputerIDs = types.SetNull(types.StringType)
	if manageMembership {
		set, setDiags := membershipSetValue(updateCtx, assignments)
		resp.Diagnostics.Append(setDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.AssignedComputerIDs = set
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, staticComputerGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated Jamf Pro static computer group", map[string]any{
		"id":      plan.ID.ValueString(),
		"members": len(assignments),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the group.
//
// An already-absent group is a success. A group something is still scoped to is
// refused, permanently, and progroups.DeleteDiagnostics names what holds it;
// that refusal is never re-issued or polled, because nothing about it is
// catching up in the background.
func (r *StaticComputerGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state StaticComputerGroupResourceModel
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
		resp.Diagnostics.AddError("Missing group identifier", "Cannot delete a Jamf Pro "+groupLabel+" without an identifier.")
		return
	}

	if err := r.client.DeleteStaticComputerGroupV3(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro static computer group already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.Append(progroups.DeleteDiagnostics(err, groupLabel)...)
	}
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
func recordCreatedGroup(ctx context.Context, client *pro.Client, plan *StaticComputerGroupResourceModel, platformID string, resp *resource.CreateResponse) {
	jamfProID, err := progroups.JamfProIDForPlatformID(ctx, client, platformID)
	if err != nil {
		return
	}
	plan.ID = types.StringValue(jamfProID)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}
