// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package user implements the jamfplatform_pro_user singular data source.
package user

import (
	"context"
	"fmt"
	"strings"
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
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

const defaultReadTimeout = 30 * time.Second

// minJamfProVersion is the minimum Jamf Pro tenant version required by this data
// source. Empty: no per-resource floor; the users endpoint has been stable since
// well before the provider's overall floor. Provider-level advisory warning still
// applies via providerdata.ConfigurePro.
const minJamfProVersion = ""

var (
	_ datasource.DataSource                     = &UserDataSource{}
	_ datasource.DataSourceWithConfigure        = &UserDataSource{}
	_ datasource.DataSourceWithConfigValidators = &UserDataSource{}
)

// NewUserDataSource returns a new instance of UserDataSource.
func NewUserDataSource() datasource.DataSource {
	return &UserDataSource{}
}

// Metadata sets the data source type name.
func (d *UserDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_user"
}

// Schema returns the data source schema.
func (d *UserDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro **inventory user** record (a person in *Users → User Inventory*) by ID or by exact username. Exactly one of `id` or `username` must be supplied. This is the user record that devices and groups are scoped against. It is **not** a Jamf Pro admin account; see `jamfplatform_pro_account` for those." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro user ID. Mutually exclusive with `username`.",
				Optional:            true,
				Computed:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "**\"Username\"** in the Jamf Pro admin UI (exact match, case-insensitive). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			// wire: realname
			"full_name": schema.StringAttribute{
				MarkdownDescription: "**\"Full Name\"** in the Jamf Pro admin UI.",
				Computed:            true,
			},
			// wire: email
			"email_address": schema.StringAttribute{
				MarkdownDescription: "**\"Email Address\"** in the Jamf Pro admin UI.",
				Computed:            true,
			},
			// wire: phone
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
				MarkdownDescription: "Whether a custom photo URL is used for this user instead of the Gravatar derived from the email address.",
				Computed:            true,
			},
			"custom_photo_url": schema.StringAttribute{
				MarkdownDescription: "Custom photo URL for this user, when `enable_custom_photo_url` is set.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / username is supplied.
func (d *UserDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("username"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *UserDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_user")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a user by ID or by exact username and populates Terraform state.
func (d *UserDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data UserDataSourceModel
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
		got *pro.User
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		// platform=false: query against the tenant-internal user IDs.
		got, err = d.client.GetUserV1(readCtx, data.ID.ValueString(), false)
		if err != nil {
			resp.Diagnostics.AddError("Unable to find Jamf Pro user", fmt.Sprintf("Failed to get user %q: %s", data.ID.ValueString(), helpers.APIErrorDetail(err)))
			return
		}
	case !data.Username.IsNull() && data.Username.ValueString() != "":
		got, err = d.lookupByUsername(readCtx, data.Username.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to find Jamf Pro user", helpers.APIErrorDetail(err))
			return
		}
	default:
		resp.Diagnostics.AddError("Missing user selector", "Exactly one of id or username must be supplied.")
		return
	}

	assignUserDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro user data source", map[string]any{"id": data.ID.ValueString(), "username": data.Username.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// lookupByUsername resolves a single user by exact (case-insensitive) username.
// The server-side RSQL `==` is itself case-insensitive and treats `*` as a
// wildcard, so the returned page is re-filtered client-side to an exact match —
// this rejects a literal `*` in the input and surfaces an unambiguous error when
// zero or multiple records match.
func (d *UserDataSource) lookupByUsername(ctx context.Context, username string) (*pro.User, error) {
	filter := fmt.Sprintf("username==%q", username)
	users, err := d.client.ListUsersV1(ctx, nil, filter, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list users with filter %s: %w", filter, err)
	}

	matches := make([]pro.User, 0, 1)
	for _, u := range users {
		if strings.EqualFold(u.Username, username) {
			matches = append(matches, u)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no Jamf Pro user found with username %q", username)
	case 1:
		match := matches[0]
		return &match, nil
	default:
		return nil, fmt.Errorf("found %d Jamf Pro users matching username %q; expected exactly one", len(matches), username)
	}
}

// assignUserDataSourceModel maps an SDK user record onto the Terraform model.
func assignUserDataSourceModel(data *UserDataSourceModel, u *pro.User) {
	data.ID = types.StringValue(u.ID)
	data.Username = types.StringValue(u.Username)
	data.FullName = helpers.StringValueOrNull(u.Realname)
	data.EmailAddress = helpers.StringValueOrNull(u.Email)
	data.PhoneNumber = helpers.StringPointerValueOrNull(u.Phone)
	data.Position = helpers.StringPointerValueOrNull(u.Position)
	data.ManagedAppleID = helpers.StringValueOrNull(u.ManagedAppleID)
	data.EnableCustomPhotoURL = types.BoolValue(u.EnableCustomPhotoURL)
	data.CustomPhotoURL = helpers.StringValueOrNull(u.CustomPhotoURL)
}
