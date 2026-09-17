// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"context"
	"time"

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

// defaultDataSourceReadTimeout is more generous than the resource's read budget
// because this read always makes the extra bridging request that resolves
// platform_id, where the resource makes it only on an import.
const defaultDataSourceReadTimeout = 90 * time.Second

// SmartComputerGroupDataSource implements the singular data source. Lookup is by
// identifier or by exact name.
type SmartComputerGroupDataSource struct {
	client *pro.Client
	pd     *providerdata.Data
}

var (
	_ datasource.DataSource                     = &SmartComputerGroupDataSource{}
	_ datasource.DataSourceWithConfigure        = &SmartComputerGroupDataSource{}
	_ datasource.DataSourceWithConfigValidators = &SmartComputerGroupDataSource{}
)

// NewSmartComputerGroupDataSource returns a new instance of SmartComputerGroupDataSource.
func NewSmartComputerGroupDataSource() datasource.DataSource {
	return &SmartComputerGroupDataSource{}
}

// Metadata sets the data source type name.
func (d *SmartComputerGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_smart_computer_group"
}

// Schema returns the singular data source schema.
func (d *SmartComputerGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(deviceType, groupKind) +
			"\n\nReads one smart computer group from Jamf Pro by identifier or by exact name. Supply exactly one of `id` or `name`. A name has to match a single group: Jamf Pro allows two groups of different kinds to share one, and the lookup reports an ambiguous name rather than picking." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro identifier of the group to read. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Exact group name to look up. Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"platform_id": schema.StringAttribute{
				MarkdownDescription: "Identifier for this group across Jamf Platform, for targeting it from a blueprint, a compliance benchmark, or any `jamfplatform_device_*` construct that asks for a group.",
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
			"criteria": schema.ListNestedAttribute{
				MarkdownDescription: "Criteria Jamf Pro evaluates to decide who is in the group, in evaluation order. A criterion naming a directory service group reports the stored reference value rather than the group name.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"priority":                schema.Int64Attribute{MarkdownDescription: "Evaluation position of the criterion, counting from zero.", Computed: true},
						"name":                    schema.StringAttribute{MarkdownDescription: "Inventory attribute the criterion tests.", Computed: true},
						"search_type":             schema.StringAttribute{MarkdownDescription: "Operator the criterion applies.", Computed: true},
						"value":                   schema.StringAttribute{MarkdownDescription: "Value the operator compares against.", Computed: true},
						"and_or":                  schema.StringAttribute{MarkdownDescription: "How this criterion joins the one before it. The first criterion's value is never used.", Computed: true},
						"has_opening_parenthesis": schema.BoolAttribute{MarkdownDescription: "Whether the criterion opens a parenthetical group.", Computed: true},
						"has_closing_parenthesis": schema.BoolAttribute{MarkdownDescription: "Whether the criterion closes a parenthetical group.", Computed: true},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id or name is supplied.
func (d *SmartComputerGroupDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source, then applies the
// family's version refusal.
func (d *SmartComputerGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, terraformName)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, terraformName)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
	d.pd = pd
}

// Read fetches one group by identifier or by name.
func (d *SmartComputerGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Check that the provider block is set up correctly.",
		)
		return
	}

	var data SmartComputerGroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultDataSourceReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	jamfProID := data.ID.ValueString()
	if !helpers.IsConfiguredValue(data.ID) || jamfProID == "" {
		if !helpers.IsConfiguredValue(data.Name) || data.Name.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing group selector",
				"Supply exactly one of `id` or `name` to read a "+groupLabel+".",
			)
			return
		}
		resolved, err := d.client.ResolveSmartComputerGroupV3IDByName(readCtx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to find a Jamf Pro "+groupLabel+" with this name", helpers.APIErrorDetail(err))
			return
		}
		jamfProID = resolved
	}

	got, err := d.client.GetSmartComputerGroupV3(readCtx, jamfProID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro "+groupLabel, helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(jamfProID)
	assignSmartComputerGroupDataSourceModel(&data, got)

	byJamfProID, bridgeDiags := progroups.PlatformIDsByJamfProID(readCtx, d.client, d.pd, deviceType, groupKind)
	resp.Diagnostics.Append(bridgeDiags...)
	data.PlatformID = progroups.PlatformIDValue(byJamfProID, jamfProID)

	tflog.Trace(ctx, "read Jamf Pro smart computer group data source", map[string]any{"id": jamfProID})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
