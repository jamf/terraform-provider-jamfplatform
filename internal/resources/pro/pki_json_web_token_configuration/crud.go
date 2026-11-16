// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateJsonWebTokenConfigurationByID   (POST   /jsonwebtokenconfigurations/id/0)
//   proclassic.GetJsonWebTokenConfigurationByID      (GET    /jsonwebtokenconfigurations/id/{id})
//   proclassic.UpdateJsonWebTokenConfigurationByID   (PUT    /jsonwebtokenconfigurations/id/{id})
//   proclassic.DeleteJsonWebTokenConfigurationByID   (DELETE /jsonwebtokenconfigurations/id/{id})
//   proclassic.ListJsonWebTokenConfigurations        (data source name lookup / list resource)
//
// Status: current.
//
// Server invariants (wire-probed — spike/JSON_WEB_TOKEN_SPIKE.md):
//   - At most ONE JSON Web Token configuration per Jamf Pro instance; a second
//     create is rejected ("Cannot create more than one JSON Web Token").
//   - Create POSTs to id="0"; the server mints the integer ID and returns an
//     id-only body (<json_web_token_configuration><id>).
//   - PUT returns 201 with no body — Update must GET to refresh state.
//   - PUT is a partial merge: omitted elements retain stored values, including
//     <encryption_key> (never echoed on GET — genuinely write-only). The key is
//     sent on every Create and on Update only when encryption_key_wo_version
//     changed.
//   - token_expiry is server-enforced 1–120 (0 and 121 are rejected); omitting
//     it on create stores 0 — the UI's "5" is a form-side default only.
//   - disabled defaults to false (= enabled); the provider exposes the inverted
//     `enabled` attribute, with the inversion confined to the builders.
//   - DELETE is a clean synchronous 200; a deleted id GETs a plain 404, so Read
//     and Delete self-heal via helpers.IsNotFoundError. No polling.

package pki_json_web_token_configuration

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro JSON Web Token configuration. Classic POSTs to
// id="0"; the server allocates the real integer ID and returns it.
func (r *JSONWebTokenConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan JSONWebTokenConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg JSONWebTokenConfigurationResourceModel
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

	// The encryption key is WriteOnly, so the plan exposes it as null — read
	// the plaintext from config. It is always sent on create (the server
	// requires it).
	created, err := r.client.CreateJsonWebTokenConfigurationByID(createCtx, "0", buildJSONWebTokenConfigurationInput(plan, helpers.OptionalStringPointer(cfg.EncryptionKey)))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro JSON Web Token configuration", helpers.APIErrorDetail(err))
		return
	}
	id := extractJSONWebTokenConfigurationID(created)
	if id == "" {
		resp.Diagnostics.AddError(
			"Create response missing record ID",
			"Jamf Pro returned 201 Created with no JSON Web Token configuration ID; cannot persist state.",
		)
		return
	}

	got, err := r.client.GetJsonWebTokenConfigurationByID(createCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro JSON Web Token configuration", helpers.APIErrorDetail(err))
		return
	}
	assignJSONWebTokenConfigurationResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jsonWebTokenConfigurationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro JSON Web Token configuration", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest record representation.
func (r *JSONWebTokenConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state JSONWebTokenConfigurationResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this JSON Web Token configuration without existing state or identity data, so the provider cannot determine which record to read.",
			)
			return
		}
		var identity jsonWebTokenConfigurationIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing record ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the JSON Web Token configuration.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(jsonWebTokenConfigurationTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro JSON Web Token configuration without ID.")
		return
	}

	got, err := r.client.GetJsonWebTokenConfigurationByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro JSON Web Token configuration not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jsonWebTokenConfigurationIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro JSON Web Token configuration", helpers.APIErrorDetail(err))
		return
	}

	assignJSONWebTokenConfigurationResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jsonWebTokenConfigurationIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro JSON Web Token configuration. The classic PUT
// returns 201 with an empty body — we must GET to refresh state.
func (r *JSONWebTokenConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan JSONWebTokenConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state JSONWebTokenConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg JSONWebTokenConfigurationResourceModel
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

	// Only include the plaintext <encryption_key> on the wire when the user
	// bumped `encryption_key_wo_version`. Otherwise omit so the server retains
	// the stored key under Classic's partial-merge semantics.
	encryptionKey := encryptionKeyForUpdate(plan.EncryptionKeyWoVersion, state.EncryptionKeyWoVersion, cfg.EncryptionKey)

	if err := r.client.UpdateJsonWebTokenConfigurationByID(updateCtx, plan.ID.ValueString(), buildJSONWebTokenConfigurationInput(plan, encryptionKey)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro JSON Web Token configuration", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetJsonWebTokenConfigurationByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro JSON Web Token configuration", helpers.APIErrorDetail(err))
		return
	}
	assignJSONWebTokenConfigurationResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jsonWebTokenConfigurationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro JSON Web Token configuration.
func (r *JSONWebTokenConfigurationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state JSONWebTokenConfigurationResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro JSON Web Token configuration without ID.")
		return
	}

	if err := r.client.DeleteJsonWebTokenConfigurationByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro JSON Web Token configuration already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro JSON Web Token configuration", helpers.APIErrorDetail(err))
	}
}
