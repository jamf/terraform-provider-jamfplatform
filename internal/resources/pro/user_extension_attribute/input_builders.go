// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_extension_attribute

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildUserExtensionAttributeInput converts a plan model into the SDK Classic
// payload used for Create and Update. The flat schema `input_type` +
// `popup_menu_choices` is folded back into the nested `<input_type>` element.
// ID is omitted on write — Create POSTs to id="0" and Update derives the ID from
// state.
func buildUserExtensionAttributeInput(ctx context.Context, plan UserExtensionAttributeResourceModel) (*proclassic.UserExtensionAttribute, diag.Diagnostics) {
	var diags diag.Diagnostics

	inputType := &proclassic.UserExtensionAttributeInputType{
		Type: helpers.OptionalStringPointer(plan.InputType),
	}

	if !plan.PopupMenuChoices.IsNull() && !plan.PopupMenuChoices.IsUnknown() {
		choices := []string{}
		diags.Append(plan.PopupMenuChoices.ElementsAs(ctx, &choices, false)...)
		if diags.HasError() {
			return nil, diags
		}
		inputType.PopupChoices = &proclassic.UserExtensionAttributeInputTypePopupChoices{Choice: &choices}
	}

	// Description is always emitted (empty when unset): the Classic PUT MERGES
	// top-level fields, so an omitted description would retain the prior value
	// instead of clearing it. Sending an empty <description/> clears it server-side
	// (wire-probed). assignUserExtensionAttributeResourceModel reconciles the
	// echoed "" back to null so a config that omits description stays consistent.
	description := plan.Description.ValueString()

	ea := &proclassic.UserExtensionAttribute{
		Name:        helpers.OptionalStringPointer(plan.Name),
		Description: &description,
		DataType:    helpers.OptionalStringPointer(plan.DataType),
		InputType:   inputType,
	}

	return ea, diags
}
