// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ldap_server

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// LdapServerResourceModel is the Terraform resource model for a Jamf Pro
// on-premises LDAP server (Classic /ldapservers). The wire envelope is
// `<ldap_server>` carrying a `connection` block (server identity + bind
// account) and a `mappings_for_users` block (user / user-group / membership
// attribute mappings). This resource manages classic AD / Open Directory /
// eDirectory / Custom directories only — cloud directories (Google, Microsoft
// Entra) are managed by `jamfplatform_pro_cloud_identity_provider`.
//
// id source asymmetry: Create POSTs to id=0 and the server returns the
// allocated integer at the top-level `<id>`; every GET nests it under
// `<connection><id>`. The state builder reads `connection.id`; Create reads
// the top-level id.
type LdapServerResourceModel struct {
	ID               types.String           `tfsdk:"id"`
	Connection       *ldapConnectionModel   `tfsdk:"connection_settings"`
	MappingsForUsers *ldapMappingsModel     `tfsdk:"mappings_for_users"`
	Timeouts         resourceTimeouts.Value `tfsdk:"timeouts"`
}

// LdapServerDataSourceModel mirrors the resource shape minus the WriteOnly
// password machinery. Either `id` or `name` selects the record (ExactlyOneOf).
type LdapServerDataSourceModel struct {
	ID               types.String             `tfsdk:"id"`
	Name             types.String             `tfsdk:"name"`
	Connection       *ldapConnectionModel     `tfsdk:"connection_settings"`
	MappingsForUsers *ldapMappingsModel       `tfsdk:"mappings_for_users"`
	Timeouts         datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// ldapConnectionModel is the `connection` block — server identity, transport,
// authentication, and timeouts. Wire names are in comments where the TF
// attribute name follows the admin-UI label instead.
type ldapConnectionModel struct {
	DisplayName        types.String      `tfsdk:"display_name"`        // name
	DirectoryService   types.String      `tfsdk:"directory_service"`   // server_type
	Hostname           types.String      `tfsdk:"hostname"`            // hostname
	Port               types.Int64       `tfsdk:"port"`                // port
	UseSSL             types.Bool        `tfsdk:"use_ssl"`             // use_ssl
	AuthenticationType types.String      `tfsdk:"authentication_type"` // authentication_type
	Account            *ldapAccountModel `tfsdk:"account"`             // account
	ConnectionTimeout  types.Int64       `tfsdk:"connection_timeout"`  // open_close_timeout
	SearchTimeout      types.Int64       `tfsdk:"search_timeout"`      // search_timeout
	ReferralResponse   types.String      `tfsdk:"referral_response"`   // referral_response
	UseWildcards       types.Bool        `tfsdk:"use_wildcards"`       // use_wildcards
	// Server-managed echoes (Computed-only).
	IsEnabled        types.Bool   `tfsdk:"is_enabled"`        // is_enabled
	MigratedToID     types.Int64  `tfsdk:"migrated_to_id"`    // migrated_to_id
	CertificatesUsed types.String `tfsdk:"certificates_used"` // certificates_used
}

// ldapAccountModel is the bind/lookup account nested in `connection`. The
// plaintext `password` is WriteOnly — sent on writes, never persisted in
// state; Jamf Pro returns only a masked `password_sha256` sentinel on read,
// which carries no drift signal and is not surfaced. `password_wo_version`
// is the rotation trigger (bump to re-send the current password). Account is
// Optional and absent for anonymous binds (authentication_type = "none").
type ldapAccountModel struct {
	DistinguishedUsername types.String `tfsdk:"distinguished_username"` // distinguished_username
	Password              types.String `tfsdk:"password"`               // account.password (WriteOnly)
	PasswordWoVersion     types.Int64  `tfsdk:"password_wo_version"`
}

// ldapMappingsModel is the `mappings_for_users` block. The server always
// echoes all three sub-blocks fully populated, so the parent and each
// sub-block are Optional+Computed with UseStateForUnknown to avoid spurious
// diffs for blocks the user omits.
type ldapMappingsModel struct {
	UserMappings                *ldapUserMappingsModel       `tfsdk:"user_mappings"`
	UserGroupMappings           *ldapUserGroupMappingsModel  `tfsdk:"user_group_mappings"`
	UserGroupMembershipMappings *ldapMembershipMappingsModel `tfsdk:"user_group_membership_mappings"`
}

// ldapUserMappingsModel is the User Mappings sub-tab.
type ldapUserMappingsModel struct {
	ObjectClassLimitation types.String `tfsdk:"object_class_limitation"` // map_object_class_to_any_or_all
	ObjectClasses         types.String `tfsdk:"object_classes"`          // object_classes
	SearchBase            types.String `tfsdk:"search_base"`             // search_base
	SearchScope           types.String `tfsdk:"search_scope"`            // search_scope
	UserID                types.String `tfsdk:"user_id"`                 // map_user_id
	Username              types.String `tfsdk:"username"`                // map_username
	RealName              types.String `tfsdk:"real_name"`               // map_realname
	EmailAddress          types.String `tfsdk:"email_address"`           // map_email_address
	AppendToEmailResults  types.String `tfsdk:"append_to_email_results"` // append_to_email_results
	Department            types.String `tfsdk:"department"`              // map_department
	Building              types.String `tfsdk:"building"`                // map_building
	Room                  types.String `tfsdk:"room"`                    // map_room
	Phone                 types.String `tfsdk:"phone"`                   // map_phone
	Position              types.String `tfsdk:"position"`                // map_position
	UserUUID              types.String `tfsdk:"user_uuid"`               // map_user_uuid
}

// ldapUserGroupMappingsModel is the User Group Mappings sub-tab.
type ldapUserGroupMappingsModel struct {
	ObjectClassLimitation types.String `tfsdk:"object_class_limitation"` // map_object_class_to_any_or_all
	ObjectClasses         types.String `tfsdk:"object_classes"`          // object_classes
	SearchBase            types.String `tfsdk:"search_base"`             // search_base
	SearchScope           types.String `tfsdk:"search_scope"`            // search_scope
	GroupID               types.String `tfsdk:"group_id"`                // map_group_id
	GroupName             types.String `tfsdk:"group_name"`              // map_group_name
	GroupUUID             types.String `tfsdk:"group_uuid"`              // map_group_uuid
}

// ldapMembershipMappingsModel is the User Group Membership Mappings sub-tab.
// The admin UI shows different fields depending on `membership_location`
// (Group Object / User Object / Other); the wire echoes a fixed superset, so
// every field is modelled Optional+Computed with no mode gating.
type ldapMembershipMappingsModel struct {
	MembershipLocation                types.String `tfsdk:"membership_location"`                 // user_group_membership_stored_in
	MemberUserMapping                 types.String `tfsdk:"member_user_mapping"`                 // map_user_membership_to_group_field (Group Object: the group's member attr, e.g. member/uniqueMember)
	GroupMembershipMapping            types.String `tfsdk:"group_membership_mapping"`            // map_group_membership_to_user_field (User Object: the user's groups attr, e.g. memberOf)
	AppendToUsername                  types.String `tfsdk:"append_to_username"`                  // append_to_username
	UseDN                             types.Bool   `tfsdk:"use_dn"`                              // use_dn
	UseLDAPCompare                    types.Bool   `tfsdk:"use_ldap_compare"`                    // user_group_membership_use_ldap_compare
	RecursiveLookups                  types.Bool   `tfsdk:"recursive_lookups"`                   // recursive_lookups
	MapUserMembershipUseDN            types.Bool   `tfsdk:"map_user_membership_use_dn"`          // map_user_membership_use_dn
	MembershipCalculationOptimization types.Bool   `tfsdk:"membership_calculation_optimization"` // membership_scoping_optimization
	ObjectClassLimitation             types.String `tfsdk:"object_class_limitation"`             // map_object_class_to_any_or_all
	ObjectClasses                     types.String `tfsdk:"object_classes"`                      // object_classes
	SearchBase                        types.String `tfsdk:"search_base"`                         // search_base
	SearchScope                       types.String `tfsdk:"search_scope"`                        // search_scope
	UsernameMapping                   types.String `tfsdk:"username_mapping"`                    // username
	GroupIDMapping                    types.String `tfsdk:"group_id_mapping"`                    // group_id
	// "Use the 'member' field for select membership queries" (User Object mode).
	UseMemberFieldForSelectQueries types.Bool `tfsdk:"use_member_field_for_select_queries"` // group_membership_enabled_when_user_membership_selected
}

// ldapServerIdentityModel is the identity object for resource imports and
// list-resource identities.
type ldapServerIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// LdapServerListResourceModel is the config model for the list resource.
// Classic /ldapservers has no RSQL, so the filter reuses the shared
// client-side substring block.
type LdapServerListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
