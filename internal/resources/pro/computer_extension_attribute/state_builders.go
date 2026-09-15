// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_extension_attribute

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignComputerExtensionAttributeResourceModel populates a resource model from
// a ComputerExtensionAttributes response. Always sourced from a GET after
// Create/Update. Jamf Pro echoes inapplicable companion fields as empty values
// ("" / [] / false) rather than null, so user-authored string fields are
// reconciled to collapse the empty echo back to null. manage_existing_data is
// write-only — never returned by Jamf Pro — and is deliberately left untouched
// so state retains the configured value.
func assignComputerExtensionAttributeResourceModel(ctx context.Context, state *ComputerExtensionAttributeResourceModel, ea *pro.ComputerExtensionAttributes) diag.Diagnostics {
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
	state.Enabled = helpers.BoolPointerValueOrNull(ea.Enabled)
	state.Script = reconcileScript(ea.ScriptContents, state.Script)
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

// assignComputerExtensionAttributeDataSourceModel populates a data source model.
func assignComputerExtensionAttributeDataSourceModel(ctx context.Context, state *ComputerExtensionAttributeDataSourceModel, ea *pro.ComputerExtensionAttributes) diag.Diagnostics {
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
	state.Enabled = helpers.BoolPointerValueOrNull(ea.Enabled)
	state.Script = helpers.StringPointerValueOrNull(ea.ScriptContents)
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

// reconcileScript collapses Jamf Pro's lossy script normalisation. Jamf appends
// a trailing newline (and may tweak trailing whitespace) to stored script
// contents, so a verbatim round-trip would perpetually diff against a config
// value that omits the trailing newline. When the server value differs from the
// current value only by trailing CR/LF/space, the current value is kept;
// otherwise the (reconciled) server value wins.
func reconcileScript(server *string, current types.String) types.String {
	if server != nil && !current.IsNull() && !current.IsUnknown() {
		if strings.TrimRight(*server, "\r\n ") == strings.TrimRight(current.ValueString(), "\r\n ") {
			return current
		}
	}
	return helpers.ReconcileOptionalStringPointer(server, current)
}

// flattenPopupMenuChoices converts the SDK popup-choices slice into a Set of
// strings. Modelled as a Set because Jamf Pro returns the choices sorted
// alphabetically, not in the submitted order — a List would perpetually diff.
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
