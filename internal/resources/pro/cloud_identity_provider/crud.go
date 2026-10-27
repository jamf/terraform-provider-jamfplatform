// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   GOOGLE (provider_name = "GOOGLE"):
//     pro.CreateCloudLdapV2
//     pro.GetCloudLdapV2
//     pro.UpdateCloudLdapV2
//     pro.DeleteCloudLdapV2
//     pro.VerifyLdapKeystoreV1   (plan-time keystore verify, best-effort)
//   ENTRA_ID (provider_name = "ENTRA_ID", wire providerName "AZURE"):
//     pro.CreateCloudAzureV1
//     pro.GetCloudAzureV1
//     pro.UpdateCloudAzureV1
//     pro.DeleteCloudAzureV1
//   Both (registry — provider discovery on import):
//     pro.GetCloudIdpV1
//     pro.ListCloudIdpV1       (collision check in collision.go)
//
// Uniqueness: display name, enforced (400 DUPLICATE_FIELD), case-, leading- and trailing-space-
// and accent-insensitive (an accent variant is refused 500 with no body), across both providers.
//
// Status: current. Last reviewed 2026-10-25.

package cloud_identity_provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// missingProviderBlockError is the diagnostic emitted when CRUD dispatches to a
// provider branch whose nested block is absent. The plan-time
// providerBlockConfigValidator normally prevents this, but the branch builders
// dereference the block, so this guard turns a would-be nil panic into a clear
// error (defense-in-depth; mirrors directory_binding's nil-safe builders).
func missingProviderBlockError(providerName, blockName string) (string, string) {
	return "Missing provider configuration block",
		"provider_name is " + providerName + " but the " + blockName + " block is not set."
}

// Create dispatches to the Google or Entra ID branch based on provider_name.
func (r *CloudIdentityProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan CloudIdentityProviderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg CloudIdentityProviderResourceModel
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

	switch plan.ProviderName.ValueString() {
	case providerGoogle:
		r.createGoogle(createCtx, plan, cfg, resp)
	case providerEntraID:
		r.createAzure(createCtx, plan, cfg, resp)
	default:
		resp.Diagnostics.AddError("Unsupported provider_name", "provider_name must be GOOGLE or ENTRA_ID.")
	}
}

// Read refreshes state. On import only the id is known, so the provider is
// discovered via the Cloud Identity Provider registry before dispatch.
func (r *CloudIdentityProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state CloudIdentityProviderResourceModel

	if req.State.Raw.IsNull() {
		// Identity-based import: no prior state, only the identity (id).
		id, ok := r.importID(ctx, req, resp)
		if !ok {
			return
		}
		state.ID = types.StringValue(id)
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(cloudIdentityProviderTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// display_name is schema-Required, so Create and Update always populate it
	// before any Read and it can only be null here on a first-time hydration —
	// the identity-only import path above, or `terraform import`, which seeds
	// only `id` into a sparse-but-non-null state. provider_name cannot serve as
	// the signal: the discovery below force-populates it. Sampled before the
	// dispatch, because assignGoogleState / assignAzureState overwrite it from
	// the wire.
	hydrating := state.DisplayName.IsNull()

	// provider_name drives CRUD dispatch but is not always present yet: on
	// import (both the identity path above and ImportStatePassthroughID, which
	// seeds only `id` into a non-null partial state) the discriminator is
	// unknown. Discover it from the Cloud Identity Provider registry whenever
	// it is missing.
	if state.ProviderName.IsNull() || state.ProviderName.IsUnknown() || state.ProviderName.ValueString() == "" {
		if state.ID.IsNull() || state.ID.ValueString() == "" {
			resp.Diagnostics.AddError("Missing ID", "Cannot refresh Jamf Pro Cloud Identity Provider without an id.")
			return
		}
		providerName, ok := r.discoverProvider(ctx, state.ID.ValueString(), resp)
		if !ok {
			return
		}
		state.ProviderName = types.StringValue(providerNameFromWire(providerName))
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	switch state.ProviderName.ValueString() {
	case providerGoogle:
		r.readGoogle(readCtx, &state, resp, hydrating)
	case providerEntraID:
		r.readAzure(readCtx, &state, resp, hydrating)
	default:
		resp.Diagnostics.AddError("Unsupported provider_name", "provider_name must be GOOGLE or ENTRA_ID.")
	}
}

// Update dispatches to the Google or Entra ID branch. provider_name is
// RequiresReplace, so plan and state always agree on the provider here.
func (r *CloudIdentityProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan, state, cfg CloudIdentityProviderResourceModel
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

	switch plan.ProviderName.ValueString() {
	case providerGoogle:
		r.updateGoogle(updateCtx, plan, state, cfg, resp)
	case providerEntraID:
		r.updateAzure(updateCtx, plan, state, cfg, resp)
	default:
		resp.Diagnostics.AddError("Unsupported provider_name", "provider_name must be GOOGLE or ENTRA_ID.")
	}
}

// Delete dispatches to the Google or Entra ID branch based on provider_name.
//
// **Warning:** deleting a Cloud Identity Provider removes the directory
// integration. Jamf objects scoped by the provider's users/groups lose that
// scoping.
func (r *CloudIdentityProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state CloudIdentityProviderResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro Cloud Identity Provider without ID.")
		return
	}

	switch state.ProviderName.ValueString() {
	case providerGoogle:
		r.deleteGoogle(deleteCtx, state, resp)
	case providerEntraID:
		r.deleteAzure(deleteCtx, state, resp)
	default:
		resp.Diagnostics.AddError("Unsupported provider_name", "provider_name must be GOOGLE or ENTRA_ID.")
	}
}

// discoverProvider reads the Cloud Identity Provider registry to learn the
// provider type for a given id. Used on import, where provider_name is not
// yet known. Returns (providerName, true) on success.
func (r *CloudIdentityProviderResource) discoverProvider(ctx context.Context, id string, resp *resource.ReadResponse) (string, bool) {
	got, err := r.client.GetCloudIdpV1(ctx, id)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			// Import-time: a missing id is a user error (wrong id), not a
			// drifted resource — surface it rather than silently importing
			// into empty state.
			resp.Diagnostics.AddError(
				"Cloud Identity Provider not found",
				"No Jamf Pro Cloud Identity Provider exists with id "+id+". Check the id and try again.",
			)
			return "", false
		}
		resp.Diagnostics.AddError("Error discovering Cloud Identity Provider type", helpers.APIErrorDetail(err))
		return "", false
	}
	if got == nil || got.ProviderName == "" {
		resp.Diagnostics.AddError("Cloud Identity Provider registry missing provider type", "The registry returned no providerName for this id; cannot determine whether to read it as Google or Azure.")
		return "", false
	}
	return got.ProviderName, true
}

// importID extracts the import id from the resource identity.
func (r *CloudIdentityProviderResource) importID(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) (string, bool) {
	if req.Identity == nil {
		resp.Diagnostics.AddError(
			"Missing resource identity",
			"Terraform requested a refresh for this Cloud Identity Provider without existing state or identity data, so the provider cannot determine which configuration to read.",
		)
		return "", false
	}
	var identity cloudIdentityProviderIdentityModel
	resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
	if resp.Diagnostics.HasError() {
		return "", false
	}
	if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing Cloud Identity Provider ID",
			"The resource identity did not include an 'id' attribute, so the provider cannot refresh the configuration.",
		)
		return "", false
	}
	return identity.ID.ValueString(), true
}
