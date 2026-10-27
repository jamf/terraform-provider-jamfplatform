// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.GetPolicyByID
//   proclassic.GetPolicyByName        (data source name lookup)
//   proclassic.CreatePolicyByID
//   proclassic.UpdatePolicyByID
//   proclassic.DeletePolicyByID
//   proclassic.ListPolicies           (list resource, name collision check in collision.go)
//
// Uniqueness: general.name, enforced (409 Duplicate name), tenant-wide across sites, case-insensitive, surrounding spaces trimmed, accent-sensitive. Create and rename (PUT) both refuse.
//
// Status: current. Last reviewed 2026-10-25.
//
// Scope semantics: within a sent <scope> the server replaces the whole subtree
// (wire-probed 2026-07-08 — any category element present, even empty, wipes
// every omitted category across targets/limitations/exclusions). Scope
// therefore uses per-category granular ownership: when the plan declares a
// scope block, Update GETs the live object first and overlays the declared
// categories onto the server's current scope (scope-only merge — no other
// section of the read is echoed back), emitting every merged category
// explicitly. Omitted categories stay owned by the admin UI; declared `[]`
// clears. See STYLE_GUIDE.md §Scope helper omission semantics.

package policy

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// Create creates a new Jamf Pro classic policy. Classic POSTs to id="0"; the
// server allocates the real integer ID and returns it in the response body.
func (r *PolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan PolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg PolicyResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
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

	// Create has no prior state — every WriteOnly secret in cfg is fresh
	// and must reach the wire.
	input, inputDiags := buildPolicyInput(createCtx, plan, accountMaintenanceSecretsForCreate(&cfg))
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreatePolicyByID(createCtx, "0", input)
	if helpers.IsDirectoryGroupMatchConflict(err) {
		// Bootstrap apply: the referenced directory is still coming up. Retry until
		// the scope group resolves (or a real wrong-name conflict persists).
		err = helpers.RetryOnDirectoryGroupMatchConflict(createCtx, func() error {
			var e error
			created, e = r.client.CreatePolicyByID(createCtx, "0", input)
			return e
		})
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro policy", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	policyID := extractPolicyID(created)
	if policyID == "" {
		resp.Diagnostics.AddError(
			"Create response missing policy ID",
			"Jamf Pro returned 201 Created with no policy ID; cannot persist state.",
		)
		return
	}
	plan.ID = types.StringValue(policyID)

	got, err := r.client.GetPolicyByID(createCtx, policyID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro policy", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignPolicyResourceModel(createCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, policyIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro policy", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest policy representation.
func (r *PolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state PolicyResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this policy without existing state or identity data, so the provider cannot determine which policy to read.",
			)
			return
		}
		var identity policyIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing policy ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the policy.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(policyTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro policy without ID.")
		return
	}

	got, err := r.client.GetPolicyByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro policy not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, policyIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro policy", helpers.APIErrorDetail(err))
		return
	}

	// firstHydration detects an unpopulated incoming model: general is a
	// schema-Required block that a genuinely managed resource's state always
	// has populated (Create/Update always set it before any Read), so
	// state.General == nil can only mean this Read call is doing first-time
	// import hydration (classic `terraform import` leaves state sparse-but-
	// non-null via ImportStatePassthroughID, or identity-only import leaves it
	// fully null — either way General was never set). Hydrate every
	// wire-present optional section in that case so a freshly imported
	// resource matches a config that already declares scope/self_service/
	// packages. On every subsequent Read (state.General != nil), the gate
	// reverts to only refreshing sections the current state already tracks.
	firstHydration := state.General == nil
	resp.Diagnostics.Append(assignPolicyResourceModel(readCtx, &state, got, firstHydration)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, policyIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates an existing Jamf Pro classic policy. Classic UpdatePolicyByID
// returns 201 with an empty body — we must GET to refresh state.
func (r *PolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state PolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg PolicyResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
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

	// Granular scope ownership: a scope PUT replaces the whole subtree, so
	// undeclared (null) categories must be re-emitted from the live object to
	// survive the write. Read-merge-write, scope-only — the wire plan carries
	// the merged scope while `plan` (used for state) keeps only the declared
	// categories. See the header comment and STYLE_GUIDE.md §Scope helper.
	wirePlan := plan
	if plan.Scope != nil {
		current, err := r.client.GetPolicyByID(updateCtx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading Jamf Pro policy before update", helpers.APIErrorDetail(err))
			return
		}
		var serverScope *scope.ComputerScopeModel
		if current != nil && current.Scope != nil {
			serverScope = &scope.ComputerScopeModel{}
			resp.Diagnostics.Append(flattenPolicyScope(updateCtx, current.Scope, serverScope, true)...)
			if resp.Diagnostics.HasError() {
				return
			}
		}
		wirePlan.Scope = scope.MergeComputerScope(plan.Scope, serverScope)
	}

	input, inputDiags := buildPolicyInput(updateCtx, wirePlan, accountMaintenanceSecretsForUpdate(&plan, &state, &cfg))
	resp.Diagnostics.Append(inputDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	omitUnchangedDeferralDate(input, &plan, &state)

	if err := helpers.RetryOnDirectoryGroupMatchConflict(updateCtx, func() error {
		return r.client.UpdatePolicyByID(updateCtx, plan.ID.ValueString(), input)
	}); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro policy", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetPolicyByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro policy", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignPolicyResourceModel(updateCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, policyIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro classic policy.
func (r *PolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PolicyResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro policy without ID.")
		return
	}

	if err := r.client.DeletePolicyByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro policy already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro policy", helpers.APIErrorDetail(err))
	}
}
