// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateAppRequestFormInputFieldV1   (POST — returns HTTP 200 with a complete echo)
//   pro.GetAppRequestFormInputFieldV1
//   pro.UpdateAppRequestFormInputFieldV1   (PUT — full-replace, returns the echo)
//   pro.DeleteAppRequestFormInputFieldV1   (DELETE — 204)
//   pro.ListAppRequestFormInputFieldsV1    (data source by title / list resource / collision check in collision.go)
//
// A normal id-keyed Pro CRUD record. Create POSTs the field and the server returns a
// complete echo (id + title + description + priority) — state is seeded directly from it,
// no GET-after-create needed (wire-probed 2026-06-13). Update is a full-replace PUT that
// also echoes the record. `title` is mutable in place (not unique on the server). `priority`
// is user-authored: the server stores it verbatim and sorts the form ascending; writes are
// cross-field independent (mutating one field does not re-sequence the others), so no
// reorder call or provider mutex is required. GET on a deleted id returns 404 (code
// INVALID_ID) — matched by helpers.IsNotFoundError.
//
// Uniqueness: title, not enforced; duplicates stored; checked only with
// include_unenforced. The strict comparison ignores case, trailing spaces and
// accents.
//
// Status: current. Last reviewed 2026-06-13.

package app_request_form_field

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro App Request form field. The POST echo is complete, so it
// seeds state directly.
func (r *AppRequestFormFieldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AppRequestFormFieldResourceModel
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

	created, err := r.client.CreateAppRequestFormInputFieldV1(createCtx, buildAppRequestFormFieldInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro App Request form field", helpers.APIErrorDetail(err))
		return
	}
	if created == nil || created.ID == nil {
		resp.Diagnostics.AddError(
			"Create response missing App Request form field ID",
			"Jamf Pro returned a success response with no form field ID; cannot persist state.",
		)
		return
	}
	assignAppRequestFormFieldResourceModel(&plan, created)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appRequestFormFieldIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro App Request form field", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest App Request form field representation.
func (r *AppRequestFormFieldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AppRequestFormFieldResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this App Request form field without existing state or identity data, so the provider cannot determine which record to read.",
			)
			return
		}
		var identity appRequestFormFieldIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing App Request form field ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the App Request form field.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(appRequestFormFieldTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro App Request form field without ID.")
		return
	}

	got, err := r.client.GetAppRequestFormInputFieldV1(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro App Request form field not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appRequestFormFieldIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro App Request form field", helpers.APIErrorDetail(err))
		return
	}

	assignAppRequestFormFieldResourceModel(&state, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appRequestFormFieldIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro App Request form field. The PUT is a full-replace and echoes
// the stored record.
func (r *AppRequestFormFieldResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AppRequestFormFieldResourceModel
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

	got, err := r.client.UpdateAppRequestFormInputFieldV1(updateCtx, plan.ID.ValueString(), buildAppRequestFormFieldInput(plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro App Request form field", helpers.APIErrorDetail(err))
		return
	}
	assignAppRequestFormFieldResourceModel(&plan, got)

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, appRequestFormFieldIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro App Request form field.
func (r *AppRequestFormFieldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AppRequestFormFieldResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro App Request form field without ID.")
		return
	}

	if err := r.client.DeleteAppRequestFormInputFieldV1(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro App Request form field already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro App Request form field", helpers.APIErrorDetail(err))
	}
}
