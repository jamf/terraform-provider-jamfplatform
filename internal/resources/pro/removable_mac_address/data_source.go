// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package removable_mac_address

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

// RemovableMacAddressDataSource implements the Terraform data source for Jamf Pro
// removable MAC addresses. The singular data source supports lookup by ID OR by
// mac_address — exactly one of the two must be supplied.
type RemovableMacAddressDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &RemovableMacAddressDataSource{}
	_ datasource.DataSourceWithConfigure        = &RemovableMacAddressDataSource{}
	_ datasource.DataSourceWithConfigValidators = &RemovableMacAddressDataSource{}
)

// NewRemovableMacAddressDataSource returns a new instance of RemovableMacAddressDataSource.
func NewRemovableMacAddressDataSource() datasource.DataSource {
	return &RemovableMacAddressDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *RemovableMacAddressDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_removable_mac_address"
}

// Schema returns the data source schema.
func (d *RemovableMacAddressDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro removable MAC address by ID or by exact MAC address. Exactly one of `id` or `mac_address` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Removable MAC address ID. Mutually exclusive with `mac_address`.",
				Optional:            true,
				Computed:            true,
			},
			"mac_address": schema.StringAttribute{
				MarkdownDescription: "MAC address (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / mac_address is supplied.
func (d *RemovableMacAddressDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("mac_address"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source via the shared
// providerdata.ConfigureProClassic helper.
func (d *RemovableMacAddressDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_removable_mac_address")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a removable MAC address by ID or by mac_address and populates state.
func (d *RemovableMacAddressDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data RemovableMacAddressDataSourceModel
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
		got *proclassic.RemovableMacAddress
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetRemovableMacAddressByID(readCtx, data.ID.ValueString())
	case !data.MacAddress.IsNull() && data.MacAddress.ValueString() != "":
		got, err = d.client.GetRemovableMacAddressByName(readCtx, data.MacAddress.ValueString())
	default:
		resp.Diagnostics.AddError("Missing removable MAC address selector", "Exactly one of id or mac_address must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro removable MAC address", helpers.APIErrorDetail(err))
		return
	}
	assignRemovableMacAddressDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro removable MAC address data source", map[string]any{"id": data.ID.ValueString(), "mac_address": data.MacAddress.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
