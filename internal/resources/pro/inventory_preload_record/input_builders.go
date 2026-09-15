// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildInventoryPreloadRecordInput converts the Terraform plan model into an SDK
// InventoryPreloadRecordV2 payload. The records PUT is full-replace (omit = clear),
// so the body is built entirely from the plan: omitted Optional+Computed scalars have
// already been carried forward from prior state by UseStateForUnknown, and
// helpers.OptionalStringPointer nils both Null and Unknown so genuinely-unset fields
// are dropped from the body. The extension_attributes set is omitted (nil pointer)
// when null or unknown; when known it is always emitted — an empty plan set becomes a
// non-nil empty slice so `[]` clears the collection server-side. Element values
// preserve the null-vs-empty-string distinction (both round-trip on the wire).
func buildInventoryPreloadRecordInput(ctx context.Context, plan InventoryPreloadRecordResourceModel) (*pro.InventoryPreloadRecordV2, diag.Diagnostics) {
	var diags diag.Diagnostics

	input := &pro.InventoryPreloadRecordV2{
		SerialNumber:       plan.SerialNumber.ValueString(),
		DeviceType:         plan.DeviceType.ValueString(),
		Username:           helpers.OptionalStringPointer(plan.Username),
		FullName:           helpers.OptionalStringPointer(plan.FullName),
		EmailAddress:       helpers.OptionalStringPointer(plan.EmailAddress),
		PhoneNumber:        helpers.OptionalStringPointer(plan.PhoneNumber),
		Position:           helpers.OptionalStringPointer(plan.Position),
		Department:         helpers.OptionalStringPointer(plan.Department),
		Building:           helpers.OptionalStringPointer(plan.Building),
		Room:               helpers.OptionalStringPointer(plan.Room),
		PoNumber:           helpers.OptionalStringPointer(plan.PoNumber),
		PoDate:             helpers.OptionalStringPointer(plan.PoDate),
		WarrantyExpiration: helpers.OptionalStringPointer(plan.WarrantyExpiration),
		LeaseExpiration:    helpers.OptionalStringPointer(plan.LeaseExpiration),
		AppleCareID:        helpers.OptionalStringPointer(plan.AppleCareID),
		LifeExpectancy:     helpers.OptionalStringPointer(plan.LifeExpectancy),
		PurchasePrice:      helpers.OptionalStringPointer(plan.PurchasePrice),
		PurchasingContact:  helpers.OptionalStringPointer(plan.PurchasingContact),
		PurchasingAccount:  helpers.OptionalStringPointer(plan.PurchasingAccount),
		BarCode1:           helpers.OptionalStringPointer(plan.BarCode1),
		BarCode2:           helpers.OptionalStringPointer(plan.BarCode2),
		AssetTag:           helpers.OptionalStringPointer(plan.AssetTag),
		Vendor:             helpers.OptionalStringPointer(plan.Vendor),
	}

	if !plan.ExtensionAttributes.IsNull() && !plan.ExtensionAttributes.IsUnknown() {
		var models []extensionAttributeModel
		diags.Append(plan.ExtensionAttributes.ElementsAs(ctx, &models, false)...)
		if diags.HasError() {
			return nil, diags
		}
		eas := make([]pro.InventoryPreloadExtensionAttribute, 0, len(models))
		for _, m := range models {
			eas = append(eas, pro.InventoryPreloadExtensionAttribute{
				Name:  m.Name.ValueString(),
				Value: helpers.OptionalStringPointer(m.Value),
			})
		}
		input.ExtensionAttributes = &eas
	}

	return input, diags
}
