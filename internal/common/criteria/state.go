// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package criteria

import (
	"cmp"
	"context"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// CriterionModel is the Terraform model for a single classic <criterion>,
// matching the attribute map returned by CriterionAttributes. Shared by the
// advanced-search resources (and available to any future ProClassic resource
// exposing the classic criterion element).
type CriterionModel struct {
	Priority              types.Int64  `tfsdk:"priority"`
	Name                  types.String `tfsdk:"name"`
	SearchType            types.String `tfsdk:"search_type"`
	Value                 types.String `tfsdk:"value"`
	AndOr                 types.String `tfsdk:"and_or"`
	HasOpeningParenthesis types.Bool   `tfsdk:"has_opening_parenthesis"`
	HasClosingParenthesis types.Bool   `tfsdk:"has_closing_parenthesis"`
}

// CriterionAttrTypes is the attr.Type map matching CriterionModel (and the schema
// returned by CriterionAttributes). Use it to build a types.List of criteria when
// the criteria collection is Optional+Computed and must therefore be a types.List
// rather than a Go slice (it can be Unknown at plan; see STYLE_GUIDE §Computed
// nested collections). Additive — Optional-only consumers keep using []CriterionModel.
func CriterionAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"priority":                types.Int64Type,
		"name":                    types.StringType,
		"search_type":             types.StringType,
		"value":                   types.StringType,
		"and_or":                  types.StringType,
		"has_opening_parenthesis": types.BoolType,
		"has_closing_parenthesis": types.BoolType,
	}
}

// CriterionObjectType is the types.ObjectType element type for a types.List of
// criteria, derived from CriterionAttrTypes.
func CriterionObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: CriterionAttrTypes()}
}

// CriteriaListValue converts criterion models into a known types.List for an
// Optional+Computed `criteria` attribute. A nil/empty slice yields a known EMPTY
// list (never null), so an explicit `criteria = []` clear round-trips and a
// create-omit Unknown resolves cleanly.
func CriteriaListValue(ctx context.Context, models []CriterionModel) (types.List, diag.Diagnostics) {
	if len(models) == 0 {
		return types.ListValueMust(CriterionObjectType(), []attr.Value{}), nil
	}
	return types.ListValueFrom(ctx, CriterionObjectType(), models)
}

// CriteriaModelsFromList decodes a types.List `criteria` attribute into criterion
// models, returning nil when the list is null or unknown (so the input builder
// emits an empty criteria array — the full-replace clear).
func CriteriaModelsFromList(ctx context.Context, list types.List) ([]CriterionModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return nil, diags
	}
	out := make([]CriterionModel, 0, len(list.Elements()))
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out, diags
}

// BuildCriterionSlice maps plan criterion models to SDK criteria. Priority is
// filled from the element index when omitted; and_or defaults to "and";
// parentheses default to false. Output is sorted by priority so the wire order
// matches the user's list order.
//
// The result is always non-nil (an empty slice for empty input) so callers can
// wrap it in an always-emitted <criteria> element. The classic API treats an
// omitted wrapper as "leave unchanged" but an empty wrapper as "clear all" —
// so emitting the empty wrapper is what lets a resource remove every criterion.
func BuildCriterionSlice(models []CriterionModel) []proclassic.Criterion {
	out := make([]proclassic.Criterion, 0, len(models))
	for idx, c := range models {
		priority := idx
		if !c.Priority.IsNull() && !c.Priority.IsUnknown() {
			priority = int(c.Priority.ValueInt64())
		}
		andOr := proclassic.CriterionAndOrAnd
		if !c.AndOr.IsNull() && !c.AndOr.IsUnknown() && c.AndOr.ValueString() != "" {
			andOr = c.AndOr.ValueString()
		}
		opening := false
		if !c.HasOpeningParenthesis.IsNull() && !c.HasOpeningParenthesis.IsUnknown() {
			opening = c.HasOpeningParenthesis.ValueBool()
		}
		closing := false
		if !c.HasClosingParenthesis.IsNull() && !c.HasClosingParenthesis.IsUnknown() {
			closing = c.HasClosingParenthesis.ValueBool()
		}

		name := c.Name.ValueString()
		searchType := c.SearchType.ValueString()
		value := c.Value.ValueString()

		out = append(out, proclassic.Criterion{
			Name:         &name,
			Priority:     &priority,
			AndOr:        &andOr,
			SearchType:   &searchType,
			Value:        &value,
			OpeningParen: &opening,
			ClosingParen: &closing,
		})
	}
	slices.SortStableFunc(out, func(a, b proclassic.Criterion) int {
		if a.Priority == nil || b.Priority == nil {
			return 0
		}
		return cmp.Compare(*a.Priority, *b.Priority)
	})
	return out
}

// FlattenCriterionSlice maps an SDK criteria slice back to plan models. Server
// is authoritative for every field, so values are copied directly (no Reconcile
// helpers — those return null when the API value is empty and prior state was
// null, which diverges between the import path and the post-apply refresh path).
// Returns nil for an empty or absent slice (a null list in state).
func FlattenCriterionSlice(src *[]proclassic.Criterion) []CriterionModel {
	if src == nil || len(*src) == 0 {
		return nil
	}
	in := *src
	out := make([]CriterionModel, len(in))
	for i, c := range in {
		priority := types.Int64Null()
		if c.Priority != nil {
			priority = types.Int64Value(int64(*c.Priority))
		}
		out[i] = CriterionModel{
			Priority:              priority,
			Name:                  helpers.StringPointerValueOrNull(c.Name),
			SearchType:            helpers.StringPointerValueOrNull(c.SearchType),
			Value:                 helpers.StringPointerValueOrNull(c.Value),
			AndOr:                 helpers.StringPointerValueOrNull(c.AndOr),
			HasOpeningParenthesis: helpers.BoolPointerValueOrNull(c.OpeningParen),
			HasClosingParenthesis: helpers.BoolPointerValueOrNull(c.ClosingParen),
		}
	}
	return out
}
