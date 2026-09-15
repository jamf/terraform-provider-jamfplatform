// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user

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
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultPluralReadTimeout = 90 * time.Second

// UserFilterSelectors enumerates the RSQL selectors accepted by the users
// endpoint. Per STYLE_GUIDE §schema rules, RSQL selectors retain their
// API-native (wire) spelling — `realname`/`email`/`phone` here map to the
// UI-aligned output attributes `full_name`/`email_address`/`phone_number`.
var UserFilterSelectors = []string{
	"id",
	"username",
	"realname",
	"email",
	"phone",
	"position",
	"managedAppleId",
}

// UsersDataSource implements the Terraform data source for Jamf Pro inventory
// user searches.
type UsersDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &UsersDataSource{}

// NewUsersDataSource returns a new instance of UsersDataSource.
func NewUsersDataSource() datasource.DataSource {
	return &UsersDataSource{}
}

// Metadata sets the data source type name.
func (d *UsersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_users"
}

// Schema returns the plural data source schema.
func (d *UsersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Search Jamf Pro **inventory users** (people in *Users → User Inventory*) using optional RSQL filters. Filter selectors use the field names Jamf Pro itself uses: `id`, `username`, `realname`, `email`, `phone`, `position`, and `managedAppleId`. Three of those are spelled differently in the output attributes: `realname` is `full_name`, `email` is `email_address`, and `phone` is `phone_number`. These are inventory user records, **not** Jamf Pro admin accounts." + pluralDataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal identifier for this data source read.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
			"filter": filters.FilterAttribute(
				filters.SelectorDescription(UserFilterSelectors),
				UserFilterSelectors,
			),
			"users": schema.ListNestedAttribute{
				MarkdownDescription: "Inventory users matching the supplied filter.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Jamf Pro user ID.",
							Computed:            true,
						},
						"username": schema.StringAttribute{
							MarkdownDescription: "**\"Username\"** in the Jamf Pro admin UI.",
							Computed:            true,
						},
						"full_name": schema.StringAttribute{
							MarkdownDescription: "**\"Full Name\"** in the Jamf Pro admin UI.",
							Computed:            true,
						},
						"email_address": schema.StringAttribute{
							MarkdownDescription: "**\"Email Address\"** in the Jamf Pro admin UI.",
							Computed:            true,
						},
						"phone_number": schema.StringAttribute{
							MarkdownDescription: "**\"Phone Number\"** in the Jamf Pro admin UI.",
							Computed:            true,
						},
						"position": schema.StringAttribute{
							MarkdownDescription: "**\"Position\"** in the Jamf Pro admin UI.",
							Computed:            true,
						},
						"managed_apple_id": schema.StringAttribute{
							MarkdownDescription: "**\"Managed Apple ID\"** in the Jamf Pro admin UI.",
							Computed:            true,
						},
						"enable_custom_photo_url": schema.BoolAttribute{
							MarkdownDescription: "Whether a custom photo URL is used for this user.",
							Computed:            true,
						},
						"custom_photo_url": schema.StringAttribute{
							MarkdownDescription: "Custom photo URL for this user, when enabled.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *UsersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_users")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches users matching the supplied filters and populates state.
func (d *UsersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data UsersDataSourceModel
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

	filterExpression := filters.BuildRSQLExpression(data.Filters, filters.AllowList(UserFilterSelectors))
	tflog.Debug(ctx, "users filter expression", map[string]any{"filter": filterExpression})

	// platform=false: query against the tenant-internal user IDs.
	got, err := d.client.ListUsersV1(readCtx, nil, filterExpression, false)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Pro users", helpers.APIErrorDetail(err))
		return
	}

	results := make([]UsersDataSourceResultModel, 0, len(got))
	for _, u := range got {
		results = append(results, UsersDataSourceResultModel{
			ID:                   types.StringValue(u.ID),
			Username:             types.StringValue(u.Username),
			FullName:             helpers.StringValueOrNull(u.Realname),
			EmailAddress:         helpers.StringValueOrNull(u.Email),
			PhoneNumber:          helpers.StringPointerValueOrNull(u.Phone),
			Position:             helpers.StringPointerValueOrNull(u.Position),
			ManagedAppleID:       helpers.StringValueOrNull(u.ManagedAppleID),
			EnableCustomPhotoURL: types.BoolValue(u.EnableCustomPhotoURL),
			CustomPhotoURL:       helpers.StringValueOrNull(u.CustomPhotoURL),
		})
	}

	data.Users = results
	data.ID = types.StringValue("users")

	tflog.Trace(ctx, "listed Jamf Pro users data source", map[string]any{
		"filter": filterExpression,
		"count":  len(results),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
