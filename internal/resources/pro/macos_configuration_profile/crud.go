// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.GetOSXConfigurationProfileByID
//   proclassic.GetOSXConfigurationProfileByName     (data source name lookup)
//   proclassic.CreateOSXConfigurationProfileByID
//   proclassic.UpdateOSXConfigurationProfileByID
//   proclassic.DeleteOSXConfigurationProfileByID
//   proclassic.ListOSXConfigurationProfiles         (list resource, collision check in collision.go)
//
// Uniqueness: name, enforced (409 Duplicate name), trimmed on save, case- and accent-insensitive; separate pool from mobile device configuration profiles.
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

package macos_configuration_profile

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/payloadhelpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// Create handles POST /api/proclassic/osxconfigurationprofiles/id/0. Classic
// allocates the ID and returns it in the response body's <id>. The provider
// then runs a GET to capture the server-canonical form (including the
// server-rewritten payload + minted UUIDs) into state. When that read-back
// shows the server stored the payload unfaithfully, the created profile is
// rolled back and the apply fails with a diagnostic naming each diverging
// value and the fix for its class of mangling (see
// payloadhelpers.PayloadFidelityErrorDetail) instead of Terraform core's
// inconsistent-result error.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", providerNotConfigured)
		return
	}
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, td := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultCreateTimeout, plan.Timeouts.Create)
	resp.Diagnostics.Append(td...)
	if resp.Diagnostics.HasError() {
		return
	}
	createCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	input, bd := buildInput(createCtx, plan, "")
	resp.Diagnostics.Append(bd...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Capture the user-authored payload before assignResourceModel
	// overwrites plan.General.Payloads with the server-canonical form.
	// ModifyPlan reads this from private state on the next plan to
	// distinguish "user changed HCL" from "Jamf-stripped key noise".
	var userAuthoredPayload string
	if plan.General != nil && !plan.General.Payloads.IsNull() && !plan.General.Payloads.IsUnknown() {
		userAuthoredPayload = plan.General.Payloads.ValueString()
	}

	created, err := r.client.CreateOSXConfigurationProfileByID(createCtx, "0", input)
	if helpers.IsDirectoryGroupMatchConflict(err) {
		// Bootstrap apply: the referenced directory is still coming up. Retry until
		// the scope group resolves (or a real wrong-name conflict persists).
		err = helpers.RetryOnDirectoryGroupMatchConflict(createCtx, func() error {
			var e error
			created, e = r.client.CreateOSXConfigurationProfileByID(createCtx, "0", input)
			return e
		})
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro macOS configuration profile", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}

	id := ""
	if created.ID != nil {
		id = helpers.StringValueFromIntPtr(created.ID).ValueString()
	} else if created.General != nil && created.General.ID != nil {
		id = helpers.StringValueFromIntPtr(created.General.ID).ValueString()
	}
	if id == "" {
		resp.Diagnostics.AddError("Create response missing profile ID", "Jamf Pro did not return an ID for the created macOS configuration profile.")
		return
	}

	got, err := r.client.GetOSXConfigurationProfileByID(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created macOS configuration profile", helpers.APIErrorDetail(err))
		return
	}
	// Capture the raw server-canonical payload bytes before
	// assignResourceModel's lenient self-healing mutates
	// plan.General.Payloads back to the user-authored form.
	var rawServerPayload []byte
	if got != nil && got.General != nil && got.General.Payloads != nil {
		rawServerPayload = []byte(string(*got.General.Payloads))
	}
	resp.Diagnostics.Append(assignResourceModel(createCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if userAuthoredPayload != "" && plan.General != nil && plan.General.Payloads.ValueString() != userAuthoredPayload {
		if delErr := r.client.DeleteOSXConfigurationProfileByID(createCtx, id); delErr != nil {
			tflog.Warn(createCtx, "rollback delete after payload verification failure", map[string]any{"id": id, "error": delErr.Error()})
		}
		resp.Diagnostics.AddError(
			payloadhelpers.PayloadFidelitySummary,
			payloadhelpers.PayloadFidelityErrorDetail([]byte(userAuthoredPayload), rawServerPayload, payloadhelpers.FidelityPhaseCreate),
		)
		return
	}
	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, identityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(writePrivatePayloadRefs(createCtx, resp.Private, userAuthoredPayload, rawServerPayload)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Trace(createCtx, "created jamfplatform_pro_macos_configuration_profile", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(createCtx, &plan)...)
}

func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", providerNotConfigured)
		return
	}
	var state ResourceModel
	isImport := req.State.Raw.IsNull()
	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh without existing state or identity data; provider cannot determine which profile to read.",
			)
			return
		}
		var identity identityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing profile ID",
				"Resource identity did not include an 'id' attribute; provider cannot refresh the profile.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(timeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	readTimeout, td := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(td...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	got, err := r.client.GetOSXConfigurationProfileByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, identityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(readCtx)
			return
		}
		resp.Diagnostics.AddError("Error reading macOS configuration profile", helpers.APIErrorDetail(err))
		return
	}
	var rawServerPayload []byte
	if got != nil && got.General != nil && got.General.Payloads != nil {
		rawServerPayload = []byte(string(*got.General.Payloads))
	}
	// firstHydration detects an unpopulated incoming model (see mac_app_store_app
	// / policy for the full rationale): general is schema-Required and always
	// populated in genuinely managed state, so state.General == nil can only
	// mean this Read call is doing first-time import hydration. Hydrate every
	// wire-present optional section in that case; subsequent Reads revert to
	// only refreshing sections the current state already tracks.
	firstHydration := state.General == nil
	// Import fidelity gate. Adopting a profile whose payload Jamf Pro cannot store
	// back unchanged is a trap: it imports and plans clean, then the first apply
	// that touches the resource rewrites the payload into a corrupted form the
	// Classic API will not accept the original back over, leaving the admin UI as
	// the only repair. Refuse at the doorway instead. No API calls and no writes —
	// the verdict comes from the payload text (payloadhelpers.PayloadImportRisk).
	//
	// Gated on firstHydration, not on req.State.Raw.IsNull(): only `terraform
	// import` arrives with null prior state, whereas a config-driven `import`
	// block runs ImportResourceState first and reaches Read with prior state
	// present. firstHydration covers both (general is schema-Required, so a nil
	// general can only mean first-time import hydration) and deliberately never
	// fires on ordinary refresh, which must keep working for an already-managed or
	// externally-corrupted profile so drift stays visible and it stays removable.
	if firstHydration {
		var gateName string
		if got != nil && got.General != nil && got.General.Name != nil {
			gateName = *got.General.Name
		}
		resp.Diagnostics.Append(payloadhelpers.ImportGateDiagnostics(
			rawServerPayload, payloadhelpers.PlatformMacOS, gateName, state.ID.ValueString(),
		)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	resp.Diagnostics.Append(assignResourceModel(readCtx, &state, got, firstHydration)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Post-self-heal drift detector. assignResourceModel's lenient
	// compare keeps state.Payloads aligned to the user-authored form
	// when the server response is only Jamf normalisation; the drift
	// detector overwrites state.Payloads with the canonical server form
	// when the strict compare against last-applied canonical surfaces
	// an admin UI edit.
	resp.Diagnostics.Append(reconcileReadDrift(readCtx, req.Private, &state, rawServerPayload)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Refresh the serverNow private reference so ModifyPlan's
	// three-way compare uses the actual current server canonical
	// rather than the self-healed state.Payloads bytes.
	resp.Diagnostics.Append(writePrivateServerNow(readCtx, resp.Private, rawServerPayload)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, identityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(readCtx, &state)...)
}

func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", providerNotConfigured)
		return
	}
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, td := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultUpdateTimeout, plan.Timeouts.Update)
	resp.Diagnostics.Append(td...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	var existingUUID string
	if state.General != nil && !state.General.UUID.IsNull() && !state.General.UUID.IsUnknown() {
		existingUUID = state.General.UUID.ValueString()
	}

	// Granular scope ownership: a scope PUT replaces the whole subtree, so
	// undeclared (null) categories must be re-emitted from the live object to
	// survive the write. Read-merge-write, scope-only — the wire plan carries
	// the merged scope while `plan` (used for state) keeps only the declared
	// categories. See the header comment and STYLE_GUIDE.md §Scope helper.
	wirePlan := plan
	if plan.Scope != nil {
		current, err := r.client.GetOSXConfigurationProfileByID(updateCtx, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading macOS configuration profile before update", helpers.APIErrorDetail(err))
			return
		}
		var serverScope *scope.ComputerScopeModel
		if current != nil && current.Scope != nil {
			serverScope = &scope.ComputerScopeModel{}
			resp.Diagnostics.Append(flattenScope(updateCtx, current.Scope, serverScope, true)...)
			if resp.Diagnostics.HasError() {
				return
			}
		}
		wirePlan.Scope = scope.MergeComputerScope(plan.Scope, serverScope)
	}

	input, bd := buildInput(updateCtx, wirePlan, existingUUID)
	resp.Diagnostics.Append(bd...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Capture the user-authored payload before assignResourceModel
	// overwrites plan.General.Payloads with the server-canonical form.
	var userAuthoredPayload string
	if plan.General != nil && !plan.General.Payloads.IsNull() && !plan.General.Payloads.IsUnknown() {
		userAuthoredPayload = plan.General.Payloads.ValueString()
	}

	id := state.ID.ValueString()
	if err := helpers.RetryOnDirectoryGroupMatchConflict(updateCtx, func() error {
		return r.client.UpdateOSXConfigurationProfileByID(updateCtx, id, input)
	}); err != nil {
		resp.Diagnostics.AddError("Error updating macOS configuration profile", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetOSXConfigurationProfileByID(updateCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated macOS configuration profile", helpers.APIErrorDetail(err))
		return
	}
	var rawServerPayload []byte
	if got != nil && got.General != nil && got.General.Payloads != nil {
		rawServerPayload = []byte(string(*got.General.Payloads))
	}
	resp.Diagnostics.Append(assignResourceModel(updateCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if userAuthoredPayload != "" && plan.General != nil && plan.General.Payloads.ValueString() != userAuthoredPayload {
		resp.Diagnostics.AddError(
			payloadhelpers.PayloadFidelitySummary,
			payloadhelpers.PayloadFidelityErrorDetail([]byte(userAuthoredPayload), rawServerPayload, payloadhelpers.FidelityPhaseUpdate),
		)
		return
	}
	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, identityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(writePrivatePayloadRefs(updateCtx, resp.Private, userAuthoredPayload, rawServerPayload)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(updateCtx, &plan)...)
}

func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Provider not configured", providerNotConfigured)
		return
	}
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, td := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultDeleteTimeout, state.Timeouts.Delete)
	resp.Diagnostics.Append(td...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	if err := r.client.DeleteOSXConfigurationProfileByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting macOS configuration profile", helpers.APIErrorDetail(err))
		return
	}
}

const providerNotConfigured = "The provider client was not configured. This is a bug in the provider — please report it."
