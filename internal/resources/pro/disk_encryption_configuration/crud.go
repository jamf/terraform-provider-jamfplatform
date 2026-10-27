// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   - GET    /diskencryptionconfigurations
//   - GET    /diskencryptionconfigurations/id/{id}
//   - GET    /diskencryptionconfigurations/name/{name}
//   - POST   /diskencryptionconfigurations/id/0
//   - PUT    /diskencryptionconfigurations/id/{id}
//   - DELETE /diskencryptionconfigurations/id/{id}
// Uniqueness: name, enforced (409), case-, leading-space-, trailing-space- and accent-insensitive. The plan-time collision check lists with proclassic.ListDiskEncryptionConfigurations (collision.go).
//
// Status: current. Last reviewed 2026-08-26.

package disk_encryption_configuration

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro disk encryption configuration. Classic
// POSTs to id="0"; the server allocates the real integer ID and returns it
// in the response body. We then GET to capture server-populated fields
// (`key`, `certificate_type`).
func (r *DiskEncryptionConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan DiskEncryptionConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg DiskEncryptionConfigurationResourceModel
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

	created, err := r.client.CreateDiskEncryptionConfigurationByID(createCtx, "0", buildDiskEncryptionConfigurationInput(plan, irkPasswordFromConfig(&cfg)))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro disk encryption configuration", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing disk encryption configuration ID",
			"Jamf Pro returned 201 Created with no ID; cannot persist state.",
		)
		return
	}
	plan.ID = helpers.StringValueFromIntPtr(created.ID)

	got, err := r.client.GetDiskEncryptionConfigurationByID(createCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro disk encryption configuration", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignDiskEncryptionConfigurationResourceModel(&plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, diskEncryptionConfigurationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro disk encryption configuration", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes Terraform state with the latest disk encryption
// configuration. Import-time refresh sources the ID from the identity
// object so users can `terraform import` by the integer ID.
func (r *DiskEncryptionConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state DiskEncryptionConfigurationResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this disk encryption configuration without existing state or identity data, so the provider cannot determine which configuration to read.",
			)
			return
		}
		var identity diskEncryptionConfigurationIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing disk encryption configuration ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the configuration.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(diskEncryptionConfigurationTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro disk encryption configuration without ID.")
		return
	}

	got, err := r.client.GetDiskEncryptionConfigurationByID(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro disk encryption configuration not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, diskEncryptionConfigurationIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro disk encryption configuration", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignDiskEncryptionConfigurationResourceModel(&state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, diskEncryptionConfigurationIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro disk encryption configuration. Classic
// /diskencryptionconfigurations applies PUT as a partial merge: omitted
// tags preserve, set tags overwrite. Notable wire quirk (audit §2.7):
// an empty `<institutional_recovery_key/>` element is treated as
// "preserve", NOT as "clear" — the cert stays server-side even when the
// user removes the block from their config. We do not attempt to work
// around this; document the limitation in the schema description.
//
// After the PUT we GET to refresh server-computed fields such as
// `key`.
func (r *DiskEncryptionConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan DiskEncryptionConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state DiskEncryptionConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var cfg DiskEncryptionConfigurationResourceModel
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

	// Only include the plaintext `<password>` element on the wire when the
	// user bumped `institutional_recovery_key.password_wo_version`. Otherwise
	// omit so the server retains the existing stored value under Classic's
	// partial-merge semantics.
	var password *string
	if irkPasswordRotated(&plan, &state) {
		password = irkPasswordFromConfig(&cfg)
	}

	if err := r.client.UpdateDiskEncryptionConfigurationByID(updateCtx, plan.ID.ValueString(), buildDiskEncryptionConfigurationInput(plan, password)); err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro disk encryption configuration", helpers.APIErrorDetail(err))
		return
	}

	got, err := r.client.GetDiskEncryptionConfigurationByID(updateCtx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated Jamf Pro disk encryption configuration", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignDiskEncryptionConfigurationResourceModel(&plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, diskEncryptionConfigurationIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro disk encryption configuration.
func (r *DiskEncryptionConfigurationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state DiskEncryptionConfigurationResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro disk encryption configuration without ID.")
		return
	}

	if err := r.client.DeleteDiskEncryptionConfigurationByID(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro disk encryption configuration already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro disk encryption configuration", helpers.APIErrorDetail(err))
	}
}

// irkPasswordFromConfig returns the user-supplied IRK plaintext password
// from the resource's WriteOnly config, or nil if the user did not provide
// one (either the whole IRK block is absent or `password` is null).
func irkPasswordFromConfig(cfg *DiskEncryptionConfigurationResourceModel) *string {
	if cfg == nil || cfg.InstitutionalRecoveryKey == nil {
		return nil
	}
	return helpers.OptionalStringPointer(cfg.InstitutionalRecoveryKey.Password)
}

// irkPasswordRotated reports whether the user bumped the IRK
// `password_wo_version` rotation trigger between state and plan. Treats
// both nil and unequal values as rotation events.
func irkPasswordRotated(plan, state *DiskEncryptionConfigurationResourceModel) bool {
	if plan == nil || plan.InstitutionalRecoveryKey == nil {
		return false
	}
	planWo := plan.InstitutionalRecoveryKey.PasswordWoVersion
	if state == nil || state.InstitutionalRecoveryKey == nil {
		// No prior IRK block in state — treat any planned wo_version as rotation.
		return !planWo.IsNull()
	}
	return !planWo.Equal(state.InstitutionalRecoveryKey.PasswordWoVersion)
}
