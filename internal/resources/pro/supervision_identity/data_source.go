// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package supervision_identity

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// SupervisionIdentityDataSource implements the Terraform data source for Jamf Pro
// supervision identities. Lookup is by id or display_name — exactly one must be
// supplied. The password and certificate are never exposed.
type SupervisionIdentityDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &SupervisionIdentityDataSource{}
	_ datasource.DataSourceWithConfigure        = &SupervisionIdentityDataSource{}
	_ datasource.DataSourceWithConfigValidators = &SupervisionIdentityDataSource{}
)

// NewSupervisionIdentityDataSource returns a new instance of SupervisionIdentityDataSource.
func NewSupervisionIdentityDataSource() datasource.DataSource {
	return &SupervisionIdentityDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *SupervisionIdentityDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_supervision_identity"
}

// Schema returns the data source schema.
func (d *SupervisionIdentityDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro supervision identity by ID or display name. Exactly one of `id` or `display_name` must be supplied. " +
			"Display names are not required to be unique. A lookup by `display_name` errors if more than one identity shares the name; use `id` to disambiguate." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Supervision identity ID. Mutually exclusive with `display_name`.",
				Optional:            true,
				Computed:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display name (exact match). Mutually exclusive with `id`. Errors if more than one identity shares the name.",
				Optional:            true,
				Computed:            true,
			},
			"common_name": schema.StringAttribute{
				MarkdownDescription: "Common name of the supervision identity's certificate.",
				Computed:            true,
			},
			"expiration_date": schema.StringAttribute{
				MarkdownDescription: "Certificate expiration date (`YYYY-MM-DD`).",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / display_name is supplied.
func (d *SupervisionIdentityDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("display_name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *SupervisionIdentityDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_supervision_identity")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an identity by id or display_name and populates Terraform state.
func (d *SupervisionIdentityDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data SupervisionIdentityDataSourceModel
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

	var (
		got *pro.SupervisionIdentity
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetSupervisionIdentityV1(readCtx, data.ID.ValueString())
	case !data.DisplayName.IsNull() && data.DisplayName.ValueString() != "":
		got, err = d.client.ResolveSupervisionIdentityV1ByName(readCtx, data.DisplayName.ValueString())
	default:
		resp.Diagnostics.AddError("Missing selector", "Exactly one of id or display_name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro supervision identity", helpers.APIErrorDetail(err))
		return
	}
	assignSupervisionIdentityDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro supervision identity data source", map[string]any{"id": data.ID.ValueString(), "display_name": data.DisplayName.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
