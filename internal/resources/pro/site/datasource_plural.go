// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package site

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultPluralReadTimeout = 90 * time.Second

// SitesDataSource implements the Terraform data source for Jamf Pro site searches.
type SitesDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource              = &SitesDataSource{}
	_ datasource.DataSourceWithConfigure = &SitesDataSource{}
)

// NewSitesDataSource returns a new instance of SitesDataSource.
func NewSitesDataSource() datasource.DataSource {
	return &SitesDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *SitesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_sites"
}

// Schema returns the plural data source schema.
func (d *SitesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List Jamf Pro sites. Supply an optional case-insensitive `name_substring` filter; filtering is applied client-side after the full list is fetched. Omit the filter to receive every site." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"filter":   filters.ClassicFilterAttribute(),
			"sites": schema.ListNestedAttribute{
				MarkdownDescription: "Sites matching the supplied filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{MarkdownDescription: "Site ID assigned by Jamf Pro.", Computed: true},
						"name": schema.StringAttribute{MarkdownDescription: "Site display name.", Computed: true},
					},
				},
			},
		},
	}
}

// Configure wires the Jamf ProClassic client into the data source via the shared
// providerdata.ConfigureProClassic helper.
func (d *SitesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_sites")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches sites and populates state. Applies the optional client-side
// substring filter after the full list is retrieved.
func (d *SitesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data SitesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultPluralReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	listResp, err := d.client.ListSites(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro sites", helpers.APIErrorDetail(err))
		return
	}

	items := []proclassic.Site{}
	if listResp != nil {
		items = listResp.Sites
	}

	filter := filters.ClassicFilterModel{}
	if data.Filter != nil {
		filter = *data.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, func(s proclassic.Site) string {
		if s.Name == nil {
			return ""
		}
		return *s.Name
	})

	results := make([]SitesDataSourceResultModel, 0, len(items))
	for _, s := range items {
		results = append(results, SitesDataSourceResultModel{
			ID:   helpers.StringValueFromIntPtr(s.ID),
			Name: helpers.StringPointerValueOrNull(s.Name),
		})
	}

	data.Sites = results
	data.ID = types.StringValue("sites")

	tflog.Trace(ctx, "listed Jamf Pro sites data source", map[string]any{
		"name_substring": filter.NameSubstring.ValueString(),
		"count":          len(results),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
