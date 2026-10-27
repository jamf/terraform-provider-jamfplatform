// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateMobileDeviceProvisioningProfileByID   (POST id/0)
//   proclassic.GetMobileDeviceProvisioningProfileByID
//   proclassic.GetMobileDeviceProvisioningProfileByName     (data source name lookup)
//   proclassic.GetMobileDeviceProvisioningProfileByUUID     (data source uuid lookup)
//   proclassic.DeleteMobileDeviceProvisioningProfileByID
//   proclassic.ListMobileDeviceProvisioningProfiles         (data source / list resource / collision check in collision.go)
//
// Uniqueness: name, enforced (409), case-, leading- and trailing-space-insensitive, accent-sensitive.
// The embedded profile UUID is not a key: two profiles with one blob are accepted under different names.
//
// UpdateMobileDeviceProvisioningProfileByID is intentionally NOT used: once a
// profile carries a blob, every PUT returns HTTP 500 (wire-probed). All
// user-writable attributes are RequiresReplace, so Update never needs to write.
//
// Status: current. Last reviewed 2026-10-25.

package mobile_device_provisioning_profile

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new provisioning profile. Classic POSTs to id="0"; the server
// allocates the real integer ID and returns it in the response body, then we GET
// to hydrate the server-parsed uuid/date fields.
func (r *ProvisioningProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ProvisioningProfileResourceModel
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

	created, err := r.client.CreateMobileDeviceProvisioningProfileByID(createCtx, "0", buildProvisioningProfileInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro mobile device provisioning profile", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing provisioning profile ID",
			"Jamf Pro returned 201 Created with no provisioning profile ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	got, err := r.client.GetMobileDeviceProvisioningProfileByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro mobile device provisioning profile", helpers.APIErrorDetail(err))
		return
	}
	assignProvisioningProfileResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, provisioningProfileIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro mobile device provisioning profile", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest profile representation.
func (r *ProvisioningProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ProvisioningProfileResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh without existing state or identity data, so the provider cannot determine which provisioning profile to read.",
			)
			return
		}
		var identity provisioningProfileIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing provisioning profile ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the profile.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(provisioningProfileTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro mobile device provisioning profile without ID.")
		return
	}

	got, err := r.client.GetMobileDeviceProvisioningProfileByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro mobile device provisioning profile not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, provisioningProfileIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro mobile device provisioning profile", helpers.APIErrorDetail(err))
		return
	}

	assignProvisioningProfileResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, provisioningProfileIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update refreshes state without issuing an SDK write. All user-writable
// attributes (name, display_name, profile_data) are RequiresReplace because a
// blob-bearing profile rejects every PUT with HTTP 500; the only changes that
// reach Update are timeouts, which require no API call. We GET to re-hydrate
// the server-derived computed fields and persist the plan.
func (r *ProvisioningProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ProvisioningProfileResourceModel
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

	got, err := r.client.GetMobileDeviceProvisioningProfileByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro mobile device provisioning profile", helpers.APIErrorDetail(err))
		return
	}
	assignProvisioningProfileResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, provisioningProfileIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a provisioning profile. Delete is synchronous (200 OK) and
// idempotent against a missing profile.
func (r *ProvisioningProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProvisioningProfileResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro mobile device provisioning profile without ID.")
		return
	}

	if err := r.client.DeleteMobileDeviceProvisioningProfileByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro mobile device provisioning profile already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro mobile device provisioning profile", helpers.APIErrorDetail(err))
	}
}
