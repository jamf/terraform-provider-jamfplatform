// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_extension_attribute

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignMobileDeviceExtensionAttributeResourceModel populates a resource model
// from a MobileDeviceExtensionAttributes response. Always sourced from a GET
// after Create/Update. Jamf Pro echoes inapplicable companion fields as empty
// values ("" / [] / false) rather than null, so user-authored string fields are
// reconciled to collapse the empty echo back to null.
func assignMobileDeviceExtensionAttributeResourceModel(ctx context.Context, state *MobileDeviceExtensionAttributeResourceModel, ea *pro.MobileDeviceExtensionAttributes) diag.Diagnostics {
	var diags diag.Diagnostics
	if ea == nil {
		return diags
	}

	state.ID = helpers.StringPointerValueOrNull(ea.ID)
	state.Name = types.StringValue(ea.Name)
	state.Description = helpers.ReconcileOptionalStringPointer(ea.Description, state.Description)
	state.DataType = types.StringValue(ea.DataType)
	state.InputType = types.StringValue(ea.InputType)
	state.InventoryDisplay = types.StringValue(ea.InventoryDisplayType)
	state.DirectoryServiceAttribute = helpers.ReconcileOptionalStringPointer(ea.LdapAttributeMapping, state.DirectoryServiceAttribute)
	state.AllowMultipleValues = helpers.BoolPointerValueOrNull(ea.LdapExtensionAttributeAllowed)

	choices, choicesDiags := flattenPopupMenuChoices(ctx, ea.PopupMenuChoices, ea.InputType == inputTypePopup)
	diags.Append(choicesDiags...)
	if diags.HasError() {
		return diags
	}
	state.PopupMenuChoices = choices

	return diags
}

// assignMobileDeviceExtensionAttributeDataSourceModel populates a data source model.
func assignMobileDeviceExtensionAttributeDataSourceModel(ctx context.Context, state *MobileDeviceExtensionAttributeDataSourceModel, ea *pro.MobileDeviceExtensionAttributes) diag.Diagnostics {
	var diags diag.Diagnostics
	if ea == nil {
		return diags
	}

	state.ID = helpers.StringPointerValueOrNull(ea.ID)
	state.Name = types.StringValue(ea.Name)
	state.Description = helpers.StringPointerValueOrNull(ea.Description)
	state.DataType = types.StringValue(ea.DataType)
	state.InputType = types.StringValue(ea.InputType)
	state.InventoryDisplay = types.StringValue(ea.InventoryDisplayType)
	state.DirectoryServiceAttribute = helpers.StringPointerValueOrNull(ea.LdapAttributeMapping)
	state.AllowMultipleValues = helpers.BoolPointerValueOrNull(ea.LdapExtensionAttributeAllowed)

	choices, choicesDiags := flattenPopupMenuChoices(ctx, ea.PopupMenuChoices, ea.InputType == inputTypePopup)
	diags.Append(choicesDiags...)
	if diags.HasError() {
		return diags
	}
	state.PopupMenuChoices = choices

	return diags
}

// flattenPopupMenuChoices converts the SDK popup-choices slice into a Set of
// strings. Modelled as a Set because Jamf Pro returns the choices sorted
// alphabetically, not in submitted order — a List would perpetually diff.
//
// isPopup keys the empty-slice handling. Jamf Pro echoes `[]` both for a POPUP EA
// with no choices AND for every non-POPUP input type, but those mean different
// things: for a non-POPUP EA the attribute is not applicable → null; for a POPUP EA
// an empty `[]` is a real (configured or cleared) value and must round-trip as an
// empty set, not null — otherwise an explicit `popup_menu_choices = []` (the clear
// mechanism) would plan `[]` but read back null and trip "inconsistent result after
// apply".
func flattenPopupMenuChoices(ctx context.Context, src *[]string, isPopup bool) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !isPopup {
		return types.SetNull(types.StringType), diags
	}
	elems := []string{}
	if src != nil {
		elems = *src
	}
	set, d := types.SetValueFrom(ctx, types.StringType, elems)
	diags.Append(d...)
	return set, diags
}
