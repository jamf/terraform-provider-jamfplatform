// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreatePatchPolicyBySoftwareTitleConfigID (POST /patchpolicies/softwaretitleconfig/id/{configID})
//   proclassic.GetPatchPolicyByID                       (GET    /patchpolicies/id/{id})
//   proclassic.UpdatePatchPolicyByID                    (PUT    /patchpolicies/id/{id}, 201 empty body → GET after)
//   proclassic.DeletePatchPolicyByID                    (DELETE /patchpolicies/id/{id})
//   pro.ListPatchPoliciesV2                             (GET    /v2/patch-policies — list resource enumeration, collision check in collision.go)
//
// ENUMERATION: the list resource enumerates on the Pro v2 collection the
// deprecation notice named, not on ProClassic /patchpolicies. Only enumeration
// moved: the by-ID CRUD funcs (Create-by-config-path, Get/Update/Delete by ID)
// are the current surface and carry no deprecation, and they remain the only
// place a policy's scope and user_interaction sections can be read.
//
// The reason for the move has changed and the conclusion has not. Those classic
// collection reads had been withdrawn from the published Classic spec, so the
// SDK stopped generating them; public-apis-oas#438 restored the whole
// patch-management family at GitOps v2082 ("patch management is where Classic
// API callers are most concentrated") and SDK v0.22.0 generates
// ListPatchPolicies and ListPatchPoliciesBySoftwareTitleConfigID again. The
// enumeration stays on Pro v2 anyway: restoring an operation is a caller-window
// concession rather than an un-deprecation, the v2 collection is what the notice
// nominates, and it is the surface that accepts a server-side RSQL query the
// list resource's filter needs. The restoration also arrived with
// ListPatchPoliciesBySoftwareTitleConfigID's return type corrected from
// *PatchPolicy to *PatchPolicies — it had been decoding into a zero-valued
// struct on every successful call since it first shipped, because the endpoint
// answers <patch_policies> against a spec declaring the singular — which is one
// more reason not to move back onto it without a wire probe of its own.
//
// Uniqueness: name per software title configuration (the same name on two titles is accepted), enforced (409), case-, surrounding-space- and accent-insensitive.
//
// Status: current. Last reviewed 2026-09-04.
//
// Server invariants (wire-probed):
//   - Create POSTs the policy <general> to the software-title-config path; the
//     server allocates the integer policy ID and returns it at the top level.
//     It DERIVES release_date, incremental_update, reboot, minimum_os, and
//     kill_apps from the target_version's patch definition (deliberately-wrong
//     submitted values are overwritten) — these are modelled Computed-only.
//   - The server fills full user_interaction defaults on create (install button
//     text "Update", deadline 7d, grace 15m, reminders 24h, …).
//   - distribution_method coerces invalid values to "prompt" server-side; the
//     provider validates to {selfservice, prompt} to surface the choice.
//   - target_version must be a version that has a package assigned on the title.
//   - Scope round-trips by id (entity scope only); the empty <users/> element the
//     wire emits is vestigial and is not modelled (write-only-unreadable).
//   - Scope subtree replace (partially wire-probed): a 2026-06-01 probe showed
//     that when <scope> is sent, the server CLEARS every category omitted from
//     it (buildings/departments/computers/limitations/exclusions all came back
//     empty after a PUT that sent only <computer_groups>) — consistent with the
//     family-wide subtree-replace law probed 2026-07-08 on 8 sibling classic
//     endpoints. NOTE: /patchpolicies was NOT among those 8 — the full law
//     (empty <scope></scope> ignored, lone all-flag at its current value
//     ignored, all-flag=true wipes conflicting targets but coexists with
//     limitations/exclusions) is ASSUMED from the family here and must be
//     wire-probed on /patchpolicies before release.
//   - Because of the subtree replace, scope uses per-category granular
//     ownership: when the plan declares a scope block, Update GETs the live
//     object first and overlays the declared categories onto the live scope
//     (scope-only merge — no other section of the read is echoed back),
//     emitting every merged category explicitly. Omitted categories stay owned
//     by the admin UI; declared `[]` clears. See STYLE_GUIDE.md §Scope helper
//     omission semantics.
//   - GET DOES echo <software_title_configuration_id>, on both the POST->GET and
//     the PUT->GET path (re-probed against Jamf Pro 11.31.1 on 2026-09-06,
//     correcting an earlier recorded finding that it did not). It is a
//     create-time-only path parameter but a readable field, so Read adopts the
//     wire value and drift on it is reported. It stays in the acceptance
//     ImportStateVerifyIgnore list only because removing it is untested here.
//   - PUT returns 201 with an empty body — Update must GET to refresh state.
//   - DELETE returns 200; a repeat DELETE / GET of a removed policy returns 404,
//     so Read and Delete self-heal via helpers.IsNotFoundError.
//
// Update semantics: like all classic endpoints the PUT is a partial-merge at
// top-section granularity. The provider always sends the full writable plan
// payload, so in-place edits to general/user_interaction converge. Removing
// the entire optional scope / user_interaction block from config omits it from
// the payload, so the server retains the previously-stored values — a known
// ProClassic limitation; for user_interaction, null the individual fields
// rather than deleting the block to clear them. Scope follows the granular
// per-category ownership contract described above instead.

package patch_policy

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro patch policy against the supplied patch software
// title configuration. The server allocates the integer policy ID and returns
// it; we then GET by that ID to capture the server-derived general fields.
func (r *PatchPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PatchPolicyResourceModel
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

	payload, buildDiags := buildPatchPolicyInput(createCtx, plan)
	resp.Diagnostics.Append(buildDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	configID := plan.SoftwareTitleConfigurationID.ValueString()
	created, err := r.client.CreatePatchPolicyBySoftwareTitleConfigID(createCtx, configID, payload)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro patch policy", helpers.APIErrorDetail(err))
		return
	}
	id := extractPatchPolicyID(created)
	if id == "" {
		resp.Diagnostics.AddError(
			"Create response missing patch policy ID",
			"Jamf Pro returned 201 Created with no patch policy ID; cannot persist state.",
		)
		return
	}

	got, err := r.client.GetPatchPolicyByID(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro patch policy", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignPatchPolicyResourceModel(createCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, patchPolicyIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro patch policy", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest patch policy representation.
func (r *PatchPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PatchPolicyResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this patch policy without existing state or identity data, so the provider cannot determine which policy to read.",
			)
			return
		}
		var identity patchPolicyIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing patch policy ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the patch policy.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(patchPolicyTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro patch policy without ID.")
		return
	}

	got, err := r.client.GetPatchPolicyByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro patch policy not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, patchPolicyIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro patch policy", helpers.APIErrorDetail(err))
		return
	}

	// firstHydration detects an unpopulated incoming model (see mac_app_store_app
	// / policy for the full rationale): name is schema-Required (no nested
	// general block on this resource) and always populated in genuinely
	// managed state, so state.Name.IsNull() can only mean this Read call is
	// doing first-time import hydration. Hydrate the wire-present optional
	// scope/user_interaction sections in that case; subsequent Reads revert to
	// only refreshing sections the current state already tracks.
	firstHydration := state.Name.IsNull()
	resp.Diagnostics.Append(assignPatchPolicyResourceModel(readCtx, &state, got, firstHydration)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, patchPolicyIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro patch policy. Classic UpdatePatchPolicyByID returns
// 201 with an empty body — we must GET to refresh state.
func (r *PatchPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PatchPolicyResourceModel
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

	// Granular scope ownership: a scope PUT replaces the whole subtree
	// (family-assumed for /patchpolicies — see the header note), so undeclared
	// (null) categories must be re-emitted from the live object to survive the
	// write. Read-merge-write, scope-only — the wire plan carries the merged
	// scope while `plan` (used for state) keeps only the declared categories.
	// See the header comment and STYLE_GUIDE.md §Scope helper.
	wirePlan := plan
	if plan.Scope != nil {
		current, err := r.client.GetPatchPolicyByID(updateCtx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading Jamf Pro patch policy before update", helpers.APIErrorDetail(err))
			return
		}
		var serverScope *PatchPolicyScopeModel
		if current != nil && current.Scope != nil {
			serverScope = &PatchPolicyScopeModel{}
			flattenScope(updateCtx, current.Scope, serverScope, true)
		}
		wirePlan.Scope = mergePatchPolicyScope(plan.Scope, serverScope)
	}

	payload, buildDiags := buildPatchPolicyInput(updateCtx, wirePlan)
	resp.Diagnostics.Append(buildDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdatePatchPolicyByID(updateCtx, plan.ID.ValueString(), payload); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro patch policy", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetPatchPolicyByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro patch policy", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignPatchPolicyResourceModel(updateCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, patchPolicyIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro patch policy.
func (r *PatchPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PatchPolicyResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro patch policy without ID.")
		return
	}

	if err := r.client.DeletePatchPolicyByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro patch policy already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro patch policy", helpers.APIErrorDetail(err))
	}
}
