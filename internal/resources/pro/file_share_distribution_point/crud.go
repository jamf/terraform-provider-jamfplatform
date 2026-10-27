// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateDistributionPointV1        (POST; returns id+href, GET-after to hydrate)
//   pro.GetDistributionPointV1
//   pro.PatchDistributionPointV1         (PATCH; merge — omitting a field preserves its stored value)
//   pro.DeleteDistributionPointV1
//   pro.ListDistributionPointsV1         (data source / list resource / collision check in collision.go)
//   pro.ResolveDistributionPointV1ByName (data source name lookup)
//
// Not adopted:
//   pro.UpdateDistributionPointV1          — PUT is full-replace; it demands the AFP/SMB
//                                            account passwords on every write, which is
//                                            incompatible with WriteOnly + _wo_version
//                                            rotation. PATCH (merge) is used instead.
//   pro.DeleteMultipleDistributionPointsV1 — bulk delete; no Terraform analogue.
//   pro.ApplyDistributionPointV1           — create-or-replace-by-name convenience.
//   pro.*DistributionPointHistory*         — object history (convention-wide exclusion).
//   ProClassic /JSSResource/distributionpoints — legacy; its passwords carry masked
//                                            `*_password_sha256` echoes. The Pro v1
//                                            endpoint's writeOnly passwords avoid that.
//
// Uniqueness: name, enforced (400 DUPLICATE_FIELD), case-, surrounding-space- and accent-insensitive.
//
// Status: current. Last reviewed 2026-06-11.

package file_share_distribution_point

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro file share distribution point. The POST
// returns only the new ID, so we GET afterwards to hydrate server-defaulted
// fields. All three plaintext passwords are sourced from req.Config (they are
// WriteOnly) and sent on create.
func (r *FileShareDistributionPointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan FileShareDistributionPointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg FileShareDistributionPointResourceModel
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

	input := buildFileShareDistributionPointInput(
		plan,
		helpers.OptionalStringPointer(cfg.ReadWritePassword),
		helpers.OptionalStringPointer(cfg.ReadOnlyPassword),
		helpers.OptionalStringPointer(cfg.HTTPSPassword),
	)

	created, err := r.client.CreateDistributionPointV1(createCtx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro file share distribution point", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	if created == nil || created.ID == "" {
		resp.Diagnostics.AddError(
			"Create response missing distribution point ID",
			"Jamf Pro returned a create response with no distribution point ID; cannot persist state.",
		)
		return
	}
	plan.ID = types.StringValue(created.ID)

	got, err := r.client.GetDistributionPointV1(createCtx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro file share distribution point", helpers.APIErrorDetail(err))
		return
	}
	assignFileShareDistributionPointResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, fileShareDistributionPointIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro file share distribution point", map[string]any{"id": created.ID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state. Import-time refresh (req.State.Raw is
// null) sources the ID from the identity object so users can import by ID.
func (r *FileShareDistributionPointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state FileShareDistributionPointResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this distribution point without existing state or identity data, so the provider cannot determine which distribution point to read.",
			)
			return
		}
		var identity fileShareDistributionPointIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing distribution point ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the distribution point.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(fileShareDistributionPointTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro file share distribution point without ID.")
		return
	}

	got, err := r.client.GetDistributionPointV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro file share distribution point not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, fileShareDistributionPointIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro file share distribution point", helpers.APIErrorDetail(err))
		return
	}

	assignFileShareDistributionPointResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, fileShareDistributionPointIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro file share distribution point. The endpoint
// merges, so any field the user omitted is preserved (UseStateForUnknown has
// already carried the prior value into the plan). Each plaintext password is
// re-sent only when its `*_wo_version` rotation trigger changed; otherwise it
// is omitted and Jamf Pro retains the stored value.
func (r *FileShareDistributionPointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FileShareDistributionPointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state FileShareDistributionPointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg FileShareDistributionPointResourceModel
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

	input := buildFileShareDistributionPointInput(
		plan,
		passwordOnRotation(plan.ReadWritePasswordWoVer, state.ReadWritePasswordWoVer, cfg.ReadWritePassword),
		passwordOnRotation(plan.ReadOnlyPasswordWoVersion, state.ReadOnlyPasswordWoVersion, cfg.ReadOnlyPassword),
		passwordOnRotation(plan.HTTPSPasswordWoVersion, state.HTTPSPasswordWoVersion, cfg.HTTPSPassword),
	)

	got, err := r.client.PatchDistributionPointV1(updateCtx, plan.ID.ValueString(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro file share distribution point", helpers.APIErrorDetail(err))
		return
	}
	assignFileShareDistributionPointResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, fileShareDistributionPointIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// passwordOnRotation returns the configured plaintext password only when the
// `*_wo_version` rotation trigger changed between state and plan; otherwise nil
// so the merge update omits the password and Jamf Pro retains the stored value.
func passwordOnRotation(planVer, stateVer types.Int64, cfgPassword types.String) *string {
	if planVer.Equal(stateVer) {
		return nil
	}
	return helpers.OptionalStringPointer(cfgPassword)
}

// Delete removes a Jamf Pro file share distribution point.
func (r *FileShareDistributionPointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FileShareDistributionPointResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro file share distribution point without ID.")
		return
	}

	if err := r.client.DeleteDistributionPointV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro file share distribution point already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro file share distribution point", helpers.APIErrorDetail(err))
	}
}
