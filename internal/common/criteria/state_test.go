// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package criteria

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestBuildCriterionSlice_PrioritiesDefaultsAndSort(t *testing.T) {
	models := []CriterionModel{
		{
			Name:       types.StringValue("Computer Name"),
			SearchType: types.StringValue("like"),
			Value:      types.StringValue("lab"),
			// priority/and_or/parens omitted → index + defaults.
			Priority:              types.Int64Null(),
			AndOr:                 types.StringNull(),
			HasOpeningParenthesis: types.BoolNull(),
			HasClosingParenthesis: types.BoolNull(),
		},
		{
			Name:       types.StringValue("Serial Number"),
			SearchType: types.StringValue("is"),
			Value:      types.StringValue("X"),
			Priority:   types.Int64Value(1),
			AndOr:      types.StringValue("or"),
		},
	}

	out := BuildCriterionSlice(models)
	if len(out) != 2 {
		t.Fatalf("expected 2 criteria, got %d", len(out))
	}
	// priority[0] from index, priority[1] explicit.
	if out[0].Priority == nil || *out[0].Priority != 0 {
		t.Errorf("priority[0] expected 0 (index), got %v", out[0].Priority)
	}
	if out[1].Priority == nil || *out[1].Priority != 1 {
		t.Errorf("priority[1] expected 1, got %v", out[1].Priority)
	}
	// and_or default "and" when null; explicit "or" preserved.
	if out[0].AndOr == nil || *out[0].AndOr != "and" {
		t.Errorf("and_or[0] expected default 'and', got %v", out[0].AndOr)
	}
	if out[1].AndOr == nil || *out[1].AndOr != "or" {
		t.Errorf("and_or[1] expected 'or', got %v", out[1].AndOr)
	}
	// Parens default to false (non-nil) when null.
	if out[0].OpeningParen == nil || *out[0].OpeningParen {
		t.Errorf("opening_paren[0] expected false, got %v", out[0].OpeningParen)
	}
	if out[0].ClosingParen == nil || *out[0].ClosingParen {
		t.Errorf("closing_paren[0] expected false, got %v", out[0].ClosingParen)
	}
}

func TestBuildCriterionSlice_SortsByPriority(t *testing.T) {
	models := []CriterionModel{
		{Name: types.StringValue("b"), SearchType: types.StringValue("is"), Value: types.StringValue("y"), Priority: types.Int64Value(2)},
		{Name: types.StringValue("a"), SearchType: types.StringValue("is"), Value: types.StringValue("x"), Priority: types.Int64Value(0)},
		{Name: types.StringValue("c"), SearchType: types.StringValue("is"), Value: types.StringValue("z"), Priority: types.Int64Value(1)},
	}
	out := BuildCriterionSlice(models)
	for i := 1; i < len(out); i++ {
		if *out[i-1].Priority > *out[i].Priority {
			t.Fatalf("not sorted by priority: %d then %d", *out[i-1].Priority, *out[i].Priority)
		}
	}
}

func TestBuildCriterionSlice_EmptyInputReturnsNonNilEmptySlice(t *testing.T) {
	// Always-emit contract: an empty input must produce a non-nil (empty) slice
	// so callers can wrap it in an always-emitted <criteria> element to clear
	// criteria server-side.
	out := BuildCriterionSlice(nil)
	if out == nil {
		t.Fatal("expected non-nil empty slice for empty input")
	}
	if len(out) != 0 {
		t.Fatalf("expected length 0, got %d", len(out))
	}
}

func TestFlattenCriterionSlice_NilAndEmptyReturnNil(t *testing.T) {
	if got := FlattenCriterionSlice(nil); got != nil {
		t.Errorf("nil wrapper should flatten to nil, got %v", got)
	}
	empty := []proclassic.Criterion{}
	if got := FlattenCriterionSlice(&empty); got != nil {
		t.Errorf("empty slice should flatten to nil, got %v", got)
	}
}

func TestFlattenCriterionSlice_CopiesFields(t *testing.T) {
	name := "Computer Name"
	st := "like"
	val := "lab"
	andOr := "and"
	pri := 3
	open := true
	closeP := false
	src := []proclassic.Criterion{{
		Name:         &name,
		SearchType:   &st,
		Value:        &val,
		AndOr:        &andOr,
		Priority:     &pri,
		OpeningParen: &open,
		ClosingParen: &closeP,
	}}

	out := FlattenCriterionSlice(&src)
	if len(out) != 1 {
		t.Fatalf("expected 1 criterion, got %d", len(out))
	}
	c := out[0]
	if c.Name.ValueString() != name {
		t.Errorf("name mismatch: %q", c.Name.ValueString())
	}
	if c.SearchType.ValueString() != st {
		t.Errorf("search_type mismatch: %q", c.SearchType.ValueString())
	}
	if c.Priority.ValueInt64() != int64(pri) {
		t.Errorf("priority mismatch: %d", c.Priority.ValueInt64())
	}
	if !c.HasOpeningParenthesis.ValueBool() {
		t.Errorf("opening paren should be true")
	}
	if c.HasClosingParenthesis.ValueBool() {
		t.Errorf("closing paren should be false")
	}
}
