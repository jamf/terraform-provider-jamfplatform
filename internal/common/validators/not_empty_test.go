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

func TestNotEmptyString(t *testing.T) {
	const detail = "Jamf Pro refuses an empty value. Omit the attribute instead."
	cases := []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"empty", types.StringValue(""), true},
		{"value", types.StringValue("x"), false},
		{"space is a value", types.StringValue(" "), false},
		{"null defers to server", types.StringNull(), false},
		{"unknown defers to apply", types.StringUnknown(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			NotEmptyString(detail).ValidateString(context.Background(), validator.StringRequest{Path: path.Root("x"), ConfigValue: tc.value}, resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("error = %v, want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
			if tc.wantErr && !strings.Contains(resp.Diagnostics[0].Detail(), detail) {
				t.Errorf("detail %q does not carry the caller's explanation", resp.Diagnostics[0].Detail())
			}
		})
	}
}
