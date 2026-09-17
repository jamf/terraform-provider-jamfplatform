// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// StaticComputerGroupDataSource looks one group up by identifier or by name.
//
// It is the only read path besides the resource that reports the member
// identifiers, and for the same reason: they come from a second request against
// a different API surface. The plural data source and the list resource report
// a count instead.
type StaticComputerGroupDataSource struct {
	client        *pro.Client
	classicClient *proclassic.Client
	pd            *providerdata.Data
}

var (
	_ datasource.DataSource                     = &StaticComputerGroupDataSource{}
	_ datasource.DataSourceWithConfigure        = &StaticComputerGroupDataSource{}
	_ datasource.DataSourceWithConfigValidators = &StaticComputerGroupDataSource{}
)

// NewStaticComputerGroupDataSource returns a new StaticComputerGroupDataSource.
func NewStaticComputerGroupDataSource() datasource.DataSource {
	return &StaticComputerGroupDataSource{}
}

// Metadata sets the data source type name.
func (d *StaticComputerGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_static_computer_group"
}

// Schema returns the data source schema.
func (d *StaticComputerGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: progroups.DeviceGroupAlternative(progroups.DeviceTypeComputer, progroups.GroupKindStatic) +
			"\n\nLooks one group up by `id` or by exact `name`, and reports the computers in it. Supply exactly one of the two." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro identifier for the group. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Group name, matched exactly. Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"platform_id": schema.StringAttribute{
				MarkdownDescription: "Identifier for this group across Jamf Platform, for targeting it from a blueprint, a compliance benchmark, or any `jamfplatform_device_*` construct that asks for a group.",
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
			"assigned_computer_ids": schema.SetAttribute{
				MarkdownDescription: "Jamf Pro identifiers of the computers in the group.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators requires exactly one of id or name.
func (d *StaticComputerGroupDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires both clients and the shared provider data onto the data source.
func (d *StaticComputerGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_static_computer_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || client == nil {
		return
	}
	classicClient, classicDiags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_static_computer_group")
	resp.Diagnostics.Append(classicDiags...)
	if resp.Diagnostics.HasError() || classicClient == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerdata.Data)
	if !ok {
		return
	}
	resp.Diagnostics.Append(progroups.RequireSupportedJamfProVersion(ctx, pd, "jamfplatform_pro_static_computer_group")...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
	d.classicClient = classicClient
	d.pd = pd
}

// Read fetches the group, its members and its platform identifier.
func (d *StaticComputerGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil || d.classicClient == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Check that the provider block is set up correctly.",
		)
		return
	}

	var data StaticComputerGroupDataSourceModel
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

	var lookupID string
	switch {
	case helpers.IsConfiguredValue(data.ID) && data.ID.ValueString() != "":
		lookupID = data.ID.ValueString()
	case helpers.IsConfiguredValue(data.Name) && data.Name.ValueString() != "":
		resolved, err := d.client.ResolveStaticComputerGroupV3IDByName(readCtx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to find a Jamf Pro "+groupLabel+" with this name", helpers.APIErrorDetail(err))
			return
		}
		lookupID = resolved
	default:
		resp.Diagnostics.AddError("Missing group selector", "Supply exactly one of `id` or `name`.")
		return
	}

	got, err := d.client.GetStaticComputerGroupV3(readCtx, lookupID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the Jamf Pro "+groupLabel, helpers.APIErrorDetail(err))
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Unable to read the Jamf Pro "+groupLabel, "Jamf Pro reported no group for identifier "+lookupID+".")
		return
	}
	data.ID = types.StringValue(lookupID)
	assignStaticComputerGroupDataSourceState(&data, got)

	group, membershipErr := d.classicClient.GetComputerGroupByID(readCtx, data.ID.ValueString())
	if membershipErr != nil {
		resp.Diagnostics.AddError(
			"Unable to read the membership of this Jamf Pro "+groupLabel,
			"Jamf Pro reports a static group's members separately from the group itself, and that read failed: "+helpers.APIErrorDetail(membershipErr),
		)
		return
	}
	members, setDiags := membershipSetValue(readCtx, flattenStaticComputerGroupMembership(group))
	resp.Diagnostics.Append(setDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.AssignedComputerIDs = members

	byJamfProID, bridgeDiags := progroups.PlatformIDsByJamfProID(readCtx, d.client, d.pd, progroups.DeviceTypeComputer, progroups.GroupKindStatic)
	resp.Diagnostics.Append(bridgeDiags...)
	data.PlatformID = progroups.PlatformIDValue(byJamfProID, data.ID.ValueString())

	tflog.Trace(ctx, "read Jamf Pro static computer group data source", map[string]any{
		"id":   data.ID.ValueString(),
		"name": data.Name.ValueString(),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
