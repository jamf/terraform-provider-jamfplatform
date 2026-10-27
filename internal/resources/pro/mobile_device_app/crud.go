// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateMobileDeviceApplicationByID  (POST /mobiledeviceapplications/id/0)
//   proclassic.GetMobileDeviceApplicationByID     (GET  /mobiledeviceapplications/id/{id})
//   proclassic.UpdateMobileDeviceApplicationByID  (PUT  /mobiledeviceapplications/id/{id})
//   proclassic.DeleteMobileDeviceApplicationByID  (DELETE)
//   proclassic.ListMobileDeviceApplications       (data source / list resource / collision check in collision.go)
//   proclassic.GetMobileDeviceApplicationByName   (data source name lookup)
//
// Uniqueness: name, not enforced; duplicates stored; checked only with
// include_unenforced. Classic trims the name on save, so the strict comparison also
// ignores leading and trailing spaces, case and accents.
//
// Status: current. Last reviewed 2026-05-31.
//
// Create semantics: the classic POST silently drops os_type (wire-probed — a
// POST never persists it, even when sent). Create therefore POSTs to allocate
// the id, then issues a follow-up PUT with the full payload to persist os_type
// (the server requires it on a PUT to an in-house app and stores+echoes it
// thereafter), then GETs to refresh state.
//
// Update semantics: the classic /mobiledeviceapplications PUT is a partial-merge,
// not a full-replace (wire-probed). The provider sends the full plan payload on
// every Update, including os_type (the server 409s on a PUT to an in-house app
// without it), so in-place edits to managed sections converge cleanly. Removing
// an entire optional block (scope / self_service / vpp / app_configuration) from
// config omits it from the payload, so the server retains the previously-stored
// block — a known limitation matching ProClassic precedent. To clear a block,
// null its individual fields rather than deleting the block.
//
// Scope is the exception: within a sent <scope> the server replaces the whole
// subtree (wire-probed 2026-07-08 — any category element present, even empty,
// wipes every omitted category across targets/limitations/exclusions). Scope
// therefore uses per-category granular ownership: when the plan declares a
// scope block, Update GETs the live object first and overlays the declared
// categories onto the server's current scope (scope-only merge — no other
// section of the read is echoed back), emitting every merged category
// explicitly. Omitted categories stay owned by the admin UI; declared `[]`
// clears. Create needs no merge (nothing to preserve on a brand-new object) —
// its POST→PUT sequence re-sends the same declared scope both times. See
// STYLE_GUIDE.md §Scope helper omission semantics.
//
// Delete semantics: the classic /mobiledeviceapplications DELETE returns a
// misleading HTTP 400 on an accepted removal (maintainer-confirmed server bug).
// The SDK no longer retries 4xx, so that 400 surfaces here. Unlike /ebooks
// (which shares the misleading 400 but is slow AND GET-sensitive), this endpoint
// clears promptly and is NOT GET-sensitive (wire-probed 2026-06-04: GET-by-id
// 404'd ~2s after DELETE, even while polling), so Delete confirms via a GET-by-id
// poll until not-found — see helpers.ConfirmAsyncDelete.

package mobile_device_app

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// Create creates a new Jamf Pro mobile device app. Classic POSTs to id="0"; the
// server allocates the real integer ID and returns it in the response body.
func (r *MobileAppResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan MobileAppResourceModel
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

	payload, buildDiags := buildMobileAppInput(createCtx, plan)
	resp.Diagnostics.Append(buildDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateMobileDeviceApplicationByID(createCtx, "0", payload)
	if helpers.IsDirectoryGroupMatchConflict(err) {
		// Bootstrap apply: the referenced directory is still coming up. Retry the
		// POST (which carries the scope) until the group resolves. The follow-up PUT
		// below then re-sends the same scope to an already-validated app.
		err = helpers.RetryOnDirectoryGroupMatchConflict(createCtx, func() error {
			var e error
			created, e = r.client.CreateMobileDeviceApplicationByID(createCtx, "0", payload)
			return e
		})
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro mobile device app", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	id := extractMobileAppID(created)
	if id == "" {
		resp.Diagnostics.AddError(
			"Create response missing app ID",
			"Jamf Pro returned 201 Created with no app ID; cannot persist state.",
		)
		return
	}

	// The classic POST silently drops os_type (wire-probed: a POST never
	// persists it, so the freshly-created app has no os_type server-side). A
	// follow-up PUT to the new id persists it — the server requires os_type on a
	// PUT to an in-house app and stores+echoes it thereafter. The full plan
	// payload is re-sent (partial-merge, idempotent); without this the app would
	// be left without the os_type the config declares.
	if err := r.client.UpdateMobileDeviceApplicationByID(createCtx, id, payload); err != nil {
		// The POST already created the app; a failed follow-up PUT would orphan it
		// (Terraform persists no state on a failed Create, so a retry would create
		// a duplicate). Best-effort rollback to keep Create all-or-nothing.
		if delErr := r.client.DeleteMobileDeviceApplicationByID(createCtx, id); delErr != nil {
			tflog.Warn(ctx, "failed to roll back partially-created Jamf Pro mobile device app after a failed finalize PUT; it may be orphaned in the tenant", map[string]any{"id": id, "delete_error": delErr.Error()})
		}
		resp.Diagnostics.AddError("Error finalizing created Jamf Pro mobile device app", fmt.Sprintf("the app was created (id %s) but persisting os_type via the follow-up update failed; rolled back the partial create: %s", id, helpers.APIErrorDetail(err)))
		return
	}

	got, err := r.client.GetMobileDeviceApplicationByID(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro mobile device app", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignMobileAppResourceModel(createCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileAppIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro mobile device app", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest app representation.
func (r *MobileAppResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state MobileAppResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this app without existing state or identity data, so the provider cannot determine which app to read.",
			)
			return
		}
		var identity mobileAppIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing app ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the app.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(mobileAppTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro mobile device app without ID.")
		return
	}

	got, err := r.client.GetMobileDeviceApplicationByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro mobile device app not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileAppIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro mobile device app", helpers.APIErrorDetail(err))
		return
	}

	// firstHydration detects an unpopulated incoming model (see mac_app_store_app
	// / policy for the full rationale): general is schema-Required and always
	// populated in genuinely managed state, so state.General == nil can only
	// mean this Read call is doing first-time import hydration. Hydrate every
	// wire-present optional section in that case; subsequent Reads revert to
	// only refreshing sections the current state already tracks.
	firstHydration := state.General == nil
	resp.Diagnostics.Append(assignMobileAppResourceModel(readCtx, &state, got, firstHydration)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileAppIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro mobile device app. Classic
// UpdateMobileDeviceApplicationByID returns 201 with an empty body — we must GET
// to refresh state.
func (r *MobileAppResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MobileAppResourceModel
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

	// Granular scope ownership: a scope PUT replaces the whole subtree, so
	// undeclared (null) categories must be re-emitted from the live object to
	// survive the write. Read-merge-write, scope-only — the wire plan carries
	// the merged scope while `plan` (used for state) keeps only the declared
	// categories. See the header comment and STYLE_GUIDE.md §Scope helper.
	wirePlan := plan
	if plan.Scope != nil {
		current, err := r.client.GetMobileDeviceApplicationByID(updateCtx, plan.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading Jamf Pro mobile device app before update", helpers.APIErrorDetail(err))
			return
		}
		var serverScope *scope.MobileScopeModelNoIbeacons
		if current != nil && current.Scope != nil {
			serverScope = &scope.MobileScopeModelNoIbeacons{}
			flattenMobileAppScope(updateCtx, current.Scope, serverScope, true)
		}
		wirePlan.Scope = scope.MergeMobileScopeNoIbeacons(plan.Scope, serverScope)
	}

	payload, buildDiags := buildMobileAppInput(updateCtx, wirePlan)
	resp.Diagnostics.Append(buildDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := helpers.RetryOnDirectoryGroupMatchConflict(updateCtx, func() error {
		return r.client.UpdateMobileDeviceApplicationByID(updateCtx, plan.ID.ValueString(), payload)
	}); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro mobile device app", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetMobileDeviceApplicationByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro mobile device app", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignMobileAppResourceModel(updateCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, mobileAppIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro mobile device app. The classic DELETE returns a
// misleading HTTP 400 on an accepted, asynchronous removal (server bug,
// maintainer-confirmed). The SDK no longer retries 4xx, so that 400 surfaces
// here; ConfirmAsyncDelete issues the DELETE once then polls GET-by-id until
// not-found, erroring only if the app is still present when the timeout elapses.
// A clean delete or already-absent app short-circuits to success.
func (r *MobileAppResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MobileAppResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro mobile device app without ID.")
		return
	}

	id := state.ID.ValueString()
	if err := helpers.ConfirmAsyncDelete(deleteCtx, deletePollInterval,
		func(c context.Context) error { return r.client.DeleteMobileDeviceApplicationByID(c, id) },
		func(c context.Context) error {
			_, getErr := r.client.GetMobileDeviceApplicationByID(c, id)
			return getErr
		},
	); err != nil {
		resp.Diagnostics.AddError("Error deleting Jamf Pro mobile device app", helpers.APIErrorDetail(err))
		return
	}
	tflog.Trace(ctx, "Jamf Pro mobile device app deletion confirmed", map[string]any{"id": id})
}
