// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package setvalidation runs the validators a resource schema declares on a
// SetNestedAttribute or ListNestedAttribute against a collection built in the
// test, so a unit test proves a validator is wired to the attribute that
// ships rather than to a copy. It is a package of its own for the reason
// keyedcategories is: testhelpers imports the provider under the acceptance
// tag, which would make an in-package unit test that uses it an import cycle.
package setvalidation

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Run finds the SetNestedAttribute at names (each name but the last a
// SingleNestedAttribute), builds a known set whose elements take the values in
// elems and null for every attribute an element leaves out, and returns the
// diagnostics of every validator the attribute declares.
func Run(t *testing.T, s schema.Schema, names []string, elems ...map[string]attr.Value) diag.Diagnostics {
	t.Helper()
	ctx := context.Background()
	at, a := find(t, s, names)
	set, ok := a.(schema.SetNestedAttribute)
	if !ok {
		t.Fatalf("%s: not a SetNestedAttribute", at)
	}
	if len(set.Validators) == 0 {
		t.Fatalf("%s declares no validators", at)
	}
	objType, values := elements(t, set.NestedObject, elems)
	value, diags := types.SetValue(objType, values)
	if diags.HasError() {
		t.Fatalf("building set: %v", diags)
	}
	var out diag.Diagnostics
	for _, v := range set.Validators {
		req := validator.SetRequest{Path: at, PathExpression: at.Expression(), ConfigValue: value}
		var resp validator.SetResponse
		v.ValidateSet(ctx, req, &resp)
		out.Append(resp.Diagnostics...)
	}
	return out
}

// RunList is Run for a ListNestedAttribute. The list keeps elems in order.
func RunList(t *testing.T, s schema.Schema, names []string, elems ...map[string]attr.Value) diag.Diagnostics {
	t.Helper()
	ctx := context.Background()
	at, a := find(t, s, names)
	list, ok := a.(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("%s: not a ListNestedAttribute", at)
	}
	if len(list.Validators) == 0 {
		t.Fatalf("%s declares no validators", at)
	}
	objType, values := elements(t, list.NestedObject, elems)
	value, diags := types.ListValue(objType, values)
	if diags.HasError() {
		t.Fatalf("building list: %v", diags)
	}
	var out diag.Diagnostics
	for _, v := range list.Validators {
		req := validator.ListRequest{Path: at, PathExpression: at.Expression(), ConfigValue: value}
		var resp validator.ListResponse
		v.ValidateList(ctx, req, &resp)
		out.Append(resp.Diagnostics...)
	}
	return out
}

// Summaries returns the summary of every error in diags.
func Summaries(diags diag.Diagnostics) []string {
	var out []string
	for _, d := range diags.Errors() {
		out = append(out, d.Summary())
	}
	return out
}

func find(t *testing.T, s schema.Schema, names []string) (path.Path, schema.Attribute) {
	t.Helper()
	attrs := s.Attributes
	p := path.Empty()
	for _, n := range names[:len(names)-1] {
		block, ok := attrs[n].(schema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%s: not a SingleNestedAttribute", n)
		}
		attrs = block.Attributes
		p = p.AtName(n)
	}
	last := names[len(names)-1]
	return p.AtName(last), attrs[last]
}

func elements(t *testing.T, nested schema.NestedAttributeObject, elems []map[string]attr.Value) (types.ObjectType, []attr.Value) {
	t.Helper()
	ctx := context.Background()
	attrTypes := make(map[string]attr.Type, len(nested.Attributes))
	for n, a := range nested.Attributes {
		attrTypes[n] = a.GetType()
	}
	values := make([]attr.Value, 0, len(elems))
	for _, e := range elems {
		for n := range e {
			if _, ok := attrTypes[n]; !ok {
				t.Fatalf("element sets %q, which the schema does not declare", n)
			}
		}
		obj := make(map[string]attr.Value, len(attrTypes))
		for n, typ := range attrTypes {
			if v, ok := e[n]; ok {
				obj[n] = v
				continue
			}
			null, err := typ.ValueFromTerraform(ctx, tftypes.NewValue(typ.TerraformType(ctx), nil))
			if err != nil {
				t.Fatalf("null %s: %v", n, err)
			}
			obj[n] = null
		}
		v, diags := types.ObjectValue(attrTypes, obj)
		if diags.HasError() {
			t.Fatalf("building element: %v", diags)
		}
		values = append(values, v)
	}
	return types.ObjectType{AttrTypes: attrTypes}, values
}
