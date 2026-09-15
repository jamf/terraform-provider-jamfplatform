// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_user_search

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// AdvancedUserSearchDataSource implements the Terraform data source for Jamf Pro
// advanced user searches. Lookup is by ID or by exact name — exactly one must be
// supplied.
type AdvancedUserSearchDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &AdvancedUserSearchDataSource{}
	_ datasource.DataSourceWithConfigure        = &AdvancedUserSearchDataSource{}
	_ datasource.DataSourceWithConfigValidators = &AdvancedUserSearchDataSource{}
)

// NewAdvancedUserSearchDataSource returns a new instance of the data source.
func NewAdvancedUserSearchDataSource() datasource.DataSource {
	return &AdvancedUserSearchDataSource{}
}

// Metadata sets the data source type name.
func (d *AdvancedUserSearchDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_advanced_user_search"
}

// Schema returns the data source schema.
func (d *AdvancedUserSearchDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro advanced user search by ID or by exact name. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Advanced user search ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Advanced user search display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"site_id":   schema.StringAttribute{MarkdownDescription: "Jamf Pro site ID scoping the search. `-1` means no site (`NONE`).", Computed: true},
			"site_name": schema.StringAttribute{MarkdownDescription: "Jamf Pro site display name.", Computed: true},
			"criteria": schema.ListNestedAttribute{
				MarkdownDescription: "Ordered criteria evaluated by Jamf Pro to populate the search.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"priority":                schema.Int64Attribute{Computed: true},
						"name":                    schema.StringAttribute{Computed: true},
						"search_type":             schema.StringAttribute{Computed: true},
						"value":                   schema.StringAttribute{Computed: true},
						"and_or":                  schema.StringAttribute{Computed: true},
						"has_opening_parenthesis": schema.BoolAttribute{Computed: true},
						"has_closing_parenthesis": schema.BoolAttribute{Computed: true},
					},
				},
			},
			"display_fields": schema.SetAttribute{
				MarkdownDescription: "Column names shown in the search results.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *AdvancedUserSearchDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *AdvancedUserSearchDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_advanced_user_search")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an advanced user search by ID or by name.
func (d *AdvancedUserSearchDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AdvancedUserSearchDataSourceModel
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

	var (
		got *proclassic.AdvancedUserSearch
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetAdvancedUserSearchByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetAdvancedUserSearchByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing advanced user search selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro advanced user search", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignAdvancedUserSearchDataSourceModel(readCtx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro advanced user search data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
