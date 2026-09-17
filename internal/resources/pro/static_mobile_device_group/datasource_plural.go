// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// defaultPluralReadTimeout caps a whole-page read, which pages internally and so
// takes longer than a single group.
const defaultPluralReadTimeout = 90 * time.Second

// StaticMobileDeviceGroupsDataSource implements the plural data source.
type StaticMobileDeviceGroupsDataSource struct {
	client *pro.Client
	pd     *providerdata.Data
}

var (
	_ datasource.DataSource              = &StaticMobileDeviceGroupsDataSource{}
	_ datasource.DataSourceWithConfigure = &StaticMobileDeviceGroupsDataSource{}
)

// NewStaticMobileDeviceGroupsDataSource returns a new plural data source
// instance.
func NewStaticMobileDeviceGroupsDataSource() datasource.DataSource {
	return &StaticMobileDeviceGroupsDataSource{}
}

// Metadata sets the data source type name.
func (d *StaticMobileDeviceGroupsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_static_mobile_device_groups"
}

// Schema returns the plural data source schema.
func (d *StaticMobileDeviceGroupsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(deviceType, groupKind) +
			" Search static mobile device groups, optionally narrowing with filter clauses. Each result reports how many mobile devices the group holds but not which ones; for that, look a single group up with `jamfplatform_pro_static_mobile_device_group`." +
			pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"filter": filters.FilterAttribute(
				filters.SelectorDescription(FilterSelectors),
				FilterSelectors,
			),
			"static_mobile_device_groups": schema.ListNestedAttribute{
				MarkdownDescription: "Static mobile device groups matching the supplied filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Jamf Pro identifier for the group.",
							Computed:            true,
						},
						"platform_id": schema.StringAttribute{
							MarkdownDescription: "Identifier for this group across Jamf Platform, which is how a blueprint or a compliance benchmark targets it.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Group display name.",
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
						"member_count": schema.Int64Attribute{
							MarkdownDescription: "Mobile devices Jamf Pro counts in the group.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure wires the Jamf Pro client and provider data into the data source.
func (d *StaticMobileDeviceGroupsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_static_mobile_device_groups")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_static_mobile_device_groups")...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
	d.pd = pd
}

// Read fetches the matching groups and resolves every platform identifier in one
// further request, rather than one per group.
func (d *StaticMobileDeviceGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Check that the provider block is set up correctly.",
		)
		return
	}

	var data StaticMobileDeviceGroupsDataSourceModel
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

	filterExpression := filters.BuildRSQLExpression(data.Filters, filters.AllowList(FilterSelectors))
	tflog.Debug(ctx, "static mobile device groups filter expression", map[string]any{"filter": filterExpression})

	groups, err := d.client.ListStaticMobileDeviceGroupsV2(readCtx, nil, filterExpression)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro static mobile device groups", helpers.APIErrorDetail(err))
		return
	}

	byJamfProID, bridgeDiags := progroups.PlatformIDsByJamfProID(readCtx, d.client, d.pd, deviceType, groupKind)
	resp.Diagnostics.Append(bridgeDiags...)

	results := make([]StaticMobileDeviceGroupsDataSourceResultModel, 0, len(groups))
	for _, g := range groups {
		results = append(results, pluralResultModel(g, byJamfProID))
	}

	data.Groups = results
	data.ID = types.StringValue("static_mobile_device_groups")

	tflog.Trace(ctx, "listed Jamf Pro static mobile device groups data source", map[string]any{
		"filter": filterExpression,
		"count":  len(results),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
