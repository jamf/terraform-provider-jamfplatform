// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package prestageskip

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var (
	tru = types.BoolValue(true)
	fls = types.BoolValue(false)
	nul = types.BoolNull()
	unk = types.BoolUnknown()
)

func TestResolve(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                     string
		cfgNew, cfgOld, stateOld types.Bool
		want                     Resolution
	}{
		{"replacement only, no history", tru, nul, nul, Resolution{SetOld: true, Old: nul}},
		{"replacement only, history", tru, nul, fls, Resolution{SetOld: true, Old: tru}},
		{"deprecated only, no history", nul, tru, nul, Resolution{SetNew: true, New: tru}},
		{"deprecated only, history", nul, fls, tru, Resolution{SetNew: true, New: fls}},
		{"neither, history", nul, nul, tru, Resolution{SetOld: true, Old: tru}},
		{"neither, no history", nul, nul, nul, Resolution{SetOld: true, Old: nul}},
		{"replacement unknown", unk, nul, tru, Resolution{}},
		{"deprecated unknown", nul, unk, tru, Resolution{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Resolve(tc.cfgNew, tc.cfgOld, tc.stateOld)
			if got.SetNew != tc.want.SetNew || got.SetOld != tc.want.SetOld {
				t.Fatalf("Set flags: got %+v, want %+v", got, tc.want)
			}
			if got.SetNew && !got.New.Equal(tc.want.New) {
				t.Errorf("New: got %v, want %v", got.New, tc.want.New)
			}
			if got.SetOld && !got.Old.Equal(tc.want.Old) {
				t.Errorf("Old: got %v, want %v", got.Old, tc.want.Old)
			}
		})
	}
}

func TestEffective(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                  string
		replacement, deprecat types.Bool
		want                  bool
	}{
		{"replacement wins", tru, fls, true},
		{"replacement false wins", fls, tru, false},
		{"falls back to deprecated", nul, tru, true},
		{"unknown replacement falls back", unk, tru, true},
		{"neither", nul, nul, false},
	}
	for _, tc := range cases {
		if got := Effective(tc.replacement, tc.deprecat); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCarryAlias(t *testing.T) {
	t.Parallel()
	if got := CarryAlias(nul, true); !got.IsNull() {
		t.Errorf("no prior value must stay null so import omits the deprecated name, got %v", got)
	}
	if got := CarryAlias(unk, true); !got.IsNull() {
		t.Errorf("unknown prior must stay null, got %v", got)
	}
	if got := CarryAlias(fls, true); !got.Equal(tru) {
		t.Errorf("prior value must refresh from the wire, got %v", got)
	}
}

func testSchema() schema.Schema {
	optComp := func() schema.Attribute { return schema.BoolAttribute{Optional: true, Computed: true} }
	return schema.Schema{Attributes: map[string]schema.Attribute{
		Block: schema.SingleNestedAttribute{
			Optional: true,
			Attributes: map[string]schema.Attribute{
				"location":             optComp(),
				"terms_and_conditions": optComp(),
				"tos": schema.BoolAttribute{
					Optional: true, Computed: true,
					Validators: Conflicts("terms_and_conditions"),
				},
			},
		},
	}}
}

func boolVal(b types.Bool) tftypes.Value {
	switch {
	case b.IsUnknown():
		return tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue)
	case b.IsNull():
		return tftypes.NewValue(tftypes.Bool, nil)
	}
	return tftypes.NewValue(tftypes.Bool, b.ValueBool())
}

var blockType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"location": tftypes.Bool, "terms_and_conditions": tftypes.Bool, "tos": tftypes.Bool,
}}

var rootType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{Block: blockType}}

func raw(location, newV, oldV types.Bool) tftypes.Value {
	block := tftypes.NewValue(blockType, map[string]tftypes.Value{
		"location": boolVal(location), "terms_and_conditions": boolVal(newV), "tos": boolVal(oldV),
	})
	return tftypes.NewValue(rootType, map[string]tftypes.Value{Block: block})
}

func rawNoBlock() tftypes.Value {
	return tftypes.NewValue(rootType, map[string]tftypes.Value{Block: tftypes.NewValue(blockType, nil)})
}

func reconcile(t *testing.T, plan, cfg, state tftypes.Value) (newV, oldV types.Bool) {
	t.Helper()
	s := testSchema()
	req := resource.ModifyPlanRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: plan},
		Config: tfsdk.Config{Schema: s, Raw: cfg},
		State:  tfsdk.State{Schema: s, Raw: state},
	}
	resp := &resource.ModifyPlanResponse{Plan: req.Plan}
	ReconcilePlan(context.Background(), req, resp, []Alias{{Old: "tos", New: "terms_and_conditions"}})
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if d := resp.Plan.GetAttribute(context.Background(), path.Root(Block).AtName("terms_and_conditions"), &newV); d.HasError() {
		t.Fatalf("read new: %v", d)
	}
	if d := resp.Plan.GetAttribute(context.Background(), path.Root(Block).AtName("tos"), &oldV); d.HasError() {
		t.Fatalf("read old: %v", d)
	}
	return newV, oldV
}

func TestReconcilePlan_CreateWithDeprecatedNameCopiesItIntoTheReplacement(t *testing.T) {
	t.Parallel()
	newV, oldV := reconcile(t,
		raw(unk, unk, tru),
		raw(nul, nul, tru),
		tftypes.NewValue(rootType, nil),
	)
	if !newV.Equal(tru) || !oldV.Equal(tru) {
		t.Errorf("both must plan true, got new=%v old=%v", newV, oldV)
	}
}

func TestReconcilePlan_CreateWithReplacementLeavesDeprecatedNull(t *testing.T) {
	t.Parallel()
	newV, oldV := reconcile(t,
		raw(unk, tru, unk),
		raw(nul, tru, nul),
		tftypes.NewValue(rootType, nil),
	)
	if !newV.Equal(tru) {
		t.Errorf("replacement must keep its configured value, got %v", newV)
	}
	if !oldV.IsNull() {
		t.Errorf("a new resource must not gain the deprecated attribute, got %v", oldV)
	}
}

func TestReconcilePlan_SwitchingNamesKeepsTheTrackedAliasInStep(t *testing.T) {
	t.Parallel()
	newV, oldV := reconcile(t,
		raw(fls, tru, unk),
		raw(fls, tru, nul),
		raw(fls, fls, fls),
	)
	if !newV.Equal(tru) || !oldV.Equal(tru) {
		t.Errorf("the alias state already holds must follow the replacement, got new=%v old=%v", newV, oldV)
	}
}

func TestReconcilePlan_UntouchedPairKeepsStateValues(t *testing.T) {
	t.Parallel()
	newV, oldV := reconcile(t,
		raw(unk, unk, unk),
		raw(nul, nul, nul),
		raw(fls, tru, tru),
	)
	if !oldV.Equal(tru) {
		t.Errorf("deprecated attribute must keep its state value, got %v", oldV)
	}
	if !newV.IsUnknown() {
		t.Errorf("the replacement is left to its own plan modifier, got %v", newV)
	}
}

func TestReconcilePlan_NeverTrackedAliasStaysNullOnUpdate(t *testing.T) {
	t.Parallel()
	_, oldV := reconcile(t,
		raw(unk, unk, unk),
		raw(nul, nul, nul),
		raw(fls, tru, nul),
	)
	if !oldV.IsNull() {
		t.Errorf("a deprecated attribute state never held must plan null, not unknown, got %v", oldV)
	}
}

func TestReconcilePlan_UnknownConfigIsLeftAlone(t *testing.T) {
	t.Parallel()
	newV, oldV := reconcile(t,
		raw(unk, unk, unk),
		raw(nul, unk, nul),
		raw(fls, tru, tru),
	)
	if !newV.IsUnknown() || !oldV.IsUnknown() {
		t.Errorf("an unknown configured value must not be second-guessed, got new=%v old=%v", newV, oldV)
	}
}

func TestReconcilePlan_OmittedBlockIsANoOp(t *testing.T) {
	t.Parallel()
	s := testSchema()
	req := resource.ModifyPlanRequest{
		Plan:   tfsdk.Plan{Schema: s, Raw: rawNoBlock()},
		Config: tfsdk.Config{Schema: s, Raw: rawNoBlock()},
		State:  tfsdk.State{Schema: s, Raw: rawNoBlock()},
	}
	resp := &resource.ModifyPlanResponse{Plan: req.Plan}
	ReconcilePlan(context.Background(), req, resp, []Alias{{Old: "tos", New: "terms_and_conditions"}})
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics: %v", resp.Diagnostics)
	}
	if !resp.Plan.Raw.Equal(req.Plan.Raw) {
		t.Errorf("an omitted block must leave the plan untouched")
	}
}

func TestConflicts_SettingBothNamesIsAnError(t *testing.T) {
	t.Parallel()
	s := testSchema()
	cases := []struct {
		name      string
		cfg       tftypes.Value
		wantError bool
	}{
		{"both set", raw(nul, tru, tru), true},
		{"only deprecated", raw(nul, nul, tru), false},
		{"only replacement", raw(nul, tru, nul), false},
		{"neither", raw(nul, nul, nul), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := tfsdk.Config{Schema: s, Raw: tc.cfg}
			var cfgOld types.Bool
			if d := cfg.GetAttribute(context.Background(), path.Root(Block).AtName("tos"), &cfgOld); d.HasError() {
				t.Fatal(d)
			}
			var diags int
			for _, v := range Conflicts("terms_and_conditions") {
				resp := &validator.BoolResponse{}
				v.ValidateBool(context.Background(), validator.BoolRequest{
					Path:           path.Root(Block).AtName("tos"),
					PathExpression: path.MatchRoot(Block).AtName("tos"),
					Config:         cfg,
					ConfigValue:    cfgOld,
				}, resp)
				if resp.Diagnostics.HasError() {
					diags++
				}
			}
			if (diags > 0) != tc.wantError {
				t.Errorf("error=%v, want %v", diags > 0, tc.wantError)
			}
		})
	}
}

func TestEffectiveOr_FallsBackOnlyWhenNeitherNameHoldsAValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                    string
		replacement, deprecated types.Bool
		fallback, want          bool
	}{
		{"replacement wins", types.BoolValue(false), types.BoolValue(true), true, false},
		{"deprecated next", types.BoolNull(), types.BoolValue(true), false, true},
		{"unknown replacement falls through", types.BoolUnknown(), types.BoolNull(), true, true},
		{"fallback last", types.BoolNull(), types.BoolUnknown(), true, true},
		{"false when nothing else", types.BoolNull(), types.BoolNull(), false, false},
	}
	for _, tc := range cases {
		if got := EffectiveOr(tc.replacement, tc.deprecated, tc.fallback); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
