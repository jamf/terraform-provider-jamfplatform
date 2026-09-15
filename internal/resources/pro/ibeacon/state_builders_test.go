// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ibeacon

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestAssignIbeaconResourceModel_ConcreteMajorMinor(t *testing.T) {
	state := IbeaconResourceModel{}
	api := &proclassic.Ibeacon{
		ID:    new(64),
		Name:  new("Test 2"),
		UUID:  new("759b0599-64e0-416a-8d31-d8e93482a4d7"),
		Major: new("1"),
		Minor: new("2"),
	}

	diags := assignIbeaconResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if state.ID.ValueString() != "64" {
		t.Errorf("expected ID 64, got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "Test 2" {
		t.Errorf("expected Name Test 2, got %q", state.Name.ValueString())
	}
	if state.UUID.ValueString() != "759b0599-64e0-416a-8d31-d8e93482a4d7" {
		t.Errorf("UUID mismatch, got %q", state.UUID.ValueString())
	}
	if state.Major.IsNull() || state.Major.ValueInt64() != 1 {
		t.Errorf("expected Major=1, got %v", state.Major)
	}
	if state.Minor.IsNull() || state.Minor.ValueInt64() != 2 {
		t.Errorf("expected Minor=2, got %v", state.Minor)
	}
	if state.IncludeAnyMajorValue.ValueBool() {
		t.Errorf("expected IncludeAnyMajor=false on concrete major, got true")
	}
	if state.IncludeAnyMinorValue.ValueBool() {
		t.Errorf("expected IncludeAnyMinor=false on concrete minor, got true")
	}
}

func TestAssignIbeaconResourceModel_BothAxesSentinel(t *testing.T) {
	state := IbeaconResourceModel{}
	api := &proclassic.Ibeacon{
		ID:    new(63),
		Name:  new("Test"),
		UUID:  new("759b0599-64e0-416a-8d31-d8e93482a4d7"),
		Major: new("-1"),
		Minor: new("-1"),
	}

	diags := assignIbeaconResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if !state.Major.IsNull() {
		t.Errorf("expected Major=null when major sentinel, got %v", state.Major)
	}
	if !state.Minor.IsNull() {
		t.Errorf("expected Minor=null when minor sentinel, got %v", state.Minor)
	}
	if !state.IncludeAnyMajorValue.ValueBool() {
		t.Errorf("expected IncludeAnyMajor=true when -1 sentinel on major, got false")
	}
	if !state.IncludeAnyMinorValue.ValueBool() {
		t.Errorf("expected IncludeAnyMinor=true when -1 sentinel on minor, got false")
	}
}

func TestAssignIbeaconResourceModel_AnyMajorConcreteMinor(t *testing.T) {
	// Independent axes — Jamf supports e.g. major=-1 minor=5 ("any major,
	// specific minor"). What was previously an error path is now valid.
	state := IbeaconResourceModel{}
	api := &proclassic.Ibeacon{
		ID:    new(99),
		Name:  new("Mixed"),
		UUID:  new("759b0599-64e0-416a-8d31-d8e93482a4d7"),
		Major: new("-1"),
		Minor: new("5"),
	}

	diags := assignIbeaconResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("expected no diagnostics for any-major + concrete-minor, got %v", diags)
	}
	if !state.IncludeAnyMajorValue.ValueBool() {
		t.Errorf("expected IncludeAnyMajor=true for major=-1")
	}
	if !state.Major.IsNull() {
		t.Errorf("expected Major=null on any-major axis")
	}
	if state.IncludeAnyMinorValue.ValueBool() {
		t.Errorf("expected IncludeAnyMinor=false for concrete minor")
	}
	if state.Minor.ValueInt64() != 5 {
		t.Errorf("expected Minor=5, got %v", state.Minor)
	}
}

func TestAssignIbeaconResourceModel_ConcreteMajorAnyMinor(t *testing.T) {
	state := IbeaconResourceModel{}
	api := &proclassic.Ibeacon{
		ID:    new(100),
		Name:  new("Reverse"),
		UUID:  new("759b0599-64e0-416a-8d31-d8e93482a4d7"),
		Major: new("42"),
		Minor: new("-1"),
	}

	diags := assignIbeaconResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("expected no diagnostics for concrete-major + any-minor, got %v", diags)
	}
	if state.IncludeAnyMajorValue.ValueBool() {
		t.Errorf("expected IncludeAnyMajor=false for concrete major")
	}
	if state.Major.ValueInt64() != 42 {
		t.Errorf("expected Major=42, got %v", state.Major)
	}
	if !state.IncludeAnyMinorValue.ValueBool() {
		t.Errorf("expected IncludeAnyMinor=true for minor=-1")
	}
	if !state.Minor.IsNull() {
		t.Errorf("expected Minor=null on any-minor axis")
	}
}

func TestAssignIbeaconResourceModel_BothNilSafe(t *testing.T) {
	state := IbeaconResourceModel{}
	api := &proclassic.Ibeacon{
		ID:    new(7),
		Name:  new("Partial"),
		UUID:  new("759b0599-64e0-416a-8d31-d8e93482a4d7"),
		Major: nil,
		Minor: nil,
	}

	diags := assignIbeaconResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("nil major/minor must not error, got %v", diags)
	}
	if !state.Major.IsNull() || !state.Minor.IsNull() {
		t.Errorf("expected both null when API omits them")
	}
	if state.IncludeAnyMajorValue.ValueBool() || state.IncludeAnyMinorValue.ValueBool() {
		t.Errorf("expected IncludeAny*=false when both API-nil")
	}
}

func TestAssignIbeaconResourceModel_PreservesIDWhenAPINil(t *testing.T) {
	state := IbeaconResourceModel{ID: types.StringValue("17")}
	api := &proclassic.Ibeacon{ID: nil}

	diags := assignIbeaconResourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.ID.ValueString() != "17" {
		t.Errorf("expected state.ID preserved as %q, got %q", "17", state.ID.ValueString())
	}
}

func TestAssignIbeaconResourceModel_NilAPIIsNoop(t *testing.T) {
	state := IbeaconResourceModel{ID: types.StringValue("11"), Name: types.StringValue("Preset")}
	diags := assignIbeaconResourceModel(&state, nil)
	if diags.HasError() {
		t.Fatalf("nil API must not error, got %v", diags)
	}
	if state.ID.ValueString() != "11" || state.Name.ValueString() != "Preset" {
		t.Errorf("expected state unchanged on nil API")
	}
}

func TestAssignIbeaconResourceModel_OutOfRangeMajorErrors(t *testing.T) {
	state := IbeaconResourceModel{}
	api := &proclassic.Ibeacon{
		ID:    new(5),
		Name:  new("OOR"),
		UUID:  new("759b0599-64e0-416a-8d31-d8e93482a4d7"),
		Major: new("99999"),
		Minor: new("1"),
	}

	diags := assignIbeaconResourceModel(&state, api)
	if !diags.HasError() {
		t.Errorf("expected out-of-range diagnostic for major=99999")
	}
}

func TestAssignIbeaconDataSourceModel_ConcreteMajorMinor(t *testing.T) {
	state := IbeaconDataSourceModel{}
	api := &proclassic.Ibeacon{
		ID:    new(64),
		Name:  new("Test 2"),
		UUID:  new("759b0599-64e0-416a-8d31-d8e93482a4d7"),
		Major: new("1"),
		Minor: new("2"),
	}

	diags := assignIbeaconDataSourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if state.Major.ValueInt64() != 1 || state.Minor.ValueInt64() != 2 {
		t.Errorf("expected 1/2, got %v/%v", state.Major, state.Minor)
	}
	if state.IncludeAnyMajorValue.ValueBool() || state.IncludeAnyMinorValue.ValueBool() {
		t.Errorf("expected IncludeAny*=false on concrete pair")
	}
}

func TestAssignIbeaconDataSourceModel_BothAxesSentinel(t *testing.T) {
	state := IbeaconDataSourceModel{}
	api := &proclassic.Ibeacon{
		ID:    new(63),
		Name:  new("Test"),
		UUID:  new("759b0599-64e0-416a-8d31-d8e93482a4d7"),
		Major: new("-1"),
		Minor: new("-1"),
	}

	diags := assignIbeaconDataSourceModel(&state, api)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !state.IncludeAnyMajorValue.ValueBool() || !state.IncludeAnyMinorValue.ValueBool() {
		t.Errorf("expected IncludeAny*=true on sentinel pair")
	}
}

func TestAssignIbeaconDataSourceModel_NilAPIIsNoop(t *testing.T) {
	state := IbeaconDataSourceModel{ID: types.StringValue("9")}
	diags := assignIbeaconDataSourceModel(&state, nil)
	if diags.HasError() {
		t.Fatalf("nil API must not error, got %v", diags)
	}
	if state.ID.ValueString() != "9" {
		t.Errorf("expected state preserved, got %q", state.ID.ValueString())
	}
}

func TestParseIbeaconNumeric(t *testing.T) {
	cases := []struct {
		name    string
		in      *string
		wantNil bool
		want    int64
		wantErr bool
	}{
		{"nil", nil, true, 0, false},
		{"empty", new(""), true, 0, false},
		{"zero", new("0"), false, 0, false},
		{"max", new("65535"), false, 65535, false},
		{"non-int", new("abc"), false, 0, true},
		{"negative", new("-5"), false, 0, true},
		{"too-large", new("65536"), false, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseIbeaconNumeric(c.in)
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if c.wantNil && !got.IsNull() {
				t.Errorf("expected null, got %v", got)
			}
			if !c.wantNil && !c.wantErr && got.ValueInt64() != c.want {
				t.Errorf("expected %d, got %d", c.want, got.ValueInt64())
			}
		})
	}
}
