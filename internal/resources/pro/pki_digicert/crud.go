// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateDigicertTrustLifecycleManagerV1   (POST; returns *HrefResponse with the new id)
//   pro.GetDigicertTrustLifecycleManagerV1       (GET; client certificate is METADATA-only on read)
//   pro.UpdateDigicertTrustLifecycleManagerV1    (PATCH application/merge-patch+json — genuine merge, omit=preserve)
//   pro.DeleteDigicertTrustLifecycleManagerV1    (DELETE; 409 when referenced by a configuration profile)
//
// Server invariants (wire-probed 2026-06-09):
//   - Create returns { id, href } (201); GET-after-create surfaces the canonical state.
//   - GET returns clientCert METADATA only (filename, serialNumber, subject, issuer,
//     expirationDate) — never the certificate bytes or password.
//   - PATCH is a genuine merge: omitted fields are preserved. The certificate is
//     all-or-nothing ("must be provided in full, or not at all"), so the provider
//     re-sends the whole certificate only on a wo_version bump.
//
// Status: current.

package pki_digicert

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new DigiCert Trust Lifecycle Manager integration, sending the
// client certificate when the block is supplied, then GETs to capture canonical
// server-derived state (display_name/host_name/revocation_enabled + cert metadata).
func (r *DigicertResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg DigicertResourceModel
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

	input, err := buildDigicertInput(plan, cfg, plan.ClientCertificate != nil)
	if err != nil {
		resp.Diagnostics.AddError("Invalid DigiCert configuration", helpers.APIErrorDetail(err))
		return
	}

	createResp, err := r.client.CreateDigicertTrustLifecycleManagerV1(createCtx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro DigiCert integration", helpers.APIErrorDetail(err))
		return
	}
	if createResp == nil || createResp.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing DigiCert integration ID",
			"Jamf Pro returned 201 Created with no integration ID; cannot persist state.",
		)
		return
	}

	got, err := r.client.GetDigicertTrustLifecycleManagerV1(createCtx, createResp.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro DigiCert integration", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignDigicertServerFields(&plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, digicertIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro DigiCert integration", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest integration representation. The
// WriteOnly certificate fields and wo_version are never touched. Import-time
// refresh (req.State.Raw null) sources the ID from the identity object.
func (r *DigicertResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state DigicertResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this DigiCert integration without existing state or identity data, so the provider cannot determine which integration to read.",
			)
			return
		}
		var identity digicertIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing DigiCert integration ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the DigiCert integration.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(digicertTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro DigiCert integration without ID.")
		return
	}

	got, err := r.client.GetDigicertTrustLifecycleManagerV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro DigiCert integration not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, digicertIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro DigiCert integration", helpers.APIErrorDetail(err))
		return
	}

	hydrating := importHydration(isImport, state.DisplayName)
	resp.Diagnostics.Append(assignDigicertServerFields(&state, got, hydrating)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, digicertIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies a merge PATCH. Scalar plan fields are sent when known and
// omitted otherwise (omit = preserve). The client certificate is re-sent only
// when client_certificate.wo_version changed versus prior state — else omitted so
// Jamf Pro retains the stored certificate.
func (r *DigicertResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg DigicertResourceModel
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

	includeCert := shouldRotateCert(plan.ClientCertificate, state.ClientCertificate)
	input, err := buildDigicertInput(plan, cfg, includeCert)
	if err != nil {
		resp.Diagnostics.AddError("Invalid DigiCert configuration", helpers.APIErrorDetail(err))
		return
	}

	if err := r.client.UpdateDigicertTrustLifecycleManagerV1(updateCtx, plan.ID.ValueString(), input); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro DigiCert integration", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetDigicertTrustLifecycleManagerV1(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro DigiCert integration", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignDigicertServerFields(&plan, got, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, digicertIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the DigiCert integration. A 409 indicates the integration is
// still referenced by a configuration profile and is surfaced actionably.
func (r *DigicertResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DigicertResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro DigiCert integration without ID.")
		return
	}

	if err := r.client.DeleteDigicertTrustLifecycleManagerV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro DigiCert integration already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting Jamf Pro DigiCert integration",
			fmt.Sprintf("API error: %s. If this is a 409 conflict, the DigiCert integration is still referenced by one or more configuration profiles; remove those references before deleting.", helpers.APIErrorDetail(err)),
		)
	}
}
