// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_extension_attribute

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildMobileDeviceExtensionAttributeInput converts a plan model into the SDK
// payload used for Create and Update. The Pro /v1 PUT is a full replace — every
// omitted optional field is cleared server-side.
//
// The two input_type-gated companion fields (directory_service_attribute,
// popup_menu_choices) are sent ONLY when their input_type is active. This keeps a
// transition clean (e.g. POPUP→TEXT drops the choices so full-replace clears them)
// and is what lets popup_menu_choices be Optional+Computed without breaking
// transitions — its UseStateForUnknown plan modifier may carry a prior value into
// the plan, but the gate here drops it whenever input_type is no longer POPUP, so a
// stale foreign value can never reach the wire (which would 400).
func buildMobileDeviceExtensionAttributeInput(ctx context.Context, plan MobileDeviceExtensionAttributeResourceModel) (*pro.MobileDeviceExtensionAttributes, diag.Diagnostics) {
	var diags diag.Diagnostics

	inputType := plan.InputType.ValueString()

	ea := &pro.MobileDeviceExtensionAttributes{
		Name:                          plan.Name.ValueString(),
		DataType:                      plan.DataType.ValueString(),
		InputType:                     inputType,
		InventoryDisplayType:          plan.InventoryDisplay.ValueString(),
		Description:                   helpers.OptionalStringPointer(plan.Description),
		LdapExtensionAttributeAllowed: helpers.OptionalBoolPointer(plan.AllowMultipleValues),
	}

	switch inputType {
	case inputTypeLDAP:
		ea.LdapAttributeMapping = helpers.OptionalStringPointer(plan.DirectoryServiceAttribute)
	case inputTypePopup:
		if !plan.PopupMenuChoices.IsNull() && !plan.PopupMenuChoices.IsUnknown() {
			choices := []string{}
			diags.Append(plan.PopupMenuChoices.ElementsAs(ctx, &choices, false)...)
			if diags.HasError() {
				return nil, diags
			}
			ea.PopupMenuChoices = &choices
		}
	}

	return ea, diags
}
