// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUUIDRegexp(t *testing.T) {
	cases := map[string]bool{
		"11111111-1111-4111-8111-111111111111":   true,
		"AAAAAAAA-BBBB-4CCC-9DDD-EEEEEEEEEEEE":   true,
		"11111111-1111-1111-8111-111111111111":   true,
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8":   true,
		"018f4f2a-7c3e-7000-8000-000000000000":   true,
		"00000000-0000-0000-0000-000000000000":   true,
		"11111111-1111-4111-c111-111111111111":   true,
		"11111111111141118111111111111111":       false,
		"11111111-1111-4111-8111-11111111111":    false,
		"11111111-1111-4111-8111-1111111111111":  false,
		"11111111-1111-4111-8111-11111111111g":   false,
		"11111111-1111-4111-8111-111111111111 ":  false,
		"{11111111-1111-4111-8111-111111111111}": false,
		"":                                       false,
	}
	for in, want := range cases {
		if got := uuidRegexp.MatchString(in); got != want {
			t.Errorf("%q: got %v, want %v", in, got, want)
		}
	}
}

func TestProviderSchemaRejectsNonUUID(t *testing.T) {
	var resp provider.SchemaResponse
	New("test")().Schema(context.Background(), provider.SchemaRequest{}, &resp)

	for _, name := range []string{"client_id", "tenant_id", "environment_id"} {
		attr, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s is not a string attribute", name)
		}
		for value, wantErr := range map[string]bool{
			"example-client-id":                    true,
			"85f69825-dc92-4522-aa3c-34eea2f20bc5": false,
			"6ba7b810-9dad-11d1-80b4-00c04fd430c8": false,
			"018f4f2a-7c3e-7000-8000-000000000000": false,
		} {
			var vr validator.StringResponse
			for _, v := range attr.Validators {
				v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root(name), ConfigValue: types.StringValue(value)}, &vr)
			}
			if got := vr.Diagnostics.HasError(); got != wantErr {
				t.Errorf("%s=%q: error=%v, want %v", name, value, got, wantErr)
			}
		}
		var vr validator.StringResponse
		for _, v := range attr.Validators {
			v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root(name), ConfigValue: types.StringUnknown()}, &vr)
		}
		if vr.Diagnostics.HasError() {
			t.Errorf("%s: unknown value must defer", name)
		}
	}
}
