// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

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
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// StaticMobileDeviceGroupDataSource implements the singular data source. Lookup
// is by identifier or by exact name, and exactly one of the two is supplied.
type StaticMobileDeviceGroupDataSource struct {
	client *pro.Client
	pd     *providerdata.Data
}

var (
	_ datasource.DataSource                     = &StaticMobileDeviceGroupDataSource{}
	_ datasource.DataSourceWithConfigure        = &StaticMobileDeviceGroupDataSource{}
	_ datasource.DataSourceWithConfigValidators = &StaticMobileDeviceGroupDataSource{}
)

// NewStaticMobileDeviceGroupDataSource returns a new data source instance.
func NewStaticMobileDeviceGroupDataSource() datasource.DataSource {
	return &StaticMobileDeviceGroupDataSource{}
}

// Metadata sets the data source type name.
func (d *StaticMobileDeviceGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_static_mobile_device_group"
}

// Schema returns the singular data source schema.
func (d *StaticMobileDeviceGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(deviceType, groupKind) +
			" Look one up by identifier or by exact name, and supply exactly one of the two. Membership comes back with it, so this is where to look up which mobile devices a group holds." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro identifier for the group. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Group display name, matched exactly. Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"platform_id": schema.StringAttribute{
				MarkdownDescription: "Identifier for this group across Jamf Platform, which is how a blueprint or a compliance benchmark targets it.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Note Jamf Pro keeps with the group.",
				Computed:            true,
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site the group belongs to. `-1` means no site.",
				Computed:            true,
			},
			"assigned_mobile_device_ids": schema.ListAttribute{
				MarkdownDescription: "Jamf Pro identifiers of the mobile devices in the group, in the order Jamf Pro reports them.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"member_count": schema.Int64Attribute{
				MarkdownDescription: "Mobile devices Jamf Pro counts in the group.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators requires exactly one of id or name.
func (d *StaticMobileDeviceGroupDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Pro client and provider data into the data source.
func (d *StaticMobileDeviceGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_static_mobile_device_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_static_mobile_device_group")...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
	d.pd = pd
}

// Read looks the group up, then fetches its membership and platform identifier.
func (d *StaticMobileDeviceGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Check that the provider block is set up correctly.",
		)
		return
	}

	var data StaticMobileDeviceGroupDataSourceModel
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

	lookupID := data.ID.ValueString()
	switch {
	case helpers.IsConfiguredValue(data.ID) && lookupID != "":
	case helpers.IsConfiguredValue(data.Name) && data.Name.ValueString() != "":
		resolved, err := d.client.ResolveStaticMobileDeviceGroupV2IDByName(readCtx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to find a Jamf Pro static mobile device group with that name", helpers.APIErrorDetail(err))
			return
		}
		lookupID = resolved
	default:
		resp.Diagnostics.AddError("Missing group selector", "Supply exactly one of `id` or `name`.")
		return
	}

	got, err := d.client.GetStaticMobileDeviceGroupV2(readCtx, lookupID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the Jamf Pro static mobile device group", helpers.APIErrorDetail(err))
		return
	}
	data.ID = types.StringValue(lookupID)
	assignDataSourceModel(&data, got)

	devices, memberErr := d.client.ListStaticMobileDeviceGroupMembershipV2(readCtx, data.ID.ValueString(), nil, "")
	if memberErr != nil {
		resp.Diagnostics.AddError("Unable to read the membership of the Jamf Pro static mobile device group", helpers.APIErrorDetail(memberErr))
		return
	}
	data.AssignedMobileDeviceIDs = flattenMembershipIDs(membershipIDs(devices))

	byJamfProID, bridgeDiags := progroups.PlatformIDsByJamfProID(readCtx, d.client, d.pd, deviceType, groupKind)
	resp.Diagnostics.Append(bridgeDiags...)
	data.PlatformID = progroups.PlatformIDValue(byJamfProID, data.ID.ValueString())

	tflog.Trace(ctx, "read Jamf Pro static mobile device group data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
