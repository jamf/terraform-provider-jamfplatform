// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package printer

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// printerConfig builds a tfsdk.Config from the real resource schema, filling
// every attribute null except those supplied in attrs (keyed by attribute
// name), so useGenericPPDConfigValidator can be driven end-to-end.
func printerConfig(t *testing.T, attrs map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	r := NewPrinterResource()
	var sr resource.SchemaResponse
	r.(*PrinterResource).Schema(context.Background(), resource.SchemaRequest{}, &sr)
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

// TestUseGenericPPDConfigValidator_DefersOnUnknownPPDPath is the §436 regression
// guard: with use_generic = false (known) and ppd_path UNKNOWN (variable /
// for_each / resource-driven), the validator MUST defer, not treat unknown as
// missing and error. See STYLE_GUIDE "Config-time validators MUST defer on
// unknown values".
func TestUseGenericPPDConfigValidator_DefersOnUnknownPPDPath(t *testing.T) {
	req := resource.ValidateConfigRequest{Config: printerConfig(t, map[string]tftypes.Value{
		"use_generic": tftypes.NewValue(tftypes.Bool, false),
		"ppd_path":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	var resp resource.ValidateConfigResponse
	useGenericPPDConfigValidator{}.ValidateResource(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("validator must defer when ppd_path is unknown, got %v", resp.Diagnostics)
	}
}

// Constructing a tfsdk.Config to drive the resource.ConfigValidator at unit
// scope is brittle (requires hand-rolling a tftypes.Object that matches the
// generated schema). The value-based logic is exercised end-to-end by the
// acceptance tests; the unit suite here covers the API contract (Description
// + MarkdownDescription non-empty) and the noLiteralSentinelValidator, which
// runs against a single types.String and is straightforward to drive.
// See ibeacon/validators_test.go for the same precedent.
func TestUseGenericPPDConfigValidator_Descriptions(t *testing.T) {
	v := useGenericPPDConfigValidator{}
	if v.Description(context.Background()) == "" {
		t.Errorf("Description must not be empty")
	}
	if v.MarkdownDescription(context.Background()) == "" {
		t.Errorf("MarkdownDescription must not be empty")
	}
}

func TestNoLiteralSentinelValidator_Descriptions(t *testing.T) {
	v := noLiteralSentinelValidator{}
	if v.Description(context.Background()) == "" {
		t.Errorf("Description must not be empty")
	}
	if v.MarkdownDescription(context.Background()) == "" {
		t.Errorf("MarkdownDescription must not be empty")
	}
}

func TestNoLiteralSentinelValidator_RejectsSentinel(t *testing.T) {
	v := noLiteralSentinelValidator{}
	req := validator.StringRequest{
		Path:        path.Root("category"),
		ConfigValue: types.StringValue(categoryUnassignedSentinel),
	}
	var resp validator.StringResponse
	v.ValidateString(context.Background(), req, &resp)
	if !resp.Diagnostics.HasError() {
		t.Errorf("expected error on literal sentinel, got none")
	}
}

func TestNoLiteralSentinelValidator_AllowsRealCategory(t *testing.T) {
	v := noLiteralSentinelValidator{}
	req := validator.StringRequest{
		Path:        path.Root("category"),
		ConfigValue: types.StringValue("Printers"),
	}
	var resp validator.StringResponse
	v.ValidateString(context.Background(), req, &resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("expected no error on real category, got %v", resp.Diagnostics)
	}
}

func TestNoLiteralSentinelValidator_AllowsNullAndUnknown(t *testing.T) {
	v := noLiteralSentinelValidator{}
	for _, cv := range []types.String{types.StringNull(), types.StringUnknown()} {
		req := validator.StringRequest{Path: path.Root("category"), ConfigValue: cv}
		var resp validator.StringResponse
		v.ValidateString(context.Background(), req, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("expected no error on null/unknown, got %v", resp.Diagnostics)
		}
	}
}

func TestIsSet(t *testing.T) {
	cases := []struct {
		name string
		in   types.String
		want bool
	}{
		{"value", types.StringValue("x"), true},
		{"empty", types.StringValue(""), false},
		{"null", types.StringNull(), false},
		{"unknown", types.StringUnknown(), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isSet(c.in); got != c.want {
				t.Errorf("isSet(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestPPDPathRejectsBundledGenericPath(t *testing.T) {
	var sr resource.SchemaResponse
	NewPrinterResource().(*PrinterResource).Schema(context.Background(), resource.SchemaRequest{}, &sr)
	attr, ok := sr.Schema.Attributes["ppd_path"].(schema.StringAttribute)
	if !ok {
		t.Fatal("ppd_path is not a StringAttribute")
	}
	cases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"bundled path", types.StringValue(bundledGenericPPDPath), true},
		{"bundled path with trailing newline", types.StringValue(bundledGenericPPDPath + "\n"), true},
		{"bundled path padded both sides", types.StringValue("  " + bundledGenericPPDPath + "\t"), true},
		{"custom path", types.StringValue("/private/etc/cups/ppd/Epson.ppd"), false},
		{"different case is not the gate", types.StringValue("/system/library" + bundledGenericPPDPath[len("/System/Library"):]), false},
		{"longer suffix is not the gate", types.StringValue(bundledGenericPPDPath + ".bak"), false},
		{"null", types.StringNull(), false},
		{"unknown defers", types.StringUnknown(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp validator.StringResponse
			for _, v := range attr.Validators {
				v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("ppd_path"), ConfigValue: tc.value}, &resp)
			}
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Errorf("error = %v, want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestBundledGenericPPDPathValidator_Descriptions(t *testing.T) {
	v := bundledGenericPPDPathValidator{}
	if v.Description(context.Background()) == "" || v.MarkdownDescription(context.Background()) == "" {
		t.Error("descriptions must be non-empty")
	}
}
