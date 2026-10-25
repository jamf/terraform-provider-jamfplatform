// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateLicensedSoftwareByID   (POST   /licensedsoftware/id/0)
//   proclassic.GetLicensedSoftwareByID      (GET    /licensedsoftware/id/{id})
//   proclassic.UpdateLicensedSoftwareByID   (PUT    /licensedsoftware/id/{id})
//   proclassic.DeleteLicensedSoftwareByID   (DELETE /licensedsoftware/id/{id})
//   proclassic.ListLicensedSoftware         (data source / list resource / collision check in collision.go)
//   proclassic.GetLicensedSoftwareByName    (data source name lookup)
//
// Uniqueness: name, enforced (409), case-, leading- and trailing-space-insensitive, accent-sensitive.
//
// Status: current. Last reviewed 2026-06-05.
//
// Server invariants (wire-probed against records 65/66/67/68):
//   - Create POSTs to id="0"; the server allocates the integer ID and returns
//     it at the top level (<licensed_software><id>).
//   - PUT returns 201 Created with an empty body — Update must GET to refresh
//     state.
//   - Licenses and software definitions carry NO server-readable id; GET-by-id
//     returns idless elements and preserves send-order, so both lists reconcile
//     positionally.
//   - The legacy font_definitions / plugin_definitions buckets are silently
//     dropped on write (never echoed back) — not modeled.
//   - The classic PUT is a partial-merge at sub-block granularity: omitting a
//     sub-block retains the server's copy; sending an EMPTY element clears it.
//     buildLicensedSoftwareInput turns that into three-way opt-out semantics
//     for software_definitions and licenses: it emits a wrapper only when the
//     plan declares that list, so an omitted list stays unmanaged, a declared
//     [] lands as an empty element and clears, and a declared list replaces
//     what is stored.
//   - The server pads unset optional strings with "" and unset numbers with 0,
//     and echoes a default <purchasing> block on every licence; state_builders
//     normalises "" / 0-sentinel back to null and suppresses unmanaged
//     purchasing echoes.
//   - DELETE returns 200; a repeat DELETE / GET of a removed record returns 404,
//     so Read and Delete self-heal via helpers.IsNotFoundError.

package licensed_software

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro licensed software record. Classic POSTs to
// id="0"; the server allocates the real integer ID and returns it.
func (r *LicensedSoftwareResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan LicensedSoftwareResourceModel
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

	payload, buildDiags := buildLicensedSoftwareInput(plan)
	resp.Diagnostics.Append(buildDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateLicensedSoftwareByID(createCtx, "0", payload)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro licensed software", helpers.APIErrorDetail(err))
		return
	}
	id := extractLicensedSoftwareID(created)
	if id == "" {
		resp.Diagnostics.AddError(
			"Create response missing record ID",
			"Jamf Pro returned 201 Created with no licensed software ID; cannot persist state.",
		)
		return
	}

	got, err := r.client.GetLicensedSoftwareByID(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro licensed software", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignLicensedSoftwareResourceModel(createCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, licensedSoftwareIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro licensed software", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest record representation.
func (r *LicensedSoftwareResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state LicensedSoftwareResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this licensed software record without existing state or identity data, so the provider cannot determine which record to read.",
			)
			return
		}
		var identity licensedSoftwareIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing record ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the licensed software record.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(licensedSoftwareTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro licensed software without ID.")
		return
	}

	got, err := r.client.GetLicensedSoftwareByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro licensed software not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, licensedSoftwareIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro licensed software", helpers.APIErrorDetail(err))
		return
	}

	// firstHydration detects an unpopulated incoming model (see mac_app_store_app
	// / policy for the full rationale): name is schema-Required (no nested
	// general block on this resource) and always populated in genuinely
	// managed state, so state.Name.IsNull() can only mean this Read call is
	// doing first-time import hydration. Hydrate the wire-present optional
	// software_definitions/licenses lists in that case; subsequent Reads
	// revert to only refreshing lists the current state already tracks.
	firstHydration := state.Name.IsNull()
	resp.Diagnostics.Append(assignLicensedSoftwareResourceModel(readCtx, &state, got, firstHydration)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, licensedSoftwareIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro licensed software record. Classic
// UpdateLicensedSoftwareByID returns 201 with an empty body — we must GET to
// refresh state.
func (r *LicensedSoftwareResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan LicensedSoftwareResourceModel
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

	payload, buildDiags := buildLicensedSoftwareInput(plan)
	resp.Diagnostics.Append(buildDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateLicensedSoftwareByID(updateCtx, plan.ID.ValueString(), payload); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro licensed software", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetLicensedSoftwareByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro licensed software", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignLicensedSoftwareResourceModel(updateCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, licensedSoftwareIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro licensed software record.
func (r *LicensedSoftwareResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state LicensedSoftwareResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro licensed software without ID.")
		return
	}

	if err := r.client.DeleteLicensedSoftwareByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro licensed software already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro licensed software", helpers.APIErrorDetail(err))
	}
}
