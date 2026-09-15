// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ibeacon

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

// IbeaconDataSource implements the Terraform data source for Jamf Pro iBeacons.
// The singular data source supports lookup by ID OR by name — exactly one of
// the two must be supplied.
type IbeaconDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &IbeaconDataSource{}
	_ datasource.DataSourceWithConfigure        = &IbeaconDataSource{}
	_ datasource.DataSourceWithConfigValidators = &IbeaconDataSource{}
)

// NewIbeaconDataSource returns a new instance of IbeaconDataSource.
func NewIbeaconDataSource() datasource.DataSource {
	return &IbeaconDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *IbeaconDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_ibeacon"
}

// Schema returns the data source schema.
func (d *IbeaconDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro iBeacon by ID or by exact name. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "iBeacon ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "iBeacon display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"uuid":                    schema.StringAttribute{MarkdownDescription: "iBeacon UUID in canonical 8-4-4-4-12 hex form.", Computed: true},
			"major":                   schema.Int64Attribute{MarkdownDescription: "iBeacon major value (0–65535). Null when `include_any_major_value` is true.", Computed: true},
			"minor":                   schema.Int64Attribute{MarkdownDescription: "iBeacon minor value (0–65535). Null when `include_any_minor_value` is true.", Computed: true},
			"include_any_major_value": schema.BoolAttribute{MarkdownDescription: "True when the iBeacon matches any major value.", Computed: true},
			"include_any_minor_value": schema.BoolAttribute{MarkdownDescription: "True when the iBeacon matches any minor value.", Computed: true},
			"timeouts":                timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *IbeaconDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source via the
// shared providerdata.ConfigureProClassic helper.
func (d *IbeaconDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_ibeacon")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an iBeacon by ID or by name and populates Terraform state.
func (d *IbeaconDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data IbeaconDataSourceModel
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
		got *proclassic.Ibeacon
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetIBeaconByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetIBeaconByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing iBeacon selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro iBeacon", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignIbeaconDataSourceModel(&data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro iBeacon data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
