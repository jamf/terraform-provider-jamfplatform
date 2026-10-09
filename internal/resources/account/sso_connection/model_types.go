// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_connection

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// ConnectionResourceModel represents the Terraform resource model for a Jamf
// Account SSO connection.
//
// Two fields are shaped by what a read cannot return rather than by the
// configuration they hold. EnabledProducts and EnabledEnvironments are
// configuration-authoritative: nothing Jamf returns echoes the tenants or
// environments back, so they are never repopulated from a read and carry no
// drift detection. EnabledProductNames is the partial signal that does come
// back — the products alone, without their tenants — and is read-only.
type ConnectionResourceModel struct {
	ID                       types.String                  `tfsdk:"id"`
	Name                     types.String                  `tfsdk:"name"`
	InternalName             types.String                  `tfsdk:"internal_name"`
	ConnectionType           types.String                  `tfsdk:"connection_type"`
	HostingRegion            types.String                  `tfsdk:"hosting_region"`
	AuthMethod               types.String                  `tfsdk:"auth_method"`
	ClientID                 types.String                  `tfsdk:"client_id"`
	ClientSecret             types.String                  `tfsdk:"client_secret"`
	ClientSecretWOVersion    types.Int64                   `tfsdk:"client_secret_wo_version"`
	Scopes                   types.String                  `tfsdk:"scopes"`
	PKCE                     types.String                  `tfsdk:"pkce"`
	SendNonce                types.Bool                    `tfsdk:"send_nonce"`
	SyncAttributesAtLogin    types.Bool                    `tfsdk:"sync_attributes_at_login"`
	OmitLoginHint            types.Bool                    `tfsdk:"omit_login_hint"`
	CustomUsernameClaimName  types.String                  `tfsdk:"custom_username_claim_name"`
	UsernameDomain           types.String                  `tfsdk:"username_domain"`
	AttributeMap             types.String                  `tfsdk:"attribute_map"`
	GroupNameFilter          *GroupNameFilterModel         `tfsdk:"group_name_filter"`
	SessionDurationMinutes   types.Int64                   `tfsdk:"session_duration_minutes"`
	InactivityTimeoutMinutes types.Int64                   `tfsdk:"inactivity_timeout_minutes"`
	Domains                  types.Set                     `tfsdk:"domains"`
	EnabledProducts          []EnabledProductModel         `tfsdk:"enabled_products"`
	EnabledEnvironments      []EnabledEnvironmentModel     `tfsdk:"enabled_environments"`
	EnabledProductNames      types.Set                     `tfsdk:"enabled_product_names"`
	TicketURL                types.String                  `tfsdk:"ticket_url"`
	ConsentFlow              types.Bool                    `tfsdk:"consent_flow"`
	EasyConfig               types.Bool                    `tfsdk:"easy_config"`
	GenericOIDC              *GenericOIDCSettingsModel     `tfsdk:"generic_oidc"`
	Entra                    *EntraSettingsModel           `tfsdk:"entra"`
	Okta                     *OktaSettingsModel            `tfsdk:"okta"`
	GoogleWorkspace          *GoogleWorkspaceSettingsModel `tfsdk:"google_workspace"`
	Timeouts                 resourceTimeouts.Value        `tfsdk:"timeouts"`
}

// GroupNameFilterModel is the two-control filter the Jamf Account console
// renders beside the group list: a joining operator and the group names.
//
// Both fields are required inside the block, which is what keeps an operator
// with no groups — a real configuration meaning "pass every group through" —
// distinguishable from the block being absent.
type GroupNameFilterModel struct {
	Operator types.String `tfsdk:"operator"`
	Groups   types.Set    `tfsdk:"groups"`
}

// EnabledProductModel is one product, and the tenants of it, a connection signs
// people in to.
type EnabledProductModel struct {
	Product          types.String `tfsdk:"product"`
	Tenants          types.Set    `tfsdk:"tenants"`
	ManagedAccountID types.String `tfsdk:"managed_account_id"`
}

// EnabledEnvironmentModel is one product, and the platform environments of it, a
// connection signs people in to.
type EnabledEnvironmentModel struct {
	Product          types.String `tfsdk:"product"`
	Environments     types.Set    `tfsdk:"environments"`
	ManagedAccountID types.String `tfsdk:"managed_account_id"`
}

// GenericOIDCSettingsModel holds the settings for a connection to a provider
// Jamf has no built-in integration with.
type GenericOIDCSettingsModel struct {
	IssuerURL             types.String `tfsdk:"issuer_url"`
	AuthorizationEndpoint types.String `tfsdk:"authorization_endpoint"`
	TokenEndpoint         types.String `tfsdk:"token_endpoint"`
	JWKSURI               types.String `tfsdk:"jwks_uri"`
	UserInfoEndpoint      types.String `tfsdk:"user_info_endpoint"`
}

// EntraSettingsModel holds the Microsoft Entra settings.
type EntraSettingsModel struct {
	Domain              types.String `tfsdk:"domain"`
	TenantDomain        types.String `tfsdk:"tenant_domain"`
	UseCommonEndpoint   types.Bool   `tfsdk:"use_common_endpoint"`
	IdentityAPI         types.String `tfsdk:"identity_api"`
	MaxGroups           types.Int64  `tfsdk:"max_groups"`
	SetEmailsVerified   types.Bool   `tfsdk:"set_emails_verified"`
	EnableUsersAPI      types.Bool   `tfsdk:"enable_users_api"`
	UseWSFed            types.Bool   `tfsdk:"use_wsfed"`
	GroupsScope         types.String `tfsdk:"groups_scope"`
	ExtendedProfile     types.Bool   `tfsdk:"extended_profile"`
	GetUserGroups       types.Bool   `tfsdk:"get_user_groups"`
	IncludeNestedGroups types.Bool   `tfsdk:"include_nested_groups"`
	BasicProfile        types.Bool   `tfsdk:"basic_profile"`
}

// OktaSettingsModel holds the Okta settings. Only the org domain is declarable —
// Jamf derives the four addresses from it.
type OktaSettingsModel struct {
	Domain                types.String `tfsdk:"domain"`
	IssuerURL             types.String `tfsdk:"issuer_url"`
	AuthorizationEndpoint types.String `tfsdk:"authorization_endpoint"`
	TokenEndpoint         types.String `tfsdk:"token_endpoint"`
	JWKSURI               types.String `tfsdk:"jwks_uri"`
}

// GoogleWorkspaceSettingsModel holds the Google Workspace settings.
type GoogleWorkspaceSettingsModel struct {
	Domain         types.String `tfsdk:"domain"`
	GetUserGroups  types.Bool   `tfsdk:"get_user_groups"`
	ExtendedGroups types.Bool   `tfsdk:"extended_groups"`
	EnableUsersAPI types.Bool   `tfsdk:"enable_users_api"`
}

// connectionIdentityModel represents the identity object for SSO connection
// resources and list results.
//
// The identifier, unlike the sibling SSO domain construct's: a connection is
// readable by identifier, the identifier is stable, and the display name is not
// a usable key because Jamf may store a uniquified form of whatever name was
// configured.
type connectionIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// ConnectionDataSourceModel represents the Terraform data source model for a
// single Jamf Account SSO connection.
//
// It carries neither the product assignments nor the client secret: no read
// returns either, so an attribute for them would be permanently null. The
// products a connection is reported as enabled for are in
// enabled_product_names.
type ConnectionDataSourceModel struct {
	ID                       types.String                  `tfsdk:"id"`
	Name                     types.String                  `tfsdk:"name"`
	ConnectionType           types.String                  `tfsdk:"connection_type"`
	HostingRegion            types.String                  `tfsdk:"hosting_region"`
	AuthMethod               types.String                  `tfsdk:"auth_method"`
	ClientID                 types.String                  `tfsdk:"client_id"`
	Scopes                   types.String                  `tfsdk:"scopes"`
	PKCE                     types.String                  `tfsdk:"pkce"`
	SendNonce                types.Bool                    `tfsdk:"send_nonce"`
	SyncAttributesAtLogin    types.Bool                    `tfsdk:"sync_attributes_at_login"`
	OmitLoginHint            types.Bool                    `tfsdk:"omit_login_hint"`
	CustomUsernameClaimName  types.String                  `tfsdk:"custom_username_claim_name"`
	UsernameDomain           types.String                  `tfsdk:"username_domain"`
	AttributeMap             types.String                  `tfsdk:"attribute_map"`
	GroupNameFilter          *GroupNameFilterModel         `tfsdk:"group_name_filter"`
	SessionDurationMinutes   types.Int64                   `tfsdk:"session_duration_minutes"`
	InactivityTimeoutMinutes types.Int64                   `tfsdk:"inactivity_timeout_minutes"`
	Domains                  types.Set                     `tfsdk:"domains"`
	EnabledProductNames      types.Set                     `tfsdk:"enabled_product_names"`
	TicketURL                types.String                  `tfsdk:"ticket_url"`
	ConsentFlow              types.Bool                    `tfsdk:"consent_flow"`
	EasyConfig               types.Bool                    `tfsdk:"easy_config"`
	GenericOIDC              *GenericOIDCSettingsModel     `tfsdk:"generic_oidc"`
	Entra                    *EntraSettingsModel           `tfsdk:"entra"`
	Okta                     *OktaSettingsModel            `tfsdk:"okta"`
	GoogleWorkspace          *GoogleWorkspaceSettingsModel `tfsdk:"google_workspace"`
	Timeouts                 datasourceTimeouts.Value      `tfsdk:"timeouts"`
}

// ConnectionsDataSourceModel represents the Terraform data source model for the
// plural SSO connection lookup.
type ConnectionsDataSourceModel struct {
	ID             types.String                       `tfsdk:"id"`
	SSOConnections []ConnectionsDataSourceResultModel `tfsdk:"sso_connections"`
	Timeouts       datasourceTimeouts.Value           `tfsdk:"timeouts"`
}

// ConnectionsDataSourceResultModel represents a single SSO connection in the
// plural data source results.
//
// It carries a strict subset of the singular data source's attributes, and the
// subset is Jamf's rather than this provider's: the collection read returns no
// provider-specific settings at all, so exposing them here would mean one extra
// round trip per connection in the organization. Use the singular data source
// for those.
type ConnectionsDataSourceResultModel struct {
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	ConnectionType        types.String `tfsdk:"connection_type"`
	HostingRegion         types.String `tfsdk:"hosting_region"`
	AuthMethod            types.String `tfsdk:"auth_method"`
	SyncAttributesAtLogin types.Bool   `tfsdk:"sync_attributes_at_login"`
	Domains               types.Set    `tfsdk:"domains"`
	EnabledProductNames   types.Set    `tfsdk:"enabled_product_names"`
	TicketURL             types.String `tfsdk:"ticket_url"`
	EasyConfig            types.Bool   `tfsdk:"easy_config"`
}

// ConnectionListResourceModel represents the config model for SSO connection
// list queries. Jamf Account exposes no filter arguments on the connection
// collection, so the model carries only the list-level timeouts.
type ConnectionListResourceModel struct {
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
