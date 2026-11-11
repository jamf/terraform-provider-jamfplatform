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

var mobileProfileCategoryAttrTypes = map[string]attr.Type{
	"id":   types.StringType,
	"name": types.StringType,
}

func categoryList(t *testing.T, attrTypes map[string]attr.Type, elems ...attr.Value) types.List {
	t.Helper()
	list, diags := types.ListValue(types.ObjectType{AttrTypes: attrTypes}, elems)
	if diags.HasError() {
		t.Fatalf("building category list: %v", diags)
	}
	return list
}

func runCategoriesList(t *testing.T, displayField, featureField string, list types.List) diag.Diagnostics {
	t.Helper()
	var all diag.Diagnostics
	for _, v := range SelfServiceCategoriesList(displayField, featureField) {
		req := validator.ListRequest{Path: path.Root("categories"), ConfigValue: list}
		var resp validator.ListResponse
		v.ValidateList(context.Background(), req, &resp)
		all.Append(resp.Diagnostics...)
	}
	return all
}

func TestSelfServiceCategoriesList_Rejects(t *testing.T) {
	cases := map[string]struct {
		list     types.List
		want     string
		wantPath path.Path
	}{
		"identical entries": {
			list: categoryList(t, categoryAttrTypes,
				category(t, types.StringValue("12"), nil, nil),
				category(t, types.StringValue("12"), nil, nil),
			),
			want:     "Category 12 is listed more than once",
			wantPath: path.Root("categories").AtListIndex(1).AtName("id"),
		},
		"same id, different flags": {
			list: categoryList(t, categoryAttrTypes,
				category(t, types.StringValue("13"), yes, nil),
				category(t, types.StringValue("12"), nil, yes),
				category(t, types.StringValue("012"), yes, nil),
			),
			want:     "Combine them into one entry with both display_in and feature_in",
			wantPath: path.Root("categories").AtListIndex(2).AtName("id"),
		},
		"featured but not displayed": {
			list: categoryList(t, categoryAttrTypes,
				category(t, types.StringValue("13"), yes, nil),
				category(t, types.StringValue("12"), no, yes),
			),
			want:     "Jamf Pro drops a featured category that is not displayed",
			wantPath: path.Root("categories").AtListIndex(1).AtName("display_in"),
		},
		"not displayed": {
			list: categoryList(t, categoryAttrTypes,
				category(t, types.StringValue("12"), no, nil),
			),
			want:     "Category 12 sets display_in = false",
			wantPath: path.Root("categories").AtListIndex(0).AtName("display_in"),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			diags := runCategoriesList(t, "display_in", "feature_in", tc.list)
			if diags.ErrorsCount() != 1 {
				t.Fatalf("want exactly 1 error, got %d:\n%s", diags.ErrorsCount(), diagText(diags))
			}
			if !strings.Contains(diagText(diags), tc.want) {
				t.Errorf("diagnostic missing %q:\n%s", tc.want, diagText(diags))
			}
			withPath, ok := diags[0].(diag.DiagnosticWithPath)
			if !ok || !withPath.Path().Equal(tc.wantPath) {
				t.Errorf("path = %v, want %v", diags[0], tc.wantPath)
			}
		})
	}
}

func TestSelfServiceCategoriesList_AcceptsAndDefers(t *testing.T) {
	objType := types.ObjectType{AttrTypes: categoryAttrTypes}
	cases := map[string]types.List{
		"null":    types.ListNull(objType),
		"unknown": types.ListUnknown(objType),
		"two categories, both flags": categoryList(t, categoryAttrTypes,
			category(t, types.StringValue("12"), yes, yes),
			category(t, types.StringValue("13"), yes, yes),
		),
		"unknown ids": categoryList(t, categoryAttrTypes,
			category(t, types.StringUnknown(), nil, nil),
			category(t, types.StringUnknown(), nil, nil),
		),
		"unknown display flag": categoryList(t, categoryAttrTypes,
			mustObject(t, categoryAttrTypes, map[string]attr.Value{
				"id": types.StringValue("12"), "name": types.StringNull(), "display_in": types.BoolUnknown(), "feature_in": types.BoolValue(true),
			}),
		),
		"unknown element": categoryList(t, categoryAttrTypes, types.ObjectUnknown(categoryAttrTypes)),
	}
	for name, list := range cases {
		t.Run(name, func(t *testing.T) {
			if diags := runCategoriesList(t, "display_in", "feature_in", list); diags.HasError() {
				t.Errorf("want no error, got:\n%s", diagText(diags))
			}
		})
	}
}

func TestSelfServiceCategoriesList_NoDisplayFlag(t *testing.T) {
	if got := len(SelfServiceCategoriesList("", "")); got != 1 {
		t.Fatalf("with no display flag only the repeated-id check applies; got %d validators", got)
	}
	obj := func(id string) attr.Value {
		return mustObject(t, mobileProfileCategoryAttrTypes, map[string]attr.Value{"id": types.StringValue(id), "name": types.StringNull()})
	}
	diags := runCategoriesList(t, "", "", categoryList(t, mobileProfileCategoryAttrTypes, obj("12"), obj("12")))
	text := diagText(diags)
	if diags.ErrorsCount() != 1 || !strings.Contains(text, "List the category once") || strings.Contains(text, "feature") || strings.Contains(text, "display") {
		t.Fatalf("repeated id with no flags: got:\n%s", text)
	}
	if diags := runCategoriesList(t, "", "", categoryList(t, mobileProfileCategoryAttrTypes, obj("12"), obj("13"))); diags.HasError() {
		t.Fatalf("valid list rejected:\n%s", diagText(diags))
	}
}
