// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.GetMobileDeviceConfigurationProfileByID
//   proclassic.GetMobileDeviceConfigurationProfileByName     (data source name lookup)
//   proclassic.CreateMobileDeviceConfigurationProfileByID
//   proclassic.UpdateMobileDeviceConfigurationProfileByID
//   proclassic.DeleteMobileDeviceConfigurationProfileByID
//   proclassic.ListMobileDeviceConfigurationProfiles         (list resource, collision check in collision.go)
//
// Uniqueness: name, enforced (409 Duplicate name), trimmed on save, case- and accent-insensitive; separate pool from macOS configuration profiles.
//
// Status: current. Last reviewed 2026-10-25.
//
// Scope semantics: within a sent <scope> the server replaces the whole
// subtree (wire-probed 2026-07-08 — any category element present, even empty,
// wipes every omitted category across targets/limitations/exclusions). Scope
// therefore uses per-category granular ownership: when the plan declares a
// scope block, Update GETs the live object first and overlays the declared
// categories onto the server's current scope (scope-only merge — no other
// section of the read is echoed back), emitting every merged category
// explicitly. Omitted categories stay owned by the admin UI; declared `[]`
// clears. See STYLE_GUIDE.md §Scope helper omission semantics.

package mobile_device_configuration_profile

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/payloadhelpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// Create posts the profile, reads the server-canonical form back into
// state, and rolls the create back when the server stored the payload
// unfaithfully. The diagnostic names each diverging value and the fix for its
// class of mangling (see payloadhelpers.PayloadFidelityErrorDetail).
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
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
	var userAuthoredPayload string
	if plan.General != nil && !plan.General.Payloads.IsNull() && !plan.General.Payloads.IsUnknown() {
		userAuthoredPayload = plan.General.Payloads.ValueString()
	}

	created, err := r.client.CreateMobileDeviceConfigurationProfileByID(createCtx, "0", input)
	if helpers.IsDirectoryGroupMatchConflict(err) {
		// Bootstrap apply: the referenced directory is still coming up. Retry until
		// the scope group resolves (or a real wrong-name conflict persists).
		err = helpers.RetryOnDirectoryGroupMatchConflict(createCtx, func() error {
			var e error
			created, e = r.client.CreateMobileDeviceConfigurationProfileByID(createCtx, "0", input)
			return e
		})
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro mobile device configuration profile", helpers.APIErrorDetail(err))
		return
	}

	id := ""
	if created.ID != nil {
		id = helpers.StringValueFromIntPtr(created.ID).ValueString()
	} else if created.General != nil && created.General.ID != nil {
		id = helpers.StringValueFromIntPtr(created.General.ID).ValueString()
	}
	if id == "" {
		resp.Diagnostics.AddError("Create response missing profile ID", "Jamf Pro did not return an ID for the created mobile device configuration profile.")
		return
	}

	got, err := r.client.GetMobileDeviceConfigurationProfileByID(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created mobile device configuration profile", helpers.APIErrorDetail(err))
		return
	}
	var rawServerPayload []byte
	if got != nil && got.General != nil && got.General.Payloads != nil {
		rawServerPayload = []byte(string(*got.General.Payloads))
	}
	resp.Diagnostics.Append(assignResourceModel(createCtx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if userAuthoredPayload != "" && plan.General != nil && plan.General.Payloads.ValueString() != userAuthoredPayload {
		if delErr := r.client.DeleteMobileDeviceConfigurationProfileByID(createCtx, id); delErr != nil {
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
	tflog.Trace(createCtx, "created jamfplatform_pro_mobile_device_configuration_profile", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(createCtx, &plan)...)
}

func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
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

	got, err := r.client.GetMobileDeviceConfigurationProfileByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, identityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(readCtx)
			return
		}
		resp.Diagnostics.AddError("Error reading mobile device configuration profile", helpers.APIErrorDetail(err))
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
	//
	// Mobile device profiles are the more exposed of the two: Jamf Pro stores
	// nearly every mobile payload type verbatim, including
	// com.apple.ManagedClient.preferences (which is faithful on macOS), so a web
	// clip URL with a query string is enough to trip this.
	if firstHydration {
		var gateName string
		if got != nil && got.General != nil && got.General.Name != nil {
			gateName = *got.General.Name
		}
		resp.Diagnostics.Append(payloadhelpers.ImportGateDiagnostics(
			rawServerPayload, payloadhelpers.PlatformMobileDevice, gateName, state.ID.ValueString(),
		)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
	resp.Diagnostics.Append(assignResourceModel(readCtx, &state, got, firstHydration)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(reconcileReadDrift(readCtx, req.Private, &state, rawServerPayload)...)
	if resp.Diagnostics.HasError() {
		return
	}
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
		current, err := r.client.GetMobileDeviceConfigurationProfileByID(updateCtx, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading mobile device configuration profile before update", helpers.APIErrorDetail(err))
			return
		}
		var serverScope *scope.MobileScopeModel
		if current != nil && current.Scope != nil {
			serverScope = &scope.MobileScopeModel{}
			resp.Diagnostics.Append(flattenScope(updateCtx, current.Scope, serverScope, true)...)
			if resp.Diagnostics.HasError() {
				return
			}
		}
		wirePlan.Scope = scope.MergeMobileScope(plan.Scope, serverScope)
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
		return r.client.UpdateMobileDeviceConfigurationProfileByID(updateCtx, id, input)
	}); err != nil {
		resp.Diagnostics.AddError("Error updating mobile device configuration profile", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetMobileDeviceConfigurationProfileByID(updateCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated mobile device configuration profile", helpers.APIErrorDetail(err))
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

	if err := r.client.DeleteMobileDeviceConfigurationProfileByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting mobile device configuration profile", helpers.APIErrorDetail(err))
		return
	}
}

const providerNotConfigured = "The provider client was not configured. This is a bug in the provider — please report it."
