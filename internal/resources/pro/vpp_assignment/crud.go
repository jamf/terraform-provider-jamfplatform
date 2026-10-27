// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateVPPAssignmentByID  (POST id/0 → 201, id-only body — GET after)
//   proclassic.GetVPPAssignmentByID
//   proclassic.UpdateVPPAssignmentByID  (PUT → 201, NO body — GET after)
//   proclassic.DeleteVPPAssignmentByID  (→ 200)
//   proclassic.ListVPPAssignments       (data source / list resource / collision check in collision.go)
//
// Writes are a MERGE (omit=retain). General scalars (name, vpp_admin_account_id)
// are emitted as planned. Content collections are opt-out (null omits, empty
// clears, populated replaces).
//
// Scope is the exception to the merge: within a sent <scope> the server
// replaces the whole subtree (wire-probed 2026-07-08 on /vppassignments — any
// category element present, even empty, wipes every omitted category across
// targets/limitations/exclusions; a body without <scope> retains it). Scope
// therefore uses per-category granular ownership: Create builds from the
// declared plan only (undeclared categories are omitted from the body); when
// the plan declares a scope block, Update GETs the live object first and
// overlays the declared categories onto the server's current scope (scope-only
// merge — no other section of the read is echoed back), emitting every merged
// category explicitly. Omitted categories stay owned by the admin UI; declared
// `[]` clears. See STYLE_GUIDE.md §Scope helper omission semantics.
//
// Uniqueness: name, not enforced; duplicates stored; checked only with
// include_unenforced. Classic trims the name on save, so the strict comparison also
// ignores leading and trailing spaces, case and accents.
//
// Status: current. Last reviewed 2026-07-08.

package vpp_assignment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

func (r *VPPAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan VPPAssignmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, d := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultCreateTimeout, plan.Timeouts.Create)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	createCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	input, d := buildVPPAssignmentInput(createCtx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateVPPAssignmentByID(createCtx, "0", input)
	if helpers.IsDirectoryGroupMatchConflict(err) {
		// Bootstrap apply: the referenced directory is still coming up. Retry until
		// the scope group resolves (or a real wrong-name conflict persists).
		err = helpers.RetryOnDirectoryGroupMatchConflict(createCtx, func() error {
			var e error
			created, e = r.client.CreateVPPAssignmentByID(createCtx, "0", input)
			return e
		})
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro VPP assignment", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError("Create response missing ID", "Jamf Pro returned 201 Created with no VPP assignment ID.")
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	got, err := r.client.GetVPPAssignmentByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro VPP assignment", helpers.APIErrorDetail(err))
		return
	}
	assignVPPAssignmentResourceModel(createCtx, &plan, got, false)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, vppAssignmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Trace(ctx, "created Jamf Pro VPP assignment", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VPPAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state VPPAssignmentResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError("Missing resource identity", "Refresh requested without state or identity data.")
			return
		}
		var identity vppAssignmentIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError("Missing ID", "The resource identity did not include an 'id'.")
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(vppAssignmentTimeoutAttributeTypes)
		state.IosAppAdamIDs = types.SetNull(types.Int64Type)
		state.MacAppAdamIDs = types.SetNull(types.Int64Type)
		state.EbookAdamIDs = types.SetNull(types.Int64Type)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	readTimeout, d := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro VPP assignment without ID.")
		return
	}

	got, err := r.client.GetVPPAssignmentByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro VPP assignment not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, vppAssignmentIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro VPP assignment", helpers.APIErrorDetail(err))
		return
	}
	// firstHydration detects an unpopulated incoming model (see mac_app_store_app
	// / policy for the full rationale): name is schema-Required and always
	// populated in genuinely managed state, so state.Name.IsNull() can only
	// mean this Read call is doing first-time import hydration (true for both
	// the classic Get()-decoded sparse state and the identity-only branch's
	// manually-seeded zero-value state above — attr.ValueStateNull is the
	// zero value, so IsNull() is safe either way). Hydrate the wire-present
	// optional scope section in that case; subsequent Reads revert to only
	// refreshing sections the current state already tracks.
	firstHydration := state.Name.IsNull()
	assignVPPAssignmentResourceModel(readCtx, &state, got, firstHydration)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, vppAssignmentIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *VPPAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VPPAssignmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, d := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultUpdateTimeout, plan.Timeouts.Update)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	// Granular scope ownership: a scope PUT replaces the whole subtree, so
	// undeclared (null) categories must be re-emitted from the live object to
	// survive the write. Read-merge-write, scope-only — the wire plan carries
	// the merged scope while `plan` (used for state) keeps only the declared
	// categories. See the header comment and STYLE_GUIDE.md §Scope helper.
	wirePlan := plan
	if plan.Scope != nil {
		current, err := r.client.GetVPPAssignmentByID(updateCtx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading Jamf Pro VPP assignment before update", helpers.APIErrorDetail(err))
			return
		}
		var serverScope *scope.UserScopeModel
		if current != nil && current.Scope != nil {
			serverScope = &scope.UserScopeModel{}
			flattenScope(updateCtx, current.Scope, serverScope, true)
		}
		wirePlan.Scope = scope.MergeUserScope(plan.Scope, serverScope)
	}

	input, d := buildVPPAssignmentInput(updateCtx, wirePlan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	// PUT returns no body — must GET to refresh server-derived fields.
	if err := helpers.RetryOnDirectoryGroupMatchConflict(updateCtx, func() error {
		return r.client.UpdateVPPAssignmentByID(updateCtx, plan.ID.ValueString(), input)
	}); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro VPP assignment", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetVPPAssignmentByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro VPP assignment", helpers.APIErrorDetail(err))
		return
	}
	assignVPPAssignmentResourceModel(updateCtx, &plan, got, false)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, vppAssignmentIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VPPAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VPPAssignmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, d := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultDeleteTimeout, state.Timeouts.Delete)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro VPP assignment without ID.")
		return
	}

	if err := r.client.DeleteVPPAssignmentByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro VPP assignment already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro VPP assignment", helpers.APIErrorDetail(err))
	}
}
