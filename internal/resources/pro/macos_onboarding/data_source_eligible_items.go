// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_onboarding

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// OnboardingEligibleItemsDataSource is a parameterised discovery data source that
// returns the Self Service objects eligible to be added to macOS onboarding, for a
// given entity type. It fans the single `entity_type` argument out to the matching
// eligible-* SDK call.
type OnboardingEligibleItemsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &OnboardingEligibleItemsDataSource{}

// NewOnboardingEligibleItemsDataSource returns a new instance of the data source.
func NewOnboardingEligibleItemsDataSource() datasource.DataSource {
	return &OnboardingEligibleItemsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *OnboardingEligibleItemsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_macos_onboarding_eligible_items"
}

// Schema returns the data source schema.
func (d *OnboardingEligibleItemsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Returns the Self Service objects eligible to be referenced from `jamfplatform_pro_macos_onboarding.onboarding_items`, for the given `entity_type`. " +
			"Use the returned `id` as `entity_id`, paired with the matching `self_service_entity_type` (`policies` → `OS_X_POLICY`, `configuration_profiles` → `OS_X_CONFIG_PROFILE`, `apps` → `OS_X_MAC_APP` / `OS_X_APP_INSTALLER`)." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read (the queried `entity_type`).",
				Computed:            true,
			},
			"entity_type": schema.StringAttribute{
				MarkdownDescription: "Which eligible catalog to return. One of `policies`, `configuration_profiles`, `apps`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validEligibleEntityTypes...),
				},
			},
			"items": schema.ListNestedAttribute{
				MarkdownDescription: "Eligible objects of the requested type.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Object ID. Use it as the `entity_id` of an onboarding item.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Object display name.",
							Computed:            true,
						},
						"scope_description": schema.StringAttribute{
							MarkdownDescription: "Human-readable scope summary.",
							Computed:            true,
						},
						"site_description": schema.StringAttribute{
							MarkdownDescription: "Site summary.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *OnboardingEligibleItemsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_macos_onboarding_eligible_items")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the eligible catalog for the requested entity_type and populates state.
func (d *OnboardingEligibleItemsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data OnboardingEligibleItemsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, defaultReadTimeout)
	defer cancel()

	entityType := data.EntityType.ValueString()
	var (
		items []pro.OnboardingEligibleItem
		err   error
	)
	switch entityType {
	case eligiblePolicies:
		items, err = d.client.ListOnboardingEligiblePoliciesV1(readCtx, nil)
	case eligibleConfigurationProfiles:
		items, err = d.client.ListOnboardingEligibleConfigurationProfilesV1(readCtx, nil)
	case eligibleApps:
		items, err = d.client.ListOnboardingEligibleAppsV1(readCtx, nil)
	default:
		// Unreachable: the OneOf validator rejects any other value at plan time.
		resp.Diagnostics.AddError("Invalid entity_type", fmt.Sprintf("Unsupported entity_type %q.", entityType))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to list eligible onboarding items", helpers.APIErrorDetail(err))
		return
	}

	data.Items = mapOnboardingEligibleItems(items)
	data.ID = types.StringValue(entityType)

	tflog.Trace(ctx, "read macOS Onboarding eligible items data source", map[string]any{"entity_type": entityType, "returned": len(data.Items)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
