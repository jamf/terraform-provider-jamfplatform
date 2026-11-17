// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_extension_attribute

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// configFromAttrs builds a tfsdk.Config from the real resource schema, filling
// every attribute with null except those supplied in attrs (keyed by attribute
// name). This drives inputTypeConfigValidator.ValidateResource end-to-end.
func configFromAttrs(t *testing.T, attrs map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	r := NewComputerExtensionAttributeResource()
	var sr resource.SchemaResponse
	r.(*ComputerExtensionAttributeResource).Schema(context.Background(), resource.SchemaRequest{}, &sr)
	if sr.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", sr.Diagnostics)
	}

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

func str(s string) tftypes.Value { return tftypes.NewValue(tftypes.String, s) }
func boolv(b bool) tftypes.Value { return tftypes.NewValue(tftypes.Bool, b) }
func popup(vs ...string) tftypes.Value {
	elems := make([]tftypes.Value, len(vs))
	for i, v := range vs {
		elems[i] = tftypes.NewValue(tftypes.String, v)
	}
	return tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, elems)
}

func validateComputer(t *testing.T, attrs map[string]tftypes.Value) resource.ValidateConfigResponse {
	t.Helper()
	req := resource.ValidateConfigRequest{Config: configFromAttrs(t, attrs)}
	var resp resource.ValidateConfigResponse
	inputTypeConfigValidator{}.ValidateResource(context.Background(), req, &resp)
	return resp
}

// TestInputTypeValidator_Computer_DefersOnUnknownCompanion is the STYLE_GUIDE §Config-time validators MUST defer on unknown values
// regression guard: when input_type is known but the required companion
// (script / directory_service_attribute) is unknown — sourced from a variable,
// for_each, or another resource — the validator MUST defer, not treat unknown
// as missing and error. See STYLE_GUIDE "Config-time validators MUST defer on
// unknown values".
func TestInputTypeValidator_Computer_DefersOnUnknownCompanion(t *testing.T) {
	unknown := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	cases := []struct {
		name  string
		attrs map[string]tftypes.Value
	}{
		{"SCRIPT with unknown script", map[string]tftypes.Value{"input_type": str("SCRIPT"), "script": unknown}},
		{"DSAM with unknown directory_service_attribute", map[string]tftypes.Value{"input_type": str("DIRECTORY_SERVICE_ATTRIBUTE_MAPPING"), "directory_service_attribute": unknown}},
		{"SCRIPT manage_existing_data with unknown enabled", map[string]tftypes.Value{"input_type": str("SCRIPT"), "script": str("x"), "enabled": tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue), "manage_existing_data": str("RETAIN")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if resp := validateComputer(t, tc.attrs); resp.Diagnostics.HasError() {
				t.Errorf("validator must defer on unknown companion, got %v", resp.Diagnostics)
			}
		})
	}
}

func TestInputTypeValidator_Computer(t *testing.T) {
	tests := []struct {
		name    string
		attrs   map[string]tftypes.Value
		wantErr bool
	}{
		// Valid combinations.
		{"text bare", map[string]tftypes.Value{"input_type": str("TEXT"), "data_type": str("STRING"), "inventory_display": str("GENERAL")}, false},
		{"script with script", map[string]tftypes.Value{"input_type": str("SCRIPT"), "script": str("echo hi")}, false},
		{"popup with choices", map[string]tftypes.Value{"input_type": str("POPUP"), "popup_menu_choices": popup("a", "b")}, false},
		{"popup no choices ok", map[string]tftypes.Value{"input_type": str("POPUP")}, false},
		{"dsam with attribute", map[string]tftypes.Value{"input_type": str("DIRECTORY_SERVICE_ATTRIBUTE_MAPPING"), "directory_service_attribute": str("mail")}, false},
		{"dsam allow multiple", map[string]tftypes.Value{"input_type": str("DIRECTORY_SERVICE_ATTRIBUTE_MAPPING"), "directory_service_attribute": str("mail"), "allow_multiple_values": boolv(true)}, false},
		{"script disabled ok", map[string]tftypes.Value{"input_type": str("SCRIPT"), "script": str("x"), "enabled": boolv(false)}, false},

		// REQUIRED violations.
		{"script missing script", map[string]tftypes.Value{"input_type": str("SCRIPT")}, true},
		{"script empty script", map[string]tftypes.Value{"input_type": str("SCRIPT"), "script": str("")}, true},
		{"dsam missing attribute", map[string]tftypes.Value{"input_type": str("DIRECTORY_SERVICE_ATTRIBUTE_MAPPING")}, true},
		{"dsam empty attribute", map[string]tftypes.Value{"input_type": str("DIRECTORY_SERVICE_ATTRIBUTE_MAPPING"), "directory_service_attribute": str("")}, true},

		// FORBIDDEN violations.
		{"text with script", map[string]tftypes.Value{"input_type": str("TEXT"), "script": str("x")}, true},
		{"text with popup", map[string]tftypes.Value{"input_type": str("TEXT"), "popup_menu_choices": popup("a")}, true},
		{"text with dsa", map[string]tftypes.Value{"input_type": str("TEXT"), "directory_service_attribute": str("mail")}, true},
		{"popup with script", map[string]tftypes.Value{"input_type": str("POPUP"), "popup_menu_choices": popup("a"), "script": str("x")}, true},
		{"script with popup", map[string]tftypes.Value{"input_type": str("SCRIPT"), "script": str("x"), "popup_menu_choices": popup("a")}, true},
		{"dsam with script", map[string]tftypes.Value{"input_type": str("DIRECTORY_SERVICE_ATTRIBUTE_MAPPING"), "directory_service_attribute": str("mail"), "script": str("x")}, true},

		// enabled / allow_multiple_values discriminator.
		{"text disabled forbidden", map[string]tftypes.Value{"input_type": str("TEXT"), "enabled": boolv(false)}, true},
		{"text allow multiple forbidden", map[string]tftypes.Value{"input_type": str("TEXT"), "allow_multiple_values": boolv(true)}, true},

		// manage_existing_data is valid only on a DISABLED SCRIPT EA (issue #302:
		// Jamf Pro 400s the field on an enabled-SCRIPT update).
		{"script disabled manage_existing_data ok", map[string]tftypes.Value{"input_type": str("SCRIPT"), "script": str("x"), "enabled": boolv(false), "manage_existing_data": str("RETAIN")}, false},
		{"script enabled manage_existing_data forbidden", map[string]tftypes.Value{"input_type": str("SCRIPT"), "script": str("x"), "enabled": boolv(true), "manage_existing_data": str("RETAIN")}, true},
		{"script omitted enabled manage_existing_data forbidden", map[string]tftypes.Value{"input_type": str("SCRIPT"), "script": str("x"), "manage_existing_data": str("RETAIN")}, true},
		{"text manage_existing_data forbidden", map[string]tftypes.Value{"input_type": str("TEXT"), "manage_existing_data": str("RETAIN")}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := validateComputer(t, tc.attrs)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Errorf("wantErr=%v, got=%v (diags: %v)", tc.wantErr, got, resp.Diagnostics)
			}
		})
	}
}
