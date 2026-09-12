// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateEbookByID   (POST   /ebooks/id/0)
//   proclassic.GetEbookByID      (GET    /ebooks/id/{id})
//   proclassic.UpdateEbookByID   (PUT    /ebooks/id/{id})
//   proclassic.DeleteEbookByID   (DELETE /ebooks/id/{id})
//   proclassic.ListEbooks        (data source / list resource)
//   proclassic.GetEbookByName    (data source name lookup)
//
// Status: current. Last reviewed 2026-06-03.
//
// Update semantics: the classic /ebooks PUT is a partial-merge, not a
// full-replace (wire-probed — omitting <general> fields and the entire <scope>
// block retained the prior values). The provider sends the full plan payload on
// every Update, so in-place edits to managed sections converge cleanly. Removing
// an entire optional block from config omits it from the payload, so the server
// retains the previously-stored block — a known ProClassic limitation. To clear
// a block, null its individual fields rather than deleting the block.
//
// Scope is the exception: within a sent <scope> the server replaces the whole
// subtree (wire-probed 2026-07-08 on /ebooks — any category element present,
// even an empty one, wipes every omitted category across targets/limitations/
// exclusions). Ebook's computer+mobile union is ONE subtree: a body carrying
// only computer categories also wipes the mobile categories (probed). Scope
// therefore uses per-category granular ownership: when the plan declares a
// scope block, Update GETs the live object first and overlays the declared
// categories onto the live scope (scope-only merge — no other section of the
// read is echoed back), emitting every merged category explicitly. Omitted
// categories stay owned by the admin UI; declared `[]` clears. See
// STYLE_GUIDE.md §Scope helper omission semantics.
//
// Scope classes are a second exception, on top of that one. Jamf Pro stores
// <classes> only while the STORED category is empty, so a scope write that
// changes anything else destroys an existing class list whatever the body
// carries (wire-probed 2026-09-12 on 11.31.1; see ebookScopeClassesCleared in
// input_builders.go for the probe and the rule). Update therefore delivers a
// scope carrying class members in two requests: the first applies the change
// with <classes> emptied, the second sets the classes against the now-empty
// category. Create needs neither — a new ebook's category starts empty, so the
// POST stores them first time. No other scope category on any classic resource
// behaves this way.
//
// Delete semantics: FIRE-AND-TRUST. The classic /ebooks DELETE is asynchronous
// behind a MISLEADING response — the server returns HTTP 400 with body
// <ebook><id>N</id></ebook> (no error envelope) even though it has ACCEPTED the
// delete and completes it server-side later. Two wire-probed rules (2026-06-04),
// both pointing the same way — leave it alone:
//
//   - Re-issuing the DELETE delays the async removal (single delete vs
//     repeated-every-15s held it present far longer).
//   - POLLING GET-by-id to confirm ALSO delays it: an ebook polled every 3s
//     stayed present past 3.5min and only cleared once polling STOPPED. (Unlike
//     /mobiledeviceapplications, which returns the same misleading 400 but
//     clears in ~2s and is not GET-sensitive.)
//
// So Delete issues the DELETE exactly once and does NOT GET to confirm. The
// accepted-but-misleading 4xx is treated as success-with-a-warning: the ebook is
// dropped from state and the server finishes shortly. A 5xx / transport error is
// a genuine failure and is surfaced. Because confirmation is impossible without
// interfering, the acceptance CheckDestroy is a documented no-op. pro/v1 ebooks
// has no delete path, so classic is the only door. The Jamf API team is aware of
// the misleading 400.

package ebook

import (
	"context"
	"fmt"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro ebook. Classic POSTs to id="0"; the server
// allocates the real integer ID and returns it in the response body.
func (r *EbookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan EbookResourceModel
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

	payload, buildDiags := buildEbookInput(createCtx, plan)
	resp.Diagnostics.Append(buildDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateEbookByID(createCtx, "0", payload)
	if helpers.IsDirectoryGroupMatchConflict(err) {
		// Bootstrap apply: the referenced directory is still coming up. Retry until
		// the scope group resolves (or a real wrong-name conflict persists).
		err = helpers.RetryOnDirectoryGroupMatchConflict(createCtx, func() error {
			var e error
			created, e = r.client.CreateEbookByID(createCtx, "0", payload)
			return e
		})
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro ebook", helpers.APIErrorDetail(err))
		return
	}
	id := extractEbookID(created)
	if id == "" {
		resp.Diagnostics.AddError(
			"Create response missing ebook ID",
			"Jamf Pro returned 201 Created with no ebook ID; cannot persist state.",
		)
		return
	}

	got, err := r.client.GetEbookByID(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro ebook", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignEbookResourceModel(createCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ebookIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro ebook", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest ebook representation.
func (r *EbookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EbookResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this ebook without existing state or identity data, so the provider cannot determine which ebook to read.",
			)
			return
		}
		var identity ebookIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing ebook ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the ebook.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(ebookTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro ebook without ID.")
		return
	}

	got, err := r.client.GetEbookByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro ebook not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ebookIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro ebook", helpers.APIErrorDetail(err))
		return
	}

	// firstHydration detects an unpopulated incoming model (see mac_app_store_app
	// / policy for the full rationale): general is schema-Required and always
	// populated in genuinely managed state, so state.General == nil can only
	// mean this Read call is doing first-time import hydration. Hydrate every
	// wire-present optional section in that case; subsequent Reads revert to
	// only refreshing sections the current state already tracks.
	firstHydration := state.General == nil
	resp.Diagnostics.Append(assignEbookResourceModel(readCtx, &state, got, firstHydration)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ebookIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro ebook. Classic UpdateEbookByID returns 201 with an
// empty body — we must GET to refresh state. A plan whose merged scope carries
// class members goes out as two requests, the first with the classes emptied;
// ebookScopeClassesCleared holds the wire rule and why that ordering is the
// safe one.
func (r *EbookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan EbookResourceModel
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

	// Granular scope ownership: a scope PUT replaces the whole subtree (both
	// halves of the computer+mobile union), so undeclared (null) categories
	// must be re-emitted from the live object to survive the write.
	// Read-merge-write, scope-only — the wire plan carries the merged scope
	// while `plan` (used for state) keeps only the declared categories. See
	// the header comment and STYLE_GUIDE.md §Scope helper.
	wirePlan := plan
	if plan.Scope != nil {
		current, err := r.client.GetEbookByID(updateCtx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading Jamf Pro ebook before update", helpers.APIErrorDetail(err))
			return
		}
		var serverScope *EbookScopeModel
		if current != nil && current.Scope != nil {
			serverScope = &EbookScopeModel{}
			flattenEbookScope(updateCtx, current.Scope, serverScope, true)
		}
		wirePlan.Scope = mergeEbookScope(plan.Scope, serverScope)
	}

	payload, buildDiags := buildEbookInput(updateCtx, wirePlan)
	resp.Diagnostics.Append(buildDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	put := func(body *proclassic.EbookPost) error {
		return helpers.RetryOnDirectoryGroupMatchConflict(updateCtx, func() error {
			return r.client.UpdateEbookByID(updateCtx, plan.ID.ValueString(), body)
		})
	}

	cleared := ebookScopeClassesCleared(payload)
	if cleared != nil {
		if err := put(cleared); err != nil {
			resp.Diagnostics.AddError(
				"Error updating Jamf Pro ebook, and its scope classes may now be empty",
				fmt.Sprintf("Jamf Pro stores an ebook's scope classes only while the stored list is empty, so the provider empties the list in one request and restores it in a second. The first request failed for ebook %s. An error return is not proof that nothing was applied, because a deadline that expires in flight leaves the write committed, so scope.targets.class_ids may now be empty in Jamf Pro. If your configuration declares class_ids, apply again to restore them. If it leaves the category unmanaged, check the ebook's scope in Jamf Pro before the next apply. (response: %s)", plan.ID.ValueString(), helpers.APIErrorDetail(err)),
			)
			return
		}
	}
	if err := put(payload); err != nil {
		if cleared != nil {
			resp.Diagnostics.AddError(
				"Ebook updated, but its scope classes are now empty in Jamf Pro",
				fmt.Sprintf("Jamf Pro stores an ebook's scope classes only while the stored list is empty, so the provider empties the list in one request and restores it in a second. The first request applied every other change to ebook %s. The second failed, so scope.targets.class_ids is now empty in Jamf Pro. Terraform state is unchanged: if your configuration declares class_ids, apply again to restore the classes; if it leaves the category unmanaged, Jamf Pro no longer holds the list it had and the provider cannot recover it. (restore response: %s)", plan.ID.ValueString(), helpers.APIErrorDetail(err)),
			)
			return
		}
		resp.Diagnostics.AddError("Error updating Jamf Pro ebook", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetEbookByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro ebook", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignEbookResourceModel(updateCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, ebookIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro ebook (fire-and-trust). See the package
// delete-semantics note in this file's header for why it never GETs to confirm.
func (r *EbookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EbookResourceModel
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

	id := state.ID.ValueString()
	if state.ID.IsNull() || id == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro ebook without ID.")
		return
	}

	// Fire-and-trust: issue the DELETE exactly once and do NOT GET to confirm
	// (polling delays the server-side async removal — wire-probed). Treat the
	// accepted-but-misleading 4xx as success-with-a-warning; surface a 5xx /
	// transport error as a genuine failure.
	delErr := r.client.DeleteEbookByID(deleteCtx, id)
	switch {
	case delErr == nil || helpers.IsNotFoundError(delErr):
		tflog.Trace(ctx, "Jamf Pro ebook deletion confirmed", map[string]any{"id": id})
	case isAcceptedAsyncDelete(delErr):
		resp.Diagnostics.AddWarning(
			"Jamf Pro ebook deletion is asynchronous and was not confirmed",
			fmt.Sprintf("The classic /ebooks DELETE for id %s returned an accepted-but-misleading client error. The ebook has been removed from Terraform state; Jamf Pro completes the deletion a short time later. Confirmation is intentionally not polled because reading the ebook back delays the removal. (delete response: %v)", id, delErr),
		)
	default:
		resp.Diagnostics.AddError("Error deleting Jamf Pro ebook", helpers.APIErrorDetail(delErr))
	}
}
