// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultPluralReadTimeout caps how long the plural device groups read will wait.
const defaultPluralReadTimeout = 90 * time.Second

// pluralDataSourceID is the fixed ID the plural data source reports. The group
// list endpoint takes no filter, so every read of this data source returns the
// same collection and there is nothing to derive an ID from.
const pluralDataSourceID = "device_groups"

// DeviceGroupsDataSource implements the Terraform data source for listing every
// Jamf Security Cloud device group.
type DeviceGroupsDataSource struct {
	client *securitycloud.Client
}

var _ datasource.DataSource = &DeviceGroupsDataSource{}

// NewDeviceGroupsDataSource returns a new instance of DeviceGroupsDataSource.
func NewDeviceGroupsDataSource() datasource.DataSource {
	return &DeviceGroupsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DeviceGroupsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_device_groups"
}

// Schema returns the plural data source schema.
//
// This is the only construct in the package that reports the built-in "Default
// Group". Filtering it out would make the data source disagree with the admin UI
// about how many groups a tenant has, which is a worse surprise than an element
// whose `id` is null — and the null plus `built_in` says exactly what is going on.
// The list resource cannot do the same: a Terraform list result must carry an
// identity, and this group has none.
func (d *DeviceGroupsDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists every Jamf Security Cloud device group on the tenant, including the built-in " +
			"group. Jamf Security Cloud exposes no filter parameters for groups, so this data source takes no " +
			"search arguments. Filter the result in Terraform." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed identifier for this data source.",
				Computed:            true,
			},
			"device_groups": schema.ListNestedAttribute{
				MarkdownDescription: "The device groups on the tenant, sorted by name by the provider. Jamf " +
					"Security Cloud exposes no sort parameter for groups, so the order is the provider's " +
					"guarantee rather than Jamf Security Cloud's.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Device group ID assigned by Jamf Security Cloud. Null for the " +
								"built-in group, which Jamf Security Cloud does not give an identifier. Filter on " +
								"`built_in` before using this value to reference a group.",
							Computed: true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Device group name.",
							Computed:            true,
						},
						"built_in": schema.BoolAttribute{
							MarkdownDescription: "Whether this is the built-in group every tenant carries. The " +
								"built-in group cannot be renamed, deleted, or referenced by ID, and is not " +
								"manageable by the `jamfplatform_security_cloud_device_group` resource.",
							Computed: true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the
// shared providerdata.ConfigureSecurityCloud helper.
func (d *DeviceGroupsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_device_groups")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches every device group and populates Terraform state.
func (d *DeviceGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data DeviceGroupsDataSourceModel
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

	groups, err := d.client.ListDeviceGroupsV2(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Security Cloud device groups", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(pluralDataSourceID)
	sorted := sortGroupsByName(groups.Groups)
	data.DeviceGroups = make([]DeviceGroupsDataSourceResultModel, 0, len(sorted))
	for _, g := range sorted {
		data.DeviceGroups = append(data.DeviceGroups, buildDeviceGroupsResultModel(g))
	}

	tflog.Trace(ctx, "read Jamf Security Cloud device groups data source", map[string]any{"count": len(data.DeviceGroups)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
