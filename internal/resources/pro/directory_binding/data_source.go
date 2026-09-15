// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package directory_binding

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// lookupByName resolves a directory binding by display name via
// ListDirectoryBindings + client-side ID match, then GetDirectoryBindingByID.
//
// The classic /directorybindings/name/{name} endpoint that
// `GetDirectoryBindingByName` calls is server-side broken: it returns HTTP
// 500 Internal Server Error for every name lookup, even when the named
// binding exists and is reachable by ID. Reproducible against the EU
// tenant 2026-05-23. The provider routes around the bug by hitting the
// list endpoint (which works) and following up with GetByID. Once Jamf
// fixes the name endpoint upstream, this helper can be deleted and the
// data source can switch back to GetDirectoryBindingByName.
//
// Behaviour mirrors the broken endpoint as far as TF callers can tell:
// returns *DirectoryBinding on exact-name match, error otherwise.
func lookupByName(ctx context.Context, c *proclassic.Client, name string) (*proclassic.DirectoryBinding, error) {
	list, err := c.ListDirectoryBindings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list directory bindings while resolving %q: %w", name, err)
	}
	if list == nil {
		return nil, fmt.Errorf("no directory binding named %q (empty list response)", name)
	}
	for _, item := range list.DirectoryBindings {
		if item.Name == nil || *item.Name != name {
			continue
		}
		id := helpers.StringValueFromIntPtr(item.ID).ValueString()
		if id == "" {
			return nil, fmt.Errorf("list returned matching directory binding name %q with no ID", name)
		}
		return c.GetDirectoryBindingByID(ctx, id)
	}
	return nil, fmt.Errorf("no directory binding named %q", name)
}

// DirectoryBindingDataSource implements the Terraform data source for Jamf
// Pro directory bindings. The singular data source supports lookup by ID OR
// by name — exactly one of the two must be supplied.
type DirectoryBindingDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &DirectoryBindingDataSource{}
	_ datasource.DataSourceWithConfigure        = &DirectoryBindingDataSource{}
	_ datasource.DataSourceWithConfigValidators = &DirectoryBindingDataSource{}
)

// NewDirectoryBindingDataSource returns a new instance of
// DirectoryBindingDataSource.
func NewDirectoryBindingDataSource() datasource.DataSource {
	return &DirectoryBindingDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *DirectoryBindingDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_directory_binding"
}

// Schema returns the data source schema.
func (d *DirectoryBindingDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro directory binding by ID or by exact name. Exactly one of `id` or `name` must be supplied. The data source never returns the plaintext bind password, because Jamf Pro does not return it on read. Use the resource to manage the password." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Directory binding ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Directory binding display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"priority":    schema.Int64Attribute{MarkdownDescription: "Binding priority.", Computed: true},
			"type":        schema.StringAttribute{MarkdownDescription: "Directory service type (`Active Directory`, `Open Directory`, `PowerBroker Identity Services`, `ADmitMac`, `Centrify`).", Computed: true},
			"domain":      schema.StringAttribute{MarkdownDescription: "Directory domain.", Computed: true},
			"username":    schema.StringAttribute{MarkdownDescription: "Bind username.", Computed: true},
			"computer_ou": schema.StringAttribute{MarkdownDescription: "Computer object's organisational unit.", Computed: true},

			"active_directory": schema.SingleNestedAttribute{
				MarkdownDescription: "Active Directory–specific configuration. Populated only when `type = \"Active Directory\"`.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"forest":                     schema.StringAttribute{Computed: true, MarkdownDescription: "Active Directory forest."},
					"create_mobile_account":      schema.BoolAttribute{Computed: true, MarkdownDescription: "**\"Create Mobile Account\"** in the Jamf Pro admin UI."},
					"require_confirmation":       schema.BoolAttribute{Computed: true, MarkdownDescription: "**\"Require confirmation before creating a mobile account\"** in the Jamf Pro admin UI."},
					"force_local_home_directory": schema.BoolAttribute{Computed: true, MarkdownDescription: "**\"Force local home directory on startup disk\"** in the Jamf Pro admin UI."},
					"use_unc_path":               schema.BoolAttribute{Computed: true, MarkdownDescription: "Use a UNC path for the network home location."},
					"network_protocol":           schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Network Protocol\"** in the Jamf Pro admin UI."},
					"default_shell":              schema.StringAttribute{Computed: true, MarkdownDescription: "Default login shell."},
					"uid_attribute_mapping":      schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Map UID to attribute\"** in the Jamf Pro admin UI."},
					"user_gid_attribute_mapping": schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Map User GID to attribute\"** in the Jamf Pro admin UI."},
					"gid_attribute_mapping":      schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Map Group GID to attribute\"** in the Jamf Pro admin UI."},
					"multiple_domains":           schema.BoolAttribute{Computed: true, MarkdownDescription: "Allow multiple AD domains."},
					"preferred_domain":           schema.StringAttribute{Computed: true, MarkdownDescription: "Preferred AD domain controller hostname."},
					"admin_groups":               schema.StringAttribute{Computed: true, MarkdownDescription: "Comma-separated AD groups granted local admin."},
				},
			},
			"open_directory": schema.SingleNestedAttribute{
				MarkdownDescription: "Open Directory–specific configuration. Populated only when `type = \"Open Directory\"`.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"encrypt_using_ssl":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Encrypt the LDAP connection."},
					"perform_secure_bind":    schema.BoolAttribute{Computed: true, MarkdownDescription: "Use a secure (authenticated) bind operation."},
					"use_for_authentication": schema.BoolAttribute{Computed: true, MarkdownDescription: "Use directory for user authentication."},
					"use_for_contacts":       schema.BoolAttribute{Computed: true, MarkdownDescription: "Use directory as a contacts source."},
				},
			},
			"admitmac": schema.SingleNestedAttribute{
				MarkdownDescription: "ADmitMac–specific configuration. Populated only when `type = \"ADmitMac\"`.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"require_confirmation":       schema.BoolAttribute{Computed: true, MarkdownDescription: "**\"Require confirmation\"** in the Jamf Pro admin UI."},
					"home_location":              schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Home Location\"** in the Jamf Pro admin UI."},
					"network_protocol":           schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Network Protocol\"** in the Jamf Pro admin UI."},
					"default_shell":              schema.StringAttribute{Computed: true, MarkdownDescription: "Default login shell."},
					"mount_network_home":         schema.BoolAttribute{Computed: true, MarkdownDescription: "**\"Mount network home as sharepoint\"** in the Jamf Pro admin UI."},
					"place_home_folders":         schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Place home folders in\"** in the Jamf Pro admin UI."},
					"uid_attribute_mapping":      schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Map UID to attribute\"** in the Jamf Pro admin UI."},
					"user_gid_attribute_mapping": schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Map User GID to attribute\"** in the Jamf Pro admin UI."},
					"gid_attribute_mapping":      schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Map Group GID to attribute\"** in the Jamf Pro admin UI."},
					"admin_group":                schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Allow administration by\"** in the Jamf Pro admin UI."},
					"cached_credentials":         schema.Int64Attribute{Computed: true, MarkdownDescription: "**\"Cached credentials\"** in the Jamf Pro admin UI."},
					"add_user_to_local":          schema.BoolAttribute{Computed: true, MarkdownDescription: "**\"Add user to local administrators group\"** in the Jamf Pro admin UI."},
					"users_ou":                   schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Users OU\"** in the Jamf Pro admin UI."},
					"groups_ou":                  schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Groups OU\"** in the Jamf Pro admin UI."},
					"printers_ou":                schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Printers OU\"** in the Jamf Pro admin UI."},
					"shared_folders_ou":          schema.StringAttribute{Computed: true, MarkdownDescription: "**\"Shared Folders OU\"** in the Jamf Pro admin UI."},
				},
			},
			"centrify": schema.SingleNestedAttribute{
				MarkdownDescription: "Centrify–specific configuration. Populated only when `type = \"Centrify\"`.",
				Computed:            true,
				Attributes: map[string]schema.Attribute{
					"workstation_mode":        schema.BoolAttribute{Computed: true, MarkdownDescription: "Bind in Workstation mode."},
					"overwrite_existing":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Overwrite an existing Centrify configuration."},
					"update_pam":              schema.BoolAttribute{Computed: true, MarkdownDescription: "Update PAM configuration."},
					"zone":                    schema.StringAttribute{Computed: true, MarkdownDescription: "Centrify zone name."},
					"preferred_domain_server": schema.StringAttribute{Computed: true, MarkdownDescription: "Preferred Centrify domain server hostname."},
				},
			},

			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *DirectoryBindingDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source via the
// shared providerdata.ConfigureProClassic helper.
func (d *DirectoryBindingDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_directory_binding")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a directory binding by ID or by name and populates Terraform
// state.
func (d *DirectoryBindingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data DirectoryBindingDataSourceModel
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
		got *proclassic.DirectoryBinding
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetDirectoryBindingByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = lookupByName(readCtx, d.client, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing directory binding selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro directory binding", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignDirectoryBindingDataSourceModel(&data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro directory binding data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
