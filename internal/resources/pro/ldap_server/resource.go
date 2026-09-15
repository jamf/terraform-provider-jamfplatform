// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package ldap_server implements the jamfplatform_pro_ldap_server resource,
// data source, and list resource backed by the Jamf ProClassic /ldapservers
// API. It manages on-premises / classic directory connections (Active
// Directory, Open Directory, eDirectory, Custom) only — cloud directories
// (Google, Microsoft Entra) are managed by
// jamfplatform_pro_cloud_identity_provider.
package ldap_server

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /ldapservers predates the provider's overall
// floor. The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below
// ProviderMinJamfProVersion.
const minJamfProVersion = ""

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// LdapServerResource implements the Terraform resource for Jamf Pro LDAP
// servers.
type LdapServerResource struct {
	client *proclassic.Client
}

var (
	_ resource.Resource                     = &LdapServerResource{}
	_ resource.ResourceWithImportState      = &LdapServerResource{}
	_ resource.ResourceWithIdentity         = &LdapServerResource{}
	_ resource.ResourceWithConfigValidators = &LdapServerResource{}
)

// NewLdapServerResource returns a new instance of LdapServerResource.
func NewLdapServerResource() resource.Resource {
	return &LdapServerResource{}
}

// Metadata sets the resource type name.
func (r *LdapServerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_ldap_server"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *LdapServerResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro LDAP server ID used to uniquely reference the server.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the LDAP server resource.
func (r *LdapServerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro on-premises LDAP server (Settings → \"LDAP Servers\"). Use this resource for classic, directly-reachable directories: Active Directory, Apple Open Directory, Novell eDirectory, or a manually-configured (`Custom`) LDAP server. Cloud directories (Google, Microsoft Entra) are managed by `jamfplatform_pro_cloud_identity_provider` instead.\n\nThe `connection_settings` block carries the server identity, transport, authentication, and (for non-anonymous binds) the lookup account. The `mappings_for_users` block maps directory attributes onto Jamf Pro user, user-group and membership fields. The bind `password` is a Terraform `WriteOnly` attribute, sent to Jamf Pro on writes but never persisted in state; pair it with `password_wo_version` to rotate.\n\nThis resource also defines the directories Jamf Pro searches when resolving directory-service user groups for scoping." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "LDAP server ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"connection_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "Server connection settings. The **Connection** tab in the admin UI.",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"display_name":        reqString("**\"Display Name\"** in the Jamf Pro admin UI. Display name for the LDAP server. Must not be empty."),
					"directory_service":   reqStringOneOf("**\"Directory Service\"** in the Jamf Pro admin UI. The directory product. Valid values: `\"Active Directory\"` (UI \"Microsoft's Active Directory\"), `\"Open Directory\"` (UI \"Apple's Open Directory\"), `\"eDirectory\"` (UI \"Novell's eDirectory\"), `\"Custom\"` (UI \"Configure Manually\").", allServerTypes),
					"hostname":            reqString("**\"Server and Port\"** (host) in the Jamf Pro admin UI. Hostname or IP address of the LDAP server."),
					"port":                optInt64("**\"Server and Port\"** (port) in the Jamf Pro admin UI. Jamf Pro defaults it to 389 (or 636 for LDAPS) on create."),
					"use_ssl":             optBool("**\"Use SSL\"** in the Jamf Pro admin UI. Connect to the LDAP server over SSL/LDAPS."),
					"authentication_type": optStringOneOf("**\"Authentication Type\"** in the Jamf Pro admin UI. Bind authentication mechanism. Valid values (case-sensitive): `none` (anonymous bind: omit the `account` block), `simple`, `CRAM-MD5`, `DIGEST-MD5`.", allAuthenticationTypes),

					"account": schema.SingleNestedAttribute{
						MarkdownDescription: "**\"LDAP Server Account\"** in the Jamf Pro admin UI. Lookup/bind account credentials. Required when `authentication_type` is anything other than `none`; omit entirely for anonymous binds. " + preserveOnOmitBlock + " To fully remove a bind account from an existing server, recreate the server.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"distinguished_username": schema.StringAttribute{
								MarkdownDescription: "**\"Distinguished Username\"** in the Jamf Pro admin UI. Distinguished name of the bind account (e.g. `CN=svc,CN=Users,DC=example,DC=com`) or another type-specific identifier. " + preserveOnOmitString,
								Optional:            true,
							},
							"password": schema.StringAttribute{
								MarkdownDescription: "**\"Password\"** in the Jamf Pro admin UI. Plaintext bind password. `WriteOnly`: sent to Jamf Pro on writes, never persisted in Terraform state. Jamf Pro never returns the plaintext on read, so rotation is driven by the companion `password_wo_version` integer.",
								Optional:            true,
								Sensitive:           true,
								WriteOnly:           true,
							},
							"password_wo_version": schema.Int64Attribute{
								MarkdownDescription: "Rotation trigger for the `WriteOnly` `password`. Bump this integer (any change) to force the next update to re-send `password`. Set `password_wo_version = 1` on create. Leaving it unset or unchanged signals \"leave the stored password alone\": the provider omits the password from the next update, so Jamf Pro retains the existing value.",
								Optional:            true,
							},
						},
					},

					"connection_timeout": optInt64("**\"Connection Timeout\"** in the Jamf Pro admin UI. Seconds to wait before cancelling a connection attempt. Jamf Pro defaults it to 15 on create."),
					"search_timeout":     optInt64("**\"Search Timeout\"** in the Jamf Pro admin UI. Seconds to wait before cancelling a search request. Jamf Pro defaults it to 60 on create."),
					"referral_response":  optStringOneOfPreserving("**\"Referral Response\"** in the Jamf Pro admin UI. Action when an LDAP referral is received. Valid values (lower-case): `\"\"` (use default from LDAP service), `follow`, `ignore`.", allReferralResponses, "Omit to leave the current value untouched; set `\"\"` to fall back to the LDAP service default."),
					"use_wildcards":      optBool("**\"Use Wildcards When Searching\"** in the Jamf Pro admin UI. Allow partial matches in directory searches. Jamf Pro defaults it to true on create."),

					"is_enabled":        computedBool("Whether the LDAP server connection is enabled. Returned by Jamf Pro; not user-settable."),
					"migrated_to_id":    computedInt64("ID of the Cloud Identity Provider this server was migrated to, or 0 if not migrated. Returned by Jamf Pro; not user-settable."),
					"certificates_used": computedString("Summary of how the server's certificates are used. Returned by Jamf Pro; not user-settable."),
				},
			},

			"mappings_for_users": schema.SingleNestedAttribute{
				MarkdownDescription: "Attribute mappings. The **Mappings** tab in the admin UI. Declare only the sub-blocks you want Terraform to manage; sub-blocks you omit are left as-is and not tracked.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"user_mappings": schema.SingleNestedAttribute{
						MarkdownDescription: "**User Mappings** sub-tab. Maps directory attributes onto Jamf Pro user fields. " + preserveOnOmitBlock,
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"object_class_limitation": optStringOneOf("**\"Object Class Limitation\"** in the Jamf Pro admin UI. `any` (\"Any ObjectClass Values\") or `all` (\"All ObjectClass Values\").", allObjectClassLimits),
							"object_classes":          optString("**\"Object Class(es)\"** in the Jamf Pro admin UI. Comma-separated object classes to limit results to (e.g. `organizationalPerson`)."),
							"search_base":             optString("**\"Search Base\"** in the Jamf Pro admin UI. Distinguished name of the user search base."),
							"search_scope":            optStringOneOf("**\"Search Scope\"** in the Jamf Pro admin UI. `All Subtrees` or `First Level Only`.", allSearchScopes),
							"user_id":                 optString("**\"User ID\"** mapping. Directory attribute mapped to the Jamf Pro user ID."),
							"username":                optString("**\"Username\"** mapping. Directory attribute mapped to the Jamf Pro username."),
							"real_name":               optString("**\"Real Name\"** mapping."),
							"email_address":           optString("**\"Email Address\"** mapping."),
							"append_to_email_results": optString("**\"Append to Email Results\"** in the Jamf Pro admin UI. Text appended to email lookups (e.g. `@mycompany.com`)."),
							"department":              optString("**\"Department\"** mapping."),
							"building":                optString("**\"Building\"** mapping."),
							"room":                    optString("**\"Room\"** mapping."),
							"phone":                   optString("**\"Phone\"** mapping."),
							"position":                optString("**\"Position\"** mapping."),
							"user_uuid":               optString("**\"User UUID\"** mapping (e.g. `objectGUID`)."),
						},
					},
					"user_group_mappings": schema.SingleNestedAttribute{
						MarkdownDescription: "**User Group Mappings** sub-tab. Maps directory attributes onto Jamf Pro user-group fields. " + preserveOnOmitBlock,
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"object_class_limitation": optStringOneOf("**\"Object Class Limitation\"** in the Jamf Pro admin UI. `any` or `all`.", allObjectClassLimits),
							"object_classes":          optString("**\"Object Class(es)\"** in the Jamf Pro admin UI. Comma-separated object classes (e.g. `group`)."),
							"search_base":             optString("**\"Search Base\"** in the Jamf Pro admin UI. Distinguished name of the group search base."),
							"search_scope":            optStringOneOf("**\"Search Scope\"** in the Jamf Pro admin UI. `All Subtrees` or `First Level Only`.", allSearchScopes),
							"group_id":                optString("**\"Group ID\"** mapping."),
							"group_name":              optString("**\"Group Name\"** mapping (e.g. `sAMAccountName`)."),
							"group_uuid":              optString("**\"Group UUID\"** mapping (e.g. `objectGUID`)."),
						},
					},
					"user_group_membership_mappings": schema.SingleNestedAttribute{
						MarkdownDescription: "**User Group Membership Mappings** sub-tab. Controls how directory group membership is resolved. All fields are optional; the set you populate depends on `membership_location`. " + preserveOnOmitBlock,
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"membership_location":                 optStringOneOf("**\"Membership Location\"** in the Jamf Pro admin UI. Where group memberships are stored: `group object` or `user object`. The admin UI's \"Other\" choice corresponds to one of these two values combined with the object-class, search, username, and group-id fields below.", allMembershipLocations),
							"member_user_mapping":                 optString("**\"Member User Mapping\"** in the Jamf Pro admin UI (Group Object mode). Directory attribute mapping member users to a group (e.g. `member`)."),
							"group_membership_mapping":            optString("**\"Group Membership Mapping\"** in the Jamf Pro admin UI (User Object mode). Directory attribute mapping a user to their groups (e.g. `memberOf`)."),
							"append_to_username":                  optString("**\"Append to Username When Searching\"** in the Jamf Pro admin UI."),
							"use_dn":                              optBool("**\"Use distinguished name of member user when searching\"** in the Jamf Pro admin UI."),
							"use_ldap_compare":                    optBool("**\"Use the LDAP compare operation when searching\"** in the Jamf Pro admin UI."),
							"recursive_lookups":                   optBool("**\"Use recursive group searches\"** in the Jamf Pro admin UI."),
							"map_user_membership_use_dn":          optBool("**\"Use distinguished name of user groups when searching\"** in the Jamf Pro admin UI (User Object mode)."),
							"membership_calculation_optimization": optBool("**\"Membership calculation optimization\"** in the Jamf Pro admin UI."),
							"object_class_limitation":             optStringOneOf("**\"Object Class Limitation\"** in the Jamf Pro admin UI (Other mode). `any` or `all`.", allObjectClassLimits),
							"object_classes":                      optString("**\"Object Class(es)\"** in the Jamf Pro admin UI (Other mode)."),
							"search_base":                         optString("**\"Search Base\"** in the Jamf Pro admin UI (Other mode)."),
							"search_scope":                        optStringOneOf("**\"Search Scope\"** in the Jamf Pro admin UI (Other mode). `All Subtrees` or `First Level Only`.", allSearchScopes),
							"username_mapping":                    optString("**\"Username Mapping\"** in the Jamf Pro admin UI (Other mode)."),
							"group_id_mapping":                    optString("**\"Group ID Mapping\"** in the Jamf Pro admin UI (Other mode)."),
							"use_member_field_for_select_queries": optBool("**\"Use the 'member' field for select membership queries\"** in the Jamf Pro admin UI (User Object mode). The `member` field must be present on the LDAP group object for the query to succeed; enabling this improves performance of some membership queries."),
						},
					},
				},
			},

			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// ConfigValidators returns the cross-field validators evaluated at plan time.
func (r *LdapServerResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		accountAuthConfigValidator{},
	}
}

// Configure wires the Jamf ProClassic client into the resource.
func (r *LdapServerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_ldap_server")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro LDAP server ID.
func (r *LdapServerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
