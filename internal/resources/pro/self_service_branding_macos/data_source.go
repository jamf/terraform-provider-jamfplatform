// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_macos

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

// SelfServiceBrandingMacosDataSource implements the read-only data source.
type SelfServiceBrandingMacosDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &SelfServiceBrandingMacosDataSource{}

// NewSelfServiceBrandingMacosDataSource returns a new instance.
func NewSelfServiceBrandingMacosDataSource() datasource.DataSource {
	return &SelfServiceBrandingMacosDataSource{}
}

// Metadata sets the data source type name.
func (d *SelfServiceBrandingMacosDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_self_service_branding_macos"
}

// Schema returns the data source schema — a read mirror of the resource.
func (d *SelfServiceBrandingMacosDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro Self Service macOS branding configuration (Settings > Self Service > Branding > macOS Branding). One configuration per tenant. Errors if no macOS branding is configured." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id":                   schema.StringAttribute{MarkdownDescription: "Fixed singleton identifier. Always `singleton`.", Computed: true},
			"application_header":   schema.StringAttribute{MarkdownDescription: "**\"Application Header\"** in the Jamf Pro admin UI.", Computed: true},
			"sidebar_heading":      schema.StringAttribute{MarkdownDescription: "**\"Sidebar - Heading\"** in the Jamf Pro admin UI.", Computed: true},
			"sidebar_subheading":   schema.StringAttribute{MarkdownDescription: "**\"Sidebar - Subheading\"** in the Jamf Pro admin UI.", Computed: true},
			"home_page_heading":    schema.StringAttribute{MarkdownDescription: "**\"Home page - Heading\"** in the Jamf Pro admin UI.", Computed: true},
			"home_page_subheading": schema.StringAttribute{MarkdownDescription: "**\"Home page - Subheading\"** in the Jamf Pro admin UI.", Computed: true},
			"icon_id":              schema.Int64Attribute{MarkdownDescription: "**\"Icon\"** in the Jamf Pro admin UI. Branding image ID, from a store separate to `jamfplatform_pro_icon`.", Computed: true},
			"banner_image_id":      schema.Int64Attribute{MarkdownDescription: "**\"Home page - Banner Image\"** in the Jamf Pro admin UI.", Computed: true},
			"timeouts":             timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *SelfServiceBrandingMacosDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_self_service_branding_macos")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current macOS branding configuration and populates state.
func (d *SelfServiceBrandingMacosDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data SelfServiceBrandingMacosDataSourceModel
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

	configs, err := d.client.ListMacOSBrandingConfigurationsV1(readCtx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro Self Service macOS branding", helpers.APIErrorDetail(err))
		return
	}
	if len(configs) == 0 {
		resp.Diagnostics.AddError(
			"No Self Service macOS branding configured",
			"No Self Service macOS branding configuration exists on this Jamf Pro tenant.",
		)
		return
	}

	assignSelfServiceBrandingMacosDataSourceModel(&data, &configs[0])
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro Self Service macOS branding data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
