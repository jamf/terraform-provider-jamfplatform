// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateAdcsSettingsV1
//   pro.GetAdcsSettingsV1
//   pro.UpdateAdcsSettingsV1   (PATCH application/merge-patch+json — genuine merge, omit=preserve)
//   pro.DeleteAdcsSettingsV1
//
// Server invariants (wire-probed 2026-06-09 — see spike/PKI_CERTIFICATES_SPIKE.md §4):
//   - Create returns {id, href} (201). The id is a string, stable for the life of
//     the integration.
//   - `outbound` is IMMUTABLE: PATCH-ing a mode flip returns 400
//     ("Cannot convert to outbound configuration"). connector_mode is therefore
//     RequiresReplace and `outbound` is never sent on update.
//   - PATCH is a genuine merge (omit=preserve); a certificate must be supplied in
//     full or not at all. The provider re-sends a certificate only when its
//     wo_version changed (per-cert rotation gate).
//   - GET returns certificate METADATA only — no bytes, no password.
//   - DELETE returns 204; 409 when configuration profiles reference the integration.
//
// SDK BLOCKER (does NOT block compile/unit-tests; blocks live GET / acc):
//   AdcsCertificateResponse.ExpirationDate is *time.Time, which fails to
//   deserialize Jamf Pro's offset-less wire value ("2036-06-06T17:42:41") — every
//   GetAdcsSettingsV1 currently fails to unmarshal. Fix: expirationDate => *string
//   in the SDK (mirrors the sibling field at pro/types.go:820). Same defect hits
//   DigiCert's CertificateResponse. Fix-prompt: spike/SDK_PKI_EXPIRATION_DATE_FIX_PROMPT.md.
//   The single coupled line is isolated in state_builders.go adcsCertExpiration().
//
// Status: current.

package pki_adcs

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create provisions a new Jamf Pro AD CS integration. POST mints the id; the
// provider then GETs to capture server-computed fields and both certificate
// metadata blocks.
func (r *AdcsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg AdcsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
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

	input, err := buildAdcsCreateInput(plan, cfg)
	if err != nil {
		resp.Diagnostics.AddError("Invalid AD CS configuration", helpers.APIErrorDetail(err))
		return
	}

	created, err := r.client.CreateAdcsSettingsV1(createCtx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro AD CS integration", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing AD CS Settings ID",
			"Jamf Pro returned 201 Created with no AD CS Settings ID; cannot persist state.",
		)
		return
	}
	plan.ID = types.StringValue(created.ID)

	got, err := r.client.GetAdcsSettingsV1(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro AD CS integration", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignAdcsResourceModel(ctx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, adcsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro AD CS integration", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest AD CS representation. Import-time
// refresh sources the id from the identity object.
func (r *AdcsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AdcsResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this AD CS integration without existing state or identity data, so the provider cannot determine which integration to read.",
			)
			return
		}
		var identity adcsIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing AD CS Settings ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the AD CS integration.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(adcsTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro AD CS integration without ID.")
		return
	}

	hydrating := importHydration(isImport, state.ConnectorMode)

	got, err := r.client.GetAdcsSettingsV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro AD CS integration not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, adcsIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro AD CS integration", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignAdcsResourceModel(ctx, &state, got, hydrating)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, adcsIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies a merge-patch to the AD CS integration. `outbound` is never sent
// (immutable; RequiresReplace handles a change). A certificate is re-sent only
// when its wo_version changed (per-cert rotation gate); other omitted fields are
// preserved by the server's merge semantics. The provider then GETs to refresh
// server-computed fields and both certificate metadata blocks.
func (r *AdcsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg AdcsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
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

	input, err := buildAdcsUpdateInput(plan, state, cfg)
	if err != nil {
		resp.Diagnostics.AddError("Invalid AD CS configuration", helpers.APIErrorDetail(err))
		return
	}

	if err := r.client.UpdateAdcsSettingsV1(updateCtx, plan.ID.ValueString(), input); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro AD CS integration", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetAdcsSettingsV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro AD CS integration", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignAdcsResourceModel(ctx, &plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, adcsIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro AD CS integration. A 409 (the integration is
// referenced by configuration profiles) is surfaced as an actionable error.
func (r *AdcsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AdcsResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro AD CS integration without ID.")
		return
	}

	if err := r.client.DeleteAdcsSettingsV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro AD CS integration already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting Jamf Pro AD CS integration",
			fmt.Sprintf("API error deleting AD CS integration %s: %s\n\nIf Jamf Pro returned 409 Conflict, the integration is still referenced by one or more configuration profiles. Remove those references (or the profiles) before deleting this resource.", state.ID.ValueString(), helpers.APIErrorDetail(err)),
		)
	}
}
