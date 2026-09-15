// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// UserGroupDataSource implements the Terraform data source for Jamf Pro user
// groups. Lookup is by ID or by exact name — exactly one must be supplied.
type UserGroupDataSource struct {
	client   *proclassic.Client
	groupRef criteria.GroupResolver
}

var (
	_ datasource.DataSource                     = &UserGroupDataSource{}
	_ datasource.DataSourceWithConfigure        = &UserGroupDataSource{}
	_ datasource.DataSourceWithConfigValidators = &UserGroupDataSource{}
)

// NewUserGroupDataSource returns a new instance of UserGroupDataSource.
func NewUserGroupDataSource() datasource.DataSource {
	return &UserGroupDataSource{}
}

// Metadata sets the data source type name.
func (d *UserGroupDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_user_group"
}

// Schema returns the data source schema.
func (d *UserGroupDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro user group by ID or by exact name. Exactly one of `id` or `name` must be supplied. The Computed `users` block carries the full Jamf Pro-resolved user list, which is useful for inspecting smart-group membership." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "User group ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "User group display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"group_type":                  schema.StringAttribute{MarkdownDescription: "Group implementation type (`static` or `smart`).", Computed: true},
			"notify_on_membership_change": schema.BoolAttribute{MarkdownDescription: "Whether Jamf Pro emits a notification when group membership changes.", Computed: true},
			"site_id":                     schema.StringAttribute{MarkdownDescription: "Jamf Pro site ID scoping the user group. `-1` means no site (`NONE`).", Computed: true},
			"site_name":                   schema.StringAttribute{MarkdownDescription: "Jamf Pro site display name.", Computed: true},
			"member_count":                schema.Int64Attribute{MarkdownDescription: "Total members reported by Jamf Pro.", Computed: true},
			"criteria": schema.ListNestedAttribute{
				MarkdownDescription: "Smart-group criteria as defined in Jamf Pro. Empty for static groups.",
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
			"users": schema.ListNestedAttribute{
				MarkdownDescription: "Resolved user members reported by Jamf Pro. For smart groups, the list is computed from criteria; for static groups, it's the explicit member set.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":            schema.StringAttribute{Computed: true},
						"username":      schema.StringAttribute{Computed: true},
						"full_name":     schema.StringAttribute{Computed: true},
						"phone_number":  schema.StringAttribute{Computed: true},
						"email_address": schema.StringAttribute{Computed: true},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *UserGroupDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *UserGroupDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_user_group")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
	d.groupRef = criteria.NewProGroupResolver(client)
}

// Read fetches a user group by ID or by name.
func (d *UserGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data UserGroupDataSourceModel
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
		got *proclassic.UserGroup
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetUserGroupByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetUserGroupByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing user group selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro user group", helpers.APIErrorDetail(err))
		return
	}
	assignUserGroupDataSourceModel(&data, got)
	// No prior config in a data-source read → reverse-resolve any Jamf-group
	// "member of" criterion id back to the group name (11.29 read regression) so a
	// consumer (and `-generate-config-out`) sees the name, not the numeric id.
	data.Criteria = fromCriterionModels(criteria.ReadbackGroupRefCriteria(readCtx, d.groupRef, dsGroupObjectType, toCriterionModels(data.Criteria), nil))

	tflog.Trace(ctx, "read Jamf Pro user group data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
