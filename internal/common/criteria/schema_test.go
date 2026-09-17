// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package criteria

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestCriterionAttributes_FieldShape(t *testing.T) {
	attrs := CriterionAttributes(Operators)

	for _, name := range []string{
		"priority", "name", "search_type", "value",
		"and_or", "has_opening_parenthesis", "has_closing_parenthesis",
	} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("CriterionAttributes missing %q", name)
		}
	}
	if len(attrs) != 7 {
		t.Errorf("expected 7 criterion attributes, got %d", len(attrs))
	}

	// name / search_type / value are Required; priority / and_or / parens are
	// Optional+Computed (server is authoritative, defaults fill omitted fields).
	if got := attrs["name"].(schema.StringAttribute); !got.Required {
		t.Errorf("name must be Required")
	}
	if got := attrs["search_type"].(schema.StringAttribute); !got.Required {
		t.Errorf("search_type must be Required")
	}
	if got := attrs["value"].(schema.StringAttribute); !got.Required {
		t.Errorf("value must be Required")
	}
	priority := attrs["priority"].(schema.Int64Attribute)
	if !priority.Optional || !priority.Computed {
		t.Errorf("priority must be Optional+Computed")
	}
}

func TestCriterionAttributes_SearchTypeUsesGivenOperatorVocabulary(t *testing.T) {
	subset := Without("in less than x days", "in more than x days")
	attrs := CriterionAttributes(subset)

	searchType := attrs["search_type"].(schema.StringAttribute)
	if len(searchType.Validators) == 0 {
		t.Fatalf("search_type must carry an OneOf validator")
	}
	// The wired description reflects the passed (subset) vocabulary, not the
	// full set — proves the operators argument is threaded through.
	desc := searchType.MarkdownDescription
	if got, want := desc, Description(subset); got != want {
		t.Errorf("search_type description not derived from given operators\n got: %q\nwant: %q", got, want)
	}
}

// TestAndOrDescriptionStatesTheJoinDirection pins that the shared and_or
// description says WHICH neighbour the value joins, and names the right one.
//
// The direction is not guessable and the wording was wrong once: every consumer
// of CriterionAttributes rendered "joins to the next" to the Terraform Registry,
// which is backwards. Wire-probed on Jamf Pro 11.32.0, 2026-09-17, with two
// criteria that cannot both match one computer — "Computer Name is A" and
// "Computer Name is B":
//
//	andOr ["or", "and"]  -> 0 members, so AND applied  (criterion 1's value)
//	andOr ["and", "or"]  -> 2 members, so OR applied   (criterion 1's value)
//
// The operator that takes effect is always the SECOND criterion's, so a
// criterion's and_or joins it to the one BEFORE it and the first entry's value
// is ignored.
//
// An operator who believes the old wording writes `or` on the first of two
// criteria to mean "A or B", gets `and`, and the group silently holds the wrong
// computers while the plan stays empty. Nothing else in the suite would catch a
// regression here, because the provider passes and_or through verbatim — the
// defect is entirely in what the documentation claims.
func TestAndOrDescriptionStatesTheJoinDirection(t *testing.T) {
	attrs := CriterionAttributes(Operators)
	andOr, ok := attrs["and_or"]
	if !ok {
		t.Fatal("and_or is missing from the shared criterion attributes")
	}
	got := andOr.GetMarkdownDescription()

	if !strings.Contains(got, "before it") {
		t.Errorf("the description must say the value joins the criterion BEFORE this one:\n%s", got)
	}
	if !strings.Contains(got, "never used") {
		t.Errorf("the description must say the first criterion's value is unused:\n%s", got)
	}
	if strings.Contains(got, "to the next") || strings.Contains(got, "joins the next") {
		t.Errorf("the description claims a forward join, which is the wire-disproven direction:\n%s", got)
	}
}
