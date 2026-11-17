// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateVenafiV1                      (POST  /pki/venafi)
//   pro.GetVenafiV1                         (GET   /pki/venafi/{id})
//   pro.UpdateVenafiV1                      (PATCH /pki/venafi/{id}, merge semantics)
//   pro.DeleteVenafiV1                      (DELETE /pki/venafi/{id}; 409 if in use)
//   pro.GetVenafiJamfPublicKeyV1            (GET   /pki/venafi/{id}/jamf-public-key)
//   pro.RegenerateVenafiJamfPublicKeyV1     (POST  /pki/venafi/{id}/jamf-public-key/regenerate)
//   pro.GetVenafiProxyTrustStoreV1          (GET   /pki/venafi/{id}/proxy-trust-store; 404 if unset)
//   pro.UploadVenafiProxyTrustStoreV1       (POST  /pki/venafi/{id}/proxy-trust-store)
//   pro.DeleteVenafiProxyTrustStoreV1       (DELETE /pki/venafi/{id}/proxy-trust-store)
//
// Server invariants (wire-probed 2026-06-09):
//   - PATCH is application/json with MERGE semantics: omit a field = preserve;
//     send "" = clear. The input builder therefore drops null/unknown plan
//     fields rather than always-emitting.
//   - refreshToken is write-only and never returned; refreshTokenConfigured
//     (RO bool) echoes whether a token is stored. Omitting refreshToken on a
//     PATCH preserves the stored secret.
//   - jamf_public_key is byte-stable across reads (no perpetual diff); the
//     regenerate endpoint changes it.
//   - proxy_trust_store round-trips byte-for-byte (GET returns the uploaded PEM
//     exactly; 404 before upload / after delete).
//   - The create response mints the id first; proxy-trust-store upload and the
//     public-key GET come after and need that id. A post-create failure must
//     persist state-with-id, not orphan the server-side record.
//
// Status: current.

package pki_venafi

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro Venafi CA. The POST mints the id; the proxy
// trust store (if set) is uploaded afterwards, then the record, jamf public
// key and proxy trust store are read back. Because the id exists the moment
// the POST returns, every post-POST failure persists state-with-id (and the
// Computed attrs initialised to known values) before surfacing the error, so
// the server-side record is never orphaned.
func (r *PkiVenafiResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PkiVenafiResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg PkiVenafiResourceModel
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

	// On create, send the refresh token whenever it is supplied in config.
	refreshToken := helpers.OptionalStringPointer(cfg.RefreshTokenWo)
	input := buildVenafiInput(plan, refreshToken)

	created, err := r.client.CreateVenafiV1(createCtx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro Venafi CA", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing Venafi CA ID",
			"Jamf Pro returned a create response with no Venafi CA ID; cannot persist state.",
		)
		return
	}

	// The id now exists server-side. Initialise every Computed attribute to a
	// known value so a partial-state Set never trips "Computed attribute not
	// set after apply".
	plan.ID = types.StringValue(created.ID)
	plan.RefreshTokenConfigured = types.BoolValue(false)
	plan.JamfPublicKey = types.StringNull()
	if plan.ProxyAddress.IsUnknown() {
		plan.ProxyAddress = types.StringNull()
	}
	if plan.ClientID.IsUnknown() {
		plan.ClientID = types.StringNull()
	}
	if plan.RevocationEnabled.IsUnknown() {
		plan.RevocationEnabled = types.BoolValue(false)
	}
	if plan.ProxyTrustStore.IsUnknown() {
		plan.ProxyTrustStore = types.StringNull()
	}

	// Upload the proxy trust store (if set) — needs the id. On failure the id
	// already exists server-side: persist best-effort state (never orphan) and
	// surface the error.
	if hasProxyTrustStore(plan.ProxyTrustStore) {
		pem := plan.ProxyTrustStore.ValueString()
		if err := r.client.UploadVenafiProxyTrustStoreV1(createCtx, plan.ID.ValueString(), []byte(pem)); err != nil {
			resp.Diagnostics.AddError("Error uploading Jamf Pro Venafi proxy trust store", helpers.APIErrorDetail(err))
			resp.Diagnostics.Append(r.refreshFromServer(createCtx, &plan)...)
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, pkiVenafiIdentityModel{ID: plan.ID})...)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			return
		}
	}

	if diags := r.refreshFromServer(createCtx, &plan); diags.HasError() {
		// refreshFromServer surfaced the error; still persist what we have so
		// the id is not lost.
		resp.Diagnostics.Append(diags...)
		resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, pkiVenafiIdentityModel{ID: plan.ID})...)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, pkiVenafiIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro Venafi CA", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest Venafi CA representation,
// including the jamf public key and proxy trust store (separate GETs).
func (r *PkiVenafiResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PkiVenafiResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this Venafi CA without existing state or identity data, so the provider cannot determine which CA to read.",
			)
			return
		}
		var identity pkiVenafiIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing Venafi CA ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the Venafi CA.",
			)
			return
		}
		state.ID = identity.ID
		state.RefreshTokenWo = types.StringNull()
		state.RefreshTokenWoVersion = types.Int64Null()
		state.JamfPublicKeyRotation = types.Int64Null()
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(pkiVenafiTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro Venafi CA without ID.")
		return
	}

	rec, err := r.client.GetVenafiV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro Venafi CA not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, pkiVenafiIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro Venafi CA", helpers.APIErrorDetail(err))
		return
	}
	assignVenafiServerFields(&state, rec)

	resp.Diagnostics.Append(r.readJamfPublicKey(readCtx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.readProxyTrustStore(readCtx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, pkiVenafiIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies the writable subset via PATCH (merge: omit = preserve), rotates
// the refresh token only when refresh_token_wo_version changed, regenerates the
// jamf public key when jamf_public_key_rotation changed, and reconciles the
// proxy trust store via its dedicated endpoints. The GET-after refreshes all
// computed fields.
func (r *PkiVenafiResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PkiVenafiResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state PkiVenafiResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg PkiVenafiResourceModel
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

	// Rotation gate: send refreshToken only when the WriteOnly version changed.
	var refreshToken *string
	if !plan.RefreshTokenWoVersion.Equal(state.RefreshTokenWoVersion) {
		refreshToken = helpers.OptionalStringPointer(cfg.RefreshTokenWo)
	}
	input := buildVenafiInput(plan, refreshToken)

	if _, err := r.client.UpdateVenafiV1(updateCtx, plan.ID.ValueString(), input); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro Venafi CA", helpers.APIErrorDetail(err))
		return
	}

	// Regenerate the Jamf public key when its rotation trigger changed.
	if shouldRotate(plan.JamfPublicKeyRotation, state.JamfPublicKeyRotation) {
		if err := r.client.RegenerateVenafiJamfPublicKeyV1(updateCtx, plan.ID.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error regenerating Jamf Pro Venafi public key", helpers.APIErrorDetail(err))
			return
		}
	}

	// Reconcile the proxy trust store (separate endpoints): upload when newly
	// set or changed, delete when cleared.
	resp.Diagnostics.Append(r.reconcileProxyTrustStore(updateCtx, plan.ID.ValueString(), plan.ProxyTrustStore, state.ProxyTrustStore)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.refreshFromServer(updateCtx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, pkiVenafiIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro Venafi CA. A 409 (the CA is referenced by config
// profiles) is surfaced as an actionable error rather than swallowed.
func (r *PkiVenafiResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PkiVenafiResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro Venafi CA without ID.")
		return
	}

	if err := r.client.DeleteVenafiV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro Venafi CA already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting Jamf Pro Venafi CA",
			fmt.Sprintf("Jamf Pro returned an error deleting Venafi CA %s. If the CA is referenced by configuration profiles, Jamf Pro returns 409 Conflict; remove those references first. API error: %s", state.ID.ValueString(), helpers.APIErrorDetail(err)),
		)
	}
}

// refreshFromServer re-reads the record, the jamf public key, and the proxy
// trust store, assigning them onto the model. Used by Create and Update after
// the write side-effects have settled.
func (r *PkiVenafiResource) refreshFromServer(ctx context.Context, m *PkiVenafiResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	rec, err := r.client.GetVenafiV1(ctx, m.ID.ValueString())
	if err != nil {
		diags.AddError("Error reading Jamf Pro Venafi CA", helpers.APIErrorDetail(err))
		return diags
	}
	assignVenafiServerFields(m, rec)

	diags.Append(r.readJamfPublicKey(ctx, m)...)
	if diags.HasError() {
		return diags
	}
	diags.Append(r.readProxyTrustStore(ctx, m)...)
	return diags
}

// readJamfPublicKey populates jamf_public_key from the dedicated GET. The key
// is byte-stable, so it is safe to store verbatim.
func (r *PkiVenafiResource) readJamfPublicKey(ctx context.Context, m *PkiVenafiResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	pem, err := r.client.GetVenafiJamfPublicKeyV1(ctx, m.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			m.JamfPublicKey = types.StringNull()
			return diags
		}
		diags.AddError("Error reading Jamf Pro Venafi public key", helpers.APIErrorDetail(err))
		return diags
	}
	if len(pem) == 0 {
		m.JamfPublicKey = types.StringNull()
		return diags
	}
	m.JamfPublicKey = types.StringValue(string(pem))
	return diags
}

// readProxyTrustStore populates proxy_trust_store from the dedicated GET. A 404
// means none is uploaded — map it to the current (planned) value so an explicit
// "" / null clear is preserved rather than collapsed.
func (r *PkiVenafiResource) readProxyTrustStore(ctx context.Context, m *PkiVenafiResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	pem, err := r.client.GetVenafiProxyTrustStoreV1(ctx, m.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			m.ProxyTrustStore = helpers.PreserveStringWhenWireEmpty(nil, m.ProxyTrustStore)
			return diags
		}
		diags.AddError("Error reading Jamf Pro Venafi proxy trust store", helpers.APIErrorDetail(err))
		return diags
	}
	s := string(pem)
	m.ProxyTrustStore = helpers.PreserveStringWhenWireEmpty(&s, m.ProxyTrustStore)
	return diags
}

// reconcileProxyTrustStore uploads or deletes the proxy trust store on Update,
// comparing plan vs prior state: newly set / changed → upload; cleared → delete;
// unchanged → no-op.
func (r *PkiVenafiResource) reconcileProxyTrustStore(ctx context.Context, id string, plan, state types.String) diag.Diagnostics {
	var diags diag.Diagnostics
	planSet := hasProxyTrustStore(plan)
	stateSet := hasProxyTrustStore(state)

	switch {
	case planSet && (!stateSet || plan.ValueString() != state.ValueString()):
		pem := plan.ValueString()
		if err := r.client.UploadVenafiProxyTrustStoreV1(ctx, id, []byte(pem)); err != nil {
			diags.AddError("Error uploading Jamf Pro Venafi proxy trust store", helpers.APIErrorDetail(err))
		}
	case !planSet && stateSet:
		if err := r.client.DeleteVenafiProxyTrustStoreV1(ctx, id); err != nil {
			diags.AddError("Error deleting Jamf Pro Venafi proxy trust store", helpers.APIErrorDetail(err))
		}
	}
	return diags
}

// hasProxyTrustStore reports whether the proxy_trust_store value carries content
// (non-null, non-unknown, non-empty).
func hasProxyTrustStore(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueString() != ""
}
