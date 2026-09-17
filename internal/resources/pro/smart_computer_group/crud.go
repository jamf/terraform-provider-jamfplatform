// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateSmartComputerGroupV3
//   pro.GetSmartComputerGroupV3
//   pro.UpdateSmartComputerGroupV3
//   pro.DeleteSmartComputerGroupV3
//   pro.GetGroupV2                        (via progroups.JamfProIDForPlatformID, create only)
//   pro.ListGroupsV2                      (via progroups.PlatformIDsByJamfProID, import and the fallback write)
//   pro.SearchLdapGroupsV1                (via the shared directory-service group criterion helpers)
//   proclassic.CreateComputerGroupByID    (fallback create — see below)
//   proclassic.UpdateComputerGroupByID    (fallback update — see below)
//   pro.PatchGroupV2                      (fallback description write — see below)
//   pro.ListSmartComputerGroupsV3         (data sources / list resource)
//   pro.ResolveSmartComputerGroupV3IDByName (data source name lookup)
//
// Why there are two write paths at all. POST and PUT
// /pro/v3/computer-groups/smart-groups validate a criterion's name and its
// operator, and that validation table disagrees with the rest of Jamf Pro: a
// patch reporting criterion, and the `has` operator on an extension attribute,
// are both offered by the admin UI and stored by /JSSResource/computergroups and
// refused here with 400 INVALID_FIELD. Both are open Jamf defects with no fix
// version, and internal/common/progroups/fallback.go carries the reproductions,
// the ticket numbers and the classifier. So a write attempts v3 first and, only
// when progroups.ModernWriteRefused says the refusal is one of those, repeats
// the whole write through /JSSResource/computergroups.
//
// Three wire facts shape that retry, each probed on Jamf Pro 11.32.0:
//
//   - A REFUSED v3 WRITE APPLIES NOTHING. A request changing name, description,
//     site and criteria at once, whose criteria were refused, left all four at
//     their previous values. So the retry starts from an unchanged group: no
//     compensation, no re-read, no rollback.
//   - The classic write is ONE request carrying name, site and criteria
//     together, and the criteria are NEVER split across two of them. Jamf Pro
//     recalculates membership synchronously with the write, so a half-written
//     group is immediately live and anything scoped to it acts on it.
//   - /JSSResource/computergroups has no description field of any kind, and it
//     preserves a description it cannot express. PATCH /pro/v2/groups/{id}
//     carrying only groupDescription answers 204, sets it, and leaves criteria
//     and membership alone — so the description is a separate merge patch, taken
//     on create and on an update that changes it. That route takes the PLATFORM
//     identifier; a Jamf Pro id answers 404.
//
// Status: current. Last reviewed 2026-09-17.

package smart_computer_group

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// writeOutcome is what a smart group write's result means for what happens next.
type writeOutcome int

const (
	// writeSucceeded is the ordinary case: nothing else to do.
	writeSucceeded writeOutcome = iota
	// writeRefusedByJamfPro is a criterion or operator combination Jamf Pro's
	// group endpoints refuse and its older group interface stores. Only this
	// outcome takes the fallback.
	writeRefusedByJamfPro
	// writeFailed is every other failure, reported as it always was.
	writeFailed
)

// classifyWrite decides which of the three a write's error is.
//
// The gate is progroups.ModernWriteRefused and nothing else. In particular it is
// NOT the criterion's name: matching "Patch Reporting: " would have covered one
// of the refusals this fallback exists for and none of the ones that come next,
// and it would have sent a genuinely malformed criterion down a second write
// path to be refused again with a vaguer message.
func classifyWrite(err error) writeOutcome {
	switch {
	case err == nil:
		return writeSucceeded
	case progroups.ModernWriteRefused(err):
		return writeRefusedByJamfPro
	default:
		return writeFailed
	}
}

// classicCriteriaRefusalPhrase is how Jamf Pro's older group interface reports a
// criterion it will not store: 409 "Problem with criteria", reproduced on
// 11.32.0 against a misspelled criterion name and recorded in
// internal/common/progroups/fallback.go.
//
// It is matched as a phrase because that surface reports no error code at all.
// Its body is an HTML status page and the SDK lifts the message into a detail
// whose code is empty, so the code classifiers in internal/common/progroups
// cannot see this refusal and the wording is the only thing left to key on.
const classicCriteriaRefusalPhrase = "problem with criteria"

// classicRefusedCriteria reports whether a fallback write failed because Jamf
// Pro's older group interface refused the criteria as well.
//
// That surface still validates criterion NAMES, so a misspelling is refused on
// both paths, and that is the one failure whose diagnosis is the criteria. Every
// other way the second write can fail — a name already in use, a refused site, a
// gateway timeout — has nothing to do with them, and saying otherwise sends the
// operator to edit a criterion that was never the problem.
func classicRefusedCriteria(err error) bool {
	apiErr := jamfplatform.AsAPIError(err)
	if apiErr == nil || !apiErr.HasStatus(http.StatusConflict) {
		return false
	}
	for _, detail := range apiErr.Errors {
		if strings.Contains(strings.ToLower(detail.Description), classicCriteriaRefusalPhrase) {
			return true
		}
	}
	return false
}

// fallbackWriteDiagnostics turns a failed fallback write into diagnostics,
// blaming the criteria only where the older interface refused them too.
//
// Everything else goes through progroups.WriteDiagnostics, which names the
// duplicate name and the refused site Jamf Pro reports with a code. The criteria
// refusal that sent the write down this path travels with it as a warning: the
// request that failed is one nothing in the configuration asked for, so the
// error beside it would otherwise arrive with no account of why it was made.
func fallbackWriteDiagnostics(modernErr, classicErr error) diag.Diagnostics {
	if classicRefusedCriteria(classicErr) {
		return progroups.BothRefusedDiagnostics(groupLabel, modernErr, classicErr)
	}
	var diags diag.Diagnostics
	diags.AddWarning(
		"Jamf Pro would not accept this "+groupLabel+"'s criteria the usual way",
		"Jamf Pro refused one of this group's criteria, although it offers the same combination in the web interface, so the provider repeated the write through Jamf Pro's older group interface. That second write then failed for its own reason, reported alongside this notice, and the criteria are not it."+
			"\n\nWhat Jamf Pro said about the criteria: "+helpers.APIErrorDetail(modernErr),
	)
	diags.Append(progroups.WriteDiagnostics(classicErr, groupLabel, path.Root("site_id"), path.Empty())...)
	return diags
}

// ModifyPlan raises the membership impact alert, then suppresses a no-op diff
// where a directory-service group criterion's planned value is a different
// representation of the group already in state (a group name swapped for the
// stored reference value, or the reverse).
//
// The alert runs first and unconditionally, because it has to cover the create
// and destroy cases the suppression pass returns early on. Suppression itself is
// soft: any resolution failure leaves the diff intact.
func (r *SmartComputerGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportMembershipImpact(ctx, req, resp)

	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() || r.ldap == nil {
		return
	}
	var plan, state SmartComputerGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || len(plan.Criteria) == 0 {
		return
	}

	suppressed := criteria.SuppressEquivalentDSGroupValues(ctx, r.ldap, plan.Criteria, state.Criteria)
	changed := false
	for i := range suppressed {
		if !suppressed[i].Value.Equal(plan.Criteria[i].Value) {
			changed = true
			break
		}
	}
	if !changed {
		return
	}
	plan.Criteria = suppressed
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// Create creates a Jamf Pro smart computer group.
//
// The create asks for the platform identifier, so its response carries the
// group's Jamf Platform UUID in place of the Jamf Pro id. Every group route
// accepts that UUID as the path identifier, but this group's read carries no
// identifier at all, so the Jamf Pro id comes from the one bridging request in
// the family. A failure there is fatal rather than best-effort: that value
// becomes the resource id, and losing it leaves the group orphaned in Jamf Pro.
//
// A refusal Jamf Pro should not be issuing sends the create through
// createThroughOlderInterface instead; any other failure surfaces as before.
// Whichever path made the group, state comes from the read that follows, because
// both write responses are lossy.
//
// deferred carries the diagnostics that must not reach the operator until state
// is written. The fallback create can leave one thing undone — a description it
// could not write — and reporting that before the state is set would fail the
// apply with the group already made and nothing in Terraform naming it.
func (r *SmartComputerGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SmartComputerGroupResourceModel
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

	var deferred diag.Diagnostics

	created, err := r.client.CreateSmartComputerGroupV3(createCtx, buildSmartComputerGroupInput(plan), true)
	switch classifyWrite(err) {
	case writeSucceeded:
		if created == nil || created.ID == "" {
			resp.Diagnostics.AddError(
				"Jamf Pro created the "+groupLabel+" without reporting an identifier",
				"Jamf Pro reported the group as created but returned no identifier for it, so Terraform cannot record what it made. Look for the group in Jamf Pro and import it, or remove it, before applying again.",
			)
			return
		}
		plan.PlatformID = types.StringValue(created.ID)

		jamfProID, idErr := progroups.JamfProIDForPlatformID(createCtx, r.client, created.ID)
		if idErr != nil {
			resp.Diagnostics.AddError(
				"Created the "+groupLabel+" but could not read back its Jamf Pro identifier",
				"The group exists in Jamf Pro and the follow-up lookup for the identifier Terraform stores failed, so the group is not under management. Look for it in Jamf Pro and import it, or remove it, before applying again."+
					"\n\nJamf Pro's response: "+helpers.APIErrorDetail(idErr),
			)
			return
		}
		plan.ID = types.StringValue(jamfProID)
	case writeRefusedByJamfPro:
		if !r.createThroughOlderInterface(createCtx, &plan, err, &resp.Diagnostics, &deferred) {
			return
		}
	default:
		resp.Diagnostics.Append(progroups.WriteDiagnostics(err, groupLabel, path.Root("site_id"), path.Empty())...)
		return
	}

	got, err := r.client.GetSmartComputerGroupV3(createCtx, plan.ID.ValueString())
	if err != nil {
		plan.Criteria = criteria.RestoreAuthoredDSGroupCriteria(plan.Criteria, authored)
		recordCreatedGroup(ctx, plan, resp)
		resp.Diagnostics.AddError(
			"Created the "+groupLabel+" but could not read it back",
			"The group exists in Jamf Pro and Terraform has recorded it, so nothing is left behind to find and import by hand. The read that fills in the rest of the values failed, so state holds what was written rather than what Jamf Pro stored, and the next plan refreshes it."+
				"\n\nJamf Pro's response: "+helpers.APIErrorDetail(err),
		)
		resp.Diagnostics.Append(deferred...)
		return
	}
	assignSmartComputerGroupResourceModel(&plan, got)
	plan.Criteria = criteria.RestoreAuthoredDSGroupCriteria(plan.Criteria, authored)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smartComputerGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro smart computer group", map[string]any{
		"id":          plan.ID.ValueString(),
		"platform_id": plan.PlatformID.ValueString(),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	resp.Diagnostics.Append(deferred...)
}

// recordCreatedGroup writes a group the create has already made to state, for
// the one failure that happens after the write: the mandatory
// read-after-create.
//
// Returning an error and no state is what orphans a group — Terraform keeps no
// record of it, the next apply is refused for a duplicate name, and the group
// has to be found and imported by hand. By this point the write has succeeded
// and plan carries the identifiers it reported, so the group is recorded and the
// follow-up refresh supplies what the read would have. It is the same reason the
// fallback create's own diagnostics wait for the state to be set.
//
// plan reaches here as the caller's own value, so a directory-service group
// criterion must already be back in the form it was authored in: what is
// recorded has to agree with the configuration, on this path exactly as on the
// one where the read answered.
func recordCreatedGroup(ctx context.Context, plan SmartComputerGroupResourceModel, resp *resource.CreateResponse) {
	recorded := writtenSmartComputerGroupState(plan)
	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smartComputerGroupIdentityModel{ID: recorded.ID})...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &recorded)...)
}

// createThroughOlderInterface repeats a refused create through Jamf Pro's older
// group interface, and reports whether Create should carry on.
//
// The order is forced by what each request can supply. The older interface has
// no way to ask for the platform identifier, so a fallback create learns only
// the Jamf Pro id; the platform id has to be resolved from it, and the
// description write needs that platform id. Hence: write, Jamf Pro id, platform
// id, description.
//
// The platform-id resolution is best-effort, as everywhere else — a null
// platform_id costs nothing but the ability to name this group from a
// jamfplatform_device_* construct, and resolves on a later refresh. It stops
// being harmless only when there is also a description to write, and that
// failure goes to deferred so Create still records the group it made.
func (r *SmartComputerGroupResource) createThroughOlderInterface(ctx context.Context, plan *SmartComputerGroupResourceModel, modernErr error, diags, deferred *diag.Diagnostics) bool {
	created, err := r.classicClient.CreateComputerGroupByID(ctx, "0", buildClassicSmartComputerGroupInput(*plan))
	if err != nil {
		diags.Append(fallbackWriteDiagnostics(modernErr, err)...)
		return false
	}
	if created == nil || created.ID == nil {
		diags.AddError(
			"Jamf Pro created the "+groupLabel+" without reporting an identifier",
			"Jamf Pro reported the group as created but returned no identifier for it, so Terraform cannot record what it made. Look for the group in Jamf Pro and import it, or remove it, before applying again.",
		)
		return false
	}
	diags.Append(progroups.FallbackWarning(groupLabel, modernErr))
	plan.ID = types.StringValue(strconv.Itoa(*created.ID))

	byJamfProID, bridgeDiags := progroups.PlatformIDsByJamfProID(ctx, r.client, r.pd, deviceType, groupKind)
	diags.Append(bridgeDiags...)
	plan.PlatformID = progroups.PlatformIDValue(byJamfProID, plan.ID.ValueString())

	if descriptionForWrite(plan.Description) == "" {
		return true
	}
	if plan.PlatformID.IsNull() {
		deferred.AddError(
			"Created the "+groupLabel+" but could not save its description",
			"The group exists in Jamf Pro with its name, site and criteria, and Terraform has recorded it. Saving the description needs the identifier this group has across Jamf Platform, and that lookup did not answer, so the description is still unset. Apply again to finish it — nothing needs changing in the configuration.",
		)
		return true
	}
	if err := r.setDescription(ctx, plan.PlatformID.ValueString(), plan.Description); err != nil {
		deferred.AddError(
			"Created the "+groupLabel+" but could not save its description",
			"The group exists in Jamf Pro with its name, site and criteria, and Terraform has recorded it. The separate write that saves the description failed, so the description is still unset. Apply again to finish it — nothing needs changing in the configuration."+
				"\n\nJamf Pro's response: "+helpers.APIErrorDetail(err),
		)
		return true
	}
	return true
}

// setDescription writes a group's description on its own.
//
// Jamf Pro's older group interface carries no description field, so a fallback
// write cannot express one, and it preserves the description already stored —
// which is why this exists as a second request rather than as part of the group
// write. The body carries the description and nothing else; see
// buildDescriptionPatch for why criteria must never join it.
//
// It takes the platform identifier. A Jamf Pro identifier on this route answers
// 404.
func (r *SmartComputerGroupResource) setDescription(ctx context.Context, platformID string, planned types.String) error {
	return r.client.PatchGroupV2(ctx, platformID, buildDescriptionPatch(planned))
}

// Read refreshes state from Jamf Pro.
//
// The response carries no identifier, so the prior id is carried through
// unchanged. platform_id is resolved only when it is absent from state, which is
// the import case: on an ordinary refresh it is already there and a group's
// platform identifier does not change.
func (r *SmartComputerGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SmartComputerGroupResourceModel

	if req.State.Raw.IsNull() {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform asked for a refresh of this "+groupLabel+" with neither prior state nor identity data, so the provider cannot tell which group to read.",
			)
			return
		}
		var identity smartComputerGroupIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing group identifier",
				"The resource identity carried no 'id' attribute, so the provider cannot refresh the "+groupLabel+".",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(smartComputerGroupTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing identifier", "Cannot read a Jamf Pro "+groupLabel+" without an identifier.")
		return
	}

	got, err := r.client.GetSmartComputerGroupV3(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro smart computer group not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smartComputerGroupIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro "+groupLabel, helpers.APIErrorDetail(err))
		return
	}

	priorCriteria := state.Criteria
	assignSmartComputerGroupResourceModel(&state, got)
	state.Criteria = criteria.ReadbackDSGroupCriteria(readCtx, r.ldap, dsGroupObjectType, state.Criteria, priorCriteria)

	if state.PlatformID.IsNull() || state.PlatformID.IsUnknown() {
		byJamfProID, bridgeDiags := progroups.PlatformIDsByJamfProID(readCtx, r.client, r.pd, deviceType, groupKind)
		resp.Diagnostics.Append(bridgeDiags...)
		if found := progroups.PlatformIDValue(byJamfProID, state.ID.ValueString()); !found.IsNull() {
			state.PlatformID = found
		} else {
			state.PlatformID = types.StringNull()
		}
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smartComputerGroupIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update rewrites a Jamf Pro smart computer group.
//
// The update response is lossy, so state comes from the read that follows it
// rather than from the write's own body.
//
// A refusal Jamf Pro should not be issuing sends the write through
// updateThroughOlderInterface instead. The prior description is read out of
// state for that path alone, which needs to know whether the description
// changed: the older interface preserves the stored one, so an unchanged
// description costs no second request.
func (r *SmartComputerGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SmartComputerGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var priorDescription types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("description"), &priorDescription)...)
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
		resp.Diagnostics.AddError("Missing identifier", "Cannot update a Jamf Pro "+groupLabel+" without an identifier.")
		return
	}

	resolved, authored, dsDiags := criteria.ResolveDSGroupCriteria(updateCtx, r.ldap, dsGroupObjectType, plan.Criteria)
	resp.Diagnostics.Append(dsDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Criteria = resolved

	var deferred diag.Diagnostics

	_, err := r.client.UpdateSmartComputerGroupV3(updateCtx, plan.ID.ValueString(), buildSmartComputerGroupInput(plan))
	switch classifyWrite(err) {
	case writeSucceeded:
	case writeRefusedByJamfPro:
		if !r.updateThroughOlderInterface(updateCtx, &plan, priorDescription, err, &resp.Diagnostics, &deferred) {
			return
		}
	default:
		resp.Diagnostics.Append(progroups.WriteDiagnostics(err, groupLabel, path.Root("site_id"), path.Empty())...)
		return
	}

	got, err := r.client.GetSmartComputerGroupV3(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading the updated Jamf Pro "+groupLabel, helpers.APIErrorDetail(err))
		return
	}
	assignSmartComputerGroupResourceModel(&plan, got)
	plan.Criteria = criteria.RestoreAuthoredDSGroupCriteria(plan.Criteria, authored)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, smartComputerGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	resp.Diagnostics.Append(deferred...)
}

// updateThroughOlderInterface repeats a refused update through Jamf Pro's older
// group interface, and reports whether Update should carry on.
//
// Name, site and criteria go in ONE write, and the criteria are never split
// across two. Membership is recalculated synchronously with the write, so a
// group written in halves is immediately live and wrong in between, and
// everything scoped to it acts on that.
//
// The description follows as a separate merge patch, and only when it changed,
// because the older interface preserves the one already stored. So an ordinary
// criteria-only edit stays a single request. That patch needs the group's
// platform identifier, which state normally holds; a state that does not — an
// import whose bridge lookup had failed, or a fallback create whose own lookup
// did — resolves it here rather than abandoning the description, and keeps what
// it resolved on the model so state carries it too.
func (r *SmartComputerGroupResource) updateThroughOlderInterface(ctx context.Context, plan *SmartComputerGroupResourceModel, priorDescription types.String, modernErr error, diags, deferred *diag.Diagnostics) bool {
	if err := r.classicClient.UpdateComputerGroupByID(ctx, plan.ID.ValueString(), buildClassicSmartComputerGroupInput(*plan)); err != nil {
		diags.Append(fallbackWriteDiagnostics(modernErr, err)...)
		return false
	}
	diags.Append(progroups.FallbackWarning(groupLabel, modernErr))

	if !descriptionNeedsWrite(plan.Description, priorDescription) {
		return true
	}

	if plan.PlatformID.IsNull() || plan.PlatformID.IsUnknown() {
		byJamfProID, bridgeDiags := progroups.PlatformIDsByJamfProID(ctx, r.client, r.pd, deviceType, groupKind)
		diags.Append(bridgeDiags...)
		plan.PlatformID = progroups.PlatformIDValue(byJamfProID, plan.ID.ValueString())
	}
	if plan.PlatformID.IsNull() {
		deferred.AddError(
			"Updated the "+groupLabel+" but could not save its description",
			"The group's name, site and criteria are saved. Saving the description needs the identifier this group has across Jamf Platform, and that lookup did not answer, so the description still reads as it did before. Apply again to finish it — nothing needs changing in the configuration.",
		)
		return true
	}
	if err := r.setDescription(ctx, plan.PlatformID.ValueString(), plan.Description); err != nil {
		deferred.AddError(
			"Updated the "+groupLabel+" but could not save its description",
			"The group's name, site and criteria are saved. The separate write that saves the description failed, so the description still reads as it did before. Apply again to finish it — nothing needs changing in the configuration."+
				"\n\nJamf Pro's response: "+helpers.APIErrorDetail(err),
		)
		return true
	}
	return true
}

// Delete removes a Jamf Pro smart computer group.
//
// A group that is already gone counts as deleted. A group something is still
// scoped to is refused, and that refusal is permanent rather than a propagation
// delay, so it is reported and never re-issued.
func (r *SmartComputerGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SmartComputerGroupResourceModel
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
		resp.Diagnostics.AddError("Missing identifier", "Cannot delete a Jamf Pro "+groupLabel+" without an identifier.")
		return
	}

	if err := r.client.DeleteSmartComputerGroupV3(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro smart computer group already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.Append(progroups.DeleteDiagnostics(err, groupLabel)...)
	}
}
