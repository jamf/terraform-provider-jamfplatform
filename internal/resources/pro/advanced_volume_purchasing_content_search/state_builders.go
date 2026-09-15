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

// assignAdvancedVolumePurchasingContentSearchResourceModel populates a resource model from
// an AdvancedSearch response. Server is authoritative for every field. Always
// sourced from a GET after Create/Update — the Pro PUT response echoes the
// submitted body (including invalid display fields the server silently drops),
// so the canonical representation must come from a fresh GET.
func assignAdvancedVolumePurchasingContentSearchResourceModel(ctx context.Context, state *AdvancedVolumePurchasingContentSearchResourceModel, search *pro.AdvancedUserContentSearch) diag.Diagnostics {
	var diags diag.Diagnostics
	if search == nil {
		return diags
	}

	state.ID = helpers.StringPointerValueOrNull(search.ID)
	state.Name = types.StringValue(search.Name)
	state.SiteID = helpers.StringPointerValueOrNull(search.SiteID)

	criteriaList, critDiags := criteria.CriteriaListValue(ctx, criteria.FlattenSmartSearchCriteria(search.Criteria))
	diags.Append(critDiags...)
	if diags.HasError() {
		return diags
	}
	state.Criteria = criteriaList

	displayFields, dfDiags := flattenDisplayFields(ctx, search.DisplayFields)
	diags.Append(dfDiags...)
	if diags.HasError() {
		return diags
	}
	state.DisplayFields = displayFields

	return diags
}

// assignAdvancedVolumePurchasingContentSearchDataSourceModel populates a data source model
// from an AdvancedSearch response. Symmetric with the resource builder.
func assignAdvancedVolumePurchasingContentSearchDataSourceModel(ctx context.Context, state *AdvancedVolumePurchasingContentSearchDataSourceModel, search *pro.AdvancedUserContentSearch) diag.Diagnostics {
	var diags diag.Diagnostics
	if search == nil {
		return diags
	}

	state.ID = helpers.StringPointerValueOrNull(search.ID)
	state.Name = types.StringValue(search.Name)
	state.SiteID = helpers.StringPointerValueOrNull(search.SiteID)

	criteriaList, critDiags := criteria.CriteriaListValue(ctx, criteria.FlattenSmartSearchCriteria(search.Criteria))
	diags.Append(critDiags...)
	if diags.HasError() {
		return diags
	}
	state.Criteria = criteriaList

	displayFields, dfDiags := flattenDisplayFields(ctx, search.DisplayFields)
	diags.Append(dfDiags...)
	if diags.HasError() {
		return diags
	}
	state.DisplayFields = displayFields

	return diags
}

// flattenDisplayFields converts the SDK display-fields slice into a Set of column
// names. Returns a known EMPTY set (never null) for an empty/absent slice, so an
// explicit `display_fields = []` clear round-trips and a create-omit Unknown
// resolves cleanly. display_fields is Optional+Computed. Modelled as a Set because
// Jamf Pro returns the columns in its own canonical order, not as submitted.
func flattenDisplayFields(ctx context.Context, src *[]string) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	elems := []string{}
	if src != nil {
		elems = *src
	}
	set, d := types.SetValueFrom(ctx, types.StringType, elems)
	diags.Append(d...)
	return set, diags
}
