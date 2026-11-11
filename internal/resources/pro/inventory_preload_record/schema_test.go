// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// optionalComputedScalars enumerates the 21 Optional+Computed full-replace string
// attributes on the resource schema.
var optionalComputedScalars = []string{
	"username", "full_name", "email_address", "phone_number", "position",
	"department", "building", "room", "po_number", "po_date",
	"warranty_expiration", "lease_expiration", "apple_care_id", "life_expectancy",
	"purchase_price", "purchasing_contact", "purchasing_account",
	"bar_code_1", "bar_code_2", "asset_tag", "vendor",
}

func TestInventoryPreloadRecordResource_Metadata(t *testing.T) {
	r := NewInventoryPreloadRecordResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*InventoryPreloadRecordResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_inventory_preload_record" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_inventory_preload_record", resp.TypeName)
	}
}

func TestInventoryPreloadRecordResource_Schema(t *testing.T) {
	r := NewInventoryPreloadRecordResource()
	var resp resource.SchemaResponse
	r.(*InventoryPreloadRecordResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	expected := append([]string{"id", "serial_number", "device_type", "extension_attributes", "timeouts"}, optionalComputedScalars...)
	for _, name := range expected {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	id := s.Attributes["id"]
	if id.IsRequired() || !id.IsComputed() {
		t.Errorf("id must be computed-only, got required=%v computed=%v", id.IsRequired(), id.IsComputed())
	}

	if !s.Attributes["serial_number"].IsRequired() {
		t.Errorf("serial_number must be required")
	}
	if s.Attributes["serial_number"].IsComputed() {
		t.Errorf("serial_number must not be computed")
	}

	if !s.Attributes["device_type"].IsRequired() {
		t.Errorf("device_type must be required")
	}

	for _, name := range optionalComputedScalars {
		attr := s.Attributes[name]
		if !attr.IsOptional() || !attr.IsComputed() {
			t.Errorf("%s must be Optional+Computed, got optional=%v computed=%v", name, attr.IsOptional(), attr.IsComputed())
		}
		strAttr, ok := attr.(schema.StringAttribute)
		if !ok {
			t.Errorf("%s must be a StringAttribute", name)
			continue
		}
		if len(strAttr.PlanModifiers) == 0 {
			t.Errorf("%s must carry the UseStateForUnknown plan modifier — Optional+Computed without it silently wipes on omit (STYLE_GUIDE §Full-replace endpoints)", name)
		}
	}

	ea := s.Attributes["extension_attributes"]
	if !ea.IsOptional() || !ea.IsComputed() {
		t.Errorf("extension_attributes must be Optional+Computed, got optional=%v computed=%v", ea.IsOptional(), ea.IsComputed())
	}
	eaAttr, ok := ea.(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("extension_attributes must be a ListNestedAttribute: Jamf Pro stores repeated entries in the order written, and a set cannot hold a repeat (issue #568)")
	}
	if len(eaAttr.PlanModifiers) != 1 {
		t.Errorf("extension_attributes must carry exactly the UseStateForUnknown plan modifier, got %d", len(eaAttr.PlanModifiers))
	}
	if len(eaAttr.Validators) != 0 {
		t.Errorf("extension_attributes must carry no validator: repeats, including a repeated name, are what Jamf Pro stores (issue #568)")
	}
	if eaAttr.CustomType != nil {
		t.Errorf("extension_attributes must be a plain list: the server keeps written order, so there is nothing to compare semantically")
	}
	if resp.Schema.Version != 1 {
		t.Errorf("schema version: got %d, want 1 (v0 held extension_attributes as a set)", resp.Schema.Version)
	}
	if !eaAttr.NestedObject.Attributes["name"].IsRequired() {
		t.Errorf("extension_attributes.name must be required")
	}
	if !eaAttr.NestedObject.Attributes["value"].IsOptional() || eaAttr.NestedObject.Attributes["value"].IsComputed() {
		t.Errorf("extension_attributes.value must be Optional-only")
	}
}

// TestInventoryPreloadRecordResource_DeviceTypeOneOf exercises the device_type OneOf
// validator directly from the schema: the two wire values pass, anything else —
// including the OpenAPI spec's read-only legacy `Unknown` — is rejected at plan time
// (the server rejects it on write).
func TestInventoryPreloadRecordResource_DeviceTypeOneOf(t *testing.T) {
	r := NewInventoryPreloadRecordResource()
	var resp resource.SchemaResponse
	r.(*InventoryPreloadRecordResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	dt, ok := resp.Schema.Attributes["device_type"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("device_type must be a StringAttribute")
	}
	if len(dt.Validators) == 0 {
		t.Fatalf("device_type must carry the OneOf validator")
	}

	cases := []struct {
		value     string
		expectErr bool
	}{
		{"Computer", false},
		{"Mobile Device", false},
		{"Unknown", true},
		{"computer", true},
		{"MobileDevice", true},
		{"", true},
	}
	for _, c := range cases {
		req := validator.StringRequest{
			Path:        path.Root("device_type"),
			ConfigValue: types.StringValue(c.value),
		}
		var vResp validator.StringResponse
		for _, v := range dt.Validators {
			v.ValidateString(context.Background(), req, &vResp)
		}
		if got := vResp.Diagnostics.HasError(); got != c.expectErr {
			t.Errorf("device_type=%q: expected error=%v, got error=%v (%v)", c.value, c.expectErr, got, vResp.Diagnostics)
		}
	}
}

func TestInventoryPreloadRecordDataSource_Metadata(t *testing.T) {
	d := NewInventoryPreloadRecordDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*InventoryPreloadRecordDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_inventory_preload_record" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_inventory_preload_record", resp.TypeName)
	}
}

func TestInventoryPreloadRecordDataSource_Schema(t *testing.T) {
	d := NewInventoryPreloadRecordDataSource()
	var resp datasource.SchemaResponse
	d.(*InventoryPreloadRecordDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	expected := append([]string{"id", "serial_number", "device_type", "extension_attributes", "timeouts"}, optionalComputedScalars...)
	for _, name := range expected {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	for _, name := range []string{"id", "serial_number"} {
		attr := s.Attributes[name]
		if !attr.IsOptional() || !attr.IsComputed() {
			t.Errorf("data source %s must be Optional+Computed for the exactly-one-of lookup, got optional=%v computed=%v", name, attr.IsOptional(), attr.IsComputed())
		}
	}

	validators := d.(*InventoryPreloadRecordDataSource).ConfigValidators(context.Background())
	if len(validators) != 1 {
		t.Errorf("expected exactly one config validator (ExactlyOneOf), got %d", len(validators))
	}
}

func TestInventoryPreloadRecordListResource_Metadata(t *testing.T) {
	r := NewInventoryPreloadRecordListResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*InventoryPreloadRecordListResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_pro_inventory_preload_record" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_pro_inventory_preload_record", resp.TypeName)
	}
}

func TestInventoryPreloadRecordListResource_Schema(t *testing.T) {
	r := NewInventoryPreloadRecordListResource()
	var resp list.ListResourceSchemaResponse
	r.(*InventoryPreloadRecordListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["filter"]; !ok {
		t.Errorf("list schema missing filter attribute")
	}
}
