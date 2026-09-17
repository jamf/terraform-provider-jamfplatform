// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package criteria

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// model builds a criterion model with the defaults the schema would compute left
// null, so the builders' defaulting is what is under test.
func model(name, searchType, value string) CriterionModel {
	return CriterionModel{
		Priority:              types.Int64Null(),
		Name:                  types.StringValue(name),
		SearchType:            types.StringValue(searchType),
		Value:                 types.StringValue(value),
		AndOr:                 types.StringNull(),
		HasOpeningParenthesis: types.BoolNull(),
		HasClosingParenthesis: types.BoolNull(),
	}
}

// TestBuildProGroupCriteriaNumbersFromIndex pins the rule the endpoints enforce:
// priorities run 0..n-1 in list order. A lone criterion at priority 5 is refused
// on the wire with 400 INVALID_REQUEST_PARAMETER_VALUE, so nothing else is legal.
func TestBuildProGroupCriteriaNumbersFromIndex(t *testing.T) {
	models := []CriterionModel{
		model("Operating System Version", "like", "15."),
		model("Computer Name", "like", "ARMADA"),
		model("Department", "is", "IT"),
	}

	computer := BuildComputerSmartGroupCriteria(models)
	if len(computer) != 3 {
		t.Fatalf("got %d criteria, want 3", len(computer))
	}
	for i, c := range computer {
		if c.Priority != i {
			t.Errorf("computer criterion %d: priority = %d, want %d", i, c.Priority, i)
		}
		if c.AndOr != pro.ComputerSmartGroupCriteriaV2AndOrAnd {
			t.Errorf("computer criterion %d: and_or = %q, want the SDK default", i, c.AndOr)
		}
		if c.OpeningParen == nil || *c.OpeningParen || c.ClosingParen == nil || *c.ClosingParen {
			t.Errorf("computer criterion %d: parentheses should default to an emitted false", i)
		}
	}

	mobile := BuildMobileDeviceSmartGroupCriteria(models)
	for i, c := range mobile {
		if c.Priority != i {
			t.Errorf("mobile criterion %d: priority = %d, want %d", i, c.Priority, i)
		}
		if c.AndOr != pro.MobileDeviceSmartGroupCriteriaV2AndOrAnd {
			t.Errorf("mobile criterion %d: and_or = %q, want the SDK default", i, c.AndOr)
		}
	}
}

// TestBuildProGroupCriteriaEmptyIsNotNil pins that empty input yields an emitted
// empty array. The endpoints full-replace, so this is how a resource clears every
// criterion; returning nil would omit the key and read as "no change" to anyone
// reasoning about the body, even though the server treats the two the same.
func TestBuildProGroupCriteriaEmptyIsNotNil(t *testing.T) {
	if got := BuildComputerSmartGroupCriteria(nil); got == nil || len(got) != 0 {
		t.Errorf("computer: got %#v, want an empty non-nil slice", got)
	}
	if got := BuildMobileDeviceSmartGroupCriteria([]CriterionModel{}); got == nil || len(got) != 0 {
		t.Errorf("mobile: got %#v, want an empty non-nil slice", got)
	}
}

// TestBuildProGroupCriteriaHonoursAuthoredValues pins that and_or and the
// parenthesis flags come from the configuration when set.
func TestBuildProGroupCriteriaHonoursAuthoredValues(t *testing.T) {
	m := model("Computer Name", "like", "x")
	m.AndOr = types.StringValue(pro.ComputerSmartGroupCriteriaV2AndOrOr)
	m.HasOpeningParenthesis = types.BoolValue(true)
	m.HasClosingParenthesis = types.BoolValue(true)

	got := BuildComputerSmartGroupCriteria([]CriterionModel{m})
	if got[0].AndOr != pro.ComputerSmartGroupCriteriaV2AndOrOr {
		t.Errorf("and_or = %q", got[0].AndOr)
	}
	if !*got[0].OpeningParen || !*got[0].ClosingParen {
		t.Error("parentheses were not carried through")
	}
}

// TestFlattenProGroupCriteria pins the round trip and that an absent or empty
// collection flattens to nil, so a group with no criteria holds a null list
// rather than an empty one.
func TestFlattenProGroupCriteria(t *testing.T) {
	if got := FlattenComputerSmartGroupCriteria(nil); got != nil {
		t.Errorf("nil should flatten to nil, got %#v", got)
	}
	empty := []pro.ComputerSmartGroupCriteriaV2{}
	if got := FlattenComputerSmartGroupCriteria(&empty); got != nil {
		t.Errorf("empty should flatten to nil, got %#v", got)
	}
	emptyMobile := []pro.MobileDeviceSmartGroupCriteriaV2{}
	if got := FlattenMobileDeviceSmartGroupCriteria(&emptyMobile); got != nil {
		t.Errorf("empty mobile should flatten to nil, got %#v", got)
	}

	models := []CriterionModel{model("Computer Name", "like", "ARMADA"), model("Department", "is", "IT")}
	wire := BuildComputerSmartGroupCriteria(models)
	back := FlattenComputerSmartGroupCriteria(&wire)
	if len(back) != 2 {
		t.Fatalf("got %d criteria back, want 2", len(back))
	}
	for i, c := range back {
		if c.Priority.ValueInt64() != int64(i) {
			t.Errorf("criterion %d: priority = %d", i, c.Priority.ValueInt64())
		}
		if c.Name.ValueString() != models[i].Name.ValueString() {
			t.Errorf("criterion %d: name = %q", i, c.Name.ValueString())
		}
		if c.HasOpeningParenthesis.IsNull() || c.HasClosingParenthesis.IsNull() {
			t.Errorf("criterion %d: parentheses must be known in state", i)
		}
	}
}

// TestFlattenProGroupCriteriaAbsentParens pins that a wire criterion omitting
// the parenthesis flags flattens to a known false rather than a null, which the
// schema's own default requires.
func TestFlattenProGroupCriteriaAbsentParens(t *testing.T) {
	wire := []pro.MobileDeviceSmartGroupCriteriaV2{{
		Name:       "Model",
		SearchType: "like",
		Value:      "iPad",
		AndOr:      pro.MobileDeviceSmartGroupCriteriaV2AndOrAnd,
		Priority:   0,
	}}
	got := FlattenMobileDeviceSmartGroupCriteria(&wire)
	if got[0].HasOpeningParenthesis.IsNull() || got[0].HasOpeningParenthesis.ValueBool() {
		t.Errorf("opening parenthesis = %v, want a known false", got[0].HasOpeningParenthesis)
	}
	if got[0].HasClosingParenthesis.IsNull() || got[0].HasClosingParenthesis.ValueBool() {
		t.Errorf("closing parenthesis = %v, want a known false", got[0].HasClosingParenthesis)
	}
}

// TestValidateCriteriaPriorities pins that an authored priority must equal its
// position, that the normal omitted case passes, and that the message names both
// the expected number and the offending one.
func TestValidateCriteriaPriorities(t *testing.T) {
	at := path.Root("criteria")

	omitted := []CriterionModel{model("a", "is", "1"), model("b", "is", "2")}
	if got := ValidateCriteriaPriorities(at, omitted); got.HasError() {
		t.Fatalf("omitted priorities should pass: %v", got)
	}

	matching := []CriterionModel{model("a", "is", "1"), model("b", "is", "2")}
	matching[0].Priority = types.Int64Value(0)
	matching[1].Priority = types.Int64Value(1)
	if got := ValidateCriteriaPriorities(at, matching); got.HasError() {
		t.Fatalf("priorities matching their positions should pass: %v", got)
	}

	wrong := []CriterionModel{model("a", "is", "1"), model("b", "is", "2")}
	wrong[1].Priority = types.Int64Value(5)
	got := ValidateCriteriaPriorities(at, wrong)
	if !got.HasError() {
		t.Fatal("a priority that is not its position should be refused")
	}
	detail := got.Errors()[0].Detail()
	for _, want := range []string{"must be 1", "not 5"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail does not say %q:\n%s", want, detail)
		}
	}

	unknown := []CriterionModel{model("a", "is", "1")}
	unknown[0].Priority = types.Int64Unknown()
	if got := ValidateCriteriaPriorities(at, unknown); got.HasError() {
		t.Fatalf("an unknown priority should pass: %v", got)
	}
}
