// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package criteria

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// AttributeSource is one side of a plan a criteria list can be read from: a
// plan, a state or a configuration. Declaring the one method keeps the reader
// usable with all three without naming the framework's concrete request types.
type AttributeSource interface {
	GetAttribute(ctx context.Context, attributePath path.Path, target any) diag.Diagnostics
}

// CriteriaAtPlanTime reads a criteria list out of one side of a plan and reports
// whether the value is settled enough to compare.
//
// It reads the one attribute rather than decoding the whole resource model, and
// the reason is a trap every criteria-bearing resource model hides. A model's
// criteria field is a Go slice of CriterionModel, and the framework's reflection
// cannot put an unknown value into a slice or a struct: it raises a "Value
// Conversion Error" carrying "This is always an error in the provider", which
// aborts the plan. A configuration whose criteria are derived from something
// Terraform has not created yet — a data source reading a resource created in
// the same apply, or a module output from one — presents exactly that unknown,
// and applies perfectly well once the value lands. Reading the attribute as a
// types.List keeps the unknown representable, so a caller can defer instead of
// crashing the plan.
//
// The second return is false when nothing can be compared yet: an unknown list,
// or a list holding an element that is itself unknown or null. An unknown value
// inside a known element is fine and is not checked for, because every field of
// a criterion is an attr.Value carrying its own unknown — priority is
// Optional+Computed and is routinely unknown at plan.
//
// A null list is settled and yields no criteria, which is the configuration that
// stores a group with no criteria at all.
func CriteriaAtPlanTime(ctx context.Context, from AttributeSource, attribute path.Path) ([]CriterionModel, bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	var configured types.List
	diags.Append(from.GetAttribute(ctx, attribute, &configured)...)
	if diags.HasError() || configured.IsUnknown() {
		return nil, false, diags
	}
	if configured.IsNull() {
		return nil, true, diags
	}
	for _, element := range configured.Elements() {
		if element.IsNull() || element.IsUnknown() {
			return nil, false, diags
		}
	}

	models, modelDiags := CriteriaModelsFromList(ctx, configured)
	diags.Append(modelDiags...)
	if diags.HasError() {
		return nil, false, diags
	}
	return models, true, diags
}
