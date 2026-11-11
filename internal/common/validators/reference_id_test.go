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

func TestClearableReferenceID(t *testing.T) {
	cases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"positive id", types.StringValue("42"), false},
		{"one", types.StringValue("1"), false},
		{"large id", types.StringValue("9223372036854775807"), false},
		{"clear value", types.StringValue("-1"), false},
		{"null defers to server", types.StringNull(), false},
		{"unknown defers to apply", types.StringUnknown(), false},
		{"empty", types.StringValue(""), true},
		{"zero", types.StringValue("0"), true},
		{"other negative", types.StringValue("-2"), true},
		{"negative zero", types.StringValue("-0"), true},
		{"leading zero", types.StringValue("007"), true},
		{"plus sign", types.StringValue("+5"), true},
		{"whitespace", types.StringValue(" 5"), true},
		{"trailing whitespace", types.StringValue("5 "), true},
		{"non-numeric", types.StringValue("abc"), true},
		{"name instead of id", types.StringValue("Utilities"), true},
		{"decimal", types.StringValue("1.5"), true},
		{"overflow", types.StringValue("9223372036854775808"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			ClearableReferenceID("category").ValidateString(context.Background(), validator.StringRequest{Path: path.Root("category_id"), ConfigValue: tc.value}, resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("error = %v, want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
			if !tc.wantErr {
				return
			}
			d := resp.Diagnostics[0]
			if !strings.Contains(d.Detail(), `"-1"`) {
				t.Errorf("detail %q does not name the clear value", d.Detail())
			}
			if !strings.Contains(d.Summary()+d.Detail(), "category") {
				t.Errorf("diagnostic %q / %q does not name the referenced object", d.Summary(), d.Detail())
			}
		})
	}
}
