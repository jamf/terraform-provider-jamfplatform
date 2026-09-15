// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_extension_attribute

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// TestAssignResourceModel_EmptyEchoNormalization verifies that the empty values
// Jamf Pro echoes for inapplicable companion fields ("" / [] / false) collapse
// to null, so a TEXT EA does not perpetually diff against an omitted config.
func TestAssignResourceModel_EmptyEchoNormalization(t *testing.T) {
	ea := &pro.ComputerExtensionAttributes{
		ID:                            new("1913"),
		Name:                          "zz-text",
		Description:                   new(""),
		DataType:                      "STRING",
		InputType:                     "TEXT",
		InventoryDisplayType:          "GENERAL",
		Enabled:                       new(true),
		ScriptContents:                new(""),
		PopupMenuChoices:              &[]string{},
		LdapAttributeMapping:          new(""),
		LdapExtensionAttributeAllowed: new(false),
		ManageExistingData:            nil,
	}

	state := &ComputerExtensionAttributeResourceModel{
		// description/script/dsa start null (config omitted them).
		Description:               types.StringNull(),
		Script:                    types.StringNull(),
		DirectoryServiceAttribute: types.StringNull(),
	}
	if diags := assignComputerExtensionAttributeResourceModel(context.Background(), state, ea); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	if !state.Description.IsNull() {
		t.Errorf("description: empty echo should be null, got %q", state.Description.ValueString())
	}
	if !state.Script.IsNull() {
		t.Errorf("script: empty echo should be null, got %q", state.Script.ValueString())
	}
	if !state.DirectoryServiceAttribute.IsNull() {
		t.Errorf("directory_service_attribute: empty echo should be null")
	}
	if !state.PopupMenuChoices.IsNull() {
		t.Errorf("popup_menu_choices: empty slice should be null list")
	}
	if state.AllowMultipleValues.ValueBool() {
		t.Errorf("allow_multiple_values should be false")
	}
	// manage_existing_data is WriteOnly — never mapped from the server response,
	// so it stays null in state.
	if !state.ManageExistingData.IsNull() {
		t.Errorf("manage_existing_data must stay null in state (WriteOnly), got %q", state.ManageExistingData.ValueString())
	}
}

// TestReconcileScript_TrailingNewline verifies the server's appended trailing
// newline does not diff against the user's config value.
func TestReconcileScript_TrailingNewline(t *testing.T) {
	// Server appended a trailing newline; current (plan) has none → keep current.
	got := reconcileScript(new("echo hi\n"), types.StringValue("echo hi"))
	if got.ValueString() != "echo hi" {
		t.Errorf("expected current value kept, got %q", got.ValueString())
	}

	// A genuine change wins.
	got = reconcileScript(new("echo bye\n"), types.StringValue("echo hi"))
	if got.ValueString() != "echo bye\n" {
		t.Errorf("expected server value on genuine change, got %q", got.ValueString())
	}

	// Import (current null) takes the server value verbatim.
	got = reconcileScript(new("echo hi\n"), types.StringNull())
	if got.ValueString() != "echo hi\n" {
		t.Errorf("expected verbatim server value on import, got %q", got.ValueString())
	}
}

// TestFlattenPopupMenuChoices verifies the isPopup-keyed empty handling: a
// non-POPUP EA flattens to null (attribute N/A); a POPUP EA flattens an empty
// slice to an empty (non-null) set so an explicit `[]` clear round-trips.
func TestFlattenPopupMenuChoices(t *testing.T) {
	// non-POPUP: always null regardless of the echoed slice.
	got, diags := flattenPopupMenuChoices(context.Background(), nil, false)
	if diags.HasError() || !got.IsNull() {
		t.Errorf("non-popup nil slice should flatten to null")
	}
	src := []string{"a", "b", "c"}
	got, _ = flattenPopupMenuChoices(context.Background(), &src, false)
	if !got.IsNull() {
		t.Errorf("non-popup should flatten to null even with a non-empty slice")
	}

	// POPUP: empty slice (and nil) → empty, non-null set; populated → hydrated.
	got, _ = flattenPopupMenuChoices(context.Background(), nil, true)
	if got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("popup nil slice should flatten to an empty (non-null) set, got %v", got)
	}
	empty := []string{}
	got, _ = flattenPopupMenuChoices(context.Background(), &empty, true)
	if got.IsNull() || len(got.Elements()) != 0 {
		t.Errorf("popup empty slice should flatten to an empty (non-null) set, got %v", got)
	}
	got, _ = flattenPopupMenuChoices(context.Background(), &src, true)
	if got.IsNull() || len(got.Elements()) != 3 {
		t.Errorf("expected 3 choices, got %v", got)
	}
}
