// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_extension_attribute

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestUserEAResource_Metadata(t *testing.T) {
	r := NewUserExtensionAttributeResource()
	var resp resource.MetadataResponse
	r.(*UserExtensionAttributeResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != "jamfplatform_pro_user_extension_attribute" {
		t.Errorf("unexpected type name %q", resp.TypeName)
	}
}

func TestUserEAResource_Schema(t *testing.T) {
	r := NewUserExtensionAttributeResource()
	var resp resource.SchemaResponse
	r.(*UserExtensionAttributeResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range []string{"id", "name", "description", "data_type", "input_type", "popup_menu_choices", "timeouts"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

func TestUserEADataSource_Schema(t *testing.T) {
	d := NewUserExtensionAttributeDataSource()
	var resp datasource.SchemaResponse
	d.(*UserExtensionAttributeDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
}

func TestUserEAListResource_Schema(t *testing.T) {
	r := NewUserExtensionAttributeListResource()
	var resp list.ListResourceSchemaResponse
	r.(*UserExtensionAttributeListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["filter"]; !ok {
		t.Errorf("list schema missing filter attribute")
	}
}

// configFromAttrs builds a tfsdk.Config from the real resource schema.
func configFromAttrs(t *testing.T, attrs map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	r := NewUserExtensionAttributeResource()
	var sr resource.SchemaResponse
	r.(*UserExtensionAttributeResource).Schema(context.Background(), resource.SchemaRequest{}, &sr)
	objType := sr.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	values := map[string]tftypes.Value{}
	for name, typ := range objType.AttributeTypes {
		if v, ok := attrs[name]; ok {
			values[name] = v
			continue
		}
		values[name] = tftypes.NewValue(typ, nil)
	}
	return tfsdk.Config{Schema: sr.Schema, Raw: tftypes.NewValue(objType, values)}
}

func TestInputTypeValidator_User(t *testing.T) {
	str := func(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
	popup := func(vs ...string) tftypes.Value {
		elems := make([]tftypes.Value, len(vs))
		for i, v := range vs {
			elems[i] = tftypes.NewValue(tftypes.String, v)
		}
		return tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, elems)
	}
	tests := []struct {
		name    string
		attrs   map[string]tftypes.Value
		wantErr bool
	}{
		{"text bare", map[string]tftypes.Value{"input_type": str("Text Field")}, false},
		{"popup with choices", map[string]tftypes.Value{"input_type": str("Pop-up Menu"), "popup_menu_choices": popup("a", "b")}, false},
		{"text with choices forbidden", map[string]tftypes.Value{"input_type": str("Text Field"), "popup_menu_choices": popup("a")}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := resource.ValidateConfigRequest{Config: configFromAttrs(t, tc.attrs)}
			var resp resource.ValidateConfigResponse
			inputTypeConfigValidator{}.ValidateResource(context.Background(), req, &resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Errorf("wantErr=%v, got=%v (diags: %v)", tc.wantErr, got, resp.Diagnostics)
			}
		})
	}
}

// TestAssignResourceModel_FlattenInputType verifies the nested Classic
// <input_type> element flattens to input_type + popup_menu_choices.
func TestAssignResourceModel_FlattenInputType(t *testing.T) {
	ea := &proclassic.UserExtensionAttribute{
		ID:          new(81),
		Name:        new("zz-popup"),
		Description: new(""),
		DataType:    new("String"),
		InputType: &proclassic.UserExtensionAttributeInputType{
			Type:         new("Pop-up Menu"),
			PopupChoices: &proclassic.UserExtensionAttributeInputTypePopupChoices{Choice: &[]string{"Red", "Green", "Blue"}},
		},
	}
	state := &UserExtensionAttributeResourceModel{Description: types.StringNull()}
	if diags := assignUserExtensionAttributeResourceModel(context.Background(), state, ea); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if state.ID.ValueString() != "81" {
		t.Errorf("id: got %q", state.ID.ValueString())
	}
	if state.InputType.ValueString() != "Pop-up Menu" {
		t.Errorf("input_type: got %q", state.InputType.ValueString())
	}
	if state.PopupMenuChoices.IsNull() || len(state.PopupMenuChoices.Elements()) != 3 {
		t.Errorf("popup_menu_choices: expected 3 ordered choices, got %v", state.PopupMenuChoices)
	}
	if !state.Description.IsNull() {
		t.Errorf("description: empty echo should be null")
	}
}

// TestAssignResourceModel_TextHasNoChoices verifies a Text Field EA flattens to
// a null popup_menu_choices list.
func TestAssignResourceModel_TextHasNoChoices(t *testing.T) {
	ea := &proclassic.UserExtensionAttribute{
		ID:        new(63),
		Name:      new("Date EA"),
		DataType:  new("Date"),
		InputType: &proclassic.UserExtensionAttributeInputType{Type: new("Text Field")},
	}
	state := &UserExtensionAttributeResourceModel{}
	if diags := assignUserExtensionAttributeResourceModel(context.Background(), state, ea); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !state.PopupMenuChoices.IsNull() {
		t.Errorf("text field must have null popup_menu_choices")
	}
}
