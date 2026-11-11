// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignInventoryPreloadRecordResourceModel populates a resource model from an
// InventoryPreloadRecordV2 response. Only overwrites state.ID when rec.ID is non-nil
// so post-create GETs that omit the ID do not clobber the value captured from the
// create response. Every Optional+Computed scalar uses ReconcileOptionalStringPointer
// so an explicit empty string the user set is preserved across refreshes (the wire
// echoes explicit null for unset fields and "" verbatim — no normalisation needed).
// extension_attributes always flattens to a known list: the wire echoes
// `extensionAttributes: []` for a record with no entries (wire-probed 2026-06-10).
// The list is positional and copies the server's order verbatim, with no alignment
// to the prior list, because Jamf Pro stores the entries in the order written and
// keeps repeats (see flattenExtensionAttributes).
func assignInventoryPreloadRecordResourceModel(ctx context.Context, state *InventoryPreloadRecordResourceModel, rec *pro.InventoryPreloadRecordV2) diag.Diagnostics {
	var diags diag.Diagnostics

	if rec.ID != nil {
		state.ID = types.StringValue(*rec.ID)
	}
	state.SerialNumber = types.StringValue(rec.SerialNumber)
	state.DeviceType = types.StringValue(rec.DeviceType)
	state.Username = helpers.ReconcileOptionalStringPointer(rec.Username, state.Username)
	state.FullName = helpers.ReconcileOptionalStringPointer(rec.FullName, state.FullName)
	state.EmailAddress = helpers.ReconcileOptionalStringPointer(rec.EmailAddress, state.EmailAddress)
	state.PhoneNumber = helpers.ReconcileOptionalStringPointer(rec.PhoneNumber, state.PhoneNumber)
	state.Position = helpers.ReconcileOptionalStringPointer(rec.Position, state.Position)
	state.Department = helpers.ReconcileOptionalStringPointer(rec.Department, state.Department)
	state.Building = helpers.ReconcileOptionalStringPointer(rec.Building, state.Building)
	state.Room = helpers.ReconcileOptionalStringPointer(rec.Room, state.Room)
	state.PoNumber = helpers.ReconcileOptionalStringPointer(rec.PoNumber, state.PoNumber)
	state.PoDate = helpers.ReconcileOptionalStringPointer(rec.PoDate, state.PoDate)
	state.WarrantyExpiration = helpers.ReconcileOptionalStringPointer(rec.WarrantyExpiration, state.WarrantyExpiration)
	state.LeaseExpiration = helpers.ReconcileOptionalStringPointer(rec.LeaseExpiration, state.LeaseExpiration)
	state.AppleCareID = helpers.ReconcileOptionalStringPointer(rec.AppleCareID, state.AppleCareID)
	state.LifeExpectancy = helpers.ReconcileOptionalStringPointer(rec.LifeExpectancy, state.LifeExpectancy)
	state.PurchasePrice = helpers.ReconcileOptionalStringPointer(rec.PurchasePrice, state.PurchasePrice)
	state.PurchasingContact = helpers.ReconcileOptionalStringPointer(rec.PurchasingContact, state.PurchasingContact)
	state.PurchasingAccount = helpers.ReconcileOptionalStringPointer(rec.PurchasingAccount, state.PurchasingAccount)
	state.BarCode1 = helpers.ReconcileOptionalStringPointer(rec.BarCode1, state.BarCode1)
	state.BarCode2 = helpers.ReconcileOptionalStringPointer(rec.BarCode2, state.BarCode2)
	state.AssetTag = helpers.ReconcileOptionalStringPointer(rec.AssetTag, state.AssetTag)
	state.Vendor = helpers.ReconcileOptionalStringPointer(rec.Vendor, state.Vendor)

	eaList, eaDiags := flattenExtensionAttributes(ctx, rec.ExtensionAttributes)
	diags.Append(eaDiags...)
	state.ExtensionAttributes = eaList

	return diags
}

// assignInventoryPreloadRecordDataSourceModel populates a data source model from an
// InventoryPreloadRecordV2 response. Data source fields are Computed-only, so nil API
// pointers map straight to null. types.StringPointerValue (not the "" -> null
// collapsing helper) keeps the wire's explicit empty strings verbatim — the endpoint
// stores "" and null distinctly.
func assignInventoryPreloadRecordDataSourceModel(ctx context.Context, state *InventoryPreloadRecordDataSourceModel, rec *pro.InventoryPreloadRecordV2) diag.Diagnostics {
	var diags diag.Diagnostics

	if rec.ID != nil {
		state.ID = types.StringValue(*rec.ID)
	}
	state.SerialNumber = types.StringValue(rec.SerialNumber)
	state.DeviceType = types.StringValue(rec.DeviceType)
	state.Username = types.StringPointerValue(rec.Username)
	state.FullName = types.StringPointerValue(rec.FullName)
	state.EmailAddress = types.StringPointerValue(rec.EmailAddress)
	state.PhoneNumber = types.StringPointerValue(rec.PhoneNumber)
	state.Position = types.StringPointerValue(rec.Position)
	state.Department = types.StringPointerValue(rec.Department)
	state.Building = types.StringPointerValue(rec.Building)
	state.Room = types.StringPointerValue(rec.Room)
	state.PoNumber = types.StringPointerValue(rec.PoNumber)
	state.PoDate = types.StringPointerValue(rec.PoDate)
	state.WarrantyExpiration = types.StringPointerValue(rec.WarrantyExpiration)
	state.LeaseExpiration = types.StringPointerValue(rec.LeaseExpiration)
	state.AppleCareID = types.StringPointerValue(rec.AppleCareID)
	state.LifeExpectancy = types.StringPointerValue(rec.LifeExpectancy)
	state.PurchasePrice = types.StringPointerValue(rec.PurchasePrice)
	state.PurchasingContact = types.StringPointerValue(rec.PurchasingContact)
	state.PurchasingAccount = types.StringPointerValue(rec.PurchasingAccount)
	state.BarCode1 = types.StringPointerValue(rec.BarCode1)
	state.BarCode2 = types.StringPointerValue(rec.BarCode2)
	state.AssetTag = types.StringPointerValue(rec.AssetTag)
	state.Vendor = types.StringPointerValue(rec.Vendor)

	eaList, eaDiags := flattenExtensionAttributes(ctx, rec.ExtensionAttributes)
	diags.Append(eaDiags...)
	state.ExtensionAttributes = eaList

	return diags
}

// expandExtensionAttributeModels converts an SDK extension attribute slice into the
// shared element models. A nil pointer flattens to an empty (not nil) slice so both
// builders produce a known empty collection. types.StringPointerValue keeps the
// wire's null-vs-empty-string value distinction verbatim — both round-trip on the
// wire and the "" -> null collapsing helper would corrupt a planned `value = ""`.
// The server-derived `modifiedName` echo field is not declared on the SDK type and
// is intentionally not modelled.
func expandExtensionAttributeModels(eas *[]pro.InventoryPreloadExtensionAttribute) []extensionAttributeModel {
	if eas == nil {
		return []extensionAttributeModel{}
	}
	models := make([]extensionAttributeModel, 0, len(*eas))
	for _, ea := range *eas {
		models = append(models, extensionAttributeModel{
			Name:  types.StringValue(ea.Name),
			Value: types.StringPointerValue(ea.Value),
		})
	}
	return models
}

// flattenExtensionAttributes flattens SDK extension attributes into a types.List, for
// both the resource and the data source. Always returns a known (possibly empty) list
// so the Computed attribute resolves from Unknown at apply.
//
// The list is positional: element i of state is element i of the response. Wire-probed
// on the EU sandbox, 2026-11-11, through the SDK's Create/Update/Get (the provider's own
// write and read path): Jamf Pro returns the entries exactly as written, on POST and on
// PUT alike. `Z, A, M, A=a, A=a2, N, N, E=""` read back unchanged; a PUT of the same
// eight entries in a different order read back in the new order, and the PUT echo
// matched the GET; two identical `B=1` entries were both kept; the same name with two
// different values was kept twice; an entry with no value and one with `""` were each
// kept as sent. Nothing is merged, deduplicated or re-sorted, so there is no server
// order to align a plan to, and a reorder made in the admin UI or the API surfaces as
// drift. A PUT that omits `extensionAttributes` clears the list, as does `[]` (the
// endpoint is full-replace), which is why the plan always carries the list forward.
func flattenExtensionAttributes(ctx context.Context, eas *[]pro.InventoryPreloadExtensionAttribute) (types.List, diag.Diagnostics) {
	return types.ListValueFrom(ctx, extensionAttributeObjectType, expandExtensionAttributeModels(eas))
}
