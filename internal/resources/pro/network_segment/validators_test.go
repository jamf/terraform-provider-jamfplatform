// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package network_segment

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestIPv4Validator_AcceptsIPv4(t *testing.T) {
	for _, in := range []string{"10.0.0.0", "192.168.1.255", "127.0.0.1", "0.0.0.0", "255.255.255.255"} {
		t.Run(in, func(t *testing.T) {
			req := validator.StringRequest{Path: path.Root("starting_address"), ConfigValue: types.StringValue(in)}
			var resp validator.StringResponse
			ipv4Address().ValidateString(context.Background(), req, &resp)
			if resp.Diagnostics.HasError() {
				t.Errorf("expected no diagnostics, got: %v", resp.Diagnostics)
			}
		})
	}
}

func TestIPv4Validator_RejectsNonIP(t *testing.T) {
	for _, in := range []string{"", "not-an-ip", "10.0.0", "10.0.0.0.0", "999.0.0.0", "10.0.0.0/24"} {
		t.Run(in, func(t *testing.T) {
			req := validator.StringRequest{Path: path.Root("starting_address"), ConfigValue: types.StringValue(in)}
			var resp validator.StringResponse
			ipv4Address().ValidateString(context.Background(), req, &resp)
			if !resp.Diagnostics.HasError() {
				t.Errorf("expected diagnostics for %q", in)
			}
		})
	}
}

func TestIPv4Validator_RejectsIPv6(t *testing.T) {
	for _, in := range []string{"::1", "fe80::1", "2001:db8::1"} {
		t.Run(in, func(t *testing.T) {
			req := validator.StringRequest{Path: path.Root("starting_address"), ConfigValue: types.StringValue(in)}
			var resp validator.StringResponse
			ipv4Address().ValidateString(context.Background(), req, &resp)
			if !resp.Diagnostics.HasError() {
				t.Errorf("expected IPv6 input %q to be rejected", in)
			}
		})
	}
}

func TestIPv4Validator_SkipsNullAndUnknown(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   types.String
	}{
		{"null", types.StringNull()},
		{"unknown", types.StringUnknown()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.StringRequest{Path: path.Root("starting_address"), ConfigValue: tc.in}
			var resp validator.StringResponse
			ipv4Address().ValidateString(context.Background(), req, &resp)
			if resp.Diagnostics.HasError() {
				t.Errorf("expected no diagnostics for %s, got: %v", tc.name, resp.Diagnostics)
			}
		})
	}
}

func TestRequiredWhenTrue(t *testing.T) {
	r := NewNetworkSegmentResource()
	var sresp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &sresp)
	s := sresp.Schema

	objType := s.Type().TerraformType(context.Background()).(tftypes.Object)
	cfg := func(override tftypes.Value, building tftypes.Value) tfsdk.Config {
		vals := map[string]tftypes.Value{}
		for k, t := range objType.AttributeTypes {
			vals[k] = tftypes.NewValue(t, nil)
		}
		vals["override_buildings"] = override
		vals["building"] = building
		return tfsdk.Config{Schema: s, Raw: tftypes.NewValue(objType, vals)}
	}
	str := func(v any) tftypes.Value { return tftypes.NewValue(tftypes.String, v) }
	boolean := func(v any) tftypes.Value { return tftypes.NewValue(tftypes.Bool, v) }

	cases := []struct {
		name     string
		override tftypes.Value
		building tftypes.Value
		wantErr  bool
	}{
		{"false without building validates", boolean(false), str(nil), false},
		{"null without building validates", boolean(nil), str(nil), false},
		{"true with building validates", boolean(true), str("HQ"), false},
		{"true without building fails", boolean(true), str(nil), true},
		{"true with unknown building defers", boolean(true), str(tftypes.UnknownValue), false},
		{"unknown override defers", boolean(tftypes.UnknownValue), str(nil), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := cfg(tc.override, tc.building)
			var val types.Bool
			if diags := c.GetAttribute(context.Background(), path.Root("override_buildings"), &val); diags.HasError() {
				t.Fatal(diags)
			}
			req := validator.BoolRequest{
				Path:           path.Root("override_buildings"),
				PathExpression: path.MatchRoot("override_buildings"),
				Config:         c,
				ConfigValue:    val,
			}
			var resp validator.BoolResponse
			requiredWhenTrue(path.MatchRoot("building")).ValidateBool(context.Background(), req, &resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Errorf("HasError=%v want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}
