// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultPluralReadTimeout caps how long the plural platform environments read
// will wait.
const defaultPluralReadTimeout = 90 * time.Second

// pluralDataSourceID is the fixed ID the plural data source reports. The
// collection takes no filter, so every read returns the same set and there is
// nothing to derive an ID from.
const pluralDataSourceID = "platform_environments"

// PlatformEnvironmentsDataSource implements the Terraform data source for
// listing every platform environment a Jamf Account organization holds.
type PlatformEnvironmentsDataSource struct {
	client *account.Client
}

var (
	_ datasource.DataSource              = &PlatformEnvironmentsDataSource{}
	_ datasource.DataSourceWithConfigure = &PlatformEnvironmentsDataSource{}
)

// NewPlatformEnvironmentsDataSource returns a new instance of
// PlatformEnvironmentsDataSource.
func NewPlatformEnvironmentsDataSource() datasource.DataSource {
	return &PlatformEnvironmentsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *PlatformEnvironmentsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_platform_environments"
}

// Schema returns the plural data source schema.
func (d *PlatformEnvironmentsDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every platform environment your Jamf Account organization holds. Jamf Account " +
			"offers no search arguments for them, so this data source takes none. Filter the result in " +
			"Terraform.\n\nThis data source does not report tenants. Read them with the " +
			"`jamfplatform_account_tenants` data source." +
			pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"platform_environments": schema.ListNestedAttribute{
				MarkdownDescription: "The platform environments the organization holds, in the order Jamf Account " +
					"returns them.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Identifier Jamf Account assigns to the platform environment.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "**\"Name\"** in the Jamf Account console.",
							Computed:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "**\"Type\"** in the Jamf Account console, such as `PRODUCTION` " +
								"or `SANDBOX`.",
							Computed: true,
						},
						"region": schema.StringAttribute{
							MarkdownDescription: "Region every tenant in the platform environment must be in. " +
								"Null for a global platform environment.",
							Computed: true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Account client into the data source via the shared
// providerdata.ConfigureAccount helper.
func (d *PlatformEnvironmentsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_platform_environments")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches every platform environment and populates Terraform state.
func (d *PlatformEnvironmentsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Check the provider block.",
		)
		return
	}

	var data PlatformEnvironmentsDataSourceModel
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

	environments, err := d.client.ListEnvironments(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Account platform environments", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(pluralDataSourceID)
	data.PlatformEnvironments = make([]PlatformEnvironmentsDataSourceResultRow, 0, len(environments))
	for _, e := range environments {
		data.PlatformEnvironments = append(data.PlatformEnvironments, buildResultRow(e))
	}

	tflog.Trace(ctx, "read Jamf Account platform environments data source", map[string]any{"count": len(data.PlatformEnvironments)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
