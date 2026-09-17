// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

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

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// SmartMobileDeviceGroupDataSource looks a group up by identifier or by exact
// name.
type SmartMobileDeviceGroupDataSource struct {
	client *pro.Client
	pd     *providerdata.Data
}

var (
	_ datasource.DataSource                     = &SmartMobileDeviceGroupDataSource{}
	_ datasource.DataSourceWithConfigure        = &SmartMobileDeviceGroupDataSource{}
	_ datasource.DataSourceWithConfigValidators = &SmartMobileDeviceGroupDataSource{}
)

// NewSmartMobileDeviceGroupDataSource returns a new instance of the data source.
func NewSmartMobileDeviceGroupDataSource() datasource.DataSource {
	return &SmartMobileDeviceGroupDataSource{}
}

// Metadata sets the data source type name.
func (d *SmartMobileDeviceGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_smart_mobile_device_group"
}

// Schema returns the data source schema. It is written out here rather than
// derived from the resource: the selectors differ, every other attribute is
// read-only, and the criterion attributes carry no validators or defaults on a
// read.
func (d *SmartMobileDeviceGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(progroups.DeviceTypeMobile, progroups.GroupKindSmart) +
			" Looks one group up by `id` or by exact `name`; supply exactly one. A name match is exact and case-sensitive, so two groups differing only in case resolve separately." +
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
				MarkdownDescription: "Free-text note stored with the group.",
				Computed:            true,
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site the group belongs to. `-1` means no site.",
				Computed:            true,
			},
			"criteria": schema.ListNestedAttribute{
				MarkdownDescription: "Criteria Jamf Pro evaluates to decide membership, in evaluation order.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"priority":                schema.Int64Attribute{MarkdownDescription: "The criterion's position in the evaluation order.", Computed: true},
						"name":                    schema.StringAttribute{MarkdownDescription: "Inventory attribute the criterion evaluates.", Computed: true},
						"search_type":             schema.StringAttribute{MarkdownDescription: "Comparison the criterion applies.", Computed: true},
						"value":                   schema.StringAttribute{MarkdownDescription: "Value the criterion compares against.", Computed: true},
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

// ConfigValidators enforces that exactly one selector is supplied.
func (d *SmartMobileDeviceGroupDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source and applies the
// family's version refusal.
func (d *SmartMobileDeviceGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_smart_mobile_device_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_smart_mobile_device_group")...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
	d.pd = pd
}

// Read fetches a group by identifier or by name.
//
// A name lookup resolves to the identifier first, so both paths share one read
// and report the same shape. platform_id is then resolved through the same
// bridge every other read path uses; it is best-effort, so a group it cannot
// place reports a null identifier rather than failing the lookup.
func (d *SmartMobileDeviceGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Check that the provider block is set up correctly.",
		)
		return
	}

	var data SmartMobileDeviceGroupDataSourceModel
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

	id := data.ID.ValueString()
	byName := !helpers.IsConfiguredValue(data.ID) || id == ""
	if byName {
		if !helpers.IsConfiguredValue(data.Name) || data.Name.ValueString() == "" {
			resp.Diagnostics.AddError("Missing group selector", "Supply exactly one of `id` or `name`.")
			return
		}
		resolved, err := d.client.ResolveSmartMobileDeviceGroupV2IDByName(readCtx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to find a Jamf Pro "+writeLabel()+" with that name", helpers.APIErrorDetail(err))
			return
		}
		id = resolved
	}

	got, err := d.client.GetSmartMobileDeviceGroupV2(readCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro "+writeLabel(), helpers.APIErrorDetail(err))
		return
	}
	data.ID = types.StringValue(id)
	assignSmartMobileDeviceGroupDataSourceModel(&data, got)
	data.Criteria = criteria.ReadbackDSGroupCriteria(readCtx, d.client, dsGroupObjectType, data.Criteria, nil)

	platformID, platformDiags := resolvePlatformIDForRead(readCtx, d.client, d.pd, data.ID, types.StringNull())
	resp.Diagnostics.Append(platformDiags...)
	data.PlatformID = platformID

	tflog.Trace(ctx, "read Jamf Pro smart mobile device group data source", map[string]any{
		"id":   data.ID.ValueString(),
		"name": data.Name.ValueString(),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
