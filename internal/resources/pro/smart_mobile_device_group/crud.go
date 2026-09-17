// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateSmartMobileDeviceGroupV2            POST   /pro/v2/mobile-device-groups/smart-groups
//   pro.GetSmartMobileDeviceGroupV2               GET    /pro/v2/mobile-device-groups/smart-groups/{id}
//   pro.UpdateSmartMobileDeviceGroupV2            PUT    /pro/v2/mobile-device-groups/smart-groups/{id}
//   pro.DeleteSmartMobileDeviceGroupV2            DELETE /pro/v2/mobile-device-groups/smart-groups/{id}
//   proclassic.CreateMobileDeviceGroupByID        POST   /JSSResource/mobiledevicegroups/id/0     (fallback create — see below)
//   proclassic.UpdateMobileDeviceGroupByID        PUT    /JSSResource/mobiledevicegroups/id/{id}  (fallback update — see below)
//   pro.PatchGroupV2                              PATCH  /pro/v2/groups/{platformId}              (fallback description write — see below)
//   pro.ListSmartMobileDeviceGroupsV2                    (plural data source / list resource)
//   pro.ResolveSmartMobileDeviceGroupV2IDByName          (data source name lookup)
//   pro.ListGroupsV2                                     (via progroups, platform_id bridge and the fallback create)
//
// # Why there are two write paths
//
// POST and PUT /pro/v2/mobile-device-groups/smart-groups validate a criterion's
// name and its operator, and that validation table disagrees with the rest of
// Jamf Pro: the `has` operator on an extension attribute is offered by the admin
// UI, stored by /JSSResource/mobiledevicegroups, and refused here with 400
// INVALID_FIELD. It is an open Jamf defect with no fix version, filed against
// computer groups only although it reproduces on this endpoint too, and
// internal/common/progroups/fallback.go carries the reproductions, the ticket
// numbers and the classifier. So a write attempts /pro/v2 first and, only when
// progroups.ModernWriteRefused says the refusal is one of that family, repeats
// the whole write through /JSSResource/mobiledevicegroups.
//
// Three wire facts shape that retry, each probed on Jamf Pro 11.32.0:
//
//   - A REFUSED /pro/v2 WRITE APPLIES NOTHING. A request changing name,
//     description, site and criteria at once, whose criteria were refused, left
//     all four at their previous values. So the retry starts from an unchanged
//     group: no compensation, no re-read, no rollback.
//   - The classic write is ONE request carrying name, site and criteria
//     together, and the criteria are NEVER split across two of them. Jamf Pro
//     recalculates membership synchronously with the write, so a half-written
//     group is immediately live and anything scoped to it acts on it.
//   - /JSSResource/mobiledevicegroups has no description field of any kind, and
//     it preserves a description it cannot express. PATCH /pro/v2/groups/{id}
//     carrying only groupDescription answers 204, sets it, and leaves criteria
//     and membership alone — so the description is a separate merge patch, taken
//     on a create and on an update that changes it, and on nothing else. That
//     route takes the PLATFORM identifier; a Jamf Pro id answers 404. The classic
//     create cannot ask for a platform identifier, so a fallback create resolves
//     one through the bridge before it can set a description.
//
// # Two things this endpoint does not share with the computer path
//
// siteId is MANDATORY on both the create and the update here, where both
// computer endpoints treat it as optional and clear the site when it is absent.
// A mobile write that omits it answers 403 with code INVALID_PRIVILEGE naming
// `field: siteId`, which reads as a permission fault and is not one — the same
// code and a null field come back for a site the integration cannot reach. The
// always-emit body this resource builds satisfies it unconditionally.
//
// The create is asked for platform identifiers, so its response carries the
// platform UUID in place of the Jamf Pro id. The GET then accepts that UUID as
// {id} and reports the Jamf Pro id in `groupId`, so both identifiers are known
// after two calls and no bridging request is needed on the happy path.
//
// Status: current. Last reviewed 2026-09-17.

package smart_mobile_device_group

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// dsGroupObjectType is the object class this resource targets, which dispatches
// the per-class directory-service group criterion allowlist. Mobile accepts four
// of the five names; the computer-only one is refused by this endpoint with 400
// INVALID_FIELD, and dispatching the right class is what turns that into a
// plan-time message naming the accepted criteria.
const dsGroupObjectType = criteria.ObjectTypeMobile

// writeLabel names the object the way the Jamf Pro admin UI does, for every
// diagnostic this resource raises.
func writeLabel() string {
	return progroups.Label(progroups.DeviceTypeMobile, progroups.GroupKindSmart)
}

// ModifyPlan raises the membership impact alert and suppresses a criteria diff
// that is only a change of representation.
//
// The suppression covers one case: a directory-service group criterion whose
// planned value names the same group as the stored value in the other form, a
// group name against the encoded value Jamf Pro keeps. It is soft, so a
// directory that cannot be reached leaves the diff standing rather than failing
// the plan, and it skips create and destroy, which have nothing to compare.
//
// The impact alert runs first, ahead of that guard, because a group entering or
// leaving management changes what everything scoped to it applies to.
//
// Both sides read `criteria` on its own through criteriaAtPlanTime rather than
// decoding the whole object, because a criteria list Terraform cannot resolve
// yet must defer here, not abort the plan — see that helper for why the decode
// of the resource model cannot carry it. Only the attribute is written back, so
// a suppression never round-trips the rest of the object.
func (r *SmartMobileDeviceGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportMembershipImpact(ctx, req, resp)

	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() || r.ldap == nil {
		return
	}

	planned, plannedSettled, plannedDiags := criteriaAtPlanTime(ctx, req.Plan)
	resp.Diagnostics.Append(plannedDiags...)
	if resp.Diagnostics.HasError() || !plannedSettled || len(planned) == 0 {
		return
	}

	prior, priorSettled, priorDiags := criteriaAtPlanTime(ctx, req.State)
	resp.Diagnostics.Append(priorDiags...)
	if resp.Diagnostics.HasError() || !priorSettled {
		return
	}

	suppressed := criteria.SuppressEquivalentDSGroupValues(ctx, r.ldap, planned, prior)
	changed := false
	for i := range suppressed {
		if !suppressed[i].Value.Equal(planned[i].Value) {
			changed = true
			break
		}
	}
	if !changed {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("criteria"), suppressed)...)
}

// newGroupID is the identifier the older group interface takes in the path of a
// create: it allocates an id of its own and returns it, so the request names the
// group that does not exist yet as zero.
const newGroupID = "0"

// Create creates a smart mobile device group.
//
// The create is issued with platform identifiers requested, so its response
// carries the platform UUID. That value is both stored as platform_id and used
// as the path identifier for the read-after-create, which is what makes the Jamf
// Pro id available without a third call.
//
// A create the endpoint refuses on a criterion or an operator is repeated
// through the older group interface, which accepts what this one will not. That
// path learns only the Jamf Pro id, because the older interface has no
// platform-identifier option, so the platform id is resolved through the bridge
// and the description follows as its own merge patch keyed on it. The order is
// forced: the group, then its Jamf Pro id, then its platform id, then the
// description.
//
// A description that cannot be set leaves a group that exists and does not match
// the configuration. State is written for what did succeed, so the group is
// managed rather than orphaned, and the error names the recovery. Writing the
// configured description into state instead would make the next plan see no
// difference and never finish the job.
//
// What that recovery is differs from the update's, which is why the two
// diagnostics do: an error returned alongside state from a CREATE leaves the
// object tainted, and Terraform plans a tainted object for replacement rather
// than for repair. See descriptionNotSetAfterCreate.
func (r *SmartMobileDeviceGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SmartMobileDeviceGroupResourceModel
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

	var (
		readID         string
		descriptionErr error
	)

	created, createErr := r.client.CreateSmartMobileDeviceGroupV2(createCtx, buildSmartMobileDeviceGroupInput(plan), true)
	switch {
	case createErr == nil:
		if created == nil || created.ID == "" {
			resp.Diagnostics.AddError(
				"Jamf Pro created the group without reporting an identifier",
				"The group may exist in Jamf Pro but Terraform cannot record it. Check for a "+writeLabel()+" named "+plan.Name.ValueString()+" and import it, or delete it and apply again.",
			)
			return
		}
		plan.PlatformID = types.StringValue(created.ID)
		readID = created.ID

	case fallbackApplies(createErr):
		classicCreated, classicErr := r.classicClient.CreateMobileDeviceGroupByID(createCtx, newGroupID, buildClassicSmartMobileDeviceGroupInput(plan))
		if classicErr != nil {
			resp.Diagnostics.Append(progroups.BothRefusedDiagnostics(writeLabel(), createErr, classicErr)...)
			return
		}
		resp.Diagnostics.Append(progroups.FallbackWarning(writeLabel(), createErr))
		if classicCreated == nil || classicCreated.ID == nil || *classicCreated.ID <= 0 {
			resp.Diagnostics.AddError(
				"Jamf Pro created the group without reporting an identifier",
				"The group may exist in Jamf Pro but Terraform cannot record it. Check for a "+writeLabel()+" named "+plan.Name.ValueString()+" and import it, or delete it and apply again.",
			)
			return
		}
		readID = strconv.Itoa(*classicCreated.ID)
		plan.ID = types.StringValue(readID)

		platformID, bridgeDiags := platformIDForDescriptionWrite(createCtx, r.client, r.pd, readID, types.StringNull())
		resp.Diagnostics.Append(bridgeDiags...)
		plan.PlatformID = types.StringNull()
		if platformID != "" {
			plan.PlatformID = types.StringValue(platformID)
		}

		if descriptionNeedsWrite(plan.Description, types.StringNull()) {
			descriptionErr = errPlatformIDUnresolved
			if platformID != "" {
				descriptionErr = r.client.PatchGroupV2(createCtx, platformID, buildDescriptionPatch(plan.Description))
			}
		}

	default:
		resp.Diagnostics.Append(progroups.WriteDiagnostics(createErr, writeLabel(), path.Root("site_id"), path.Empty())...)
		return
	}

	got, err := r.client.GetSmartMobileDeviceGroupV2(createCtx, readID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading the created Jamf Pro "+writeLabel(), helpers.APIErrorDetail(err))
		return
	}
	assignSmartMobileDeviceGroupResourceModel(&plan, got)
	if plan.ID.IsNull() || plan.ID.IsUnknown() || plan.ID.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Jamf Pro reported the created group without an identifier",
			"The read that follows a create returns the group's Jamf Pro identifier, and this response carried none, so Terraform cannot record what it made. Check for a "+writeLabel()+" named "+plan.Name.ValueString()+" and import it.",
		)
		return
	}
	plan.Criteria = criteria.RestoreAuthoredDSGroupCriteria(plan.Criteria, authored)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smartMobileDeviceGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro smart mobile device group", map[string]any{
		"id":          plan.ID.ValueString(),
		"platform_id": plan.PlatformID.ValueString(),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	if descriptionErr != nil {
		resp.Diagnostics.Append(descriptionNotSetAfterCreate(writeLabel(), descriptionErr))
	}
}

// Read refreshes state from Jamf Pro.
func (r *SmartMobileDeviceGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SmartMobileDeviceGroupResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform asked for a refresh with neither prior state nor identity data, so the provider cannot tell which group to read.",
			)
			return
		}
		var identity smartMobileDeviceGroupIdentityModel
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
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(smartMobileDeviceGroupTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing group identifier", "Cannot read a Jamf Pro "+writeLabel()+" without its identifier.")
		return
	}

	got, err := r.client.GetSmartMobileDeviceGroupV2(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro smart mobile device group not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smartMobileDeviceGroupIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro "+writeLabel(), helpers.APIErrorDetail(err))
		return
	}

	priorCriteria := state.Criteria
	priorPlatformID := state.PlatformID
	assignSmartMobileDeviceGroupResourceModel(&state, got)
	state.Criteria = criteria.ReadbackDSGroupCriteria(readCtx, r.ldap, dsGroupObjectType, state.Criteria, priorCriteria)

	platformID, platformDiags := resolvePlatformIDForRead(readCtx, r.client, r.pd, state.ID, priorPlatformID)
	resp.Diagnostics.Append(platformDiags...)
	state.PlatformID = platformID

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smartMobileDeviceGroupIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update rewrites the group.
//
// The write full-replaces the group's scalars and its criteria, and its response
// is a narrower shape than the read, so state is taken from a following read
// rather than from the write.
//
// An update the endpoint refuses on a criterion or an operator is repeated
// through the older group interface as ONE request carrying the name, the site
// and every criterion. The description is the only thing that interface cannot
// express, and it preserves what it cannot express, so the separate merge patch
// runs only when the planned description differs from the stored one. A
// criteria-only edit therefore stays a single write, which is the point: Jamf Pro
// recalculates membership synchronously, so every extra write on this path is a
// window in which a half-built group is live.
//
// The prior description is read as one attribute rather than by decoding the
// whole of prior state, because it is the only thing that decision needs from
// it.
//
// Both paths settle platform_id, and neither may leave it unknown. It is
// Computed with UseStateForUnknown, and that modifier declines a prior state
// holding null — which a fallback create produces whenever the bridge misses,
// and which a refresh preserves for the same reason — so an edit to such a group
// plans platform_id as unknown, and an unknown value returned from an apply
// aborts it as a provider bug on a configuration that is correct. The fallback
// arm resolves the identifier the description write takes and keeps it, which
// costs nothing when the plan already carried one; the arm that did not resolve
// one asks the bridge after the read, collapsing a miss to null rather than
// returning the unknown the resolver's prior argument would hand back.
func (r *SmartMobileDeviceGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SmartMobileDeviceGroupResourceModel
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

	if plan.ID.IsNull() || plan.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing group identifier", "Cannot update a Jamf Pro "+writeLabel()+" without its identifier.")
		return
	}

	resolved, authored, dsDiags := criteria.ResolveDSGroupCriteria(updateCtx, r.ldap, dsGroupObjectType, plan.Criteria)
	resp.Diagnostics.Append(dsDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Criteria = resolved

	var priorDescription types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("description"), &priorDescription)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var descriptionErr error
	if _, updateErr := r.client.UpdateSmartMobileDeviceGroupV2(updateCtx, plan.ID.ValueString(), buildSmartMobileDeviceGroupInput(plan)); updateErr != nil {
		if !fallbackApplies(updateErr) {
			resp.Diagnostics.Append(progroups.WriteDiagnostics(updateErr, writeLabel(), path.Root("site_id"), path.Empty())...)
			return
		}
		if classicErr := r.classicClient.UpdateMobileDeviceGroupByID(updateCtx, plan.ID.ValueString(), buildClassicSmartMobileDeviceGroupInput(plan)); classicErr != nil {
			resp.Diagnostics.Append(progroups.BothRefusedDiagnostics(writeLabel(), updateErr, classicErr)...)
			return
		}
		resp.Diagnostics.Append(progroups.FallbackWarning(writeLabel(), updateErr))

		platformID, bridgeDiags := platformIDForDescriptionWrite(updateCtx, r.client, r.pd, plan.ID.ValueString(), plan.PlatformID)
		resp.Diagnostics.Append(bridgeDiags...)
		plan.PlatformID = types.StringNull()
		if platformID != "" {
			plan.PlatformID = types.StringValue(platformID)
		}

		if descriptionNeedsWrite(plan.Description, priorDescription) {
			descriptionErr = errPlatformIDUnresolved
			if platformID != "" {
				descriptionErr = r.client.PatchGroupV2(updateCtx, platformID, buildDescriptionPatch(plan.Description))
			}
		}
	}

	got, err := r.client.GetSmartMobileDeviceGroupV2(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading the updated Jamf Pro "+writeLabel(), helpers.APIErrorDetail(err))
		return
	}
	assignSmartMobileDeviceGroupResourceModel(&plan, got)
	plan.Criteria = criteria.RestoreAuthoredDSGroupCriteria(plan.Criteria, authored)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smartMobileDeviceGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	if descriptionErr != nil {
		resp.Diagnostics.Append(descriptionNotSetAfterUpdate(writeLabel(), descriptionErr))
	}
}

// Delete removes the group.
//
// An already-absent group is a success. A group something is still scoped to is
// refused permanently, not while a dependency catches up, so the refusal is
// translated and returned rather than re-issued or polled.
func (r *SmartMobileDeviceGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SmartMobileDeviceGroupResourceModel
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
		resp.Diagnostics.AddError("Missing group identifier", "Cannot delete a Jamf Pro "+writeLabel()+" without its identifier.")
		return
	}

	if err := r.client.DeleteSmartMobileDeviceGroupV2(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro smart mobile device group already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.Append(progroups.DeleteDiagnostics(err, writeLabel())...)
	}
}
