// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_extension_attribute

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// MobileDeviceExtensionAttributeDataSource implements the Terraform data source
// for Jamf Pro mobile device extension attributes. Lookup is by ID or by exact
// name — exactly one must be supplied.
type MobileDeviceExtensionAttributeDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &MobileDeviceExtensionAttributeDataSource{}
	_ datasource.DataSourceWithConfigure        = &MobileDeviceExtensionAttributeDataSource{}
	_ datasource.DataSourceWithConfigValidators = &MobileDeviceExtensionAttributeDataSource{}
)

// NewMobileDeviceExtensionAttributeDataSource returns a new instance of the data
// source.
func NewMobileDeviceExtensionAttributeDataSource() datasource.DataSource {
	return &MobileDeviceExtensionAttributeDataSource{}
}

// Metadata sets the data source type name.
func (d *MobileDeviceExtensionAttributeDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_extension_attribute"
}

// Schema returns the data source schema.
func (d *MobileDeviceExtensionAttributeDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro mobile device extension attribute by ID or by exact name. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Mobile device extension attribute ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Mobile device extension attribute display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"description":                 schema.StringAttribute{MarkdownDescription: "Description of the extension attribute.", Computed: true},
			"data_type":                   schema.StringAttribute{MarkdownDescription: "Data type collected (`STRING`, `INTEGER`, `DATE`).", Computed: true},
			"input_type":                  schema.StringAttribute{MarkdownDescription: "Input type (`TEXT`, `POPUP`, `DIRECTORY_SERVICE_ATTRIBUTE_MAPPING`).", Computed: true},
			"inventory_display":           schema.StringAttribute{MarkdownDescription: "Inventory display category.", Computed: true},
			"directory_service_attribute": schema.StringAttribute{MarkdownDescription: "Mapped directory-service attribute name.", Computed: true},
			"allow_multiple_values":       schema.BoolAttribute{MarkdownDescription: "Whether multiple values are collected for a directory-service-mapped attribute.", Computed: true},
			"popup_menu_choices": schema.SetAttribute{
				MarkdownDescription: "Pop-up menu choices (POPUP input type). Returned sorted alphabetically.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *MobileDeviceExtensionAttributeDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *MobileDeviceExtensionAttributeDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_extension_attribute")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a mobile device extension attribute by ID or by name.
func (d *MobileDeviceExtensionAttributeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data MobileDeviceExtensionAttributeDataSourceModel
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
		got *pro.MobileDeviceExtensionAttributes
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetMobileDeviceExtensionAttributeV1(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.ResolveMobileDeviceExtensionAttributeV1ByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing mobile device extension attribute selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro mobile device extension attribute", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignMobileDeviceExtensionAttributeDataSourceModel(readCtx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro mobile device extension attribute data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
