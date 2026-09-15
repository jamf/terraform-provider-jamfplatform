// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateSupervisionIdentityV1   (generate path — certificate_data omitted)
//   pro.UploadSupervisionIdentityV1   (import path — certificate_data supplied)
//   pro.GetSupervisionIdentityV1
//   pro.UpdateSupervisionIdentityV1   (rename only — display_name)
//   pro.DeleteSupervisionIdentityV1
//   pro.ListSupervisionIdentitiesV1   (data source / list resource)
//
// pro.DownloadSupervisionIdentityV1 is intentionally NOT used: it returns the
// .p12 with its private key, which must never be exported into Terraform state.
//
// The endpoint carries the `supervision-identities-preview` OpenAPI tag. SDK
// status codes (create 200, upload 201, delete 204) were wire-probed against
// Jamf Pro 11.28.1 on 2026-06-12.
//
// Status: current. Last reviewed 2026-06-12.

package supervision_identity

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a supervision identity. The presence of certificate_data (read
// from req.Config because it is WriteOnly) selects the path: supplied -> import
// the .p12; omitted -> Jamf Pro generates a new identity. Both responses carry
// the full read shape, so state is populated directly from the response.
func (r *SupervisionIdentityResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg SupervisionIdentityResourceModel
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

	var created *pro.SupervisionIdentity
	var err error
	if hasCertificateData(cfg) {
		input, buildErr := buildUploadInput(plan, cfg)
		if buildErr != nil {
			resp.Diagnostics.AddError("Invalid certificate_data", helpers.APIErrorDetail(buildErr))
			return
		}
		created, err = r.client.UploadSupervisionIdentityV1(createCtx, input)
	} else {
		created, err = r.client.CreateSupervisionIdentityV1(createCtx, buildGenerateInput(plan, cfg))
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro supervision identity", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == 0 {
		resp.Diagnostics.AddError(
			"Create response missing supervision identity ID",
			"Jamf Pro returned a success response with no supervision identity ID; cannot persist state.",
		)
		return
	}

	assignSupervisionIdentityResourceModel(&plan, created)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, supervisionIdentityIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro supervision identity", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest identity representation.
// The password and certificate are never reconciled (WriteOnly, never echoed).
func (r *SupervisionIdentityResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SupervisionIdentityResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh without existing state or identity data, so the provider cannot determine which supervision identity to read.",
			)
			return
		}
		var identity supervisionIdentityIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing supervision identity ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the identity.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(supervisionIdentityTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro supervision identity without ID.")
		return
	}

	got, err := r.client.GetSupervisionIdentityV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro supervision identity not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, supervisionIdentityIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro supervision identity", helpers.APIErrorDetail(err))
		return
	}

	assignSupervisionIdentityResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, supervisionIdentityIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update renames a supervision identity. display_name is the only mutable field;
// the password and certificate cannot be changed in place (the schema documents
// that changing them requires replacement). The response re-hydrates the
// read-only computed fields.
func (r *SupervisionIdentityResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SupervisionIdentityResourceModel
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

	got, err := r.client.UpdateSupervisionIdentityV1(updateCtx, plan.ID.ValueString(), buildUpdateInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro supervision identity", helpers.APIErrorDetail(err))
		return
	}
	assignSupervisionIdentityResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, supervisionIdentityIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a supervision identity. Delete is idempotent against a missing
// identity (Jamf Pro reports a not-found that IsNotFoundError covers).
func (r *SupervisionIdentityResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SupervisionIdentityResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro supervision identity without ID.")
		return
	}

	if err := r.client.DeleteSupervisionIdentityV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro supervision identity already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro supervision identity", helpers.APIErrorDetail(err))
	}
}
