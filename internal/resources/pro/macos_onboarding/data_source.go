// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_onboarding

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// OnboardingDataSource implements the Terraform data source for the Jamf Pro macOS
// Onboarding settings singleton.
type OnboardingDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &OnboardingDataSource{}

// NewOnboardingDataSource returns a new instance of the data source.
func NewOnboardingDataSource() datasource.DataSource {
	return &OnboardingDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *OnboardingDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_macos_onboarding"
}

// Schema returns the data source schema.
func (d *OnboardingDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the current Jamf Pro macOS Onboarding configuration (Settings > Self Service > macOS Onboarding). One record per tenant. Items are returned in priority order." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether macOS Onboarding is enabled for the tenant.",
				Computed:            true,
			},
			"onboarding_items": schema.ListNestedAttribute{
				MarkdownDescription: "Ordered list of Self Service items presented during macOS onboarding, in priority order.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"entity_id": schema.StringAttribute{
							MarkdownDescription: "ID of the referenced Jamf Pro object.",
							Computed:            true,
						},
						"self_service_entity_type": schema.StringAttribute{
							MarkdownDescription: "Type of the referenced object (`OS_X_POLICY`, `OS_X_CONFIG_PROFILE`, `OS_X_MAC_APP`, `OS_X_APP_INSTALLER`).",
							Computed:            true,
						},
						"priority": schema.Int64Attribute{
							MarkdownDescription: "Presentation order (1-based).",
							Computed:            true,
						},
						"id": schema.StringAttribute{
							MarkdownDescription: "Identifier for this onboarding item. Returned by Jamf Pro; not user-settable.",
							Computed:            true,
						},
						"entity_name": schema.StringAttribute{
							MarkdownDescription: "Display name of the referenced object.",
							Computed:            true,
						},
						"scope_description": schema.StringAttribute{
							MarkdownDescription: "Scope summary of the referenced object (the \"Scope\" column in the admin UI).",
							Computed:            true,
						},
						"site_description": schema.StringAttribute{
							MarkdownDescription: "Site summary of the referenced object (the \"Site\" column in the admin UI).",
							Computed:            true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *OnboardingDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_macos_onboarding")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current macOS Onboarding settings and populates Terraform state.
func (d *OnboardingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data OnboardingDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	got, err := d.client.GetOnboardingV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro macOS Onboarding settings", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignOnboardingDataSourceModel(readCtx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro macOS Onboarding settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
