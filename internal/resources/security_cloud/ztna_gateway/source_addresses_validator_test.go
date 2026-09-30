// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// sourceAddressesConfig builds a gateway configuration from the resource's real schema with every
// attribute null except the two the validator reads: the ipsec block, present with null contents
// or absent, and ipsec_source_ip_addresses, which is null, unknown or the given addresses.
func sourceAddressesConfig(t *testing.T, ipsecPresent bool, addresses tftypes.Value) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	var schemaResp resource.SchemaResponse
	(&GatewayResource{}).Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", schemaResp.Diagnostics)
	}
	rootType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema root is not an object")
	}
	values := make(map[string]tftypes.Value, len(rootType.AttributeTypes))
	for name, attrType := range rootType.AttributeTypes {
		values[name] = tftypes.NewValue(attrType, nil)
	}
	if ipsecPresent {
		ipsecType, ok := rootType.AttributeTypes["ipsec"].(tftypes.Object)
		if !ok {
			t.Fatal("ipsec is not an object")
		}
		inner := make(map[string]tftypes.Value, len(ipsecType.AttributeTypes))
		for name, attrType := range ipsecType.AttributeTypes {
			inner[name] = tftypes.NewValue(attrType, nil)
		}
		values["ipsec"] = tftypes.NewValue(ipsecType, inner)
	}
	values["ipsec_source_ip_addresses"] = addresses
	return tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(rootType, values)}
}

// TestIPSecSourceAddressesValidator pins both directions of the rule: Jamf Security Cloud refuses
// source addresses on a dedicated internet gateway, and, wire-probed 2026-09-30, refuses an IPsec
// gateway without them on create and refuses emptying them on update. An unknown value must pass,
// because the apply is what resolves it.
func TestIPSecSourceAddressesValidator(t *testing.T) {
	setType := tftypes.Set{ElementType: tftypes.String}
	someAddresses := tftypes.NewValue(setType, []tftypes.Value{tftypes.NewValue(tftypes.String, "3.66.107.208")})
	noAddresses := tftypes.NewValue(setType, nil)
	unknownAddresses := tftypes.NewValue(setType, tftypes.UnknownValue)

	cases := map[string]struct {
		ipsec     bool
		addresses tftypes.Value
		wantError bool
	}{
		"ipsec gateway with addresses":       {ipsec: true, addresses: someAddresses},
		"ipsec gateway without addresses":    {ipsec: true, addresses: noAddresses, wantError: true},
		"ipsec gateway with unknown":         {ipsec: true, addresses: unknownAddresses},
		"internet gateway without addresses": {addresses: noAddresses},
		"internet gateway with addresses":    {addresses: someAddresses, wantError: true},
		"internet gateway with unknown":      {addresses: unknownAddresses},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := resource.ValidateConfigRequest{Config: sourceAddressesConfig(t, tc.ipsec, tc.addresses)}
			var resp resource.ValidateConfigResponse
			ipsecSourceAddressesValidator{}.ValidateResource(context.Background(), req, &resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantError {
				t.Fatalf("HasError = %v, want %v: %v", got, tc.wantError, resp.Diagnostics)
			}
			if !tc.wantError {
				return
			}
			for _, d := range resp.Diagnostics.Errors() {
				withPath, ok := d.(interface{ Path() path.Path })
				if !ok || !withPath.Path().Equal(path.Root("ipsec_source_ip_addresses")) {
					t.Errorf("error %q is not attributed to ipsec_source_ip_addresses", d.Summary())
				}
			}
		})
	}
}
