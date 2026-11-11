// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package validators

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func runSetRule(t *testing.T, rule UniqueFieldRule, value types.Set) validator.SetResponse {
	t.Helper()
	req := validator.SetRequest{Path: path.Root("test_set"), ConfigValue: value}
	var resp validator.SetResponse
	UniqueStringFieldSetRule(rule).ValidateSet(context.Background(), req, &resp)
	return resp
}

var foldRule = UniqueFieldRule{
	Field:   "name",
	Key:     strings.ToLower,
	Summary: "Recipient listed more than once",
	Detail:  "%s is listed more than once. Jamf Pro refuses this.",
}

func TestUniqueStringFieldSetRule_KeyAndDetail(t *testing.T) {
	resp := runSetRule(t, foldRule, setOf(t,
		element(t, types.StringValue("a@example.com"), types.StringValue("A")),
		element(t, types.StringValue("A@Example.com"), types.StringValue("A")),
	))
	if got := resp.Diagnostics.ErrorsCount(); got != 1 {
		t.Fatalf("want 1 error for a case-folded repeat, got %d: %v", got, resp.Diagnostics)
	}
	d := resp.Diagnostics[0]
	if d.Summary() != foldRule.Summary {
		t.Errorf("summary = %q, want %q", d.Summary(), foldRule.Summary)
	}
	if !strings.HasSuffix(d.Detail(), "is listed more than once. Jamf Pro refuses this.") || !strings.Contains(strings.ToLower(d.Detail()), "a@example.com") {
		t.Errorf("detail = %q", d.Detail())
	}
}

func TestUniqueStringFieldSetRule_ReportsEachKeyOnce(t *testing.T) {
	resp := runSetRule(t, foldRule, setOf(t,
		element(t, types.StringValue("a@example.com"), types.StringValue("1")),
		element(t, types.StringValue("A@example.com"), types.StringValue("2")),
		element(t, types.StringValue("a@EXAMPLE.com"), types.StringValue("3")),
		element(t, types.StringValue("b@example.com"), types.StringValue("4")),
		element(t, types.StringValue("B@example.com"), types.StringValue("5")),
	))
	if got := resp.Diagnostics.ErrorsCount(); got != 2 {
		t.Errorf("want one error per repeated key (2), got %d: %v", got, resp.Diagnostics)
	}
}

func TestUniqueStringFieldSetRule_ExactWithoutKey(t *testing.T) {
	rule := foldRule
	rule.Key = nil
	resp := runSetRule(t, rule, setOf(t,
		element(t, types.StringValue("a@example.com"), types.StringValue("A")),
		element(t, types.StringValue("A@example.com"), types.StringValue("A")),
	))
	if resp.Diagnostics.HasError() {
		t.Errorf("a nil Key compares exactly; got %v", resp.Diagnostics)
	}
}

func TestUniqueStringFieldListRule_KeyDetailAndIndex(t *testing.T) {
	req := validator.ListRequest{Path: path.Root("test_list"), ConfigValue: listOf(t,
		element(t, types.StringValue("a@example.com"), types.StringValue("A")),
		element(t, types.StringValue("b@example.com"), types.StringValue("B")),
		element(t, types.StringValue("A@example.com"), types.StringValue("A")),
		element(t, types.StringUnknown(), types.StringValue("A")),
	)}
	var resp validator.ListResponse
	UniqueStringFieldListRule(foldRule).ValidateList(context.Background(), req, &resp)
	if got := resp.Diagnostics.ErrorsCount(); got != 1 {
		t.Fatalf("want 1 error, got %d: %v", got, resp.Diagnostics)
	}
	d := resp.Diagnostics[0]
	if d.Summary() != foldRule.Summary || !strings.Contains(d.Detail(), "A@example.com is listed more than once") {
		t.Errorf("diagnostic = %q / %q", d.Summary(), d.Detail())
	}
	if withPath, ok := d.(interface{ Path() path.Path }); !ok || !withPath.Path().Equal(path.Root("test_list").AtListIndex(2).AtName("name")) {
		t.Errorf("want the repeat's own index path, got %v", d)
	}
}

func TestUniqueStringFieldSetRule_DefersOnUnknowns(t *testing.T) {
	cases := map[string]types.Set{
		"null set":    types.SetNull(testElementType),
		"unknown set": types.SetUnknown(testElementType),
		"unknown keys": setOf(t,
			element(t, types.StringUnknown(), types.StringValue("a")),
			element(t, types.StringUnknown(), types.StringValue("b")),
		),
		"unknown key beside known": setOf(t,
			element(t, types.StringUnknown(), types.StringValue("a")),
			element(t, types.StringValue("a@example.com"), types.StringValue("b")),
		),
	}
	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			if resp := runSetRule(t, foldRule, set); resp.Diagnostics.HasError() {
				t.Errorf("expected defer, got: %v", resp.Diagnostics)
			}
		})
	}
}
