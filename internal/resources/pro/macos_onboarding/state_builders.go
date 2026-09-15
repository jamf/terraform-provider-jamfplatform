// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_onboarding

import (
	"cmp"
	"context"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// onboardingItemsFromList decodes the onboarding_items types.List into item models.
// Returns an empty (non-nil) slice when the list is null or unknown, so the input
// builder emits an empty onboardingItems array — the full-replace "clear all" body.
func onboardingItemsFromList(ctx context.Context, list types.List) ([]onboardingItemModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return []onboardingItemModel{}, diags
	}
	out := make([]onboardingItemModel, 0, len(list.Elements()))
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out, diags
}

// onboardingItemsListValue converts item models into a known types.List for the
// onboarding_items attribute. A nil/empty slice yields a known EMPTY list (never
// null), so an explicit `onboarding_items = []` clear round-trips.
func onboardingItemsListValue(ctx context.Context, models []onboardingItemModel) (types.List, diag.Diagnostics) {
	if len(models) == 0 {
		return types.ListValueMust(onboardingItemObjectType(), []attr.Value{}), nil
	}
	return types.ListValueFrom(ctx, onboardingItemObjectType(), models)
}

// flattenOnboardingItems maps the SDK onboarding items into plan models, sorted by
// priority ascending. The wire returns items in an arbitrary (insertion) order that
// is NOT priority order, so the sort is load-bearing: it makes the list index align
// with the canonical presentation order, so state round-trips against a config whose
// order is the user's intended order. Server is authoritative for every field.
func flattenOnboardingItems(items []pro.OnboardingItem) []onboardingItemModel {
	if len(items) == 0 {
		return nil
	}
	sorted := make([]pro.OnboardingItem, len(items))
	copy(sorted, items)
	slices.SortStableFunc(sorted, func(a, b pro.OnboardingItem) int {
		return cmp.Compare(a.Priority, b.Priority)
	})
	out := make([]onboardingItemModel, len(sorted))
	for i := range sorted {
		out[i] = onboardingItemModel{
			EntityID:              types.StringValue(sorted[i].EntityID),
			SelfServiceEntityType: types.StringValue(sorted[i].SelfServiceEntityType),
			Priority:              types.Int64Value(int64(sorted[i].Priority)),
			ID:                    helpers.StringPointerValueOrNull(sorted[i].ID),
			EntityName:            helpers.StringPointerValueOrNull(sorted[i].EntityName),
			ScopeDescription:      helpers.StringPointerValueOrNull(sorted[i].ScopeDescription),
			SiteDescription:       helpers.StringPointerValueOrNull(sorted[i].SiteDescription),
		}
	}
	return out
}

// assignOnboardingResourceModel populates a resource model from a GET response. The
// onboarding_items list is sorted by priority and converted to a known types.List.
// The assigner does not write state.ID — the CRUD handler stamps helpers.SingletonID.
func assignOnboardingResourceModel(ctx context.Context, state *OnboardingResourceModel, c *pro.OnboardingConfiguration) diag.Diagnostics {
	state.Enabled = types.BoolValue(c.Enabled)
	list, diags := onboardingItemsListValue(ctx, flattenOnboardingItems(c.OnboardingItems))
	state.OnboardingItems = list
	return diags
}

// assignOnboardingDataSourceModel populates a data source model from a GET response.
// Same priority-sort and list-conversion semantics as the resource assigner.
func assignOnboardingDataSourceModel(ctx context.Context, state *OnboardingDataSourceModel, c *pro.OnboardingConfiguration) diag.Diagnostics {
	state.Enabled = types.BoolValue(c.Enabled)
	list, diags := onboardingItemsListValue(ctx, flattenOnboardingItems(c.OnboardingItems))
	state.OnboardingItems = list
	return diags
}

// mapOnboardingEligibleItems converts SDK eligible items into data source models. The
// slice is always non-nil so an empty result serialises as an empty list, not null.
func mapOnboardingEligibleItems(items []pro.OnboardingEligibleItem) []onboardingEligibleItemModel {
	out := make([]onboardingEligibleItemModel, 0, len(items))
	for i := range items {
		out = append(out, onboardingEligibleItemModel{
			ID:               types.StringValue(items[i].ID),
			Name:             types.StringValue(items[i].Name),
			ScopeDescription: types.StringValue(items[i].ScopeDescription),
			SiteDescription:  types.StringValue(items[i].SiteDescription),
		})
	}
	return out
}
