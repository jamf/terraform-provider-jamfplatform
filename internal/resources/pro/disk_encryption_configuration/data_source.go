// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package disk_encryption_configuration

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

// DiskEncryptionConfigurationDataSource implements the Terraform data
// source for Jamf Pro disk encryption configurations. Lookup supports
// either `id` or `name` — exactly one must be supplied.
//
// Audit reference: `/diskencryptionconfigurations/name/{name}` returns
// 200 OK and the resource document (probed 2026-05-23, EU tenant). No
// list+ID-match workaround required — the SDK's
// `GetDiskEncryptionConfigurationByName` is used directly.
type DiskEncryptionConfigurationDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &DiskEncryptionConfigurationDataSource{}
	_ datasource.DataSourceWithConfigure        = &DiskEncryptionConfigurationDataSource{}
	_ datasource.DataSourceWithConfigValidators = &DiskEncryptionConfigurationDataSource{}
)

// NewDiskEncryptionConfigurationDataSource returns a new instance of the
// data source.
func NewDiskEncryptionConfigurationDataSource() datasource.DataSource {
	return &DiskEncryptionConfigurationDataSource{}
}

// Metadata sets the data source type name.
func (d *DiskEncryptionConfigurationDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_disk_encryption_configuration"
}

// Schema returns the data source schema.
func (d *DiskEncryptionConfigurationDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro disk encryption configuration by ID or by exact name. Exactly one of `id` or `name` must be supplied. The data source never returns the plaintext IRK password; Jamf Pro does not return it on read. Manage the password through the resource instead." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Disk encryption configuration ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Disk encryption configuration display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"key_type": schema.StringAttribute{
				MarkdownDescription: "Recovery key type (`Individual`, `Institutional`, `Individual and Institutional`).",
				Computed:            true,
			},
			"file_vault_enabled_users": schema.StringAttribute{
				MarkdownDescription: "Enabled FileVault 2 user (`Current or Next User`, `Management Account`).",
				Computed:            true,
			},
			"institutional_recovery_key": schema.SingleNestedAttribute{
				MarkdownDescription: "Institutional recovery key block. Populated only when the configuration stores an IRK certificate (see the resource documentation for the empty-block read quirk).",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"key": schema.StringAttribute{
						MarkdownDescription: "Certificate Subject DN. Returned by Jamf Pro; not user-settable.",
						Computed:            true,
					},
					"certificate_type": schema.StringAttribute{
						MarkdownDescription: "Certificate format (`PKCS12`, `DER`, `PEM`).",
						Computed:            true,
					},
					"data": schema.StringAttribute{
						MarkdownDescription: "Base64-encoded recovery certificate payload.",
						Computed:            true,
						Sensitive:           true,
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *DiskEncryptionConfigurationDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *DiskEncryptionConfigurationDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_disk_encryption_configuration")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a disk encryption configuration by ID or by name.
func (d *DiskEncryptionConfigurationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data DiskEncryptionConfigurationDataSourceModel
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
		got *proclassic.DiskEncryptionConfiguration
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetDiskEncryptionConfigurationByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetDiskEncryptionConfigurationByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing disk encryption configuration selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro disk encryption configuration", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignDiskEncryptionConfigurationDataSourceModel(&data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro disk encryption configuration data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
