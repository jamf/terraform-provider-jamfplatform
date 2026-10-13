// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package planmodifiers

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var keyedElemType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"id":   tftypes.String,
	"mode": tftypes.String,
	"fut":  tftypes.Bool,
}}

func keyedElem(id string, mode, fut any) tftypes.Value {
	return tftypes.NewValue(keyedElemType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, id),
		"mode": tftypes.NewValue(tftypes.String, mode),
		"fut":  tftypes.NewValue(tftypes.Bool, fut),
	})
}

func keyedRoot(elems ...tftypes.Value) tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"items": tftypes.Set{ElementType: keyedElemType},
	}}, map[string]tftypes.Value{
		"items": tftypes.NewValue(tftypes.Set{ElementType: keyedElemType}, elems),
	})
}

func keyedSchema() schema.Schema {
	return schema.Schema{Attributes: map[string]schema.Attribute{
		"items": schema.SetNestedAttribute{
			Optional: true,
			NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"id":   schema.StringAttribute{Required: true},
				"mode": schema.StringAttribute{Optional: true, Computed: true},
				"fut":  schema.BoolAttribute{Optional: true, Computed: true},
			}},
		},
	}}
}

// planElement builds the planned element's value as the framework would hand
// it to the modifier: the key known, the computed attributes Unknown.
func planElement(t *testing.T, id string) attr.Value {
	t.Helper()
	return types.ObjectValueMust(
		map[string]attr.Type{"id": types.StringType, "mode": types.StringType, "fut": types.BoolType},
		map[string]attr.Value{"id": types.StringValue(id), "mode": types.StringUnknown(), "fut": types.BoolUnknown()},
	)
}

func TestUseKeyedStateForUnknown(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sch := keyedSchema()

	unknownElem := func(id string) tftypes.Value { return keyedElem(id, tftypes.UnknownValue, tftypes.UnknownValue) }

	for _, tc := range []struct {
		name        string
		state       tftypes.Value
		stateIsNull bool
		planID      string
		wantFut     types.Bool
		wantMode    types.String
	}{
		{
			name:     "same key → carried from the matching element",
			state:    keyedRoot(keyedElem("1", "Install", true)),
			planID:   "1",
			wantFut:  types.BoolValue(true),
			wantMode: types.StringValue("Install"),
		},
		{
			name:     "re-keyed element → nothing carried from the old one",
			state:    keyedRoot(keyedElem("1", "Install", true)),
			planID:   "2",
			wantFut:  types.BoolUnknown(),
			wantMode: types.StringUnknown(),
		},
		{
			name:     "reordered set → paired by key, not by position",
			state:    keyedRoot(keyedElem("1", "Install", true), keyedElem("2", "Cache", false)),
			planID:   "2",
			wantFut:  types.BoolValue(false),
			wantMode: types.StringValue("Cache"),
		},
		{
			name:     "duplicate key in state → ambiguous, nothing carried",
			state:    keyedRoot(keyedElem("1", "Install", true), keyedElem("1", "Cache", false)),
			planID:   "1",
			wantFut:  types.BoolUnknown(),
			wantMode: types.StringUnknown(),
		},
		{
			name:     "prior value null → left Unknown",
			state:    keyedRoot(keyedElem("1", nil, nil)),
			planID:   "1",
			wantFut:  types.BoolUnknown(),
			wantMode: types.StringUnknown(),
		},
		{
			name:        "create (no state) → left Unknown",
			state:       keyedRoot(),
			stateIsNull: true,
			planID:      "1",
			wantFut:     types.BoolUnknown(),
			wantMode:    types.StringUnknown(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state := tfsdk.State{Schema: sch, Raw: tc.state}
			if tc.stateIsNull {
				state.Raw = tftypes.NewValue(tc.state.Type(), nil)
			}
			plan := tfsdk.Plan{Schema: sch, Raw: keyedRoot(unknownElem(tc.planID))}
			elem := path.Root("items").AtSetValue(planElement(t, tc.planID))

			boolResp := &planmodifier.BoolResponse{PlanValue: types.BoolUnknown()}
			UseKeyedStateForUnknownBool("id").PlanModifyBool(ctx, planmodifier.BoolRequest{
				Path: elem.AtName("fut"), Plan: plan, State: state,
				PlanValue: types.BoolUnknown(), ConfigValue: types.BoolNull(),
			}, boolResp)
			if boolResp.Diagnostics.HasError() {
				t.Fatalf("diagnostics: %v", boolResp.Diagnostics)
			}
			if !boolResp.PlanValue.Equal(tc.wantFut) {
				t.Errorf("fut = %v, want %v", boolResp.PlanValue, tc.wantFut)
			}

			strResp := &planmodifier.StringResponse{PlanValue: types.StringUnknown()}
			UseKeyedStateForUnknownString("id").PlanModifyString(ctx, planmodifier.StringRequest{
				Path: elem.AtName("mode"), Plan: plan, State: state,
				PlanValue: types.StringUnknown(), ConfigValue: types.StringNull(),
			}, strResp)
			if strResp.Diagnostics.HasError() {
				t.Fatalf("diagnostics: %v", strResp.Diagnostics)
			}
			if !strResp.PlanValue.Equal(tc.wantMode) {
				t.Errorf("mode = %v, want %v", strResp.PlanValue, tc.wantMode)
			}
		})
	}
}

func TestUseKeyedStateForUnknown_KnownPlanValueIsKept(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sch := keyedSchema()
	state := tfsdk.State{Schema: sch, Raw: keyedRoot(keyedElem("1", "Install", true))}
	plan := tfsdk.Plan{Schema: sch, Raw: keyedRoot(keyedElem("1", "Cache", false))}

	resp := &planmodifier.BoolResponse{PlanValue: types.BoolValue(false)}
	UseKeyedStateForUnknownBool("id").PlanModifyBool(ctx, planmodifier.BoolRequest{
		Path: path.Root("items").AtSetValue(planElement(t, "1")).AtName("fut"), Plan: plan, State: state,
		PlanValue: types.BoolValue(false), ConfigValue: types.BoolValue(false),
	}, resp)
	if !resp.PlanValue.Equal(types.BoolValue(false)) {
		t.Errorf("a configured value was overwritten: %v", resp.PlanValue)
	}
}

// TestUseKeyedStateForUnknown_List covers a list-nested parent, which the
// framework also pairs by index: inserting an element ahead of an existing one
// must not hand the new element the existing one's values.
func TestUseKeyedStateForUnknown_List(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	listType := tftypes.List{ElementType: keyedElemType}
	root := func(elems ...tftypes.Value) tftypes.Value {
		return tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{"items": listType}},
			map[string]tftypes.Value{"items": tftypes.NewValue(listType, elems)})
	}
	sch := schema.Schema{Attributes: map[string]schema.Attribute{
		"items": schema.ListNestedAttribute{
			Optional: true,
			NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"id":   schema.StringAttribute{Required: true},
				"mode": schema.StringAttribute{Optional: true, Computed: true},
				"fut":  schema.BoolAttribute{Optional: true, Computed: true},
			}},
		},
	}}
	// State holds one element, id 1. The plan inserts id 2 ahead of it, so id 1
	// moves to index 1 and id 2 takes index 0.
	state := tfsdk.State{Schema: sch, Raw: root(keyedElem("1", "Install", true))}
	unknown := func(id string) tftypes.Value { return keyedElem(id, tftypes.UnknownValue, tftypes.UnknownValue) }
	plan := tfsdk.Plan{Schema: sch, Raw: root(unknown("2"), unknown("1"))}

	for _, tc := range []struct {
		idx     int
		id      string
		wantFut types.Bool
	}{
		{0, "2", types.BoolUnknown()},
		{1, "1", types.BoolValue(true)},
	} {
		resp := &planmodifier.BoolResponse{PlanValue: types.BoolUnknown()}
		UseKeyedStateForUnknownBool("id").PlanModifyBool(ctx, planmodifier.BoolRequest{
			Path: path.Root("items").AtListIndex(tc.idx).AtName("fut"), Plan: plan, State: state,
			PlanValue: types.BoolUnknown(), ConfigValue: types.BoolNull(),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("diagnostics: %v", resp.Diagnostics)
		}
		if !resp.PlanValue.Equal(tc.wantFut) {
			t.Errorf("id %s: fut = %v, want %v", tc.id, resp.PlanValue, tc.wantFut)
		}
	}
}
