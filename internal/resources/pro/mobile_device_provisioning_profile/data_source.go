// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_provisioning_profile

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// ProvisioningProfileDataSource implements the Terraform data source for Jamf
// Pro mobile device provisioning profiles. Lookup is by id, name, or uuid —
// exactly one must be supplied.
type ProvisioningProfileDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &ProvisioningProfileDataSource{}
	_ datasource.DataSourceWithConfigure        = &ProvisioningProfileDataSource{}
	_ datasource.DataSourceWithConfigValidators = &ProvisioningProfileDataSource{}
)

// NewProvisioningProfileDataSource returns a new instance of ProvisioningProfileDataSource.
func NewProvisioningProfileDataSource() datasource.DataSource {
	return &ProvisioningProfileDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ProvisioningProfileDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_provisioning_profile"
}

// Schema returns the data source schema.
func (d *ProvisioningProfileDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro mobile device provisioning profile by ID, name, or UUID. Exactly one of `id`, `name`, or `uuid` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Provisioning profile ID. Mutually exclusive with `name` and `uuid`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Profile name (exact match). Mutually exclusive with `id` and `uuid`.",
				Optional:            true,
				Computed:            true,
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "Profile UUID (exact match). Mutually exclusive with `id` and `name`.",
				Optional:            true,
				Computed:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display name shown in the Jamf Pro admin UI.",
				Computed:            true,
			},
			"profile_data": schema.StringAttribute{
				MarkdownDescription: "Base64-encoded signed `.mobileprovision` profile as stored by Jamf Pro.",
				Computed:            true,
			},
			"creation_date_utc": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (UTC).",
				Computed:            true,
			},
			"creation_date_epoch": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp as epoch milliseconds.",
				Computed:            true,
			},
			"expiration_date_utc": schema.StringAttribute{
				MarkdownDescription: "Expiration timestamp (UTC).",
				Computed:            true,
			},
			"expiration_date_epoch": schema.StringAttribute{
				MarkdownDescription: "Expiration timestamp as epoch milliseconds.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name / uuid is supplied.
func (d *ProvisioningProfileDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
			path.MatchRoot("uuid"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source via the shared
// providerdata.ConfigureProClassic helper.
func (d *ProvisioningProfileDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_provisioning_profile")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a profile by id, name, or uuid and populates Terraform state.
func (d *ProvisioningProfileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ProvisioningProfileDataSourceModel
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
		got *proclassic.MobileDeviceProvisioningProfile
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetMobileDeviceProvisioningProfileByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetMobileDeviceProvisioningProfileByName(readCtx, data.Name.ValueString())
	case !data.UUID.IsNull() && data.UUID.ValueString() != "":
		got, err = d.client.GetMobileDeviceProvisioningProfileByUUID(readCtx, data.UUID.ValueString())
	default:
		resp.Diagnostics.AddError("Missing selector", "Exactly one of id, name, or uuid must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro mobile device provisioning profile", helpers.APIErrorDetail(err))
		return
	}
	assignProvisioningProfileDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro mobile device provisioning profile data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString(), "uuid": data.UUID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
