// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package planmodifiers

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var (
	existingBlockLeafType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{"leaf": tftypes.String}}
	existingBlockRootType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"top": tftypes.String,
		"blk": existingBlockLeafType,
	}}
)

func existingBlockSchema() schema.Schema {
	return schema.Schema{Attributes: map[string]schema.Attribute{
		"top": schema.StringAttribute{Optional: true, Computed: true},
		"blk": schema.SingleNestedAttribute{
			Optional:   true,
			Attributes: map[string]schema.Attribute{"leaf": schema.StringAttribute{Optional: true, Computed: true}},
		},
	}}
}

// existingBlockState builds state with the block present (or null) and the
// given prior values. A nil prior is a null leaf.
func existingBlockState(blockPresent bool, top, leaf *string) tfsdk.State {
	str := func(s *string) tftypes.Value {
		if s == nil {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, *s)
	}
	blk := tftypes.NewValue(existingBlockLeafType, nil)
	if blockPresent {
		blk = tftypes.NewValue(existingBlockLeafType, map[string]tftypes.Value{"leaf": str(leaf)})
	}
	return tfsdk.State{
		Schema: existingBlockSchema(),
		Raw:    tftypes.NewValue(existingBlockRootType, map[string]tftypes.Value{"top": str(top), "blk": blk}),
	}
}

func TestUseStateForUnknownInExistingBlockString(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	x := "x"
	leafPath := path.Root("blk").AtName("leaf")

	for _, tc := range []struct {
		name          string
		path          path.Path
		state         tfsdk.State
		stateValue    types.String
		planValue     types.String
		configUnknown bool
		want          types.String
	}{
		{
			name:       "block exists, prior null: null carried, so a plan that collapses elsewhere stays empty",
			path:       leafPath,
			state:      existingBlockState(true, nil, nil),
			stateValue: types.StringNull(),
			planValue:  types.StringUnknown(),
			want:       types.StringNull(),
		},
		{
			name:       "block is new, prior null: Unknown, so apply can store the server's default",
			path:       leafPath,
			state:      existingBlockState(false, nil, nil),
			stateValue: types.StringNull(),
			planValue:  types.StringUnknown(),
			want:       types.StringUnknown(),
		},
		{
			name:       "block exists, prior set: carried",
			path:       leafPath,
			state:      existingBlockState(true, nil, &x),
			stateValue: types.StringValue(x),
			planValue:  types.StringUnknown(),
			want:       types.StringValue(x),
		},
		{
			name:       "top-level leaf, prior null: carried like UseStateForUnknown",
			path:       path.Root("top"),
			state:      existingBlockState(false, nil, nil),
			stateValue: types.StringNull(),
			planValue:  types.StringUnknown(),
			want:       types.StringNull(),
		},
		{
			name:       "leaf in a list element, prior null: Unknown, a new element has no prior",
			path:       path.Root("blk").AtListIndex(0).AtName("leaf"),
			state:      existingBlockState(true, nil, nil),
			stateValue: types.StringNull(),
			planValue:  types.StringUnknown(),
			want:       types.StringUnknown(),
		},
		{
			name:       "leaf in a list element, prior set: carried",
			path:       path.Root("blk").AtListIndex(0).AtName("leaf"),
			state:      existingBlockState(true, nil, &x),
			stateValue: types.StringValue(x),
			planValue:  types.StringUnknown(),
			want:       types.StringValue(x),
		},
		{
			name:       "create (no state): left Unknown",
			path:       leafPath,
			state:      tfsdk.State{Schema: existingBlockSchema(), Raw: tftypes.NewValue(existingBlockRootType, nil)},
			stateValue: types.StringNull(),
			planValue:  types.StringUnknown(),
			want:       types.StringUnknown(),
		},
		{
			name:          "config value unknown: left Unknown",
			path:          leafPath,
			state:         existingBlockState(true, nil, &x),
			stateValue:    types.StringValue(x),
			planValue:     types.StringUnknown(),
			configUnknown: true,
			want:          types.StringUnknown(),
		},
		{
			name:       "plan value already known: untouched",
			path:       leafPath,
			state:      existingBlockState(true, nil, &x),
			stateValue: types.StringValue(x),
			planValue:  types.StringValue("configured"),
			want:       types.StringValue("configured"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := types.StringNull()
			if tc.configUnknown {
				cfg = types.StringUnknown()
			}
			req := planmodifier.StringRequest{Path: tc.path, State: tc.state, ConfigValue: cfg, StateValue: tc.stateValue, PlanValue: tc.planValue}
			resp := &planmodifier.StringResponse{PlanValue: tc.planValue}
			UseStateForUnknownInExistingBlockString().PlanModifyString(ctx, req, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			if !resp.PlanValue.Equal(tc.want) {
				t.Errorf("plan value = %#v, want %#v", resp.PlanValue, tc.want)
			}
		})
	}
}

// TestUseStateForUnknownInExistingBlock_BoolAndInt64 covers the two siblings
// for the case that matters: the block is new, or it exists with a null prior.
func TestUseStateForUnknownInExistingBlock_BoolAndInt64(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	leafPath := path.Root("blk").AtName("leaf")

	for _, tc := range []struct {
		name         string
		blockPresent bool
		wantCarried  bool
	}{
		{"block exists, prior null: carried", true, true},
		{"block is new, prior null: Unknown", false, false},
	} {
		t.Run("bool/"+tc.name, func(t *testing.T) {
			t.Parallel()
			req := planmodifier.BoolRequest{Path: leafPath, State: existingBlockState(tc.blockPresent, nil, nil), ConfigValue: types.BoolNull(), StateValue: types.BoolNull(), PlanValue: types.BoolUnknown()}
			resp := &planmodifier.BoolResponse{PlanValue: req.PlanValue}
			UseStateForUnknownInExistingBlockBool().PlanModifyBool(ctx, req, resp)
			if got := !resp.PlanValue.IsUnknown(); got != tc.wantCarried {
				t.Errorf("carried = %v, want %v", got, tc.wantCarried)
			}
		})
		t.Run("int64/"+tc.name, func(t *testing.T) {
			t.Parallel()
			req := planmodifier.Int64Request{Path: leafPath, State: existingBlockState(tc.blockPresent, nil, nil), ConfigValue: types.Int64Null(), StateValue: types.Int64Null(), PlanValue: types.Int64Unknown()}
			resp := &planmodifier.Int64Response{PlanValue: req.PlanValue}
			UseStateForUnknownInExistingBlockInt64().PlanModifyInt64(ctx, req, resp)
			if got := !resp.PlanValue.IsUnknown(); got != tc.wantCarried {
				t.Errorf("carried = %v, want %v", got, tc.wantCarried)
			}
		})
	}
}
