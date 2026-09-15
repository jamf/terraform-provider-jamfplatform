// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_extension_attribute

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

func TestMobileEAResource_Metadata(t *testing.T) {
	r := NewMobileDeviceExtensionAttributeResource()
	var resp resource.MetadataResponse
	r.(*MobileDeviceExtensionAttributeResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != "jamfplatform_pro_mobile_device_extension_attribute" {
		t.Errorf("unexpected type name %q", resp.TypeName)
	}
}

func TestMobileEAResource_Schema(t *testing.T) {
	r := NewMobileDeviceExtensionAttributeResource()
	var resp resource.SchemaResponse
	r.(*MobileDeviceExtensionAttributeResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	s := resp.Schema
	for _, name := range []string{"id", "name", "description", "data_type", "input_type", "inventory_display", "popup_menu_choices", "directory_service_attribute", "allow_multiple_values", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	// Mobile must NOT carry the script-only fields.
	for _, absent := range []string{"script", "enabled", "manage_existing_data"} {
		if _, ok := s.Attributes[absent]; ok {
			t.Errorf("mobile schema must not have %q", absent)
		}
	}
}

func TestMobileEADataSource_Schema(t *testing.T) {
	d := NewMobileDeviceExtensionAttributeDataSource()
	var resp datasource.SchemaResponse
	d.(*MobileDeviceExtensionAttributeDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
}

func TestMobileEAListResource_Schema(t *testing.T) {
	r := NewMobileDeviceExtensionAttributeListResource()
	var resp list.ListResourceSchemaResponse
	r.(*MobileDeviceExtensionAttributeListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["filter"]; !ok {
		t.Errorf("list schema missing filter attribute")
	}
}

func TestMobileAssignResourceModel_EmptyEchoNormalization(t *testing.T) {
	ea := &pro.MobileDeviceExtensionAttributes{
		ID:                            new("153"),
		Name:                          "zz-text",
		Description:                   new(""),
		DataType:                      "STRING",
		InputType:                     "TEXT",
		InventoryDisplayType:          "GENERAL",
		PopupMenuChoices:              &[]string{},
		LdapAttributeMapping:          new(""),
		LdapExtensionAttributeAllowed: new(false),
	}
	state := &MobileDeviceExtensionAttributeResourceModel{
		Description:               types.StringNull(),
		DirectoryServiceAttribute: types.StringNull(),
	}
	if diags := assignMobileDeviceExtensionAttributeResourceModel(context.Background(), state, ea); diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if !state.Description.IsNull() || !state.DirectoryServiceAttribute.IsNull() || !state.PopupMenuChoices.IsNull() {
		t.Errorf("empty echoes should normalise to null")
	}
}
