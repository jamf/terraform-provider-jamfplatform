// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var categoryAttrTypes = map[string]attr.Type{
	"id":         types.StringType,
	"name":       types.StringType,
	"display_in": types.BoolType,
	"feature_in": types.BoolType,
}

var mobileCategoryAttrTypes = map[string]attr.Type{
	"id":         types.StringType,
	"name":       types.StringType,
	"display_in": types.BoolType,
}

// category builds one element; a nil flag is null.
func category(t *testing.T, id types.String, display, feature *bool) attr.Value {
	t.Helper()
	obj, diags := types.ObjectValue(categoryAttrTypes, map[string]attr.Value{
		"id":         id,
		"name":       types.StringNull(),
		"display_in": boolOrNull(display),
		"feature_in": boolOrNull(feature),
	})
	if diags.HasError() {
		t.Fatalf("building category: %v", diags)
	}
	return obj
}

func mobileCategory(t *testing.T, id types.String, display types.Bool) attr.Value {
	t.Helper()
	obj, diags := types.ObjectValue(mobileCategoryAttrTypes, map[string]attr.Value{
		"id":         id,
		"name":       types.StringNull(),
		"display_in": display,
	})
	if diags.HasError() {
		t.Fatalf("building mobile category: %v", diags)
	}
	return obj
}

func boolOrNull(b *bool) types.Bool {
	if b == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*b)
}

func categorySet(t *testing.T, attrTypes map[string]attr.Type, elems ...attr.Value) types.Set {
	t.Helper()
	set, diags := types.SetValue(types.ObjectType{AttrTypes: attrTypes}, elems)
	if diags.HasError() {
		t.Fatalf("building category set: %v", diags)
	}
	return set
}

func runCategories(t *testing.T, featureField string, set types.Set) diag.Diagnostics {
	t.Helper()
	var all diag.Diagnostics
	for _, v := range SelfServiceCategories("display_in", featureField) {
		req := validator.SetRequest{Path: path.Root("categories"), ConfigValue: set}
		var resp validator.SetResponse
		v.ValidateSet(context.Background(), req, &resp)
		all.Append(resp.Diagnostics...)
	}
	return all
}

func diagText(diags diag.Diagnostics) string {
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.Summary())
		b.WriteString(": ")
		b.WriteString(d.Detail())
		b.WriteString("\n")
	}
	return b.String()
}

var (
	yes = new(true)
	no  = new(false)
)

func TestSelfServiceCategories_Rejects(t *testing.T) {
	cases := map[string]struct {
		set  types.Set
		want []string
	}{
		"same id, feature on one and display on the other": {
			set: categorySet(t, categoryAttrTypes,
				category(t, types.StringValue("12"), nil, yes),
				category(t, types.StringValue("12"), yes, nil),
			),
			want: []string{"Category 12 is listed more than once", "Combine them into one entry with both display_in and feature_in"},
		},
		"same id, differing feature flag": {
			set: categorySet(t, categoryAttrTypes,
				category(t, types.StringValue("12"), yes, yes),
				category(t, types.StringValue("12"), yes, no),
			),
			want: []string{"Category 12 is listed more than once"},
		},
		"same integer written two ways": {
			set: categorySet(t, categoryAttrTypes,
				category(t, types.StringValue("12"), nil, nil),
				category(t, types.StringValue("012"), nil, nil),
			),
			want: []string{"listed more than once"},
		},
		"featured but not displayed": {
			set: categorySet(t, categoryAttrTypes,
				category(t, types.StringValue("12"), no, yes),
				category(t, types.StringValue("13"), yes, nil),
			),
			want: []string{"feature_in requires display_in", "Category 12 sets feature_in = true and display_in = false", "Jamf Pro drops a featured category that is not displayed"},
		},
		"not displayed, feature unset": {
			set: categorySet(t, categoryAttrTypes,
				category(t, types.StringValue("12"), no, nil),
			),
			want: []string{"Category 12 sets display_in = false", "remove the entry"},
		},
		"not displayed, feature false": {
			set: categorySet(t, categoryAttrTypes,
				category(t, types.StringValue("12"), no, no),
			),
			want: []string{"Category 12 sets display_in = false"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			diags := runCategories(t, "feature_in", tc.set)
			if diags.ErrorsCount() != 1 {
				t.Fatalf("want exactly 1 error, got %d:\n%s", diags.ErrorsCount(), diagText(diags))
			}
			text := diagText(diags)
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("diagnostic missing %q:\n%s", w, text)
				}
			}
		})
	}
}

func TestSelfServiceCategories_Accepts(t *testing.T) {
	cases := map[string]types.Set{
		"null":    types.SetNull(types.ObjectType{AttrTypes: categoryAttrTypes}),
		"unknown": types.SetUnknown(types.ObjectType{AttrTypes: categoryAttrTypes}),
		"empty":   categorySet(t, categoryAttrTypes),
		"two categories, both flags": categorySet(t, categoryAttrTypes,
			category(t, types.StringValue("12"), yes, yes),
			category(t, types.StringValue("13"), yes, yes),
		),
		"flags unset": categorySet(t, categoryAttrTypes,
			category(t, types.StringValue("12"), nil, nil),
		),
		"featured with display unset": categorySet(t, categoryAttrTypes,
			category(t, types.StringValue("12"), nil, yes),
		),
	}
	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			if diags := runCategories(t, "feature_in", set); diags.HasError() {
				t.Errorf("want no error, got:\n%s", diagText(diags))
			}
		})
	}
}

func TestSelfServiceCategories_DefersOnUnknowns(t *testing.T) {
	unknownElem, diags := types.ObjectValue(categoryAttrTypes, map[string]attr.Value{
		"id": types.StringValue("12"), "name": types.StringNull(), "display_in": types.BoolNull(), "feature_in": types.BoolNull(),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	cases := map[string]types.Set{
		"unknown ids": categorySet(t, categoryAttrTypes,
			category(t, types.StringUnknown(), yes, yes),
			category(t, types.StringUnknown(), yes, no),
		),
		"unknown id beside a known one": categorySet(t, categoryAttrTypes,
			category(t, types.StringUnknown(), yes, nil),
			category(t, types.StringValue("12"), yes, yes),
		),
		"unknown display flag, feature true": categorySet(t, categoryAttrTypes,
			mustObject(t, categoryAttrTypes, map[string]attr.Value{
				"id": types.StringValue("12"), "name": types.StringNull(), "display_in": types.BoolUnknown(), "feature_in": types.BoolValue(true),
			}),
		),
		"unknown element": categorySet(t, categoryAttrTypes,
			types.ObjectUnknown(categoryAttrTypes),
			unknownElem,
		),
	}
	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			if diags := runCategories(t, "feature_in", set); diags.HasError() {
				t.Errorf("want defer, got:\n%s", diagText(diags))
			}
		})
	}
}

func TestSelfServiceCategories_UnknownFeatureStillRejectsDisplayFalse(t *testing.T) {
	set := categorySet(t, categoryAttrTypes, mustObject(t, categoryAttrTypes, map[string]attr.Value{
		"id": types.StringValue("12"), "name": types.StringNull(), "display_in": types.BoolValue(false), "feature_in": types.BoolUnknown(),
	}))
	diags := runCategories(t, "feature_in", set)
	if diags.ErrorsCount() != 1 || !strings.Contains(diagText(diags), "Category 12 sets display_in = false") {
		t.Fatalf("display_in = false is dropped whatever feature_in resolves to; got:\n%s", diagText(diags))
	}
}

func TestSelfServiceCategories_NoFeatureFlag(t *testing.T) {
	dup := categorySet(t, mobileCategoryAttrTypes,
		mobileCategory(t, types.StringValue("12"), types.BoolNull()),
		mobileCategory(t, types.StringValue("12"), types.BoolValue(true)),
	)
	diags := runCategories(t, "", dup)
	if diags.ErrorsCount() != 1 || !strings.Contains(diagText(diags), "List the category once") {
		t.Fatalf("repeated id: got:\n%s", diagText(diags))
	}
	if strings.Contains(diagText(diags), "feature") {
		t.Errorf("a resource with no feature flag must not mention one:\n%s", diagText(diags))
	}

	hidden := categorySet(t, mobileCategoryAttrTypes, mobileCategory(t, types.StringValue("12"), types.BoolValue(false)))
	diags = runCategories(t, "", hidden)
	if diags.ErrorsCount() != 1 || !strings.Contains(diagText(diags), "Category 12 sets display_in = false") {
		t.Fatalf("display_in = false: got:\n%s", diagText(diags))
	}

	ok := categorySet(t, mobileCategoryAttrTypes,
		mobileCategory(t, types.StringValue("12"), types.BoolValue(true)),
		mobileCategory(t, types.StringValue("13"), types.BoolNull()),
	)
	if diags := runCategories(t, "", ok); diags.HasError() {
		t.Fatalf("valid set rejected:\n%s", diagText(diags))
	}
}

func mustObject(t *testing.T, attrTypes map[string]attr.Type, values map[string]attr.Value) attr.Value {
	t.Helper()
	obj, diags := types.ObjectValue(attrTypes, values)
	if diags.HasError() {
		t.Fatalf("building object: %v", diags)
	}
	return obj
}
