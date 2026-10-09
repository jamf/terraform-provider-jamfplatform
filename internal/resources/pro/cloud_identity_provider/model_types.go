// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// CloudIdentityProviderResourceModel is the Terraform resource model for the
// umbrella Jamf Pro Cloud Identity Provider. A single resource type covers
// both providers; `provider_name` is the discriminator (RequiresReplace) and
// selects which of the mutually-exclusive `google` / `entra_id` blocks is
// permitted. CRUD dispatches on `provider_name` (the wire value for ENTRA_ID is
// the legacy "AZURE"):
//
//   - GOOGLE   → the Cloud LDAP (Google Secure LDAP) endpoints.
//   - ENTRA_ID → the Cloud Azure (Microsoft Entra ID) endpoints.
//
// `id` and `display_name` are shared across both providers (they map to the
// common Cloud Identity Provider registry record). Everything provider-
// specific lives inside the corresponding block.
type CloudIdentityProviderResourceModel struct {
	ID           types.String                      `tfsdk:"id"`
	DisplayName  types.String                      `tfsdk:"display_name"`
	ProviderName types.String                      `tfsdk:"provider_name"`
	Google       *cloudIdentityProviderGoogleModel `tfsdk:"google"`
	Azure        *cloudIdentityProviderAzureModel  `tfsdk:"entra_id"`
	Timeouts     resourceTimeouts.Value            `tfsdk:"timeouts"`
}

// cloudIdentityProviderGoogleModel is the Google (Secure LDAP) branch.
type cloudIdentityProviderGoogleModel struct {
	Server   *cloudLdapServerModel   `tfsdk:"server"`
	Mappings *cloudLdapMappingsModel `tfsdk:"mappings"`
}

// cloudLdapServerModel is the Google LDAP server connection configuration.
//
// `Keystore` holds the client certificate. `File` and `Password` are
// WriteOnly (sent on writes, never persisted in state); `WoVersion`
// (`wo_version`) is the re-upload rotation trigger (bump it to re-send the
// keystore). The remaining keystore fields are server-derived echoes
// (Computed-only).
type cloudLdapServerModel struct {
	ServerURL                                types.String            `tfsdk:"server_url"`
	DomainName                               types.String            `tfsdk:"domain_name"`
	Port                                     types.Int64             `tfsdk:"port"`
	ConnectionType                           types.String            `tfsdk:"connection_type"`
	ConnectionTimeout                        types.Int64             `tfsdk:"connection_timeout"`
	SearchTimeout                            types.Int64             `tfsdk:"search_timeout"`
	UseWildcards                             types.Bool              `tfsdk:"use_wildcards"`
	Enabled                                  types.Bool              `tfsdk:"enabled"`
	MembershipCalculationOptimizationEnabled types.Bool              `tfsdk:"membership_calculation_optimization_enabled"`
	Keystore                                 *cloudLdapKeystoreModel `tfsdk:"keystore"`
}

// cloudLdapKeystoreModel is the Google client-certificate keystore.
//
// File + Password are WriteOnly; WoVersion (`wo_version`) is the rotation
// trigger (a plain Optional Int64 the framework persists). FileName / Type / Subject /
// ExpirationDate are server-derived echoes. ExpirationDate is a plain
// string — Jamf Pro emits a timezone-less ISO-8601 datetime the SDK now
// surfaces as *string (see SDK_CLOUD_LDAP_KEYSTORE_DATE_FIX_PROMPT.md).
type cloudLdapKeystoreModel struct {
	File           types.String `tfsdk:"file"`
	Password       types.String `tfsdk:"password"`
	WoVersion      types.Int64  `tfsdk:"wo_version"`
	FileName       types.String `tfsdk:"file_name"`
	Type           types.String `tfsdk:"type"`
	Subject        types.String `tfsdk:"subject"`
	ExpirationDate types.String `tfsdk:"expiration_date"`
}

// cloudLdapMappingsModel is the Google attribute-mapping configuration. The
// whole block is Optional (NOT Computed — a Computed nested object over a
// typed-pointer model decodes as unknown-on-omit and fails Plan.Get; see
// STYLE_GUIDE §SingleNestedAttribute blocks). Omit it and the server generates
// Google defaults (the state builder keeps it null so there is no perpetual
// diff); supply it to override. All three sub-blocks ride inline in the
// create/update payload.
type cloudLdapMappingsModel struct {
	UserMappings       *cloudLdapUserMappingsModel       `tfsdk:"user_mappings"`
	GroupMappings      *cloudLdapGroupMappingsModel      `tfsdk:"group_mappings"`
	MembershipMappings *cloudLdapMembershipMappingsModel `tfsdk:"membership_mappings"`
}

// cloudLdapUserMappingsModel maps Google LDAP user attributes.
type cloudLdapUserMappingsModel struct {
	ObjectClassLimitation types.String `tfsdk:"object_class_limitation"`
	ObjectClasses         types.String `tfsdk:"object_classes"`
	SearchBase            types.String `tfsdk:"search_base"`
	SearchScope           types.String `tfsdk:"search_scope"`
	AdditionalSearchBase  types.String `tfsdk:"additional_search_base"`
	UserID                types.String `tfsdk:"user_id"`
	Username              types.String `tfsdk:"username"`
	RealName              types.String `tfsdk:"real_name"`
	EmailAddress          types.String `tfsdk:"email_address"`
	Department            types.String `tfsdk:"department"`
	Building              types.String `tfsdk:"building"`
	Room                  types.String `tfsdk:"room"`
	Phone                 types.String `tfsdk:"phone"`
	Position              types.String `tfsdk:"position"`
	UserUUID              types.String `tfsdk:"user_uuid"`
}

// cloudLdapGroupMappingsModel maps Google LDAP group attributes.
type cloudLdapGroupMappingsModel struct {
	ObjectClassLimitation types.String `tfsdk:"object_class_limitation"`
	ObjectClasses         types.String `tfsdk:"object_classes"`
	SearchBase            types.String `tfsdk:"search_base"`
	SearchScope           types.String `tfsdk:"search_scope"`
	GroupID               types.String `tfsdk:"group_id"`
	GroupName             types.String `tfsdk:"group_name"`
	GroupUUID             types.String `tfsdk:"group_uuid"`
}

// cloudLdapMembershipMappingsModel maps the Google LDAP group-membership
// attribute.
type cloudLdapMembershipMappingsModel struct {
	GroupMembershipMapping types.String `tfsdk:"group_membership_mapping"`
}

// cloudIdentityProviderAzureModel is the Microsoft Entra ID branch.
//
// There is no `code` attribute: the OAuth consent code is a single-use
// artifact not surfaced in the Jamf admin UI. The provider sends a non-blank
// placeholder code on create (the server rejects a blank code but does not
// validate consent at create time); the update contract has no code field at
// all. After apply the admin must complete the manual "refresh consent" flow
// in the Jamf UI to activate the connection. `Type` / `Migrated` /
// `DeprecatedConsent` are server-derived echoes.
type cloudIdentityProviderAzureModel struct {
	TenantID                                 types.String             `tfsdk:"tenant_id"`
	SearchTimeout                            types.Int64              `tfsdk:"search_timeout"`
	Enabled                                  types.Bool               `tfsdk:"enabled"`
	MembershipCalculationOptimizationEnabled types.Bool               `tfsdk:"membership_calculation_optimization_enabled"`
	TransitiveMembershipEnabled              types.Bool               `tfsdk:"transitive_membership_enabled"`
	TransitiveMembershipUserField            types.String             `tfsdk:"transitive_membership_user_field"`
	TransitiveDirectoryMembershipEnabled     types.Bool               `tfsdk:"transitive_directory_membership_enabled"`
	Type                                     types.String             `tfsdk:"type"`
	Migrated                                 types.Bool               `tfsdk:"migrated"`
	DeprecatedConsent                        types.Bool               `tfsdk:"deprecated_consent"`
	Mappings                                 *cloudAzureMappingsModel `tfsdk:"mappings"`
}

// cloudAzureMappingsModel maps Entra ID attributes. Flat (no object-class /
// search-base concepts — Entra is queried via Graph, not LDAP).
type cloudAzureMappingsModel struct {
	UserID     types.String `tfsdk:"user_id"`
	UserName   types.String `tfsdk:"user_name"`
	RealName   types.String `tfsdk:"real_name"`
	Email      types.String `tfsdk:"email"`
	Department types.String `tfsdk:"department"`
	Building   types.String `tfsdk:"building"`
	Room       types.String `tfsdk:"room"`
	Phone      types.String `tfsdk:"phone"`
	Position   types.String `tfsdk:"position"`
	GroupID    types.String `tfsdk:"group_id"`
	GroupName  types.String `tfsdk:"group_name"`
}

// cloudIdentityProviderIdentityModel is the identity object for resource
// imports.
type cloudIdentityProviderIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// CloudIdentityProviderDataSourceModel is the singular registry data source model. Either
// `id` or `display_name` selects the record (ExactlyOneOf). Read-only;
// covers both Google and Azure registry entries.
type CloudIdentityProviderDataSourceModel struct {
	ID                  types.String             `tfsdk:"id"`
	DisplayName         types.String             `tfsdk:"display_name"`
	ProviderName        types.String             `tfsdk:"provider_name"`
	Enabled             types.Bool               `tfsdk:"enabled"`
	ProviderDescription types.String             `tfsdk:"provider_description"`
	Timeouts            datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// CloudIdentityProvidersDataSourceModel is the plural registry data source model.
type CloudIdentityProvidersDataSourceModel struct {
	CloudIdentityProviders []CloudIdentityProviderDataSourceEntryModel `tfsdk:"cloud_identity_providers"`
	Timeouts               datasourceTimeouts.Value                    `tfsdk:"timeouts"`
}

// CloudIdentityProviderDataSourceEntryModel is one row in the plural registry data source.
type CloudIdentityProviderDataSourceEntryModel struct {
	ID                  types.String `tfsdk:"id"`
	DisplayName         types.String `tfsdk:"display_name"`
	ProviderName        types.String `tfsdk:"provider_name"`
	Enabled             types.Bool   `tfsdk:"enabled"`
	ProviderDescription types.String `tfsdk:"provider_description"`
}

// CloudIdentityProviderListResourceModel is the config model for the list
// resource. The Cloud Identity Provider list endpoint has no filter parameters,
// so the filter shape reuses the shared client-side substring block.
type CloudIdentityProviderListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}

// CloudIdentityProviderDefaultsDataSourceModel is the defaults reference data
// source model. Both blocks are always populated: the defaults Jamf Pro reports
// exist whether or not the tenant has a connection of either kind.
type CloudIdentityProviderDefaultsDataSourceModel struct {
	EntraID  *cloudIdentityProviderEntraIDDefaultsModel `tfsdk:"entra_id"`
	Google   *cloudIdentityProviderGoogleDefaultsModel  `tfsdk:"google"`
	Timeouts datasourceTimeouts.Value                   `tfsdk:"timeouts"`
}

// cloudIdentityProviderEntraIDDefaultsModel carries the Entra ID defaults.
// Mappings reuses the resource's own model so the two objects share their tfsdk
// tags by construction and one assigns straight into the other.
type cloudIdentityProviderEntraIDDefaultsModel struct {
	Type                                     types.String             `tfsdk:"type"`
	SearchTimeout                            types.Int64              `tfsdk:"search_timeout"`
	TransitiveMembershipEnabled              types.Bool               `tfsdk:"transitive_membership_enabled"`
	TransitiveMembershipUserField            types.String             `tfsdk:"transitive_membership_user_field"`
	TransitiveDirectoryMembershipEnabled     types.Bool               `tfsdk:"transitive_directory_membership_enabled"`
	MembershipCalculationOptimizationEnabled types.Bool               `tfsdk:"membership_calculation_optimization_enabled"`
	Mappings                                 *cloudAzureMappingsModel `tfsdk:"mappings"`
}

// cloudIdentityProviderGoogleDefaultsModel carries the Google Secure LDAP
// defaults, which come from two separate reads.
type cloudIdentityProviderGoogleDefaultsModel struct {
	Server   *cloudIdentityProviderGoogleServerDefaultsModel `tfsdk:"server"`
	Mappings *cloudLdapMappingsModel                         `tfsdk:"mappings"`
}

// cloudIdentityProviderGoogleServerDefaultsModel carries the Google server
// connection defaults. domain_name and keystore are absent because the defaults
// report neither: they are the two things only the practitioner supplies.
type cloudIdentityProviderGoogleServerDefaultsModel struct {
	ServerURL                                types.String `tfsdk:"server_url"`
	Port                                     types.Int64  `tfsdk:"port"`
	ConnectionType                           types.String `tfsdk:"connection_type"`
	ConnectionTimeout                        types.Int64  `tfsdk:"connection_timeout"`
	SearchTimeout                            types.Int64  `tfsdk:"search_timeout"`
	UseWildcards                             types.Bool   `tfsdk:"use_wildcards"`
	MembershipCalculationOptimizationEnabled types.Bool   `tfsdk:"membership_calculation_optimization_enabled"`
}
