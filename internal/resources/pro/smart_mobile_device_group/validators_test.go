// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
)

// The validator itself reads the configuration through the framework, which a
// unit test cannot assemble without a live schema, so these exercise the rule it
// delegates to and the decision it makes about unknown values. The acceptance
// suite covers the wiring with a real plan.

// TestValidateCriteriaPriorities_AcceptsTheOmittedCase is the normal
// configuration: nobody writes a priority, and the schema computes it.
func TestValidateCriteriaPriorities_AcceptsTheOmittedCase(t *testing.T) {
	models := []criteria.CriterionModel{
		criterion("Model", "like", "iPad"),
		criterion("Supervised", "is", "true"),
	}
	if diags := criteria.ValidateCriteriaPriorities(path.Root("criteria"), models); diags.HasError() {
		t.Errorf("omitted priorities must pass, got %v", diags)
	}
}

// TestValidateCriteriaPriorities_AcceptsPositionalValues covers an author who
// numbers the list the way the endpoint requires.
func TestValidateCriteriaPriorities_AcceptsPositionalValues(t *testing.T) {
	models := []criteria.CriterionModel{
		criterion("Model", "like", "iPad"),
		criterion("Supervised", "is", "true"),
	}
	models[0].Priority = types.Int64Value(0)
	models[1].Priority = types.Int64Value(1)
	if diags := criteria.ValidateCriteriaPriorities(path.Root("criteria"), models); diags.HasError() {
		t.Errorf("positional priorities must pass, got %v", diags)
	}
}

// TestValidateCriteriaPriorities_RejectsANonPositionalValue is the whole point:
// the endpoint accepts nothing but 0..n-1 in order, and refusing the value beats
// overwriting it, which would abort the apply as a post-apply inconsistency.
func TestValidateCriteriaPriorities_RejectsANonPositionalValue(t *testing.T) {
	models := []criteria.CriterionModel{criterion("Model", "like", "iPad")}
	models[0].Priority = types.Int64Value(5)
	diags := criteria.ValidateCriteriaPriorities(path.Root("criteria"), models)
	if !diags.HasError() {
		t.Fatal("a priority that is not the criterion's position must be refused")
	}
}

// TestValidateCriteriaPriorities_DefersOnUnknown pins the rule that a config
// validator must never error on a value Terraform cannot resolve yet, or the
// resource becomes unusable from a module.
func TestValidateCriteriaPriorities_DefersOnUnknown(t *testing.T) {
	models := []criteria.CriterionModel{criterion("Model", "like", "iPad")}
	models[0].Priority = types.Int64Unknown()
	if diags := criteria.ValidateCriteriaPriorities(path.Root("criteria"), models); diags.HasError() {
		t.Errorf("an unknown priority must defer, got %v", diags)
	}
}

func TestCriteriaPrioritiesConfigValidator_Describes(t *testing.T) {
	v := criteriaPrioritiesConfigValidator{}
	if v.Description(t.Context()) == "" {
		t.Error("the validator must describe itself")
	}
	if v.MarkdownDescription(t.Context()) != v.Description(t.Context()) {
		t.Error("the two descriptions must agree")
	}
}
