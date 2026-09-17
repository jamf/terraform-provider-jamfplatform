// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/devicegroups"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// DeviceGroupDataSource implements the Terraform data source for Jamf device groups.
type DeviceGroupDataSource struct {
	client    *devicegroups.Client
	proClient *pro.Client
	pd        *providerdata.Data
	groupRef  criteria.GroupResolver
}

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &DeviceGroupDataSource{}

// NewDeviceGroupDataSource returns a new instance of DeviceGroupDataSource.
func NewDeviceGroupDataSource() datasource.DataSource {
	return &DeviceGroupDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DeviceGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_device_group"
}

// Schema sets the Terraform schema for the data source.
func (d *DeviceGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Jamf device group by ID. Requires **Device Group Inventory API** access." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Device group Platform ID to query.",
				Required:            true,
			},
			"jamf_pro_id": schema.StringAttribute{
				// Wire source: pro/v2 groups lookup, bridging the Platform group UUID to
				// the numeric Jamf Pro ID that scope blocks require.
				MarkdownDescription: "Numeric Jamf Pro ID for this group, looked up in Jamf Pro. Use it to scope Jamf Pro resources to the group: policies, configuration profiles and restricted software all target groups by this ID. Null when the API integration lacks the **Inventory → Device groups → Read** permission in Jamf Account, when the group cannot be found in Jamf Pro, or when the lookup transiently fails.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Device group name.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Device group Description.",
				Computed:            true,
			},
			"device_type": schema.StringAttribute{
				MarkdownDescription: "Device type value returned in lowercase.",
				Computed:            true,
			},
			"group_type": schema.StringAttribute{
				MarkdownDescription: "Group type value returned in lowercase.",
				Computed:            true,
			},
			"member_count": schema.Int64Attribute{
				MarkdownDescription: "Number of members in the group.",
				Computed:            true,
			},
			"members": schema.ListAttribute{
				MarkdownDescription: "Devices currently assigned to the group (Jamf Pro Management IDs).",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"criteria": schema.ListNestedAttribute{
				MarkdownDescription: "Smart-group criteria returned by the platform.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"order": schema.Int64Attribute{
							MarkdownDescription: "Server-evaluated order for the criterion.",
							Computed:            true,
						},
						"criteria": schema.StringAttribute{
							MarkdownDescription: "Inventory attribute used in the criterion.",
							Computed:            true,
						},
						"operator": schema.StringAttribute{
							MarkdownDescription: "Comparison operator.",
							Computed:            true,
						},
						"value": schema.StringAttribute{
							MarkdownDescription: "Comparison value, when applicable.",
							Computed:            true,
						},
						"and_or": schema.StringAttribute{
							MarkdownDescription: "How this criterion joins the one before it. The first criterion's value is never used.",
							Computed:            true,
						},
						"has_opening_parenthesis": schema.BoolAttribute{
							MarkdownDescription: "Whether the criterion starts a parenthetical expression.",
							Computed:            true,
						},
						"has_closing_parenthesis": schema.BoolAttribute{
							MarkdownDescription: "Whether the criterion ends a parenthetical expression.",
							Computed:            true,
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure sets up the API client for the data source from the provider configuration.
func (d *DeviceGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	resp.Diagnostics.Append(pd.RequireScope("jamfplatform_device_group", providerdata.DeviceGroupsScopes...)...)
	if resp.Diagnostics.HasError() {
		return
	}

	d.client = devicegroups.New(pd.Client)
	d.proClient = pro.New(pd.Client)
	d.groupRef = criteria.NewProGroupResolver(proclassic.New(pd.Client))
	d.pd = pd
}

// Read fetches a device group by ID and populates the Terraform state.
func (d *DeviceGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data DeviceGroupDataSourceModel

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

	if data.ID.IsNull() || data.ID.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing ID",
			"The id attribute must be provided to read a device group.",
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

	grp, err := d.client.GetDeviceGroup(readCtx, data.ID.ValueString())

	if err != nil {
		resp.Diagnostics.AddError("Unable to find device group", helpers.APIErrorDetail(err))
		return
	}

	members, err := d.client.ListDeviceGroupMembers(readCtx, grp.ID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read device group members", helpers.APIErrorDetail(err))
		return
	}

	listMembers, diags := types.ListValueFrom(ctx, types.StringType, members)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	description := helpers.StringValueOrNull(grp.Description)
	deviceType := helpers.StringValueOrNull(strings.ToLower(grp.DeviceType))
	groupType := helpers.StringValueOrNull(strings.ToLower(grp.GroupType))

	var grpCriteria []devicegroups.DeviceGroupCriteriaRepresentationV1
	if grp.Criteria != nil {
		grpCriteria = *grp.Criteria
	}

	timeoutsConfig := data.Timeouts

	jamfProID, jamfProDiags := resolveJamfProID(readCtx, d.proClient, d.pd, grp.ID)
	resp.Diagnostics.Append(jamfProDiags...)

	data = DeviceGroupDataSourceModel{
		ID:          types.StringValue(grp.ID),
		JamfProID:   jamfProID,
		Name:        types.StringValue(grp.Name),
		Description: description,
		DeviceType:  deviceType,
		GroupType:   groupType,
		Criteria:    flattenDeviceGroupCriteria(grpCriteria, nil),
		Members:     listMembers,
		MemberCount: types.Int64Value(int64(grp.MemberCount)),
		Timeouts:    timeoutsConfig,
	}
	// No prior state in a data-source read → reverse-resolve any Jamf-group
	// "member of" criterion id back to the group name (11.29 read regression).
	data.Criteria = readbackGroupRefCriteria(readCtx, d.groupRef, dsObjectType(deviceType.ValueString()), data.Criteria, nil)

	tflog.Trace(ctx, "read device group data source", map[string]any{
		"id": grp.ID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
