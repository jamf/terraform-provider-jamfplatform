// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_distribution_point

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

// CloudDistributionPointDataSource implements the read-only data source.
type CloudDistributionPointDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &CloudDistributionPointDataSource{}

// NewCloudDistributionPointDataSource returns a new instance.
func NewCloudDistributionPointDataSource() datasource.DataSource {
	return &CloudDistributionPointDataSource{}
}

// Metadata sets the data source type name.
func (d *CloudDistributionPointDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_cloud_distribution_point"
}

// Schema returns the data source schema — a read mirror of the resource.
// WriteOnly secrets are omitted (never returned by the API). A `cdn_type` of
// `NONE` is surfaced as-is, indicating no cloud distribution point is configured.
func (d *CloudDistributionPointDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro cloud distribution point configuration. One record per tenant. `cdn_type` is `NONE` when no cloud distribution point is configured." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},
			"cdn_type": schema.StringAttribute{
				MarkdownDescription: "Content delivery network type: `JAMF_CLOUD`, `AMAZON_S3`, `AKAMAI`, `RACKSPACE_CLOUD_FILES`, or `NONE` (not configured).",
				Computed:            true,
			},
			"master":                      schema.BoolAttribute{MarkdownDescription: "Whether this is the master (primary) distribution point.", Computed: true},
			"username":                    schema.StringAttribute{MarkdownDescription: "Connection username (non-JCDS types).", Computed: true},
			"directory":                   schema.StringAttribute{MarkdownDescription: "Directory / bucket path (non-JCDS types).", Computed: true},
			"upload_url":                  schema.StringAttribute{MarkdownDescription: "Upload endpoint URL.", Computed: true},
			"download_url":                schema.StringAttribute{MarkdownDescription: "Download endpoint URL.", Computed: true},
			"cdn_url":                     schema.StringAttribute{MarkdownDescription: "CDN URL.", Computed: true},
			"require_signed_urls":         schema.BoolAttribute{MarkdownDescription: "Whether downloads require signed URLs.", Computed: true},
			"key_pair_id":                 schema.StringAttribute{MarkdownDescription: "Signed-URL key pair identifier.", Computed: true},
			"expiration_seconds":          schema.Int64Attribute{MarkdownDescription: "Signed-URL expiration window in seconds.", Computed: true},
			"secondary_auth_required":     schema.BoolAttribute{MarkdownDescription: "Whether secondary authentication is required.", Computed: true},
			"secondary_auth_time_to_live": schema.Int64Attribute{MarkdownDescription: "Secondary authentication token time-to-live in seconds.", Computed: true},
			"secondary_auth_status_code":  schema.Int64Attribute{MarkdownDescription: "Secondary authentication status code.", Computed: true},
			"has_connection_succeeded":    schema.BoolAttribute{MarkdownDescription: "Whether the most recent connection test succeeded.", Computed: true},
			"message":                     schema.StringAttribute{MarkdownDescription: "Connection status message.", Computed: true},
			"inventory_id":                schema.StringAttribute{MarkdownDescription: "Inventory identifier for the distribution point. Returned by Jamf Pro; not user-settable.", Computed: true},
			"timeouts":                    timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *CloudDistributionPointDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_cloud_distribution_point")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current cloud distribution point and populates state.
func (d *CloudDistributionPointDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data CloudDistributionPointDataSourceModel
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

	got, err := d.client.GetCloudDistributionPointV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro cloud distribution point", helpers.APIErrorDetail(err))
		return
	}
	assignCloudDistributionPointDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro cloud distribution point data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
