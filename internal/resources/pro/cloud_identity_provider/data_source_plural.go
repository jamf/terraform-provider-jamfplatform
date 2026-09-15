// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//
//	pro.ListCloudIdpV1
//
// Status: current. Last reviewed 2026-05-30.
package cloud_identity_provider

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

// CloudIdentityProvidersDataSource implements the Terraform plural data source for the
// Jamf Pro Cloud Identity Provider registry. Returns all providers (Google
// and Entra ID) in a single computed list with no filter.
type CloudIdentityProvidersDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource              = &CloudIdentityProvidersDataSource{}
	_ datasource.DataSourceWithConfigure = &CloudIdentityProvidersDataSource{}
)

// NewCloudIdentityProvidersDataSource returns a new instance of CloudIdentityProvidersDataSource.
func NewCloudIdentityProvidersDataSource() datasource.DataSource {
	return &CloudIdentityProvidersDataSource{}
}

// Metadata sets the data source type name.
func (d *CloudIdentityProvidersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_cloud_identity_providers"
}

// Schema returns the data source schema.
func (d *CloudIdentityProvidersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists all Jamf Pro Cloud Identity Provider registry entries (Google Secure LDAP and Microsoft Entra ID). " +
			"Returns a computed `cloud_identity_providers` list with summary fields for each provider. " +
			"To look up a specific provider by `id` or `display_name`, use the singular `jamfplatform_pro_cloud_identity_provider` data source." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"cloud_identity_providers": schema.ListNestedAttribute{
				MarkdownDescription: "All Cloud Identity Providers registered on this Jamf Pro tenant.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Cloud Identity Provider ID assigned by Jamf Pro.",
							Computed:            true,
						},
						"display_name": schema.StringAttribute{
							MarkdownDescription: "Cloud Identity Provider display name.",
							Computed:            true,
						},
						"provider_name": schema.StringAttribute{
							MarkdownDescription: "Cloud identity provider type (`GOOGLE` or `ENTRA_ID`).",
							Computed:            true,
						},
						"enabled": schema.BoolAttribute{
							MarkdownDescription: "Whether the Cloud Identity Provider is enabled.",
							Computed:            true,
						},
						"provider_description": schema.StringAttribute{
							MarkdownDescription: "Human-readable description of the provider type.",
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
func (d *CloudIdentityProvidersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_cloud_identity_providers")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches all Cloud Identity Provider registry entries and populates state.
func (d *CloudIdentityProvidersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data CloudIdentityProvidersDataSourceModel
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

	all, err := d.client.ListCloudIdpV1(readCtx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro Cloud Identity Providers", helpers.APIErrorDetail(err))
		return
	}

	entries := make([]CloudIdentityProviderDataSourceEntryModel, 0, len(all))
	for _, item := range all {
		entries = append(entries, cloudIdpEntryFromResponse(item))
	}
	data.CloudIdentityProviders = entries

	tflog.Trace(ctx, "read Jamf Pro Cloud Identity Providers plural data source", map[string]any{
		"count": len(entries),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// cloudIdpEntryFromResponse maps a CloudIDPCommonResponse to the entry model.
func cloudIdpEntryFromResponse(item pro.CloudIDPCommonResponse) CloudIdentityProviderDataSourceEntryModel {
	return CloudIdentityProviderDataSourceEntryModel{
		ID:                  types.StringValue(item.ID),
		DisplayName:         types.StringValue(item.DisplayName),
		ProviderName:        types.StringValue(providerNameFromWire(item.ProviderName)),
		Enabled:             types.BoolValue(item.Enabled),
		ProviderDescription: types.StringValue(item.ProviderDescription),
	}
}
