// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package directory_binding

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type blockState int

const (
	blockNull blockState = iota
	blockSet
	blockUnknown
)

func nullObject(t tftypes.Type) tftypes.Value {
	obj := t.(tftypes.Object)
	attrs := make(map[string]tftypes.Value, len(obj.AttributeTypes))
	for name, at := range obj.AttributeTypes {
		attrs[name] = tftypes.NewValue(at, nil)
	}
	return tftypes.NewValue(t, attrs)
}

func validateTypeBlocks(t *testing.T, bindingType any, blocks map[string]blockState) []string {
	t.Helper()
	ctx := context.Background()
	var sresp resource.SchemaResponse
	NewDirectoryBindingResource().Schema(ctx, resource.SchemaRequest{}, &sresp)
	rootType := sresp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	attrs := make(map[string]tftypes.Value, len(rootType.AttributeTypes))
	for name, at := range rootType.AttributeTypes {
		attrs[name] = tftypes.NewValue(at, nil)
	}
	attrs["type"] = tftypes.NewValue(tftypes.String, bindingType)
	for name, st := range blocks {
		bt := rootType.AttributeTypes[name]
		switch st {
		case blockSet:
			attrs[name] = nullObject(bt)
		case blockUnknown:
			attrs[name] = tftypes.NewValue(bt, tftypes.UnknownValue)
		}
	}
	req := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: sresp.Schema, Raw: tftypes.NewValue(rootType, attrs)}}
	resp := &resource.ValidateConfigResponse{}
	typeBlockConfigValidator{}.ValidateResource(ctx, req, resp)
	var out []string
	for _, d := range resp.Diagnostics {
		out = append(out, d.Summary())
	}
	return out
}

func TestTypeBlockConfigValidator(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		bindingType any
		blocks      map[string]blockState
		want        []string
	}{
		{"AD with its block", typeActiveDirectory, map[string]blockState{"active_directory": blockSet}, nil},
		{"AD without its block", typeActiveDirectory, nil, []string{"active_directory required"}},
		{"AD with an unknown block", typeActiveDirectory, map[string]blockState{"active_directory": blockUnknown}, nil},
		{"AD with an unknown wrong block", typeActiveDirectory, map[string]blockState{"active_directory": blockSet, "centrify": blockUnknown}, nil},
		{"PowerBroker with an unknown block", typePowerBroker, map[string]blockState{"admitmac": blockUnknown}, nil},
		{"AD with the wrong block", typeActiveDirectory, map[string]blockState{"active_directory": blockSet, "centrify": blockSet}, []string{"centrify forbidden"}},
		{"AD with only the wrong block", typeActiveDirectory, map[string]blockState{"admitmac": blockSet}, []string{"active_directory required", "admitmac forbidden"}},
		{"AD with two wrong blocks", typeActiveDirectory, map[string]blockState{"active_directory": blockSet, "open_directory": blockSet, "centrify": blockSet}, []string{"open_directory forbidden", "centrify forbidden"}},
		{"Open Directory without its block", typeOpenDirectory, nil, []string{"open_directory required"}},
		{"ADmitMac without its block", typeADmitMac, nil, []string{"admitmac required"}},
		{"Centrify without its block", typeCentrify, nil, []string{"centrify required"}},
		{"Centrify with its block", typeCentrify, map[string]blockState{"centrify": blockSet}, nil},
		{"PowerBroker with no block", typePowerBroker, nil, nil},
		{"PowerBroker with a block", typePowerBroker, map[string]blockState{"admitmac": blockSet}, []string{"admitmac forbidden"}},
		{"unknown type", nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := validateTypeBlocks(t, tc.bindingType, tc.blocks)
			if len(got) != len(tc.want) {
				t.Fatalf("want diagnostics %v, got %v", tc.want, got)
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Fatalf("want diagnostics %v, got %v", tc.want, got)
				}
			}
		})
	}
}

func TestDirectoryBindingResource_TypeChangeReplaces(t *testing.T) {
	t.Parallel()
	var resp resource.SchemaResponse
	NewDirectoryBindingResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	attr, ok := resp.Schema.Attributes["type"].(rschema.StringAttribute)
	if !ok {
		t.Fatal("type must be a string attribute")
	}
	var found bool
	for _, m := range attr.PlanModifiers {
		if strings.Contains(m.Description(context.Background()), "destroy and recreate") {
			found = true
		}
	}
	if !found {
		t.Error("type must carry RequiresReplace: Jamf Pro keeps the type a binding was created with")
	}
}
