// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_group

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultPluralReadTimeout = 90 * time.Second

// UserGroupsDataSource implements the Terraform data source for Jamf Pro user
// group searches.
type UserGroupsDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource              = &UserGroupsDataSource{}
	_ datasource.DataSourceWithConfigure = &UserGroupsDataSource{}
)

// NewUserGroupsDataSource returns a new instance of UserGroupsDataSource.
func NewUserGroupsDataSource() datasource.DataSource {
	return &UserGroupsDataSource{}
}

// Metadata sets the data source type name.
func (d *UserGroupsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_user_groups"
}

// Schema returns the plural data source schema. The nested object reflects
// exactly what the classic /usergroups list endpoint returns — id, name,
// is_smart (surfaced as group_type), and is_notify_on_change (surfaced as
// notify_on_membership_change). Per-item criteria, users, and site require
// a singular `jamfplatform_pro_user_group` data source lookup.
func (d *UserGroupsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "List Jamf Pro user groups. Supply an optional case-insensitive `name_substring` filter applied client-side after the full list is fetched. Per-item criteria, users, and site require a singular `jamfplatform_pro_user_group` lookup." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"filter":   filters.ClassicFilterAttribute(),
			"user_groups": schema.ListNestedAttribute{
				MarkdownDescription: "User groups matching the supplied filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                          schema.StringAttribute{MarkdownDescription: "User group ID assigned by Jamf Pro.", Computed: true},
						"name":                        schema.StringAttribute{MarkdownDescription: "User group display name.", Computed: true},
						"group_type":                  schema.StringAttribute{MarkdownDescription: "Group implementation type (`static` or `smart`).", Computed: true},
						"notify_on_membership_change": schema.BoolAttribute{MarkdownDescription: "Whether Jamf Pro emits a notification when group membership changes.", Computed: true},
					},
				},
			},
		},
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *UserGroupsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_user_groups")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches user groups and populates state. Applies the optional
// client-side substring filter after the full list is retrieved.
func (d *UserGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data UserGroupsDataSourceModel
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

	listResp, err := d.client.ListUserGroups(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro user groups", helpers.APIErrorDetail(err))
		return
	}

	items := []proclassic.UserGroupsItemUserGroup{}
	if listResp != nil {
		items = listResp.UserGroups
	}

	filter := filters.ClassicFilterModel{}
	if data.Filter != nil {
		filter = *data.Filter
	}
	items = filters.ApplyClassicFilter(items, filter, func(u proclassic.UserGroupsItemUserGroup) string {
		if u.Name == nil {
			return ""
		}
		return *u.Name
	})

	results := make([]UserGroupsDataSourceResultModel, 0, len(items))
	for _, u := range items {
		results = append(results, UserGroupsDataSourceResultModel{
			ID:                       helpers.StringValueFromIntPtr(u.ID),
			Name:                     helpers.StringPointerValueOrNull(u.Name),
			GroupType:                groupTypeFromIsSmart(u.IsSmart),
			NotifyOnMembershipChange: helpers.BoolPointerValueOrNull(u.IsNotifyOnChange),
		})
	}

	data.UserGroups = results
	data.ID = types.StringValue("user_groups")

	tflog.Trace(ctx, "listed Jamf Pro user groups data source", map[string]any{
		"name_substring": filter.NameSubstring.ValueString(),
		"count":          len(results),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
