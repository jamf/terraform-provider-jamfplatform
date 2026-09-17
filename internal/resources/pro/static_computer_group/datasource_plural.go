// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

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

const defaultPluralReadTimeout = 90 * time.Second

// StaticComputerGroupsDataSource searches static computer groups.
//
// It reports each group's member count, which the search result carries, and
// not the member identifiers, which it does not. Reading those means one request
// per group against a second API surface, so a page of forty groups would cost
// forty extra requests; the singular data source is the place for that.
type StaticComputerGroupsDataSource struct {
	client *pro.Client
	pd     *providerdata.Data
}

var (
	_ datasource.DataSource              = &StaticComputerGroupsDataSource{}
	_ datasource.DataSourceWithConfigure = &StaticComputerGroupsDataSource{}
)

// NewStaticComputerGroupsDataSource returns a new StaticComputerGroupsDataSource.
func NewStaticComputerGroupsDataSource() datasource.DataSource {
	return &StaticComputerGroupsDataSource{}
}

// Metadata sets the data source type name.
func (d *StaticComputerGroupsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_static_computer_groups"
}

// Schema returns the plural data source schema.
func (d *StaticComputerGroupsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(progroups.DeviceTypeComputer, progroups.GroupKindStatic) +
			"\n\nSearches static computer groups, optionally filtered. Each result carries how many computers the group holds; to read which computers those are, look the group up with `jamfplatform_pro_static_computer_group`." +
			pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"filter": filters.FilterAttribute(
				filters.SelectorDescription(StaticComputerGroupFilterSelectors),
				StaticComputerGroupFilterSelectors,
			),
			"static_computer_groups": schema.ListNestedAttribute{
				MarkdownDescription: "Static computer groups matching the supplied filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Jamf Pro identifier for the group.",
							Computed:            true,
						},
						"platform_id": schema.StringAttribute{
							MarkdownDescription: "Identifier for this group across Jamf Platform, for targeting it from a blueprint, a compliance benchmark, or any `jamfplatform_device_*` construct that asks for a group.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Group name.",
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
							MarkdownDescription: "How many computers the group holds.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure wires the Jamf Pro client and the shared provider data onto the data
// source. No classic client: this read never touches membership.
func (d *StaticComputerGroupsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_static_computer_groups")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_static_computer_groups")...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
	d.pd = pd
}

// Read fetches the matching groups and populates state.
func (d *StaticComputerGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Check that the provider block is set up correctly.",
		)
		return
	}

	var data StaticComputerGroupsDataSourceModel
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

	filterExpression := filters.BuildRSQLExpression(data.Filters, filters.AllowList(StaticComputerGroupFilterSelectors))
	tflog.Debug(ctx, "static computer groups filter expression", map[string]any{"filter": filterExpression})

	groups, err := d.client.ListStaticComputerGroupsV3(readCtx, nil, filterExpression)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro static computer groups", helpers.APIErrorDetail(err))
		return
	}

	// One request covers the whole page, because the bridge narrows to this
	// construct's own kind of group and each result is then looked up in the map
	// it returns.
	byJamfProID, bridgeDiags := progroups.PlatformIDsByJamfProID(readCtx, d.client, d.pd, progroups.DeviceTypeComputer, progroups.GroupKindStatic)
	resp.Diagnostics.Append(bridgeDiags...)

	results := make([]StaticComputerGroupsDataSourceResultModel, 0, len(groups))
	for _, group := range groups {
		results = append(results, StaticComputerGroupsDataSourceResultModel{
			ID:          types.StringValue(group.ID),
			PlatformID:  progroups.PlatformIDValue(byJamfProID, group.ID),
			Name:        types.StringValue(group.Name),
			Description: helpers.StringPointerValueOrNull(group.Description),
			SiteID:      progroups.SiteIDForState(group.SiteID),
			MemberCount: types.Int64Value(int64(group.Count)),
		})
	}

	data.StaticComputerGroups = results
	data.ID = types.StringValue("static_computer_groups")

	tflog.Trace(ctx, "listed Jamf Pro static computer groups data source", map[string]any{
		"filter": filterExpression,
		"count":  len(results),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
