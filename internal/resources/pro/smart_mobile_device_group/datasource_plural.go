// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

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

// defaultPluralReadTimeout bounds the collection read, which pages until the
// tenant's groups are exhausted, plus the single bridge request that resolves
// every result's platform identifier.
const defaultPluralReadTimeout = 90 * time.Second

// SmartMobileDeviceGroupFilterSelectors enumerates the fields the collection
// accepts a filter clause on. They keep their API-native spelling because they
// travel to Jamf Pro verbatim.
//
// siteId is offered because Jamf Pro accepts it, with one caveat worth knowing
// before it is used: only an administrator with full access filters on it, and a
// site-restricted administrator has the filter applied for them regardless of
// what they ask for.
var SmartMobileDeviceGroupFilterSelectors = []string{
	"groupId",
	"groupName",
	"siteId",
}

// SmartMobileDeviceGroupsDataSource reads the smart mobile device group
// collection.
type SmartMobileDeviceGroupsDataSource struct {
	client *pro.Client
	pd     *providerdata.Data
}

var (
	_ datasource.DataSource              = &SmartMobileDeviceGroupsDataSource{}
	_ datasource.DataSourceWithConfigure = &SmartMobileDeviceGroupsDataSource{}
)

// NewSmartMobileDeviceGroupsDataSource returns a new instance of the plural data
// source.
func NewSmartMobileDeviceGroupsDataSource() datasource.DataSource {
	return &SmartMobileDeviceGroupsDataSource{}
}

// Metadata sets the data source type name.
func (d *SmartMobileDeviceGroupsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_smart_mobile_device_groups"
}

// Schema returns the plural data source schema. Each result is a summary of one
// group: the collection read reports no criteria at all, so a group's criteria
// need a singular `jamfplatform_pro_smart_mobile_device_group` lookup.
func (d *SmartMobileDeviceGroupsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(progroups.DeviceTypeMobile, progroups.GroupKindSmart) +
			" Lists the smart mobile device groups in the tenant, narrowed by optional filter clauses. Each result is a summary and carries no criteria; read one group with `jamfplatform_pro_smart_mobile_device_group` for those." +
			pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"filter": filters.FilterAttribute(
				filters.SelectorDescription(SmartMobileDeviceGroupFilterSelectors),
				SmartMobileDeviceGroupFilterSelectors,
			),
			"smart_mobile_device_groups": schema.ListNestedAttribute{
				MarkdownDescription: "Groups matching the supplied filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{MarkdownDescription: "Identifier Jamf Pro assigns to the group.", Computed: true},
						"platform_id": schema.StringAttribute{MarkdownDescription: "Identifier for this group across Jamf Platform, which is how a blueprint or a compliance benchmark targets it.", Computed: true},
						"name":        schema.StringAttribute{MarkdownDescription: "Group display name.", Computed: true},
						"description": schema.StringAttribute{MarkdownDescription: "Free-text note stored with the group.", Computed: true},
						"site_id":     schema.StringAttribute{MarkdownDescription: "Jamf Pro site the group belongs to. `-1` means no site.", Computed: true},
						"member_count": schema.Int64Attribute{
							MarkdownDescription: "Mobile devices Jamf Pro currently counts in the group. It moves as inventory changes.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure wires the Jamf Pro client into the data source and applies the
// family's version refusal.
func (d *SmartMobileDeviceGroupsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_smart_mobile_device_groups")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_smart_mobile_device_groups")...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
	d.pd = pd
}

// Read fetches the collection and populates state.
//
// The platform identifiers for the whole page come from one bridge request
// rather than one per group, and a group the bridge cannot place reports a null
// `platform_id` rather than failing the read.
func (d *SmartMobileDeviceGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Check that the provider block is set up correctly.",
		)
		return
	}

	var data SmartMobileDeviceGroupsDataSourceModel
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

	filterExpression := filters.BuildRSQLExpression(data.Filters, filters.AllowList(SmartMobileDeviceGroupFilterSelectors))
	tflog.Debug(ctx, "smart mobile device groups filter expression", map[string]any{"filter": filterExpression})

	groups, err := d.client.ListSmartMobileDeviceGroupsV2(readCtx, nil, filterExpression)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro smart mobile device groups", helpers.APIErrorDetail(err))
		return
	}

	byPlatformID, bridgeDiags := progroups.PlatformIDsByJamfProID(readCtx, d.client, d.pd, progroups.DeviceTypeMobile, progroups.GroupKindSmart)
	resp.Diagnostics.Append(bridgeDiags...)

	data.Groups = smartMobileDeviceGroupsResults(groups, byPlatformID)
	data.ID = types.StringValue("smart_mobile_device_groups")

	tflog.Trace(ctx, "listed Jamf Pro smart mobile device groups data source", map[string]any{
		"filter": filterExpression,
		"count":  len(data.Groups),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
