// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package prestagealias

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func b(v bool) types.Bool { return types.BoolValue(v) }

func TestResolveBool(t *testing.T) {
	ctx := context.Background()
	null, unk := types.BoolNull(), types.BoolUnknown()
	tests := []struct {
		name                  string
		cfgNew, cfgOld, stOld types.Bool
		invert                bool
		want                  Resolution
	}{
		{"neither set, no state", null, null, null, false, Resolution{SetOld: true, Old: null}},
		{"neither set, state holds old", null, null, b(true), false, Resolution{SetOld: true, Old: b(true)}},
		{"new set, no old in state", b(true), null, null, false, Resolution{SetOld: true, Old: null}},
		{"new set, old in state follows", b(true), null, b(false), false, Resolution{SetOld: true, Old: b(true)}},
		{"old set copies to new", null, b(true), b(true), false, Resolution{SetNew: true, New: b(true)}},
		{"inverted: old set negates into new", null, b(true), null, true, Resolution{SetNew: true, New: b(false)}},
		{"inverted: new set negates into old held in state", b(true), null, b(true), true, Resolution{SetOld: true, Old: b(false)}},
		{"inverted: new set, no old in state", b(true), null, null, true, Resolution{SetOld: true, Old: null}},
		{"unknown config writes nothing", unk, null, null, false, Resolution{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Resolve(ctx, tc.cfgNew, tc.cfgOld, tc.stOld, tc.invert)
			if got.SetNew != tc.want.SetNew || got.SetOld != tc.want.SetOld {
				t.Fatalf("flags = %+v, want %+v", got, tc.want)
			}
			eq := func(a, b attr.Value) bool { return (a == nil && b == nil) || (a != nil && b != nil && a.Equal(b)) }
			if tc.want.SetNew && !eq(got.New, tc.want.New) {
				t.Errorf("New = %v, want %v", got.New, tc.want.New)
			}
			if tc.want.SetOld && !eq(got.Old, tc.want.Old) {
				t.Errorf("Old = %v, want %v", got.Old, tc.want.Old)
			}
		})
	}
}

func TestResolveString(t *testing.T) {
	ctx := context.Background()
	got := Resolve(ctx, types.StringNull(), types.StringValue("hi"), types.StringNull(), false)
	if !got.SetNew || !got.New.Equal(types.StringValue("hi")) || got.SetOld {
		t.Fatalf("configured old should copy to new and leave itself alone: %+v", got)
	}
	got = Resolve(ctx, types.StringValue("hi"), types.StringNull(), types.StringNull(), false)
	if got.SetNew || !got.SetOld || !got.Old.IsNull() {
		t.Fatalf("old should plan null while state holds none: %+v", got)
	}
}

func TestEffectiveAndCarry(t *testing.T) {
	null := types.BoolNull()
	if !Effective(null, b(false), true, false) {
		t.Error("inverted deprecated false should send true")
	}
	if Effective(b(false), b(true), true, true) {
		t.Error("replacement wins over deprecated")
	}
	if !Effective(null, null, false, true) {
		t.Error("fallback used when neither is held")
	}
	if got := CarryBool(null, true, true); !got.IsNull() {
		t.Errorf("carry with no prior = %v, want null", got)
	}
	if got := CarryBool(b(true), true, true); got.ValueBool() {
		t.Errorf("inverted carry = %v, want false", got)
	}
	if got := CarryString(types.StringNull(), "x"); !got.IsNull() {
		t.Errorf("string carry with no prior = %v", got)
	}
	if got := EffectiveString(types.StringNull(), types.StringValue("o"), "f"); got != "o" {
		t.Errorf("EffectiveString = %q", got)
	}
	prior := types.SetNull(types.StringType)
	if got := CarrySet(prior, types.SetValueMust(types.StringType, []attr.Value{types.StringValue("1")})); !got.IsNull() {
		t.Errorf("set carry with no prior = %v", got)
	}
}
