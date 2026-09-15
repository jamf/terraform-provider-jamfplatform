// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_extension_attribute

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildComputerExtensionAttributeInput converts a plan model into the SDK
// payload used for Create and Update. The Pro /v1 PUT is a full replace — every
// omitted optional field is cleared server-side.
//
// The three input_type-gated companion fields (script, directory_service_attribute,
// popup_menu_choices) are sent ONLY when their input_type is active. This keeps a
// transition clean: on e.g. SCRIPT→TEXT the script is not sent, so full-replace
// clears it. It is also what lets popup_menu_choices be Optional+Computed without
// breaking transitions — its UseStateForUnknown plan modifier may carry a prior
// value into the plan, but the gate here drops it whenever input_type is no longer
// POPUP, so a stale foreign value can never reach the wire (which would 400).
// manageExistingData is the WriteOnly value, read from config by the caller (it
// is null in plan/state, so it cannot come from `plan`). isCreate must be true
// only when building the Create payload — see manageExistingDataFor for the
// wire law that governs when the field may be sent.
func buildComputerExtensionAttributeInput(ctx context.Context, plan ComputerExtensionAttributeResourceModel, manageExistingData types.String, isCreate bool) (*pro.ComputerExtensionAttributes, diag.Diagnostics) {
	var diags diag.Diagnostics

	inputType := plan.InputType.ValueString()

	ea := &pro.ComputerExtensionAttributes{
		Name:                          plan.Name.ValueString(),
		DataType:                      plan.DataType.ValueString(),
		InputType:                     inputType,
		InventoryDisplayType:          plan.InventoryDisplay.ValueString(),
		Description:                   helpers.OptionalStringPointer(plan.Description),
		Enabled:                       helpers.OptionalBoolPointer(plan.Enabled),
		LdapExtensionAttributeAllowed: helpers.OptionalBoolPointer(plan.AllowMultipleValues),
	}

	switch inputType {
	case inputTypeScript:
		ea.ScriptContents = helpers.OptionalStringPointer(plan.Script)
		ea.ManageExistingData = manageExistingDataFor(plan.Enabled, manageExistingData, isCreate)
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

// manageExistingDataFor decides whether the SCRIPT-only manageExistingData
// instruction belongs on this request, and with what value. It returns nil when
// the field must be omitted.
//
// Wire law (live-probed 2026-07-28, issue #302 — the field says what to do with
// already-collected inventory values when a SCRIPT EA is *disabled*, so Jamf Pro
// only tolerates it on a request that lands the EA disabled):
//
//	POST (any enabled value)      → must be absent
//	                                400 "[INVALID_CONTENT] manageExistingData: This field
//	                                should be blank for first time CEA creation."
//	PUT landing enabled = true    → must be absent
//	                                400 "[INVALID_CONTENT] manageExistingData: This field
//	                                should be blank if the input type is not 'SCRIPT' and
//	                                enabled value is not false."
//	PUT landing enabled = false   → REQUIRED on the enabled true→false transition
//	                                (400 "This field is required and possible values can
//	                                be [ DELETE, RETAIN ]." when omitted); accepted, and a
//	                                no-op, when the EA is already disabled.
//
// So it is sent on every update that lands enabled = false — which covers the
// transition without needing prior state — and never otherwise. enabled is
// Computed with a true default, so a null/unknown plan value means "enabled".
func manageExistingDataFor(enabled types.Bool, manageExistingData types.String, isCreate bool) *string {
	if isCreate {
		return nil
	}
	if enabled.IsNull() || enabled.IsUnknown() || enabled.ValueBool() {
		return nil
	}
	mode := manageExistingDataDefault
	if v := helpers.OptionalStringPointer(manageExistingData); v != nil {
		mode = *v
	}
	return &mode
}
