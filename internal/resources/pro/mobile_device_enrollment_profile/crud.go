// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   proclassic.CreateMobileDeviceEnrollmentProfileByID   (POST id/0)
//   proclassic.GetMobileDeviceEnrollmentProfileByID
//   proclassic.GetMobileDeviceEnrollmentProfileByName       (data source)
//   proclassic.GetMobileDeviceEnrollmentProfileByInvitation (data source)
//   proclassic.UpdateMobileDeviceEnrollmentProfileByID   (PUT, returns no body — GET after)
//   proclassic.DeleteMobileDeviceEnrollmentProfileByID
//   proclassic.ListMobileDeviceEnrollmentProfiles        (data source / list resource / collision check in collision.go)
//
// Writes are a MERGE (omit=retain, empty=clear); see input_builders.go.
//
// Uniqueness: name, not enforced; duplicates stored; checked only with
// include_unenforced. Classic trims the name on save, so the strict comparison also
// ignores leading and trailing spaces, case and accents.
//
// Status: current. Last reviewed 2026-06-05.

package mobile_device_enrollment_profile

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

func (r *EnrollmentProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan EnrollmentProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, d := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultCreateTimeout, plan.Timeouts.Create)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	createCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	created, err := r.client.CreateMobileDeviceEnrollmentProfileByID(createCtx, "0", buildEnrollmentProfileInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro mobile device enrollment profile", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError("Create response missing ID", "Jamf Pro returned 201 Created with no enrollment profile ID.")
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	got, err := r.client.GetMobileDeviceEnrollmentProfileByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro mobile device enrollment profile", helpers.APIErrorDetail(err))
		return
	}
	assignEnrollmentProfileResourceModel(&plan, got, false)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, enrollmentProfileIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Trace(ctx, "created Jamf Pro mobile device enrollment profile", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EnrollmentProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state EnrollmentProfileResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError("Missing resource identity", "Refresh requested without state or identity data.")
			return
		}
		var identity enrollmentProfileIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError("Missing ID", "The resource identity did not include an 'id'.")
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(enrollmentProfileTimeoutAttributeTypes)
	} else {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	readTimeout, d := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultReadTimeout, state.Timeouts.Read)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro mobile device enrollment profile without ID.")
		return
	}

	hydrating := importHydration(isImport, state.Name)

	got, err := r.client.GetMobileDeviceEnrollmentProfileByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro mobile device enrollment profile not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, enrollmentProfileIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro mobile device enrollment profile", helpers.APIErrorDetail(err))
		return
	}
	assignEnrollmentProfileResourceModel(&state, got, hydrating)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, enrollmentProfileIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *EnrollmentProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan EnrollmentProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, d := helpers.ResolveTimeout(ctx, plan.Timeouts.IsNull(), plan.Timeouts.IsUnknown(), defaultUpdateTimeout, plan.Timeouts.Update)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateCtx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	// PUT returns no body — must GET to refresh server-derived fields.
	if err := r.client.UpdateMobileDeviceEnrollmentProfileByID(updateCtx, plan.ID.ValueString(), buildEnrollmentProfileInput(plan)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro mobile device enrollment profile", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetMobileDeviceEnrollmentProfileByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro mobile device enrollment profile", helpers.APIErrorDetail(err))
		return
	}
	assignEnrollmentProfileResourceModel(&plan, got, false)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, enrollmentProfileIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EnrollmentProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EnrollmentProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, d := helpers.ResolveTimeout(ctx, state.Timeouts.IsNull(), state.Timeouts.IsUnknown(), defaultDeleteTimeout, state.Timeouts.Delete)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteCtx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro mobile device enrollment profile without ID.")
		return
	}

	if err := r.client.DeleteMobileDeviceEnrollmentProfileByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro mobile device enrollment profile already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro mobile device enrollment profile", helpers.APIErrorDetail(err))
	}
}
