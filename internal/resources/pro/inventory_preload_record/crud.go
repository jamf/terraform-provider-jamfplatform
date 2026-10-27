// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.CreateInventoryPreloadRecordV2
//   pro.GetInventoryPreloadRecordV2
//   pro.UpdateInventoryPreloadRecordV2
//   pro.DeleteInventoryPreloadRecordV2
//   pro.ListInventoryPreloadRecordsV2 (list resource / collision check in collision.go)
//   pro.ResolveInventoryPreloadRecordV2BySerialNumber (data source serial_number lookup)
//
// Not adopted:
//   CSV endpoints — UploadInventoryPreloadCsvV2, ValidateInventoryPreloadCsvV2,
//     DownloadInventoryPreloadCsvV2, DownloadInventoryPreloadCsvTemplateV2,
//     ExportInventoryPreloadV2. This resource manages records individually via the
//     JSON record endpoints; the CSV path is the UI's bulk workflow.
//   History endpoints — ListInventoryPreloadHistoryV2, CreateInventoryPreloadHistoryNoteV2.
//   pro.ListInventoryPreloadExtensionAttributeColumnsV2 — enumerates the distinct
//     extension attribute names already present in preload records (derived column
//     metadata), NOT an extension-attribute catalog; useless as a validation source.
//   pro.DeleteAllInventoryPreloadRecordsV2 — NEVER called anywhere in this provider:
//     it is a tenant-wide wipe of every Inventory Preload record (the UI's
//     "Delete Data" button).
//
// The records PUT is full-replace (omit = clear) and returns a 200 with the full
// record body, so Update sources state from the PUT echo directly — no GET-after-write
// needed (wire-probed 2026-06-10; the echo is not lossy).
//
// Uniqueness: serial_number, enforced (400 DUPLICATE_FIELD) across both device types, case-, trailing-space- and accent-insensitive; a leading space is distinct.
//
// Status: current. Last reviewed 2026-06-10.

package inventory_preload_record

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// Create creates a new Jamf Pro Inventory Preload record.
func (r *InventoryPreloadRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var plan InventoryPreloadRecordResourceModel
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

	input, diags := buildInventoryPreloadRecordInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	createResp, err := r.client.CreateInventoryPreloadRecordV2(createCtx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Jamf Pro inventory preload record", helpers.APIErrorDetail(err)+r.collisions.CreateConflictHint(err, resourceTypeName))
		return
	}

	plan.ID = types.StringValue(createResp.ID)

	got, err := r.client.GetInventoryPreloadRecordV2(createCtx, createResp.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading created Jamf Pro inventory preload record", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignInventoryPreloadRecordResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, inventoryPreloadRecordIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "created Jamf Pro inventory preload record", map[string]any{"id": createResp.ID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest record representation.
func (r *InventoryPreloadRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	defer r.collisions.NoteManagedState(ctx, resourceTypeName, &resp.State)
	var state InventoryPreloadRecordResourceModel
	isImport := req.State.Raw.IsNull()

	if isImport {
		if req.Identity == nil {
			resp.Diagnostics.AddError(
				"Missing resource identity",
				"Terraform requested a refresh for this inventory preload record without existing state or identity data, so the provider cannot determine which record to read.",
			)
			return
		}
		var identity inventoryPreloadRecordIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if identity.ID.IsNull() || identity.ID.IsUnknown() || identity.ID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing inventory preload record ID",
				"The resource identity did not include an 'id' attribute, so the provider cannot refresh the inventory preload record.",
			)
			return
		}
		state.ID = identity.ID
		state.Timeouts = helpers.NewResourceTimeoutsNullValue(inventoryPreloadRecordTimeoutAttributeTypes)
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
		resp.Diagnostics.AddError("Missing ID", "Cannot read Jamf Pro inventory preload record without ID.")
		return
	}

	got, err := r.client.GetInventoryPreloadRecordV2(readCtx, state.ID.ValueString())
	if err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro inventory preload record not found, removing from state", map[string]any{"id": state.ID.ValueString()})
			resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, inventoryPreloadRecordIdentityModel{ID: state.ID})...)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Jamf Pro inventory preload record", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignInventoryPreloadRecordResourceModel(ctx, &state, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, inventoryPreloadRecordIdentityModel{ID: state.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates a Jamf Pro Inventory Preload record. State is sourced from the
// update response body — the endpoint echoes the full record, so no follow-up read
// is required (see the package annotation block).
func (r *InventoryPreloadRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan InventoryPreloadRecordResourceModel
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

	input, diags := buildInventoryPreloadRecordInput(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.UpdateInventoryPreloadRecordV2(updateCtx, plan.ID.ValueString(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Jamf Pro inventory preload record", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignInventoryPreloadRecordResourceModel(ctx, &plan, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(helpers.SetIdentity(ctx, resp.Identity, inventoryPreloadRecordIdentityModel{ID: plan.ID})...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jamf Pro Inventory Preload record. The delete is synchronous
// (204; a follow-up read returns a true 404) and an already-absent record is treated
// as success.
func (r *InventoryPreloadRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state InventoryPreloadRecordResourceModel
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
		resp.Diagnostics.AddError("Missing ID", "Cannot delete Jamf Pro inventory preload record without ID.")
		return
	}

	if err := r.client.DeleteInventoryPreloadRecordV2(deleteCtx, state.ID.ValueString()); err != nil {
		if helpers.IsNotFoundError(err) {
			tflog.Info(ctx, "Jamf Pro inventory preload record already removed", map[string]any{"id": state.ID.ValueString()})
			return
		}
		resp.Diagnostics.AddError("Error deleting Jamf Pro inventory preload record", helpers.APIErrorDetail(err))
	}
}
