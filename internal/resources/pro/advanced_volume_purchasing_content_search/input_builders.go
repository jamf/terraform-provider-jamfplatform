// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_volume_purchasing_content_search

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildAdvancedVolumePurchasingContentSearchInput converts a plan model into the SDK
// payload used for Create and Update. The Pro /v1 advanced-search PUT is a full
// replace. criteria and display_fields are Optional+Computed with
// UseStateForUnknown, so when omitted the plan carries the prior value forward
// (omit = preserve) and the builder re-emits it; an explicit empty list/set is a
// known-empty plan value and clears the collection. On first create with the field
// omitted the plan value is Unknown, decoded here to an empty slice, so the server
// applies its empty default.
func buildAdvancedVolumePurchasingContentSearchInput(ctx context.Context, plan AdvancedVolumePurchasingContentSearchResourceModel) (*pro.AdvancedUserContentSearch, diag.Diagnostics) {
	var diags diag.Diagnostics

	displayFields, dfDiags := buildDisplayFields(ctx, plan.DisplayFields)
	diags.Append(dfDiags...)
	if diags.HasError() {
		return nil, diags
	}

	criteriaModels, cDiags := criteria.CriteriaModelsFromList(ctx, plan.Criteria)
	diags.Append(cDiags...)
	if diags.HasError() {
		return nil, diags
	}
	criteriaSlice := criteria.BuildSmartSearchCriteria(criteriaModels)
	siteID := plan.SiteID.ValueString()

	search := &pro.AdvancedUserContentSearch{
		Name:          plan.Name.ValueString(),
		SiteID:        &siteID,
		Criteria:      &criteriaSlice,
		DisplayFields: &displayFields,
	}

	return search, diags
}

// buildDisplayFields converts the plan display_fields set into the SDK string
// slice. Always returns a non-nil slice (empty when null/unknown) so the
// `displayFields` array is always emitted; an empty array clears all columns
// on the full-replace PUT.
func buildDisplayFields(ctx context.Context, set types.Set) ([]string, diag.Diagnostics) {
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
	return names, diags
}
