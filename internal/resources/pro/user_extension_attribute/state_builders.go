// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_extension_attribute

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignUserExtensionAttributeResourceModel populates a resource model from a
// UserExtensionAttribute response. The nested `<input_type>` element is
// flattened back to `input_type` + `popup_menu_choices`. state.ID is only
// overwritten when the API ID is non-nil so a transient GET that drops the ID
// does not clobber the ID already persisted from Create.
func assignUserExtensionAttributeResourceModel(ctx context.Context, state *UserExtensionAttributeResourceModel, ea *proclassic.UserExtensionAttribute) diag.Diagnostics {
	var diags diag.Diagnostics
	if ea == nil {
		return diags
	}

	if ea.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(ea.ID)
	}
	if ea.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(ea.Name)
	}
	state.Description = helpers.ReconcileOptionalStringPointer(ea.Description, state.Description)
	if ea.DataType != nil {
		state.DataType = helpers.StringPointerValueOrNull(ea.DataType)
	}

	inputType, popupChoices := flattenInputType(ctx, ea.InputType)
	state.InputType = inputType
	choices, choicesDiags := popupChoices()
	diags.Append(choicesDiags...)
	if diags.HasError() {
		return diags
	}
	state.PopupMenuChoices = choices

	return diags
}

// assignUserExtensionAttributeDataSourceModel populates a data source model.
func assignUserExtensionAttributeDataSourceModel(ctx context.Context, state *UserExtensionAttributeDataSourceModel, ea *proclassic.UserExtensionAttribute) diag.Diagnostics {
	var diags diag.Diagnostics
	if ea == nil {
		return diags
	}

	if ea.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(ea.ID)
	}
	if ea.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(ea.Name)
	}
	state.Description = helpers.StringPointerValueOrNull(ea.Description)
	if ea.DataType != nil {
		state.DataType = helpers.StringPointerValueOrNull(ea.DataType)
	}

	inputType, popupChoices := flattenInputType(ctx, ea.InputType)
	state.InputType = inputType
	choices, choicesDiags := popupChoices()
	diags.Append(choicesDiags...)
	if diags.HasError() {
		return diags
	}
	state.PopupMenuChoices = choices

	return diags
}

// flattenInputType unpacks the nested Classic `<input_type>` element into the
// flat input_type string and a deferred popup-choices builder (so the caller can
// collect its diagnostics). Returns a null List for absent or empty choices.
func flattenInputType(ctx context.Context, it *proclassic.UserExtensionAttributeInputType) (types.String, func() (types.List, diag.Diagnostics)) {
	if it == nil {
		return types.StringNull(), func() (types.List, diag.Diagnostics) {
			return types.ListNull(types.StringType), nil
		}
	}

	inputType := helpers.StringPointerValueOrNull(it.Type)

	return inputType, func() (types.List, diag.Diagnostics) {
		var diags diag.Diagnostics
		if it.PopupChoices == nil || it.PopupChoices.Choice == nil || len(*it.PopupChoices.Choice) == 0 {
			return types.ListNull(types.StringType), diags
		}
		list, d := types.ListValueFrom(ctx, types.StringType, *it.PopupChoices.Choice)
		diags.Append(d...)
		return list, diags
	}
}
