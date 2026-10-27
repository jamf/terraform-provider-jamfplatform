// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// createGoogle provisions a Google Secure LDAP Cloud Identity Provider:
// POST /v2/cloud-ldaps then GET to capture server-populated fields.
func (r *CloudIdentityProviderResource) createGoogle(ctx context.Context, plan, cfg CloudIdentityProviderResourceModel, resp *resource.CreateResponse) {
	if plan.Google == nil || plan.Google.Server == nil {
		resp.Diagnostics.AddError(missingProviderBlockError(providerGoogle, "google"))
		return
	}
	body, diags := buildGoogleCreateRequest(plan, cfg)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateCloudLdapV2(ctx, body)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro Cloud Identity Provider (Google)", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	if created == nil || created.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing Cloud Identity Provider ID",
			"Jamf Pro returned 201 Created with no id; cannot persist state.",
		)
		return
	}

	// Pin the id from the create response so it survives even if the read-back
	// returns a response without cloudIdPCommon (assignGoogleState only sets id
	// when that block is present). Mirrors the codebase create→GET convention.
	plan.ID = types.StringValue(created.ID)

	got, err := r.client.GetCloudLdapV2(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro Cloud Identity Provider (Google)", helpers.APIErrorDetail(err))
		return
	}
	// Preserve the user's rotation trigger across the read-back.
	assignGoogleState(&plan, got, keystoreWoVersion(plan), false)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, cloudIdentityProviderIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Trace(ctx, "created Jamf Pro Cloud Identity Provider (Google)", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// readGoogle refreshes state from GET /v2/cloud-ldaps/{id}.
func (r *CloudIdentityProviderResource) readGoogle(ctx context.Context, state *CloudIdentityProviderResourceModel, resp *resource.ReadResponse, hydrating bool) {
	priorWoVersion := keystoreWoVersion(*state)

	got, err := r.client.GetCloudLdapV2(ctx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro Cloud Identity Provider (Google) not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro Cloud Identity Provider (Google)", helpers.APIErrorDetail(err))
		return
	}

	assignGoogleState(state, got, priorWoVersion, hydrating)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, cloudIdentityProviderIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// updateGoogle full-replaces via PUT /v2/cloud-ldaps/{id}, then GET. The
// keystore is re-sent only when wo_version changed (buildGoogleUpdateRequest);
// otherwise it is omitted and Jamf Pro preserves the stored certificate.
func (r *CloudIdentityProviderResource) updateGoogle(ctx context.Context, plan, state, cfg CloudIdentityProviderResourceModel, resp *resource.UpdateResponse) {
	if plan.Google == nil || plan.Google.Server == nil {
		resp.Diagnostics.AddError(missingProviderBlockError(providerGoogle, "google"))
		return
	}
	body, diags := buildGoogleUpdateRequest(plan, state, cfg)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.UpdateCloudLdapV2(ctx, plan.ID.ValueString(), body); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro Cloud Identity Provider (Google)", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetCloudLdapV2(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro Cloud Identity Provider (Google)", helpers.APIErrorDetail(err))
		return
	}
	assignGoogleState(&plan, got, keystoreWoVersion(plan), false)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, cloudIdentityProviderIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// deleteGoogle removes the configuration via DELETE /v2/cloud-ldaps/{id}.
func (r *CloudIdentityProviderResource) deleteGoogle(ctx context.Context, state CloudIdentityProviderResourceModel, resp *resource.DeleteResponse) {
	if err := r.client.DeleteCloudLdapV2(ctx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro Cloud Identity Provider (Google) already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro Cloud Identity Provider (Google)", helpers.APIErrorDetail(err))
	}
}
