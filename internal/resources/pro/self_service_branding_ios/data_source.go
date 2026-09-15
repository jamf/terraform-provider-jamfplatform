// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_branding_ios

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

// SelfServiceBrandingIosDataSource implements the read-only data source.
type SelfServiceBrandingIosDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &SelfServiceBrandingIosDataSource{}

// NewSelfServiceBrandingIosDataSource returns a new instance.
func NewSelfServiceBrandingIosDataSource() datasource.DataSource {
	return &SelfServiceBrandingIosDataSource{}
}

// Metadata sets the data source type name.
func (d *SelfServiceBrandingIosDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_self_service_branding_ios"
}

// Schema returns the data source schema — a read mirror of the resource.
func (d *SelfServiceBrandingIosDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro Self Service iOS & iPadOS branding configuration (Settings > Self Service > Branding > iOS & iPadOS Branding). One configuration per tenant. Errors if no iOS branding is configured." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id":                           schema.StringAttribute{MarkdownDescription: "Fixed singleton identifier. Always `singleton`.", Computed: true},
			"main_header":                  schema.StringAttribute{MarkdownDescription: "**\"Main Header\"** in the Jamf Pro admin UI.", Computed: true},
			"branding_name_color_code":     schema.StringAttribute{MarkdownDescription: "Hex colour of the Main Header text.", Computed: true},
			"header_background_color_code": schema.StringAttribute{MarkdownDescription: "Hex colour of the header background.", Computed: true},
			"menu_icon_color_code":         schema.StringAttribute{MarkdownDescription: "Hex colour of the menu icons.", Computed: true},
			"status_bar_text_color":        schema.StringAttribute{MarkdownDescription: "Status bar text appearance, `light` or `dark`.", Computed: true},
			"icon_id":                      schema.Int64Attribute{MarkdownDescription: "**\"Icon\"** in the Jamf Pro admin UI. Branding image ID, from a store separate to `jamfplatform_pro_icon`.", Computed: true},
			"timeouts":                     timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *SelfServiceBrandingIosDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_self_service_branding_ios")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current iOS branding configuration and populates state.
func (d *SelfServiceBrandingIosDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data SelfServiceBrandingIosDataSourceModel
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

	configs, err := d.client.ListIOSBrandingConfigurationsV1(readCtx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro Self Service iOS branding", helpers.APIErrorDetail(err))
		return
	}
	if len(configs) == 0 {
		resp.Diagnostics.AddError(
			"No Self Service iOS branding configured",
			"No Self Service iOS branding configuration exists on this Jamf Pro tenant.",
		)
		return
	}

	assignSelfServiceBrandingIosDataSourceModel(&data, &configs[0])
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro Self Service iOS branding data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
