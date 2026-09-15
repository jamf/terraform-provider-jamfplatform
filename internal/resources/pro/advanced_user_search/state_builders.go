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

// assignAdvancedUserSearchResourceModel populates a resource model from an
// AdvancedUserSearch response. Server is authoritative for every field; the
// matched-user result set (search.Users) is intentionally not surfaced.
func assignAdvancedUserSearchResourceModel(ctx context.Context, state *AdvancedUserSearchResourceModel, search *proclassic.AdvancedUserSearch) diag.Diagnostics {
	var diags diag.Diagnostics
	if search == nil {
		return diags
	}

	if search.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(search.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(search.Name)

	siteID, siteName := scope.FlattenSiteObject(search.Site)
	state.SiteID = helpers.ReconcileOptionalStringPointer(siteID, state.SiteID)
	state.SiteName = helpers.StringPointerValueOrNull(siteName)

	state.Criteria = criteria.FlattenCriterionSlice(criterionSlice(search.Criteria))

	displayFields, dfDiags := flattenDisplayFields(ctx, search.DisplayFields)
	diags.Append(dfDiags...)
	if diags.HasError() {
		return diags
	}
	state.DisplayFields = displayFields

	return diags
}

// assignAdvancedUserSearchDataSourceModel populates a data source model from an
// AdvancedUserSearch response. Symmetric with the resource builder.
func assignAdvancedUserSearchDataSourceModel(ctx context.Context, state *AdvancedUserSearchDataSourceModel, search *proclassic.AdvancedUserSearch) diag.Diagnostics {
	var diags diag.Diagnostics
	if search == nil {
		return diags
	}

	if search.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(search.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(search.Name)

	siteID, siteName := scope.FlattenSiteObject(search.Site)
	state.SiteID = helpers.StringPointerValueOrNull(siteID)
	state.SiteName = helpers.StringPointerValueOrNull(siteName)

	state.Criteria = criteria.FlattenCriterionSlice(criterionSlice(search.Criteria))

	displayFields, dfDiags := flattenDisplayFields(ctx, search.DisplayFields)
	diags.Append(dfDiags...)
	if diags.HasError() {
		return diags
	}
	state.DisplayFields = displayFields

	return diags
}

// criterionSlice returns the inner criterion slice pointer from the wrapper, or
// nil when the wrapper is absent.
func criterionSlice(wrapper *proclassic.AdvancedUserSearchCriteria) *[]proclassic.Criterion {
	if wrapper == nil {
		return nil
	}
	return wrapper.Criterion
}

// flattenDisplayFields converts the SDK display-fields wrapper into a Set of
// column names. Returns a null set for an empty or absent wrapper. Modelled as a
// Set because Jamf Pro returns the columns in its own canonical order, not the
// order they were submitted.
func flattenDisplayFields(ctx context.Context, wrapper *proclassic.AdvancedUserSearchDisplayFields) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	if wrapper == nil || wrapper.DisplayField == nil || len(*wrapper.DisplayField) == 0 {
		return types.SetNull(types.StringType), diags
	}
	src := *wrapper.DisplayField
	names := make([]string, 0, len(src))
	for _, f := range src {
		if f.Name == nil {
			continue
		}
		names = append(names, *f.Name)
	}
	if len(names) == 0 {
		return types.SetNull(types.StringType), diags
	}
	set, d := types.SetValueFrom(ctx, types.StringType, names)
	diags.Append(d...)
	return set, diags
}
