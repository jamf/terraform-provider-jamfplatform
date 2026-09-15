// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_user_search

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// buildAdvancedUserSearchInput converts a plan model into the SDK payload used
// for Create and Update. Every field is emitted unconditionally: the classic API
// merges omitted fields (leaving the server value unchanged), so to make the
// Terraform config authoritative — and to let users clear criteria and display
// fields — we always send the full representation. An empty <criteria> /
// <display_fields> wrapper clears the corresponding collection.
func buildAdvancedUserSearchInput(ctx context.Context, plan AdvancedUserSearchResourceModel) (*proclassic.AdvancedUserSearch, diag.Diagnostics) {
	var diags diag.Diagnostics

	name := plan.Name.ValueString()

	displayFields, dfDiags := buildDisplayFieldsWrapper(ctx, plan.DisplayFields)
	diags.Append(dfDiags...)
	if diags.HasError() {
		return nil, diags
	}

	search := &proclassic.AdvancedUserSearch{
		Name:          &name,
		Site:          scope.BuildSiteObject(plan.SiteID),
		Criteria:      buildCriteriaWrapper(plan.Criteria),
		DisplayFields: displayFields,
	}

	return search, diags
}

// buildCriteriaWrapper wraps the shared criterion-slice builder in the
// user-search wrapper struct. Always returns a non-nil wrapper (empty Criterion
// slice when there are no criteria) so the <criteria> element is always emitted
// — an empty element clears all criteria server-side.
func buildCriteriaWrapper(models []criteria.CriterionModel) *proclassic.AdvancedUserSearchCriteria {
	slice := criteria.BuildCriterionSlice(models)
	return &proclassic.AdvancedUserSearchCriteria{Criterion: &slice}
}

// buildDisplayFieldsWrapper converts the plan display_fields set into the SDK
// wrapper. Always returns a non-nil wrapper (empty when null/unknown) so the
// <display_fields> element is always emitted; an empty element clears all
// columns server-side.
func buildDisplayFieldsWrapper(ctx context.Context, set types.Set) (*proclassic.AdvancedUserSearchDisplayFields, diag.Diagnostics) {
	var diags diag.Diagnostics
	names := []string{}
	if !set.IsNull() && !set.IsUnknown() {
		extracted, d := helpers.SetToStringSlice(ctx, set)
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		names = extracted
	}
	items := make([]proclassic.AdvancedUserSearchDisplayFieldsDisplayFieldItem, 0, len(names))
	for _, n := range names {
		name := n
		items = append(items, proclassic.AdvancedUserSearchDisplayFieldsDisplayFieldItem{Name: &name})
	}
	return &proclassic.AdvancedUserSearchDisplayFields{DisplayField: &items}, diags
}
