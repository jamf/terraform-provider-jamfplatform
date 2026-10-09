// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_onboarding

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// OnboardingResourceModel is the Terraform resource model for the macOS Onboarding
// settings singleton (Settings > Self Service > macOS Onboarding). The onboarding
// items are an ordered list — order is significant (UI "Order" column / wire
// `priority`), so the collection is a types.List, never a Set.
type OnboardingResourceModel struct {
	ID              types.String           `tfsdk:"id"`
	Enabled         types.Bool             `tfsdk:"enabled"`
	OnboardingItems types.List             `tfsdk:"onboarding_items"`
	Timeouts        resourceTimeouts.Value `tfsdk:"timeouts"`
}

// OnboardingDataSourceModel is the Terraform data source model for the singleton.
type OnboardingDataSourceModel struct {
	ID              types.String             `tfsdk:"id"`
	Enabled         types.Bool               `tfsdk:"enabled"`
	OnboardingItems types.List               `tfsdk:"onboarding_items"`
	Timeouts        datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// onboardingItemModel is one element of onboarding_items. entity_id and
// self_service_entity_type are the only user-authored fields; priority is derived
// from the list index, and id/entity_name/scope_description/site_description are
// server-derived echoes (all Computed). The per-item id is reminted by the server
// on every write, so it is unstable across applies and must not be relied on as a
// reference.
type onboardingItemModel struct {
	EntityID              types.String `tfsdk:"entity_id"`
	SelfServiceEntityType types.String `tfsdk:"self_service_entity_type"`
	Priority              types.Int64  `tfsdk:"priority"`
	ID                    types.String `tfsdk:"id"`
	EntityName            types.String `tfsdk:"entity_name"`
	ScopeDescription      types.String `tfsdk:"scope_description"`
	SiteDescription       types.String `tfsdk:"site_description"`
}

// onboardingIdentityModel is the identity object used on import.
type onboardingIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// OnboardingEligibleItemsDataSourceModel is the model for the parameterised
// eligible-items discovery data source. entity_type selects which catalog to
// return; items carries the matching eligible objects.
type OnboardingEligibleItemsDataSourceModel struct {
	ID         types.String                  `tfsdk:"id"`
	EntityType types.String                  `tfsdk:"entity_type"`
	Items      []onboardingEligibleItemModel `tfsdk:"items"`
	Timeouts   datasourceTimeouts.Value      `tfsdk:"timeouts"`
}

// onboardingEligibleItemModel is one eligible object returned by an eligible-*
// endpoint.
type onboardingEligibleItemModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	ScopeDescription types.String `tfsdk:"scope_description"`
	SiteDescription  types.String `tfsdk:"site_description"`
}
