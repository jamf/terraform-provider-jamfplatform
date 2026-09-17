// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package progroups

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
)

// named builds a criterion model carrying only a name, which is all the
// patch-reporting guard looks at.
func named(name string) criteria.CriterionModel {
	return criteria.CriterionModel{
		Priority:              types.Int64Null(),
		Name:                  types.StringValue(name),
		SearchType:            types.StringValue("is"),
		Value:                 types.StringValue("1.0"),
		AndOr:                 types.StringNull(),
		HasOpeningParenthesis: types.BoolNull(),
		HasClosingParenthesis: types.BoolNull(),
	}
}

// TestValidatePatchReportingCriteria pins which criterion names are refused. The
// prefix has to match Jamf Pro's generated per-title form exactly: too loose and
// it blocks ordinary criteria, too tight and the apply fails with a raw 400.
func TestValidatePatchReportingCriteria(t *testing.T) {
	at := path.Root("criteria")

	tests := []struct {
		name    string
		crit    string
		refused bool
	}{
		{name: "a generated patch reporting criterion", crit: "Patch Reporting: Zulu OpenJDK 9", refused: true},
		{name: "another title", crit: "Patch Reporting: Google Chrome", refused: true},
		{name: "a title containing a colon", crit: "Patch Reporting: Adobe: Acrobat", refused: true},
		{name: "an ordinary criterion", crit: "Operating System Version"},
		{name: "a criterion merely mentioning patches", crit: "Number of Available Updates"},
		{name: "a directory service group criterion", crit: "Assigned User directory service group"},
		{name: "the prefix without a title is not the generated form", crit: "Patch Reporting"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidatePatchReportingCriteria(at, []criteria.CriterionModel{named(tc.crit)})
			if got.HasError() != tc.refused {
				t.Fatalf("%q: refused = %v, want %v (%v)", tc.crit, got.HasError(), tc.refused, got)
			}
			if !tc.refused {
				return
			}
			detail := got.Errors()[0].Detail()
			for _, want := range []string{tc.crit, "PI-1183", "import"} {
				if !strings.Contains(detail, want) {
					t.Errorf("detail does not mention %q:\n%s", want, detail)
				}
			}
		})
	}
}

// TestValidatePatchReportingCriteriaAnchorsTheOffendingElement pins that the
// error lands on the criterion that caused it rather than on the whole list, so
// an operator with twelve criteria is told which one to remove.
func TestValidatePatchReportingCriteriaAnchorsTheOffendingElement(t *testing.T) {
	at := path.Root("criteria")
	models := []criteria.CriterionModel{
		named("Operating System Version"),
		named("Patch Reporting: Zulu OpenJDK 9"),
		named("Computer Name"),
	}
	got := ValidatePatchReportingCriteria(at, models)
	if len(got.Errors()) != 1 {
		t.Fatalf("expected exactly one error, got %d: %v", len(got.Errors()), got)
	}
	want := at.AtListIndex(1).AtName("name")
	if gotPath := got.Errors()[0].(interface{ Path() path.Path }).Path(); !gotPath.Equal(want) {
		t.Fatalf("anchored at %s, want %s", gotPath, want)
	}
}

// TestValidatePatchReportingCriteriaSkipsUnknown pins that a criterion name
// interpolated from another resource does not trip the guard at plan time. A
// name that is not yet known cannot be checked, and erroring on it would break
// the ordinary compose pattern.
func TestValidatePatchReportingCriteriaSkipsUnknown(t *testing.T) {
	at := path.Root("criteria")
	unknown := named("x")
	unknown.Name = types.StringUnknown()
	if got := ValidatePatchReportingCriteria(at, []criteria.CriterionModel{unknown}); got.HasError() {
		t.Fatalf("an unknown criterion name must not be refused: %v", got)
	}
	null := named("x")
	null.Name = types.StringNull()
	if got := ValidatePatchReportingCriteria(at, []criteria.CriterionModel{null}); got.HasError() {
		t.Fatalf("a null criterion name must not be refused: %v", got)
	}
}
