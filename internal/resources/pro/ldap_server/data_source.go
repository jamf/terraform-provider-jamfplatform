// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ldap_server

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// dsStr / dsBool / dsInt are Computed-only data-source attribute shorthands.
func dsStr(desc string) dsschema.StringAttribute {
	return dsschema.StringAttribute{Computed: true, MarkdownDescription: desc}
}
func dsBool(desc string) dsschema.BoolAttribute {
	return dsschema.BoolAttribute{Computed: true, MarkdownDescription: desc}
}
func dsInt(desc string) dsschema.Int64Attribute {
	return dsschema.Int64Attribute{Computed: true, MarkdownDescription: desc}
}

// LdapServerDataSource implements the Terraform data source for Jamf Pro LDAP
// servers. Lookup by ID OR by exact name — exactly one of the two.
type LdapServerDataSource struct {
	client *proclassic.Client
}

var (
	_ datasource.DataSource                     = &LdapServerDataSource{}
	_ datasource.DataSourceWithConfigure        = &LdapServerDataSource{}
	_ datasource.DataSourceWithConfigValidators = &LdapServerDataSource{}
)

// NewLdapServerDataSource returns a new instance of LdapServerDataSource.
func NewLdapServerDataSource() datasource.DataSource {
	return &LdapServerDataSource{}
}

// Metadata sets the data source type name.
func (d *LdapServerDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_ldap_server"
}

// Schema returns the data source schema.
func (d *LdapServerDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Look up a Jamf Pro on-premises LDAP server by ID or by exact display name. Exactly one of `id` or `name` must be supplied. The data source never returns the bind password; Jamf Pro does not return it on read." + dataSourcePrivileges,
		Attributes: map[string]dsschema.Attribute{
			"id": dsschema.StringAttribute{
				MarkdownDescription: "LDAP server ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": dsschema.StringAttribute{
				MarkdownDescription: "LDAP server display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"connection_settings": dsschema.SingleNestedAttribute{
				MarkdownDescription: "Server connection settings.",
				Computed:            true,
				Attributes: map[string]dsschema.Attribute{
					"display_name":        dsStr("Display name for the LDAP server."),
					"directory_service":   dsStr("Directory service type (`Active Directory`, `Open Directory`, `eDirectory`, `Custom`)."),
					"hostname":            dsStr("Hostname or IP address of the LDAP server."),
					"port":                dsInt("LDAP port."),
					"use_ssl":             dsBool("Whether the connection uses SSL/LDAPS."),
					"authentication_type": dsStr("Bind authentication mechanism (`none`, `simple`, `CRAM-MD5`, `DIGEST-MD5`)."),
					"account": dsschema.SingleNestedAttribute{
						MarkdownDescription: "Bind account. The password and its rotation companion are never populated by the data source.",
						Computed:            true,
						Attributes: map[string]dsschema.Attribute{
							"distinguished_username": dsStr("Distinguished name of the bind account."),
							"password":               dsschema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Always null. Jamf Pro never returns the bind password."},
							"password_wo_version":    dsschema.Int64Attribute{Computed: true, MarkdownDescription: "Always null in the data source."},
						},
					},
					"connection_timeout": dsInt("Seconds to wait before cancelling a connection attempt."),
					"search_timeout":     dsInt("Seconds to wait before cancelling a search request."),
					"referral_response":  dsStr("Referral response action (`\"\"`, `follow`, `ignore`)."),
					"use_wildcards":      dsBool("Whether partial matches are allowed in searches."),
					"is_enabled":         dsBool("Whether the LDAP server connection is enabled."),
					"migrated_to_id":     dsInt("ID of the Cloud Identity Provider this server was migrated to, or 0."),
					"certificates_used":  dsStr("Server-reported certificate usage summary."),
				},
			},
			"mappings_for_users": dsschema.SingleNestedAttribute{
				MarkdownDescription: "Attribute mappings.",
				Computed:            true,
				Attributes: map[string]dsschema.Attribute{
					"user_mappings": dsschema.SingleNestedAttribute{
						MarkdownDescription: "User mappings.",
						Computed:            true,
						Attributes: map[string]dsschema.Attribute{
							"object_class_limitation": dsStr("Object class limitation (`any` / `all`)."),
							"object_classes":          dsStr("Comma-separated object classes."),
							"search_base":             dsStr("User search base DN."),
							"search_scope":            dsStr("Search scope."),
							"user_id":                 dsStr("User ID mapping."),
							"username":                dsStr("Username mapping."),
							"real_name":               dsStr("Real name mapping."),
							"email_address":           dsStr("Email address mapping."),
							"append_to_email_results": dsStr("Text appended to email lookups."),
							"department":              dsStr("Department mapping."),
							"building":                dsStr("Building mapping."),
							"room":                    dsStr("Room mapping."),
							"phone":                   dsStr("Phone mapping."),
							"position":                dsStr("Position mapping."),
							"user_uuid":               dsStr("User UUID mapping."),
						},
					},
					"user_group_mappings": dsschema.SingleNestedAttribute{
						MarkdownDescription: "User group mappings.",
						Computed:            true,
						Attributes: map[string]dsschema.Attribute{
							"object_class_limitation": dsStr("Object class limitation (`any` / `all`)."),
							"object_classes":          dsStr("Comma-separated object classes."),
							"search_base":             dsStr("Group search base DN."),
							"search_scope":            dsStr("Search scope."),
							"group_id":                dsStr("Group ID mapping."),
							"group_name":              dsStr("Group name mapping."),
							"group_uuid":              dsStr("Group UUID mapping."),
						},
					},
					"user_group_membership_mappings": dsschema.SingleNestedAttribute{
						MarkdownDescription: "User group membership mappings.",
						Computed:            true,
						Attributes: map[string]dsschema.Attribute{
							"membership_location":                 dsStr("Where group memberships are stored (`group object` or `user object`)."),
							"member_user_mapping":                 dsStr("Member user mapping."),
							"group_membership_mapping":            dsStr("Group membership mapping."),
							"append_to_username":                  dsStr("Text appended to the username when searching."),
							"use_dn":                              dsBool("Use distinguished name of the member user when searching."),
							"use_ldap_compare":                    dsBool("Use the LDAP compare operation when searching."),
							"recursive_lookups":                   dsBool("Use recursive group searches."),
							"map_user_membership_use_dn":          dsBool("Use distinguished name of user groups when searching."),
							"membership_calculation_optimization": dsBool("Membership calculation optimization."),
							"object_class_limitation":             dsStr("Object class limitation (Other mode)."),
							"object_classes":                      dsStr("Object classes (Other mode)."),
							"search_base":                         dsStr("Search base (Other mode)."),
							"search_scope":                        dsStr("Search scope (Other mode)."),
							"username_mapping":                    dsStr("Username mapping (Other mode)."),
							"group_id_mapping":                    dsStr("Group ID mapping (Other mode)."),
							"use_member_field_for_select_queries": dsBool("Whether the `member` field is used for select membership queries (User Object mode)."),
						},
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *LdapServerDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf ProClassic client into the data source.
func (d *LdapServerDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_ldap_server")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an LDAP server by ID or by name and populates state.
func (d *LdapServerDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data LdapServerDataSourceModel
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
		got *proclassic.LdapServer
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetLDAPServerByID(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.GetLDAPServerByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing LDAP server selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro LDAP server", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignLdapServerDataSourceModel(&data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro LDAP server data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
