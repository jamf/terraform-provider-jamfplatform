// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// PlatformEnvironmentDataSource implements the Terraform data source for a single
// Jamf Account platform environment.
type PlatformEnvironmentDataSource struct {
	client *account.Client
}

var (
	_ datasource.DataSource                     = &PlatformEnvironmentDataSource{}
	_ datasource.DataSourceWithConfigure        = &PlatformEnvironmentDataSource{}
	_ datasource.DataSourceWithConfigValidators = &PlatformEnvironmentDataSource{}
)

// NewPlatformEnvironmentDataSource returns a new instance of
// PlatformEnvironmentDataSource.
func NewPlatformEnvironmentDataSource() datasource.DataSource {
	return &PlatformEnvironmentDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *PlatformEnvironmentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_platform_environment"
}

// Schema returns the data source schema.
func (d *PlatformEnvironmentDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a platform environment in Jamf Account by `id` or by `name`.\n\n" +
			"Names are not unique. A lookup by `name` that matches more than one platform environment fails " +
			"and lists their identifiers, so use `id` where names may collide.\n\n" +
			"The tenants a platform environment holds are not reported here. Read them with the " +
			"`jamfplatform_account_tenants` data source." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier Jamf Account assigned to the platform environment. Set either " +
					"`id` or `name`.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Name\"** in the Jamf Account console. Set either `id` or `name`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "**\"Type\"** in the Jamf Account console: what the platform environment is " +
					"for, such as `PRODUCTION` or `SANDBOX`.",
				Computed: true,
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region every tenant in the platform environment must be in. Null for a " +
					"global platform environment, which accepts tenants from any region.",
				Computed: true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators requires exactly one of `id` and `name`.
func (d *PlatformEnvironmentDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Account client into the data source via the shared
// providerdata.ConfigureAccount helper.
func (d *PlatformEnvironmentDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_platform_environment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a platform environment by identifier or name and populates
// Terraform state.
func (d *PlatformEnvironmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data PlatformEnvironmentDataSourceModel
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

	environments, err := d.client.ListEnvironments(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Account platform environments", helpers.APIErrorDetail(err))
		return
	}

	var found *account.Environment
	if !data.ID.IsNull() && !data.ID.IsUnknown() {
		found = findEnvironmentByID(environments, data.ID.ValueString())
		if found == nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("id"),
				"Unable to find Jamf Account platform environment",
				"Your organization has no platform environment with the identifier \""+data.ID.ValueString()+"\". "+
					"Use the `jamfplatform_account_platform_environments` data source to list the ones it holds.",
			)
			return
		}
	} else {
		matches := findEnvironmentsByName(environments, data.Name.ValueString())
		switch len(matches) {
		case 0:
			resp.Diagnostics.AddAttributeError(
				path.Root("name"),
				"Unable to find Jamf Account platform environment",
				"Your organization has no platform environment named \""+data.Name.ValueString()+"\". "+
					"Use the `jamfplatform_account_platform_environments` data source to list the ones it holds.",
			)
			return
		case 1:
			found = &matches[0]
		default:
			appendDuplicateNameDiagnostics(&resp.Diagnostics, data.Name.ValueString(), matches)
			return
		}
	}

	assignDataSourceModel(&data, found)

	tflog.Trace(ctx, "read Jamf Account platform environment data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
