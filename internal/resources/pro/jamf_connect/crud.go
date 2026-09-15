// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.ListJamfConnectConfigProfilesV1   (GET  /v1/jamf-connect/config-profiles; paged)
//       — the list-and-match Read path AND the profile_id → UUID resolver.
//   pro.UpdateJamfConnectConfigProfileV1  (PUT  /v1/jamf-connect/config-profiles/{uuid}; 200 echo)
//       — the sole write; Create and Update both funnel through it.
//
// Not adopted:
//   pro.GetJamfConnectSettingsV1                 — 204 presence probe only (no body); cannot Read a profile.
//   pro.ListJamfConnectDeploymentTasksV1 /
//     pro.RetryJamfConnectDeploymentTasksV1      — operational deployment-task plumbing (possible future action).
//   pro.ListJamfConnectHistoryV1 /
//     pro.CreateJamfConnectHistoryNoteV1         — object history / notes; convention-wide exclusion.
//   pro.ResolveJamfConnectConfigProfileV1ByName /
//     pro.ResolveJamfConnectConfigProfileV1IDByName — used only by the data source's by-name lookup.
//
// Status: current. Last reviewed 2026-06-13.
//
// Update-only adoption. There is no server-side create or delete for what this
// resource manages — a configuration profile is "linked to Jamf Connect"
// because it already carries a Jamf Connect payload. The resource keys on the
// configuration profile's Jamf Pro ID (profile_id) and resolves it to the
// server-minted Jamf Connect profile UUID by listing and matching, then PUTs
// the deployment settings to that UUID. The write is a full replace, so the
// input always emits version when a non-NONE type requires it. Delete is a
// state-only no-op: it leaves Jamf Connect on the profile and the applied
// deployment settings in place — it only stops Terraform managing them.

package jamf_connect

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// resolveByProfileID lists the Jamf Connect-linked configuration profiles and
// returns the one whose Jamf Pro profile ID matches. Returns (nil, nil) when
// no linked profile has that ID — the profile either does not exist or does
// not carry a Jamf Connect payload.
func (r *JamfConnectResource) resolveByProfileID(ctx context.Context, profileID int64) (*pro.LinkedConnectProfile, error) {
	profiles, err := r.client.ListJamfConnectConfigProfilesV1(ctx, nil, "")
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		if profiles[i].ProfileID != nil && int64(*profiles[i].ProfileID) == profileID {
			return &profiles[i], nil
		}
	}
	return nil, nil
}

// profileNotLinkedDetail explains a failed profile_id resolution.
func profileNotLinkedDetail(profileID int64) string {
	return fmt.Sprintf(
		"No Jamf Connect-linked configuration profile has ID %d. The configuration profile must already exist and contain a Jamf Connect payload (it then appears under Settings → Jamf apps → Jamf Connect).",
		profileID,
	)
}

// Create adopts the configuration profile identified by profile_id and applies
// the planned deployment settings. There is no server-side create — the
// profile already exists; Create resolves it to its Jamf Connect UUID and PUTs
// the settings. The 200 echo is authoritative state.
func (r *JamfConnectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan JamfConnectResourceModel
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

	profileID := plan.ProfileID.ValueInt64()
	linked, err := r.resolveByProfileID(createCtx, profileID)
	if err != nil {
		resp.Diagnostics.AddError("Error resolving Jamf Connect configuration profile", helpers.APIErrorDetail(err))
		return
	}
	if linked == nil {
		resp.Diagnostics.AddError("Configuration profile is not linked to Jamf Connect", profileNotLinkedDetail(profileID))
		return
	}

	got, err := r.client.UpdateJamfConnectConfigProfileV1(createCtx, helpers.DerefString(linked.UUID), buildJamfConnectInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Connect deployment settings", helpers.APIErrorDetail(err))
		return
	}

	assignJamfConnectResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfConnectIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "applied Jamf Connect deployment settings", map[string]any{"profile_id": profileID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes state by listing the Jamf Connect-linked profiles and
// matching on profile_id. A missing match means the profile lost its Jamf
// Connect payload (or was deleted), so the resource is removed from state.
func (r *JamfConnectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var state JamfConnectResourceModel
	isImport := req.State.Raw.IsNull()
	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh without existing state or identity data, so the provider cannot determine which profile to read.",
			)
			return
		}
		var identity jamfConnectIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(jamfConnectTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	profileID, ok := resolveStateProfileID(state)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid Jamf Connect identifier",
			"The resource state did not contain a numeric profile_id (or an id to derive it from), so the provider cannot refresh it.",
		)
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	linked, err := r.resolveByProfileID(readCtx, profileID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Connect deployment settings", helpers.APIErrorDetail(err))
		return
	}
	if linked == nil {
		tflog.Info(ctx, "Jamf Connect-linked profile not found, removing from state", map[string]any{"profile_id": profileID})
		resp.State.RemoveResource(ctx)
		return
	}

	assignJamfConnectResourceModel(&state, linked)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfConnectIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update re-applies the deployment settings. profile_id is RequiresReplace, so
// the managed profile never changes here; the UUID is re-resolved from
// profile_id (robust to an out-of-band UUID change) and the settings PUT in a
// single full-replace write. The 200 echo is authoritative state.
func (r *JamfConnectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError(helpers.ProviderNotConfiguredError())
		return
	}

	var plan JamfConnectResourceModel
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

	profileID := plan.ProfileID.ValueInt64()
	linked, err := r.resolveByProfileID(updateCtx, profileID)
	if err != nil {
		resp.Diagnostics.AddError("Error resolving Jamf Connect configuration profile", helpers.APIErrorDetail(err))
		return
	}
	if linked == nil {
		resp.Diagnostics.AddError("Configuration profile is not linked to Jamf Connect", profileNotLinkedDetail(profileID))
		return
	}

	got, err := r.client.UpdateJamfConnectConfigProfileV1(updateCtx, helpers.DerefString(linked.UUID), buildJamfConnectInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Connect deployment settings", helpers.APIErrorDetail(err))
		return
	}

	assignJamfConnectResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, jamfConnectIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated Jamf Connect deployment settings", map[string]any{"profile_id": profileID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a state-only no-op. There is no server-side delete: removing this
// resource does not remove Jamf Connect from the configuration profile and
// does not change the profile — it only stops Terraform managing the
// deployment-and-update settings, which remain as last applied.
func (r *JamfConnectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	tflog.Info(ctx, "jamfplatform_pro_jamf_connect destroyed (state only); Jamf Connect remains on the configuration profile with its current deployment settings")
}

// resolveStateProfileID derives the profile_id from state, falling back to
// parsing the string id (set on import). Returns ok=false when neither yields
// a usable numeric ID.
func resolveStateProfileID(state JamfConnectResourceModel) (int64, bool) {
	if !state.ProfileID.IsNull() && !state.ProfileID.IsUnknown() && state.ProfileID.ValueInt64() != 0 {
		return state.ProfileID.ValueInt64(), true
	}
	if !state.ID.IsNull() && !state.ID.IsUnknown() {
		if id, err := strconv.ParseInt(state.ID.ValueString(), 10, 64); err == nil && id != 0 {
			return id, true
		}
	}
	return 0, false
}
