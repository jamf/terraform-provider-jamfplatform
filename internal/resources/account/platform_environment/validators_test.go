// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNoSurroundingWhitespace(t *testing.T) {
	cases := []struct {
		name    string
		value   types.String
		wantErr bool
		wantMsg string
	}{
		{"plain", types.StringValue("Elevate"), false, ""},
		{"inner spaces", types.StringValue("Elevate Hand Roll - EU"), false, ""},
		{"leading", types.StringValue(" Elevate"), true, `"Elevate"`},
		{"trailing", types.StringValue("Elevate\n"), true, `"Elevate"`},
		{"null", types.StringNull(), false, ""},
		{"unknown", types.StringUnknown(), false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			NoSurroundingWhitespace().ValidateString(context.Background(), validator.StringRequest{Path: path.Root("name"), ConfigValue: tc.value}, resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("error = %v, want %v (%v)", got, tc.wantErr, resp.Diagnostics)
			}
			if tc.wantErr && !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), tc.wantMsg) {
				t.Errorf("detail %q does not name the spelling to use", resp.Diagnostics.Errors()[0].Detail())
			}
		})
	}
}
