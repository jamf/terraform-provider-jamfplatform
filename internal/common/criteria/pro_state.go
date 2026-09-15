// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package criteria

import (
	"cmp"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// BuildSmartSearchCriteria maps plan criterion models to the Pro JSON
// pro.SmartSearchCriterion wire type. It is the JSON-SDK counterpart to
// BuildCriterionSlice (which targets the proclassic XML wire type) — the two
// share the CriterionModel and CriterionAttributes building blocks but
// intentionally marshal to different SDK structs (per STYLE_GUIDE: keep the
// XML/JSON marshalling per-SDK). Consumed by the Pro advanced-search resources
// (jamfplatform_pro_advanced_mobile_device_search,
// jamfplatform_pro_advanced_volume_purchasing_content_search).
//
// Priority is filled from the element index when omitted; and_or defaults to
// "and"; parentheses default to false. Output is sorted by priority so the wire
// order matches the user's list order.
//
// The result is always non-nil (an empty slice for empty input) so callers can
// always emit the `criteria` array. The Pro `/v1` advanced-search PUT is a full
// replace — an omitted or empty `criteria` array clears every criterion — so
// emitting the empty slice is what lets a resource remove all criteria.
func BuildSmartSearchCriteria(models []CriterionModel) []pro.SmartSearchCriterion {
	out := make([]pro.SmartSearchCriterion, 0, len(models))
	for idx, c := range models {
		priority := idx
		if !c.Priority.IsNull() && !c.Priority.IsUnknown() {
			priority = int(c.Priority.ValueInt64())
		}
		// The Pro JSON SmartSearchCriterion types andOr as a plain string with no
		// generated enum, so the constant comes from the classic Criterion,
		// which is the only generated vocabulary for the concept and carries
		// the identical two values. Nothing to conflict with.
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

		p := priority
		out = append(out, pro.SmartSearchCriterion{
			Name:         c.Name.ValueString(),
			SearchType:   c.SearchType.ValueString(),
			Value:        c.Value.ValueString(),
			AndOr:        andOr,
			Priority:     &p,
			OpeningParen: &opening,
			ClosingParen: &closing,
		})
	}
	slices.SortStableFunc(out, func(a, b pro.SmartSearchCriterion) int {
		if a.Priority == nil || b.Priority == nil {
			return 0
		}
		return cmp.Compare(*a.Priority, *b.Priority)
	})
	return out
}

// FlattenSmartSearchCriteria maps a Pro JSON criteria slice back to plan models.
// Server is authoritative for every field, so values are copied directly.
// Returns nil for an empty or absent slice (a null list in state) — symmetric
// with FlattenCriterionSlice.
func FlattenSmartSearchCriteria(src *[]pro.SmartSearchCriterion) []CriterionModel {
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
		opening := types.BoolValue(false)
		if c.OpeningParen != nil {
			opening = types.BoolValue(*c.OpeningParen)
		}
		closing := types.BoolValue(false)
		if c.ClosingParen != nil {
			closing = types.BoolValue(*c.ClosingParen)
		}
		out[i] = CriterionModel{
			Priority:              priority,
			Name:                  types.StringValue(c.Name),
			SearchType:            types.StringValue(c.SearchType),
			Value:                 types.StringValue(c.Value),
			AndOr:                 types.StringValue(c.AndOr),
			HasOpeningParenthesis: opening,
			HasClosingParenthesis: closing,
		}
	}
	return out
}
