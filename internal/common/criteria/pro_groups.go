// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package criteria

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// The Pro group endpoints enforce what the classic and advanced-search surfaces
// do not: a criterion's priority must start at zero and increment by one.
// `PUT /pro/v3/computer-groups/smart-groups/{id}` with a lone criterion at
// priority 5 answers 400 INVALID_REQUEST_PARAMETER_VALUE on `field: priority`,
// "Priority must start with 0 and increment by one per new criteria added."
// (wire-probed 2026-09-17, Jamf Pro 11.32.0). The list index is the only
// numbering that satisfies it, so these builders emit the index and
// ValidateCriteriaPriorities rejects an authored value that disagrees rather than
// overriding it silently — an Optional+Computed attribute whose written value
// differs from the plan is a post-apply inconsistency, not a nicety.

// BuildComputerSmartGroupCriteria maps plan criterion models to the wire type
// `POST`/`PUT /pro/v3/computer-groups/smart-groups` accepts.
//
// The result is always non-nil, an empty slice for empty input, because the
// endpoint full-replaces: a body omitting `criteria` clears every criterion
// (wire-probed), so emitting the empty slice is how a resource removes them all
// and is indistinguishable from omitting the key.
func BuildComputerSmartGroupCriteria(models []CriterionModel) []pro.ComputerSmartGroupCriteriaV2 {
	norm := normaliseProGroupCriteria(models, pro.ComputerSmartGroupCriteriaV2AndOrAnd)
	out := make([]pro.ComputerSmartGroupCriteriaV2, 0, len(norm))
	for _, c := range norm {
		opening, closing := c.OpeningParen, c.ClosingParen
		out = append(out, pro.ComputerSmartGroupCriteriaV2{
			Name:         c.Name,
			SearchType:   c.SearchType,
			Value:        c.Value,
			AndOr:        c.AndOr,
			Priority:     c.Priority,
			OpeningParen: &opening,
			ClosingParen: &closing,
		})
	}
	return out
}

// FlattenComputerSmartGroupCriteria maps the computer smart-group criteria read
// back from Jamf Pro to plan models. The server is authoritative for every
// field. Returns nil for an absent or empty slice, so a group with no criteria
// round-trips as a null list rather than an empty one.
func FlattenComputerSmartGroupCriteria(src *[]pro.ComputerSmartGroupCriteriaV2) []CriterionModel {
	if src == nil || len(*src) == 0 {
		return nil
	}
	in := *src
	out := make([]CriterionModel, len(in))
	for i, c := range in {
		out[i] = flattenProGroupCriterion(c.Name, c.SearchType, c.Value, c.AndOr, c.Priority, c.OpeningParen, c.ClosingParen)
	}
	return out
}

// BuildMobileDeviceSmartGroupCriteria maps plan criterion models to the wire type
// `POST`/`PUT /pro/v2/mobile-device-groups/smart-groups` accepts. Same
// always-non-nil contract as the computer builder, for the same reason.
func BuildMobileDeviceSmartGroupCriteria(models []CriterionModel) []pro.MobileDeviceSmartGroupCriteriaV2 {
	norm := normaliseProGroupCriteria(models, pro.MobileDeviceSmartGroupCriteriaV2AndOrAnd)
	out := make([]pro.MobileDeviceSmartGroupCriteriaV2, 0, len(norm))
	for _, c := range norm {
		opening, closing := c.OpeningParen, c.ClosingParen
		out = append(out, pro.MobileDeviceSmartGroupCriteriaV2{
			Name:         c.Name,
			SearchType:   c.SearchType,
			Value:        c.Value,
			AndOr:        c.AndOr,
			Priority:     c.Priority,
			OpeningParen: &opening,
			ClosingParen: &closing,
		})
	}
	return out
}

// FlattenMobileDeviceSmartGroupCriteria maps the mobile smart-group criteria read
// back from Jamf Pro to plan models. Same contract as the computer flattener.
func FlattenMobileDeviceSmartGroupCriteria(src *[]pro.MobileDeviceSmartGroupCriteriaV2) []CriterionModel {
	if src == nil || len(*src) == 0 {
		return nil
	}
	in := *src
	out := make([]CriterionModel, len(in))
	for i, c := range in {
		out[i] = flattenProGroupCriterion(c.Name, c.SearchType, c.Value, c.AndOr, c.Priority, c.OpeningParen, c.ClosingParen)
	}
	return out
}

// ValidateCriteriaPriorities rejects an authored priority that is not the
// criterion's own position in the list.
//
// The endpoints require 0..n-1 in order, so position is the only legal
// numbering and the builders emit it. Rejecting a disagreeing value is better
// than quietly replacing it two ways over: a silently overridden
// Optional+Computed value aborts the apply with "provider produced inconsistent
// result after apply", and an operator who wrote a priority meant something by
// it. Reorder the list instead.
//
// A null or unknown priority is the normal case and always passes — the schema
// computes it.
func ValidateCriteriaPriorities(criteriaPath path.Path, models []CriterionModel) diag.Diagnostics {
	var diags diag.Diagnostics
	for i, c := range models {
		if c.Priority.IsNull() || c.Priority.IsUnknown() {
			continue
		}
		if got := c.Priority.ValueInt64(); got != int64(i) {
			diags.AddAttributeError(
				criteriaPath.AtListIndex(i).AtName("priority"),
				"Criterion priority must match its position",
				fmt.Sprintf(
					"Jamf Pro evaluates a group's criteria in list order and requires the priorities to run from 0 upwards with no gaps, so this criterion's priority must be %d, not %d. Omit `priority` and Jamf Pro takes the list order, or move the criterion to position %d in the list.",
					i, got, got,
				),
			)
		}
	}
	return diags
}

// proGroupCriterion is the field set both Pro group criterion types carry. They
// are generated separately, one per endpoint, and are structurally identical, so
// the defaulting and index numbering happen once here rather than twice.
type proGroupCriterion struct {
	Name         string
	SearchType   string
	Value        string
	AndOr        string
	Priority     int
	OpeningParen bool
	ClosingParen bool
}

// normaliseProGroupCriteria applies the defaults both builders share: priority
// from the element index, and_or from defaultAndOr, parentheses false.
//
// Unlike BuildSmartSearchCriteria, which honours an authored priority and sorts
// by it, this ignores the authored value and uses the index — the endpoints
// accept nothing else, and ValidateCriteriaPriorities has already refused any
// authored value that disagrees, so there is nothing left to honour.
func normaliseProGroupCriteria(models []CriterionModel, defaultAndOr string) []proGroupCriterion {
	out := make([]proGroupCriterion, 0, len(models))
	for idx, c := range models {
		andOr := defaultAndOr
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
		out = append(out, proGroupCriterion{
			Name:         c.Name.ValueString(),
			SearchType:   c.SearchType.ValueString(),
			Value:        c.Value.ValueString(),
			AndOr:        andOr,
			Priority:     idx,
			OpeningParen: opening,
			ClosingParen: closing,
		})
	}
	return out
}

// flattenProGroupCriterion builds one plan model from the fields both Pro group
// criterion types carry. The parenthesis flags are pointers on the wire and
// default to false when absent, matching the schema default.
func flattenProGroupCriterion(name, searchType, value, andOr string, priority int, opening, closing *bool) CriterionModel {
	openValue := types.BoolValue(false)
	if opening != nil {
		openValue = types.BoolValue(*opening)
	}
	closeValue := types.BoolValue(false)
	if closing != nil {
		closeValue = types.BoolValue(*closing)
	}
	return CriterionModel{
		Priority:              types.Int64Value(int64(priority)),
		Name:                  types.StringValue(name),
		SearchType:            types.StringValue(searchType),
		Value:                 types.StringValue(value),
		AndOr:                 types.StringValue(andOr),
		HasOpeningParenthesis: openValue,
		HasClosingParenthesis: closeValue,
	}
}
