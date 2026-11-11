// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package setvalidation runs the validators a resource schema declares on a
// SetNestedAttribute against a set built in the test, so a unit test proves a
// validator is wired to the attribute that ships rather than to a copy. It is
// a package of its own for the reason keyedcategories is: testhelpers imports
// the provider under the acceptance tag, which would make an in-package unit
// test that uses it an import cycle.
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
	set, ok := attrs[last].(schema.SetNestedAttribute)
	if !ok {
		t.Fatalf("%s: not a SetNestedAttribute", last)
	}
	if len(set.Validators) == 0 {
		t.Fatalf("%s declares no validators", last)
	}

	attrTypes := make(map[string]attr.Type, len(set.NestedObject.Attributes))
	for n, a := range set.NestedObject.Attributes {
		attrTypes[n] = a.GetType()
	}
	values := make([]attr.Value, 0, len(elems))
	for _, e := range elems {
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
		for n := range e {
			if _, ok := attrTypes[n]; !ok {
				t.Fatalf("element sets %q, which the schema does not declare", n)
			}
		}
		v, diags := types.ObjectValue(attrTypes, obj)
		if diags.HasError() {
			t.Fatalf("building element: %v", diags)
		}
		values = append(values, v)
	}
	value, diags := types.SetValue(types.ObjectType{AttrTypes: attrTypes}, values)
	if diags.HasError() {
		t.Fatalf("building set: %v", diags)
	}

	var out diag.Diagnostics
	for _, v := range set.Validators {
		req := validator.SetRequest{Path: p.AtName(last), PathExpression: p.AtName(last).Expression(), ConfigValue: value}
		var resp validator.SetResponse
		v.ValidateSet(ctx, req, &resp)
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
