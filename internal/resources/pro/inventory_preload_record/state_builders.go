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
// extension_attributes always flattens to a known set: the wire echoes
// `extensionAttributes: []` for a record with no entries (wire-probed 2026-06-10).
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

	eaSet, eaDiags := flattenExtensionAttributesSet(ctx, rec.ExtensionAttributes)
	diags.Append(eaDiags...)
	state.ExtensionAttributes = eaSet

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

	eaList, eaDiags := flattenExtensionAttributesList(ctx, rec.ExtensionAttributes)
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

// flattenExtensionAttributesSet flattens SDK extension attributes into the resource
// model's types.Set. Always returns a known (possibly empty) set so the Computed
// attribute resolves from Unknown at apply.
func flattenExtensionAttributesSet(ctx context.Context, eas *[]pro.InventoryPreloadExtensionAttribute) (types.Set, diag.Diagnostics) {
	return types.SetValueFrom(ctx, extensionAttributeObjectType, expandExtensionAttributeModels(eas))
}

// flattenExtensionAttributesList flattens SDK extension attributes into the data
// source model's types.List.
func flattenExtensionAttributesList(ctx context.Context, eas *[]pro.InventoryPreloadExtensionAttribute) (types.List, diag.Diagnostics) {
	return types.ListValueFrom(ctx, extensionAttributeObjectType, expandExtensionAttributeModels(eas))
}
