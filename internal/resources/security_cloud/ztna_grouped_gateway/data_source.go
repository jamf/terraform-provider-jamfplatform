// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// GroupedGatewayDataSource implements the Terraform data source for a single Jamf
// Security Cloud ZTNA grouped gateway.
type GroupedGatewayDataSource struct {
	client *securitycloud.Client
}

var (
	_ datasource.DataSource                     = &GroupedGatewayDataSource{}
	_ datasource.DataSourceWithConfigValidators = &GroupedGatewayDataSource{}
)

// NewGroupedGatewayDataSource returns a new instance of GroupedGatewayDataSource.
func NewGroupedGatewayDataSource() datasource.DataSource {
	return &GroupedGatewayDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *GroupedGatewayDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_cloud_ztna_grouped_gateway"
}

// Schema returns the data source schema.
func (d *GroupedGatewayDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Security Cloud ZTNA grouped gateway by ID or by name. Use it to " +
			"resolve the ID a custom DNS zone name server needs." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Grouped gateway ID to look up. Exactly one of `id` or `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Grouped gateway name to look up. Exactly one of `id` or `name` must be set.",
				Optional:            true,
				Computed:            true,
			},
			"gateway_ids": schema.ListAttribute{
				MarkdownDescription: "IDs of the member gateways, in priority order.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"routing_strategy": schema.StringAttribute{
				MarkdownDescription: "Which member a device uses: `Nearest`, `Random` or `First available`.",
				Computed:            true,
			},
			"required_gateway_stability": schema.StringAttribute{
				MarkdownDescription: "How long a recovered member must stay healthy before traffic returns to it.",
				Computed:            true,
			},
			"tenant_ids": schema.ListAttribute{
				MarkdownDescription: "IDs of the tenants granted access to this grouped gateway.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "When the grouped gateway was created.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id or name is supplied.
func (d *GroupedGatewayDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Security Cloud client into the data source via the
// shared providerdata.ConfigureSecurityCloud helper.
func (d *GroupedGatewayDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureSecurityCloud(ctx, req.ProviderData, "jamfplatform_security_cloud_ztna_grouped_gateway")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a grouped gateway by ID or name and populates Terraform state.
func (d *GroupedGatewayDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data GroupedGatewayDataSourceModel
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

	if !data.ID.IsNull() && data.ID.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("id"),
			"ZTNA grouped gateway ID is empty",
			"`id` is set to an empty string, which ExactlyOneOf still counts as configured, so `name` cannot "+
				"be used instead. This usually means a variable or a reference resolved to \"\" — set `id` to "+
				"an ID, or remove it and set `name`.",
		)
		return
	}

	var group *securitycloud.GroupedGateway
	var err error
	byName := data.ID.IsNull()
	if byName {
		group, err = d.client.ResolveZtnaGroupedGatewayV1ByName(readCtx, data.Name.ValueString())
	} else {
		group, err = d.client.GetZtnaGroupedGatewayV1(readCtx, data.ID.ValueString())
	}
	if err != nil {
		if ambiguous, ok := errors.AsType[*jamfplatform.AmbiguousMatchError](err); ok {
			resp.Diagnostics.AddError(
				"Multiple Jamf Security Cloud ZTNA grouped gateways share that name",
				"Jamf Security Cloud does not require grouped gateway names to be unique, and more than one is "+
					"named "+data.Name.ValueString()+". Look it up by `id` instead. Matching IDs: "+
					strings.Join(ambiguous.Matches, ", "),
			)
			return
		}
		if byName && helpers.IsNotFoundError(err) {
			resp.Diagnostics.AddAttributeError(
				path.Root("name"),
				"Unable to find Jamf Security Cloud ZTNA grouped gateway",
				"No ZTNA grouped gateway on this tenant is named \""+data.Name.ValueString()+"\". Names are matched "+
					"exactly, so one differing only in capitalisation or surrounding whitespace will not be "+
					"found. Use the `jamfplatform_security_cloud_ztna_grouped_gateways` data source to list what exists.",
			)
			return
		}
		resp.Diagnostics.AddError("Unable to find Jamf Security Cloud ZTNA grouped gateway", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignGroupedGatewayDataSourceModel(ctx, &data, group)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Security Cloud ZTNA grouped gateway data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
