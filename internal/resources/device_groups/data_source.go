// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_groups

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultReadTimeout = 90 * time.Second

var DeviceGroupFilterSelectors = []string{
	"name",
	"description",
	"deviceType",
	"groupType",
}

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &DeviceGroupsDataSource{}

// NewDeviceGroupsDataSource returns a new instance of DeviceGroupsDataSource.
func NewDeviceGroupsDataSource() datasource.DataSource {
	return &DeviceGroupsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DeviceGroupsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_groups"
}

// Schema sets the Terraform schema for the data source.
func (d *DeviceGroupsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Searches Jamf device groups with optional filters. Requires **Device Group Inventory API** access." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"filter": filters.FilterAttribute(
				filters.SelectorDescription(DeviceGroupFilterSelectors),
				DeviceGroupFilterSelectors,
			),
			"device_groups": schema.ListNestedAttribute{
				MarkdownDescription: "Device groups that matched the applied filters.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Device group Platform ID.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Device group name.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "Device group description.",
							Computed:            true,
						},
						"device_type": schema.StringAttribute{
							MarkdownDescription: "Device type for the group (lowercase).",
							Computed:            true,
						},
						"group_type": schema.StringAttribute{
							MarkdownDescription: "Group type for the group (lowercase).",
							Computed:            true,
						},
						"member_count": schema.Int64Attribute{
							MarkdownDescription: "Number of members in the group.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure sets up the API client for the data source from the provider configuration.
func (d *DeviceGroupsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *providerdata.Data, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_device_groups", providerdata.DeviceGroupsScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d.client = devicegroups.New(pd.Client)
}

// Read fetches device groups that match the provided filters.
func (d *DeviceGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DeviceGroupsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	filterExpression := filters.BuildRSQLExpression(data.Filters, filters.AllowList(DeviceGroupFilterSelectors))

	tflog.Debug(ctx, "devices filter expression", map[string]any{
		"filter": filterExpression,
	})

	groups, err := d.client.ListDeviceGroups(readCtx, nil, filterExpression)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list device groups", helpers.APIErrorDetail(err))
		return
	}

	results := make([]DeviceGroupsDataSourceResultModel, 0, len(groups))
	for _, grp := range groups {
		description := helpers.StringValueOrNull(grp.Description)
		deviceType := helpers.StringValueOrNull(strings.ToLower(grp.DeviceType))
		groupType := helpers.StringValueOrNull(strings.ToLower(grp.GroupType))

		result := DeviceGroupsDataSourceResultModel{
			ID:          types.StringValue(grp.ID),
			Name:        types.StringValue(grp.Name),
			Description: description,
			DeviceType:  deviceType,
			GroupType:   groupType,
			MemberCount: types.Int64Value(int64(grp.MemberCount)),
		}
		results = append(results, result)
	}

	data.DeviceGroups = results
	data.ID = types.StringValue("device_groups")

	tflog.Trace(ctx, "listed device group data source", map[string]any{
		"filter": filterExpression,
		"count":  len(results),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
