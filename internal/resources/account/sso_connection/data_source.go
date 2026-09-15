// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// ConnectionDataSource implements the Terraform data source for a single Jamf
// Account SSO connection.
type ConnectionDataSource struct {
	client *account.Client
}

var (
	_ datasource.DataSource                     = &ConnectionDataSource{}
	_ datasource.DataSourceWithConfigValidators = &ConnectionDataSource{}
)

// NewConnectionDataSource returns a new instance of ConnectionDataSource.
func NewConnectionDataSource() datasource.DataSource {
	return &ConnectionDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ConnectionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_sso_connection"
}

// ConfigValidators requires exactly one lookup key.
//
// Both keys are offered because both are useful and neither is sufficient. The
// identifier is exact and stable but opaque, and a practitioner does not see it
// in the console; the name is what they know, but Jamf may hold a uniquified form
// of it and two connections can end up answering to the same one — which the
// read reports rather than resolving by picking.
func (d *ConnectionDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Schema returns the data source schema.
func (d *ConnectionDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up one SSO connection in your Jamf Account organization, by identifier or by " +
			"the name Jamf Account holds for it. Exactly one of `id` and `name` is required.\n\nThis is also the " +
			"construct for a connection the `jamfplatform_account_sso_connection` resource refuses to manage: " +
			"one built with Microsoft's admin-consent flow in the Jamf Account console, which has no client " +
			"registration of its own and cannot be written back. Reading it here takes no ownership of " +
			"it.\n\nTwo things no read returns, so neither appears here: the client secret, which Jamf Account " +
			"never gives back, and the tenants each product is enabled for. `enabled_product_names` reports the " +
			"products alone, which is the only part of that assignment that can be read." +
			dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the connection to look up. Set this or `name`, not both.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name Jamf Account holds for the connection, matched exactly. Set this or " +
					"`id`, not both. Jamf Account may store a uniquified form of the name a connection was " +
					"created with, so the name to match is the one the console lists rather than the one that " +
					"was asked for. Use `jamfplatform_account_sso_connections` to see them.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"connection_type": schema.StringAttribute{
				MarkdownDescription: "The identity provider family: one of " +
					markdownValueList(connectionTypeValues()) + ".",
				Computed: true,
			},
			"hosting_region": schema.StringAttribute{
				MarkdownDescription: "The region the connection's provider details are held in and its sign-in " +
					"traffic is routed through.",
				Computed: true,
			},
			"auth_method": schema.StringAttribute{
				MarkdownDescription: "How Jamf Account proves itself to the provider when it redeems an authorization code.",
				Computed:            true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "Identifier of the application registered with the provider. Empty for a " +
					"connection using Microsoft admin consent, which has no client of its own.",
				Computed: true,
			},
			"scopes": schema.StringAttribute{
				MarkdownDescription: "The OAuth scopes Jamf Account asks the provider for, separated by spaces. " +
					"Empty for a family that takes none.",
				Computed: true,
			},
			"pkce": schema.StringAttribute{
				MarkdownDescription: "The Proof Key for Code Exchange method used with the provider.",
				Computed:            true,
			},
			"send_nonce": schema.BoolAttribute{
				MarkdownDescription: "Whether a nonce is sent on the authentication request.",
				Computed:            true,
			},
			"sync_attributes_at_login": schema.BoolAttribute{
				MarkdownDescription: "Whether a person's profile details are refreshed from the provider every " +
					"time they sign in.",
				Computed: true,
			},
			"omit_login_hint": schema.BoolAttribute{
				MarkdownDescription: "Whether the address someone typed at Jamf Account is withheld from the " +
					"provider, so they type it again there.",
				Computed: true,
			},
			"custom_username_claim_name": schema.StringAttribute{
				MarkdownDescription: "The claim a username is read from, where it is not the standard one.",
				Computed:            true,
			},
			"username_domain": schema.StringAttribute{
				MarkdownDescription: "Domain appended to a bare username from the provider to form the person's " +
					"email address.",
				Computed: true,
			},
			"attribute_map": schema.StringAttribute{
				MarkdownDescription: "How claims from the provider are mapped onto Jamf Account user details, as " +
					"a JSON object string.",
				Computed: true,
			},
			"group_name_filter": schema.SingleNestedAttribute{
				MarkdownDescription: "Which of the provider's groups are passed through to Jamf Account. Empty " +
					"when the connection carries no filter at all, which is different from a filter with no " +
					"groups in it.",
				Computed: true,
				Attributes: map[string]schema.Attribute{
					"operator": schema.StringAttribute{
						MarkdownDescription: "How the group names are joined: `or` passes a group matching any " +
							"entry, `and` requires every entry.",
						Computed: true,
					},
					"groups": schema.SetAttribute{
						MarkdownDescription: "The group names filtered on. Empty means no filtering.",
						Computed:            true,
						ElementType:         types.StringType,
					},
				},
			},
			"session_duration_minutes": schema.Int64Attribute{
				MarkdownDescription: "How long a session lasts before the person signs in again, however active " +
					"they are. Empty where the Jamf Account default applies.",
				Computed: true,
			},
			"inactivity_timeout_minutes": schema.Int64Attribute{
				MarkdownDescription: "How long a session survives without activity. Empty where the Jamf Account " +
					"default applies.",
				Computed: true,
			},
			"domains": schema.SetAttribute{
				MarkdownDescription: "The domain names this connection signs people in for.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"enabled_product_names": schema.SetAttribute{
				MarkdownDescription: "The Jamf products Jamf Account reports this connection as enabled for. The " +
					"tenants of each product are never returned, so this is the whole of what can be read about " +
					"the assignment.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"ticket_url": schema.StringAttribute{
				MarkdownDescription: "Address of the Google Workspace administrator consent request for this " +
					"connection, where one is outstanding.",
				Computed: true,
			},
			"consent_flow": schema.BoolAttribute{
				MarkdownDescription: "Whether the connection authenticates through Microsoft's admin-consent flow " +
					"rather than through a registered client. Such a connection cannot be managed as a " +
					"`jamfplatform_account_sso_connection` resource, so this is how to read one.",
				Computed: true,
			},
			"easy_config": schema.BoolAttribute{
				MarkdownDescription: "Whether the connection was built by Jamf Account's guided setup rather " +
					"than configured directly.",
				Computed: true,
			},
			"generic_oidc":     dataSourceGenericOIDCAttribute(),
			"entra":            dataSourceEntraAttribute(),
			"okta":             dataSourceOktaAttribute(),
			"google_workspace": dataSourceGoogleAttribute(),
			"timeouts":         timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Account client into the data source via the shared
// providerdata.ConfigureAccount helper.
func (d *ConnectionDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_sso_connection")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches one connection and populates Terraform state.
//
// Both calls are made whichever key was given. The collection read resolves a
// name to an identifier and supplies the two values only it carries; the single
// read supplies the per-provider settings. A connection the collection lists but
// which cannot be read on its own identifier is reported rather than returned
// empty — an empty result would read as "this connection has no settings", which
// is a different and wrong statement.
func (d *ConnectionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ConnectionDataSourceModel
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

	summaries, err := d.client.ListConnections(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Jamf Account SSO connections", helpers.APIErrorDetail(err))
		return
	}

	summary, ok := resolveSummary(&resp.Diagnostics, summaries, data.ID, data.Name)
	if !ok {
		return
	}

	found, err := d.client.GetConnection(readCtx, summary.ID)
	if err != nil {
		if helpers.IsNotFoundError(err) {
			appendGhostConnectionDiagnostics(&resp.Diagnostics, summary.ID, summary.Name)
			return
		}
		resp.Diagnostics.AddError("Unable to read Jamf Account SSO connection", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignConnectionDataSourceModel(&data, found, summary)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Account SSO connection data source", map[string]any{"id": data.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// resolveSummary picks the collection entry the configured key names.
//
// An identifier that is not in the collection is reported as absent without a
// second call, and a name is reported as absent or as ambiguous — never resolved
// by picking one, because the stored name is not a unique key and silently
// choosing would give a configuration that reads a different connection from one
// run to the next.
func resolveSummary(
	diags *diag.Diagnostics,
	summaries []account.ConnectionSummary,
	id types.String,
	name types.String,
) (*account.ConnectionSummary, bool) {
	if helpers.IsConfiguredValue(id) {
		if summary := findSummary(summaries, id.ValueString()); summary != nil {
			return summary, true
		}
		diags.AddAttributeError(
			path.Root("id"),
			"Unable to find Jamf Account SSO connection",
			"Your Jamf Account organization has no connection with the identifier \""+id.ValueString()+"\". Use "+
				"the `jamfplatform_account_sso_connections` data source to list the connections it holds.",
		)
		return nil, false
	}

	matches := findSummariesByName(summaries, name.ValueString())
	switch len(matches) {
	case 1:
		return &matches[0], true
	case 0:
		diags.AddAttributeError(
			path.Root("name"),
			"Unable to find Jamf Account SSO connection",
			"Your Jamf Account organization has no connection named \""+name.ValueString()+"\". Jamf Account may hold a "+
				"uniquified form of the name a connection was created with, so the name to match is the one the "+
				"Jamf Account console lists — use the `jamfplatform_account_sso_connections` data source to see "+
				"them.",
		)
		return nil, false
	default:
		diags.AddAttributeError(
			path.Root("name"),
			"More than one Jamf Account SSO connection has this name",
			"Your Jamf Account organization holds "+strconv.Itoa(len(matches))+" connections named \""+
				name.ValueString()+"\", so the name does not identify one. Look one up by `id` instead: "+
				summaryNames(matches)+".",
		)
		return nil, false
	}
}

// dataSourceGenericOIDCAttribute describes the generic OpenID Connect block as a
// data source reports it. Empty for every other provider family.
func dataSourceGenericOIDCAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Settings of a generic OpenID Connect connection. Empty for every other family.",
		Computed:            true,
		Attributes: map[string]schema.Attribute{
			"issuer_url": schema.StringAttribute{
				MarkdownDescription: "The provider's issuer, as it appears in the tokens it signs.",
				Computed:            true,
			},
			"authorization_endpoint": schema.StringAttribute{
				MarkdownDescription: "Address people are redirected to in order to sign in.",
				Computed:            true,
			},
			"token_endpoint": schema.StringAttribute{
				MarkdownDescription: "Address an authorization code is exchanged at.",
				Computed:            true,
			},
			"jwks_uri": schema.StringAttribute{
				MarkdownDescription: "Address the provider publishes its signing keys at.",
				Computed:            true,
			},
			"user_info_endpoint": schema.StringAttribute{
				MarkdownDescription: "Address profile details are read from, where the identity token does not " +
					"carry everything Jamf Account needs.",
				Computed: true,
			},
		},
	}
}

// dataSourceEntraAttribute describes the Microsoft Entra block as a data source
// reports it. Empty for every other provider family.
func dataSourceEntraAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Settings of a Microsoft Entra connection. Empty for every other family.",
		Computed:            true,
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				MarkdownDescription: "Primary domain of the Entra tenant.",
				Computed:            true,
			},
			"tenant_domain": schema.StringAttribute{
				MarkdownDescription: "Domain identifying the Entra tenant authenticated against.",
				Computed:            true,
			},
			"use_common_endpoint": schema.BoolAttribute{
				MarkdownDescription: "Whether Microsoft's multi-tenant sign-in address is used instead of the " +
					"tenant's own.",
				Computed: true,
			},
			"identity_api": schema.StringAttribute{
				MarkdownDescription: "The Microsoft identity platform version the connection uses.",
				Computed:            true,
			},
			"max_groups": schema.Int64Attribute{
				MarkdownDescription: "The most groups read for one person.",
				Computed:            true,
			},
			"set_emails_verified": schema.BoolAttribute{
				MarkdownDescription: "Whether addresses from Entra are treated as already confirmed.",
				Computed:            true,
			},
			"enable_users_api": schema.BoolAttribute{
				MarkdownDescription: "Whether Microsoft Graph is queried for details the token does not carry.",
				Computed:            true,
			},
			"use_wsfed": schema.BoolAttribute{
				MarkdownDescription: "Whether WS-Federation is used instead of OpenID Connect.",
				Computed:            true,
			},
			"groups_scope": schema.StringAttribute{
				MarkdownDescription: "The Microsoft Graph permission groups are read with.",
				Computed:            true,
			},
			"extended_profile": schema.BoolAttribute{
				MarkdownDescription: "Whether a person's extended profile details are read from Entra.",
				Computed:            true,
			},
			"get_user_groups": schema.BoolAttribute{
				MarkdownDescription: "Whether a person's Entra group memberships are passed through to Jamf Account.",
				Computed:            true,
			},
			"include_nested_groups": schema.BoolAttribute{
				MarkdownDescription: "Whether groups inherited through nested membership are included.",
				Computed:            true,
			},
			"basic_profile": schema.BoolAttribute{
				MarkdownDescription: "Whether a person's basic profile is read from Entra. Always on.",
				Computed:            true,
			},
		},
	}
}

// dataSourceOktaAttribute describes the Okta block as a data source reports it.
// Empty for every other provider family.
func dataSourceOktaAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Settings of an Okta connection. Empty for every other family.",
		Computed:            true,
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				MarkdownDescription: "The Okta org domain.",
				Computed:            true,
			},
			"issuer_url": schema.StringAttribute{
				MarkdownDescription: "Issuer of the Okta authorization host, worked out from the domain.",
				Computed:            true,
			},
			"authorization_endpoint": schema.StringAttribute{
				MarkdownDescription: "Address people are redirected to in order to sign in.",
				Computed:            true,
			},
			"token_endpoint": schema.StringAttribute{
				MarkdownDescription: "Address an authorization code is exchanged at.",
				Computed:            true,
			},
			"jwks_uri": schema.StringAttribute{
				MarkdownDescription: "Address Okta publishes its signing keys at.",
				Computed:            true,
			},
		},
	}
}

// dataSourceGoogleAttribute describes the Google Workspace block as a data source
// reports it. Empty for every other provider family.
func dataSourceGoogleAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Settings of a Google Workspace connection. Empty for every other family.",
		Computed:            true,
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				MarkdownDescription: "Primary domain of the Google Workspace account.",
				Computed:            true,
			},
			"get_user_groups": schema.BoolAttribute{
				MarkdownDescription: "Whether a person's Google Workspace group memberships are passed through " +
					"to Jamf Account.",
				Computed: true,
			},
			"extended_groups": schema.BoolAttribute{
				MarkdownDescription: "Whether groups are read through the Google Directory rather than from the " +
					"token.",
				Computed: true,
			},
			"enable_users_api": schema.BoolAttribute{
				MarkdownDescription: "Whether the Google Directory is queried for details the token does not " +
					"carry.",
				Computed: true,
			},
		},
	}
}
