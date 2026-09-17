// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

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

// defaultPluralReadTimeout caps the plural read, which pages the whole search.
const defaultPluralReadTimeout = 90 * time.Second

// pluralDataSourceStateID is the placeholder identifier the plural read stores,
// since a search has no identity of its own.
const pluralDataSourceStateID = "smart_computer_groups"

// SmartComputerGroupFilterSelectors are the fields the smart computer group
// search accepts a filter clause on. They keep their API-native spelling, which
// is the one exemption from the snake_case attribute rule: the value passes
// through to Jamf Pro verbatim.
var SmartComputerGroupFilterSelectors = []string{
	"id",
	"name",
	"siteId",
}

// SmartComputerGroupsDataSource implements the plural data source.
type SmartComputerGroupsDataSource struct {
	client *pro.Client
	pd     *providerdata.Data
}

var (
	_ datasource.DataSource              = &SmartComputerGroupsDataSource{}
	_ datasource.DataSourceWithConfigure = &SmartComputerGroupsDataSource{}
)

// NewSmartComputerGroupsDataSource returns a new instance of SmartComputerGroupsDataSource.
func NewSmartComputerGroupsDataSource() datasource.DataSource {
	return &SmartComputerGroupsDataSource{}
}

// Metadata sets the data source type name.
func (d *SmartComputerGroupsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_smart_computer_groups"
}

// Schema returns the plural data source schema.
func (d *SmartComputerGroupsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(deviceType, groupKind) +
			"\n\nSearches the smart computer groups in Jamf Pro, with optional RSQL filter clauses. Each result reports the group's membership count and omits its criteria: Jamf Pro leaves the criteria out of a search, so read one group with `jamfplatform_pro_smart_computer_group` when you need them. Filtering on `siteId` works only for an account with full access; a site-restricted account has the filter applied for it." +
			pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"filter": filters.FilterAttribute(
				filters.SelectorDescription(SmartComputerGroupFilterSelectors),
				SmartComputerGroupFilterSelectors,
			),
			"smart_computer_groups": schema.ListNestedAttribute{
				MarkdownDescription: "Smart computer groups matching the supplied filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Jamf Pro identifier of the group.",
							Computed:            true,
						},
						"platform_id": schema.StringAttribute{
							MarkdownDescription: "Identifier for this group across Jamf Platform, for targeting it from a blueprint, a compliance benchmark, or any `jamfplatform_device_*` construct that asks for a group.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Group name as it appears in Jamf Pro.",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "Free-text note stored beside the group.",
							Computed:            true,
						},
						"site_id": schema.StringAttribute{
							MarkdownDescription: "Jamf Pro site the group belongs to. `" + progroups.NoSiteID + "` means no site.",
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

// Configure wires the Jamf Pro client into the data source, then applies the
// family's version refusal.
func (d *SmartComputerGroupsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_smart_computer_groups")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_smart_computer_groups")...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
	d.pd = pd
}

// Read fetches the groups matching the supplied filter and populates state. The
// platform identifiers for the whole page come from one bridging request.
func (d *SmartComputerGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Check that the provider block is set up correctly.",
		)
		return
	}

	var data SmartComputerGroupsDataSourceModel
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

	filterExpression := filters.BuildRSQLExpression(data.Filters, filters.AllowList(SmartComputerGroupFilterSelectors))
	tflog.Debug(ctx, "smart computer groups filter expression", map[string]any{"filter": filterExpression})

	groups, err := d.client.ListSmartComputerGroupsV3(readCtx, nil, filterExpression)
	if err != nil {
		resp.Diagnostics.AddError("Unable to search Jamf Pro smart computer groups", helpers.APIErrorDetail(err))
		return
	}

	byJamfProID, bridgeDiags := progroups.PlatformIDsByJamfProID(readCtx, d.client, d.pd, deviceType, groupKind)
	resp.Diagnostics.Append(bridgeDiags...)

	results := make([]SmartComputerGroupsDataSourceResultModel, 0, len(groups))
	for _, group := range groups {
		results = append(results, searchResultModel(group, byJamfProID))
	}

	data.Groups = results
	data.ID = types.StringValue(pluralDataSourceStateID)

	tflog.Trace(ctx, "searched Jamf Pro smart computer groups data source", map[string]any{
		"filter": filterExpression,
		"count":  len(results),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
