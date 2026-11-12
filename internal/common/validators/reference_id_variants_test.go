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

func TestClearableReferenceIDWith(t *testing.T) {
	cases := []struct {
		value   string
		wantErr bool
	}{
		{"42", false}, {"-1", false}, {"-2", false},
		{"", true}, {"0", true}, {"-3", true}, {"007", true}, {"abc", true},
	}
	v := ClearableReferenceIDWith("distribution point", "-2")
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			resp := &validator.StringResponse{}
			v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("x"), ConfigValue: types.StringValue(tc.value)}, resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("error = %v, want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
			if tc.wantErr && !strings.Contains(resp.Diagnostics[0].Detail(), `"-2"`) {
				t.Errorf("detail %q does not name -2", resp.Diagnostics[0].Detail())
			}
		})
	}
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringNull()}, resp)
	v.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringUnknown()}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("null and unknown must defer: %v", resp.Diagnostics)
	}
}

func TestPositiveReferenceID(t *testing.T) {
	cases := []struct {
		value   string
		wantErr bool
	}{
		{"42", false}, {"", true}, {"0", true}, {"-1", true}, {"007", true}, {"abc", true},
	}
	v := PositiveReferenceID("icon", "The icon cannot be removed.")
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			resp := &validator.StringResponse{}
			v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("x"), ConfigValue: types.StringValue(tc.value)}, resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("error = %v, want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
			if tc.wantErr && !strings.Contains(resp.Diagnostics[0].Detail(), "cannot be removed") {
				t.Errorf("detail %q lacks the hint", resp.Diagnostics[0].Detail())
			}
		})
	}
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringNull()}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("null must defer: %v", resp.Diagnostics)
	}
}

func TestInt64ReferenceIDs(t *testing.T) {
	run := func(v validator.Int64, val types.Int64) bool {
		resp := &validator.Int64Response{}
		v.ValidateInt64(context.Background(), validator.Int64Request{Path: path.Root("x"), ConfigValue: val}, resp)
		return resp.Diagnostics.HasError()
	}
	clearable := ClearableReferenceIDInt64("site")
	for n, wantErr := range map[int64]bool{5: false, 1: false, -1: false, 0: true, -2: true} {
		if got := run(clearable, types.Int64Value(n)); got != wantErr {
			t.Errorf("clearable %d: error = %v, want %v", n, got, wantErr)
		}
	}
	positive := PositiveReferenceIDInt64("smart group", "Remove the attribute for any group.")
	for n, wantErr := range map[int64]bool{5: false, -1: true, 0: true} {
		if got := run(positive, types.Int64Value(n)); got != wantErr {
			t.Errorf("positive %d: error = %v, want %v", n, got, wantErr)
		}
	}
	for _, v := range []validator.Int64{clearable, positive} {
		if run(v, types.Int64Null()) || run(v, types.Int64Unknown()) {
			t.Error("null and unknown must defer")
		}
	}
}
