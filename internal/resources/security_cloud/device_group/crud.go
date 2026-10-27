// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   securitycloud.CreateDeviceGroupV1
//   securitycloud.GetDeviceGroupV1
//   securitycloud.UpdateDeviceGroupV2
//   securitycloud.DeleteDeviceGroupV1
//   securitycloud.ListDeviceGroupsV2 (data sources, list resource, the
//                                     singular data source's name lookup, and
//                                     the collision check in collision.go)
//
// Uniqueness: name, enforced (409 GROUP_ALREADY_EXISTS), case- and accent-sensitive, leading and trailing spaces trimmed.
//
// Deliberately not used:
//   securitycloud.ResolveDeviceGroupV2ByName
//                                      cannot express the built-in group. Where
//                                      the match is the implicit "Default Group",
//                                      which the list returns with no id key, the
//                                      resolver fails with "matched element has no
//                                      id field" and discards the matched element,
//                                      so the singular data source could never
//                                      reach its own id-less refusal. The name
//                                      lookup matches over ListDeviceGroupsV2
//                                      locally instead — see groupsNamedExactly.
//   securitycloud.ApplyDeviceGroupV*   generated name-keyed upsert; Terraform owns
//                                      the create-versus-update decision.
//
// Status: current. Last reviewed 2026-09-04.

package device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Security Cloud device group.
//
// The create response already carries the stored representation — id and name —
// but the group is read back anyway. The name is the one field the server may
// return differently from what was sent, and although the plan-time validator
// rejects the whitespace that would cause it, reading back keeps the resource
// honest if the server ever normalises something else.
//
// The empty-id guard before that read is not defensive noise: an empty id builds
// the collection path, and this API answers GET /v1/groups/ with the whole group
// list rather than a 404 (wire-probed 2026-08-29), so an unguarded read-back would
// surface as a decode failure rather than as the missing identifier it is.
//
// A read-back that fails after the create has succeeded still commits state. The group
// exists on the tenant by then, and returning without state would orphan it: group
// names are unique per tenant, so the retry is refused as a name already in use —
// naming the group the operator does not know they created. What is committed is the
// plan carrying the new ID, which the next refresh reconciles; an errored apply does
// not run Terraform's plan-consistency check. Nothing needs nulling first: the ID is
// this schema's only Computed value.
func (r *DeviceGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan DeviceGroupResourceModel
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

	created, err := r.client.CreateDeviceGroupV1(createCtx, buildGroupCreateInput(plan))
	if err != nil {
		if !appendWriteDiagnosticsHint(&resp.Diagnostics, err, r.collisions.CreateConflictHint(err, resourceTypeName)) {
			resp.Diagnostics.AddError("Error creating Jamf Security Cloud device group", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		}
		return
	}

	if created.ID == "" {
		resp.Diagnostics.AddError(
			"Jamf Security Cloud returned no device group ID",
			"The create call succeeded but the response carried no identifier, so the group cannot be read back "+
				"or tracked in state. The group may still have been created — check the Jamf Security Cloud admin "+
				"UI before retrying, and remove it if it is there.",
		)
		return
	}

	plan.ID = types.StringValue(created.ID)

	got, err := r.client.GetDeviceGroupV1(createCtx, created.ID)
	if err != nil {
		resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, deviceGroupIdentityModel{ID: plan.ID})...)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		resp.Diagnostics.AddError(
			"Error reading created Jamf Security Cloud device group",
			"The group was created with ID \""+created.ID+"\" but could not be read back, so Terraform has "+
				"recorded its ID and the configured name without confirming what was stored. The next plan will "+
				"refresh it — do not re-create it: group names are unique per tenant, so a second create would be "+
				"refused as a name already in use. Underlying error: "+helpers.APIErrorDetail(err),
		)
		return
	}
	assignDeviceGroupResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, deviceGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Security Cloud device group", map[string]any{"id": created.ID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest device group representation.
func (r *DeviceGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state DeviceGroupResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this device group without existing state or identity data, so the provider cannot determine which group to read.",
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Security Cloud device group without ID.")
		return
	}

	got, err := r.client.GetDeviceGroupV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud device group not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, deviceGroupIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Security Cloud device group", helpers.APIErrorDetail(err))
		return
	}

	assignDeviceGroupResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, deviceGroupIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update renames a Jamf Security Cloud device group.
//
// A rename is the only update this resource can make, and state comes from a
// fresh read afterwards for the same reason Create does it.
//
// The write goes to PUT /securitycloud/v2/groups/{id}, which answers 204 with no
// body, so that read-back is the only source of the stored name — unlike the v1
// PUT this used to call, which echoed the stored object. That route was
// unrouted when this resource shipped — 403 BAD_PERMISSIONS through both curl and
// the SDK, indistinguishable from a bogus path — and Update called the deprecated
// v1 PUT under a staticcheck suppression while the defect was open. It was fixed on
// 2026-09-04 and SDK v0.22.0 withdrew both v1 write paths with the spec, so the
// suppression and the fallback are gone. POST and GET/DELETE by id remain at v1;
// only the list and the update moved.
//
// That history is also why the read-back is compared rather than merely assigned. A
// 204 says the gateway accepted the request, not that the handler applied it, and
// this exact route has already been both: it answered 403 on 2026-08-29, the refusal
// cleared on 2026-09-03 when the authorization policy deployed, and the handler
// behind it then 404'd until it was fixed on 2026-09-04. A handler that takes a write
// and discards it would be worse than one that refuses it, because Terraform would
// report a converged rename over a group still holding its old name — so a served
// name that differs from the planned one is an error naming the route, not a value to
// commit. Assigning it instead would overwrite the plan and hide the whole failure.
//
// A read-back that fails errors without writing state, which is the opposite of what
// Create does and for the opposite reason. The rename has already landed on the
// tenant, and the group is already tracked, so leaving state on the previous name
// costs one refresh to reconcile; there is nothing to orphan and no unique name to
// collide with on a retry. The diagnostic has to say so, or an operator reading "could
// not be read" will assume the rename did not happen.
//
// Both calls share the one updateCtx built from a single timeout, so a PUT that spends
// most of the 60s default leaves the GET whatever remains and the read-back is what
// fails. Deliberate: the timeout is the operator's budget for the update as a whole,
// and a per-call split would let a configured 60s take 120s. The failure mode is the
// one described above — state one refresh behind, never a silent divergence.
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

	if plan.ID.IsNull() || plan.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot update Jamf Security Cloud device group without ID.")
		return
	}

	if err := r.client.UpdateDeviceGroupV2(updateCtx, plan.ID.ValueString(), buildGroupUpdateInput(plan)); err != nil {
		if !appendWriteDiagnostics(&resp.Diagnostics, err) {
			resp.Diagnostics.AddError("Error updating Jamf Security Cloud device group", helpers.APIErrorDetail(err))
		}
		return
	}

	got, err := r.client.GetDeviceGroupV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error confirming the renamed Jamf Security Cloud device group",
			"Jamf Security Cloud accepted the rename of group \""+plan.ID.ValueString()+"\", so the group "+
				"already carries the new name on the tenant. The provider could not read it back to confirm "+
				"what was stored, so Terraform's state still holds the previous name. Run \"terraform plan\" "+
				"again to reconcile it, and do not rename the group back. Underlying error: "+
				helpers.APIErrorDetail(err),
		)
		return
	}

	if got.Name != plan.Name.ValueString() {
		resp.Diagnostics.AddError(
			"Jamf Security Cloud accepted the rename without applying it",
			"PUT /securitycloud/v2/groups/"+plan.ID.ValueString()+" reported success, but reading the group "+
				"back returned the name \""+got.Name+"\" rather than the configured \""+plan.Name.ValueString()+
				"\". Jamf Security Cloud accepted the write and dropped it. Terraform has left state on the "+
				"previous name rather than recording a rename that did not happen. Retry the apply. If it "+
				"persists, report it to Jamf quoting that route, because no change to the configuration can "+
				"work around a write the service accepts and drops.",
		)
		return
	}
	assignDeviceGroupResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, deviceGroupIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Security Cloud device group.
//
// The endpoint is idempotent: it answers 204 for a group that was already gone
// and for an identifier that never existed at all, so the not-found branch every
// other resource here needs is unreachable. It is kept anyway, because an
// endpoint that starts returning 404 should not turn a converged destroy into an
// error.
//
// The delete is not refused when something still references the group. See the
// package doc comment — the referrer is silently left assigned to nothing, which
// is why the resource description warns about it rather than the diagnostics.
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Security Cloud device group without ID.")
		return
	}

	if err := r.client.DeleteDeviceGroupV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Security Cloud device group already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Security Cloud device group", helpers.APIErrorDetail(err))
	}
}
