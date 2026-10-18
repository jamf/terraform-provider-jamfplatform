// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// uncheckedPayloadConfig builds a blueprint configuration from the resource's own schema, setting
// each value at its path, so a test drives uncheckedPayloadWarnings with the shape Terraform sends.
func uncheckedPayloadConfig(t *testing.T, values map[string]attr.Value) tfsdk.Config {
	t.Helper()
	ctx := context.Background()

	var resp resource.SchemaResponse
	NewBlueprintResource().(*BlueprintResource).Schema(ctx, resource.SchemaRequest{}, &resp)
	state := tfsdk.State{Schema: resp.Schema, Raw: tftypes.NewValue(resp.Schema.Type().TerraformType(ctx), nil)}
	for name, value := range values {
		if diags := state.SetAttribute(ctx, path.Root(name), value); diags.HasError() {
			t.Fatalf("setting %s: %v", name, diags)
		}
	}
	return tfsdk.Config{Schema: resp.Schema, Raw: state.Raw}
}

// elementType returns the element type the schema declares for a collection at p.
func elementType(t *testing.T, p path.Path) attr.Type {
	t.Helper()
	ctx := context.Background()

	var resp resource.SchemaResponse
	NewBlueprintResource().(*BlueprintResource).Schema(ctx, resource.SchemaRequest{}, &resp)
	typ, diags := resp.Schema.TypeAtPath(ctx, p)
	if diags.HasError() {
		t.Fatalf("type at %s: %v", p, diags)
	}
	switch collection := typ.(type) {
	case types.ListType:
		return collection.ElemType
	case types.SetType:
		return collection.ElemType
	}
	t.Fatalf("%s is %T, not a collection", p, typ)
	return nil
}

// objectOf builds an object of the given type, nulling every attribute the test does not set.
func objectOf(t *testing.T, typ attr.Type, set map[string]attr.Value) types.Object {
	t.Helper()
	ctx := context.Background()

	objectType, ok := typ.(types.ObjectType)
	if !ok {
		t.Fatalf("%T is not an object type", typ)
	}
	values := make(map[string]attr.Value, len(objectType.AttrTypes))
	for name, attrType := range objectType.AttrTypes {
		if value, ok := set[name]; ok {
			values[name] = value
			continue
		}
		null, err := attrType.ValueFromTerraform(ctx, tftypes.NewValue(attrType.TerraformType(ctx), nil))
		if err != nil {
			t.Fatalf("null %s: %v", name, err)
		}
		values[name] = null
	}
	object, diags := types.ObjectValue(objectType.AttrTypes, values)
	if diags.HasError() {
		t.Fatalf("building object: %v", diags)
	}
	return object
}

// listOf builds a list of the given element type.
func listOf(t *testing.T, elem attr.Type, elements ...attr.Value) types.List {
	t.Helper()
	list, diags := types.ListValue(elem, elements)
	if diags.HasError() {
		t.Fatalf("building list: %v", diags)
	}
	return list
}

// blockWith builds a one-block component_blocks list carrying the given attributes.
func blockWith(t *testing.T, set map[string]attr.Value) types.List {
	t.Helper()
	blockType := elementType(t, path.Root("component_blocks"))
	set["name"] = types.StringValue("Block")
	return listOf(t, blockType, objectOf(t, blockType, set))
}

// TestUncheckedPayloadWarnings covers the ModifyPlan half of #484. The validators skip an unknown
// silently, because Terraform calls them before planning with module variables still unknown; a value
// still unknown when ModifyPlan runs is known only at apply, and the apply writes it unchecked. That
// must be said, once per payload, on the payload's own path, and a known value must say nothing.
func TestUncheckedPayloadWarnings(t *testing.T) {
	t.Parallel()

	blockLegacy := path.Root("component_blocks").AtListIndex(0).AtName("legacy_payloads")
	blockDeclarations := path.Root("component_blocks").AtListIndex(0).AtName("apple_declarations")
	blockCustom := path.Root("component_blocks").AtListIndex(0).AtName("custom_declarations")
	customSet := path.Root("custom_declarations").AtName("declaration")

	legacy := func(t *testing.T, settings types.String) types.List {
		t.Helper()
		elem := elementType(t, blockLegacy)
		return listOf(t, elem, objectOf(t, elem, map[string]attr.Value{
			"payload_type": types.StringValue(mcxPayloadType),
			"settings":     settings,
		}))
	}
	declaration := func(t *testing.T, payload types.String) types.List {
		t.Helper()
		elem := elementType(t, blockDeclarations)
		return listOf(t, elem, objectOf(t, elem, map[string]attr.Value{
			"channel": types.StringValue("SYSTEM"),
			"type":    types.StringValue("com.apple.configuration.siri.settings"),
			"payload": payload,
		}))
	}
	custom := func(t *testing.T, at path.Path, payload types.String) types.Object {
		t.Helper()
		ctx := context.Background()
		var resp resource.SchemaResponse
		NewBlueprintResource().(*BlueprintResource).Schema(ctx, resource.SchemaRequest{}, &resp)
		componentType, diags := resp.Schema.TypeAtPath(ctx, at)
		if diags.HasError() {
			t.Fatalf("type at %s: %v", at, diags)
		}
		elem := elementType(t, at.AtName("declaration"))
		set, diags := types.SetValue(elem, []attr.Value{objectOf(t, elem, map[string]attr.Value{
			"channel": types.StringValue("SYSTEM"),
			"kind":    types.StringValue("CONFIGURATION"),
			"type":    types.StringValue("com.apple.configuration.siri.settings"),
			"payload": payload,
		})})
		if diags.HasError() {
			t.Fatalf("building declaration set: %v", diags)
		}
		return objectOf(t, componentType, map[string]attr.Value{"declaration": set})
	}

	cases := []struct {
		name    string
		values  func(*testing.T) map[string]attr.Value
		want    []path.Path
		summary string
	}{
		{
			name: "block legacy payload with unknown settings",
			values: func(t *testing.T) map[string]attr.Value {
				return map[string]attr.Value{"component_blocks": blockWith(t, map[string]attr.Value{
					"legacy_payloads": legacy(t, types.StringUnknown()),
				})}
			},
			want:    []path.Path{blockLegacy.AtListIndex(0)},
			summary: uncheckedLegacyPayloadSummary,
		},
		{
			name: "block legacy payload with known settings",
			values: func(t *testing.T) map[string]attr.Value {
				return map[string]attr.Value{"component_blocks": blockWith(t, map[string]attr.Value{
					"legacy_payloads": legacy(t, types.StringValue(`{"PayloadContent":{"com.example.app":{}}}`)),
				})}
			},
		},
		{
			name: "unknown legacy payload list",
			values: func(t *testing.T) map[string]attr.Value {
				return map[string]attr.Value{"component_blocks": blockWith(t, map[string]attr.Value{
					"legacy_payloads": types.ListUnknown(elementType(t, blockLegacy)),
				})}
			},
			want:    []path.Path{blockLegacy},
			summary: uncheckedLegacyPayloadSummary,
		},
		{
			name: "apple declaration with unknown payload",
			values: func(t *testing.T) map[string]attr.Value {
				return map[string]attr.Value{"component_blocks": blockWith(t, map[string]attr.Value{
					"apple_declarations": declaration(t, types.StringUnknown()),
				})}
			},
			want:    []path.Path{blockDeclarations.AtListIndex(0)},
			summary: uncheckedDeclarationSummary,
		},
		{
			name: "apple declaration with known payload",
			values: func(t *testing.T) map[string]attr.Value {
				return map[string]attr.Value{"component_blocks": blockWith(t, map[string]attr.Value{
					"apple_declarations": declaration(t, types.StringValue(`{"Enabled":true}`)),
				})}
			},
		},
		{
			name: "block custom declaration with unknown payload",
			values: func(t *testing.T) map[string]attr.Value {
				return map[string]attr.Value{"component_blocks": blockWith(t, map[string]attr.Value{
					"custom_declarations": custom(t, blockCustom, types.StringUnknown()),
				})}
			},
			want:    []path.Path{blockCustom.AtName("declaration")},
			summary: uncheckedDeclarationSummary,
		},
		{
			name: "top-level custom declaration with unknown payload",
			values: func(t *testing.T) map[string]attr.Value {
				return map[string]attr.Value{"custom_declarations": custom(t, path.Root("custom_declarations"), types.StringUnknown())}
			},
			want:    []path.Path{customSet},
			summary: uncheckedDeclarationSummary,
		},
		{
			name: "flat legacy payloads with a nested unknown",
			values: func(t *testing.T) map[string]attr.Value {
				settings := types.ObjectValueMust(
					map[string]attr.Type{"PayloadContent": types.StringType},
					map[string]attr.Value{"PayloadContent": types.StringUnknown()},
				)
				return map[string]attr.Value{"legacy_payloads": types.DynamicValue(settings)}
			},
			want:    []path.Path{path.Root("legacy_payloads")},
			summary: uncheckedLegacyPayloadSummary,
		},
		{
			name: "flat legacy payloads fully known",
			values: func(t *testing.T) map[string]attr.Value {
				return map[string]attr.Value{"legacy_payloads": types.DynamicValue(types.StringValue("x"))}
			},
		},
		{
			name:   "nothing configured",
			values: func(*testing.T) map[string]attr.Value { return map[string]attr.Value{} },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			diags := uncheckedPayloadWarnings(context.Background(), uncheckedPayloadConfig(t, tc.values(t)))
			if diags.HasError() {
				t.Fatalf("produced errors: %v", diags.Errors())
			}
			warnings := diags.Warnings()
			if len(warnings) != len(tc.want) {
				t.Fatalf("warnings = %v, want %d", warnings, len(tc.want))
			}
			for i, warning := range warnings {
				if warning.Summary() != tc.summary {
					t.Errorf("summary = %q, want %q", warning.Summary(), tc.summary)
				}
				withPath, ok := warning.(diag.DiagnosticWithPath)
				if !ok {
					t.Fatalf("expected an attribute warning, got %T", warning)
				}
				if !withPath.Path().Equal(tc.want[i]) {
					t.Errorf("path = %s, want %s", withPath.Path(), tc.want[i])
				}
			}
		})
	}
}
