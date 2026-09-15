// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package cloud_identity_provider implements the umbrella
// jamfplatform_pro_cloud_identity_provider resource (Google Secure LDAP +
// Microsoft Entra ID) and the read-only jamfplatform_pro_cloud_identity_provider registry
// data sources.
package cloud_identity_provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: the Cloud Identity Provider endpoints are a long-standing
// part of the Pro API, present at the provider's overall floor. The
// provider-level advisory still fires through providerdata.ConfigurePro when
// the tenant is below ProviderMinJamfProVersion.
const minJamfProVersion = ""

// CloudIdentityProviderResource implements the umbrella Cloud Identity
// Provider resource. A single resource type covers both Google (Secure LDAP)
// and Microsoft Entra ID; `provider_name` discriminates and drives CRUD
// dispatch.
type CloudIdentityProviderResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                     = &CloudIdentityProviderResource{}
	_ resource.ResourceWithImportState      = &CloudIdentityProviderResource{}
	_ resource.ResourceWithIdentity         = &CloudIdentityProviderResource{}
	_ resource.ResourceWithConfigValidators = &CloudIdentityProviderResource{}
	_ resource.ResourceWithModifyPlan       = &CloudIdentityProviderResource{}
)

const (
	defaultCreateTimeout = 90 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 90 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewCloudIdentityProviderResource returns a new instance of the resource.
func NewCloudIdentityProviderResource() resource.Resource {
	return &CloudIdentityProviderResource{}
}

// Metadata sets the resource type name.
func (r *CloudIdentityProviderResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_cloud_identity_provider"
}

// IdentitySchema defines the identifier used for import.
func (r *CloudIdentityProviderResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Cloud Identity Provider ID used to uniquely reference the configuration.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource.
func (r *CloudIdentityProviderResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro Cloud Identity Provider: the integration that lets Jamf Pro look up users and groups in a cloud directory. " +
			"One resource type covers both supported providers; set `provider_name` to choose, and supply the matching nested block (`google` for Google Secure LDAP, `entra_id` for Microsoft Entra ID). " +
			"Changing `provider_name` forces replacement. Multiple Cloud Identity Providers can coexist on a tenant. " +
			"For Microsoft Entra ID, complete the manual **\"refresh consent\"** step in the Jamf Pro admin UI after the first apply: sign into Entra ID and authorise the Jamf cloud connector. Until that consent exists the connection is unusable, and Entra rejects later updates." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Cloud Identity Provider ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display name for the Cloud Identity Provider. Shown in the Jamf Pro admin UI and in the Cloud Identity Provider registry. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"provider_name": schema.StringAttribute{
				MarkdownDescription: "Cloud identity provider type. One of `GOOGLE` (Google Secure LDAP) or `ENTRA_ID` (Microsoft Entra ID). Selects which nested block is required. Changing this forces replacement.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(providerGoogle, providerEntraID),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"google": schema.SingleNestedAttribute{
				MarkdownDescription: "Google Secure LDAP configuration. Required when `provider_name = \"GOOGLE\"`; must be omitted otherwise (a plan-time error fires if it does not match `provider_name`).",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"server": schema.SingleNestedAttribute{
						MarkdownDescription: "Google LDAP server connection configuration.",
						Required:            true,
						Attributes: map[string]schema.Attribute{
							"server_url":         defaultedString("**\"Server\"** in the Jamf Pro admin UI. Google Secure LDAP hostname. Defaults to `ldap.google.com`.", "ldap.google.com"),
							"domain_name":        requiredString("**\"Domain\"** in the Jamf Pro admin UI. The directory domain (e.g. `example.com`)."),
							"port":               defaultedInt64("**\"Port\"** in the Jamf Pro admin UI. LDAPS port. Defaults to `636`.", 636),
							"connection_type":    defaultedStringOneOf("**\"Connection type\"** in the Jamf Pro admin UI. Defaults to `LDAPS`.", pro.CloudLdapServerRequestConnectionTypeLdaps, []string{pro.CloudLdapServerRequestConnectionTypeLdaps}),
							"connection_timeout": defaultedInt64("**\"Connection timeout\"** in the Jamf Pro admin UI, in seconds. Defaults to `15`.", 15),
							"search_timeout":     defaultedInt64("**\"Search timeout\"** in the Jamf Pro admin UI, in seconds. Defaults to `60`.", 60),
							"use_wildcards":      defaultedBool("**\"Use wildcards\"** in the Jamf Pro admin UI. Defaults to `true`.", true),
							"enabled":            defaultedBool("Whether the Google LDAP connection is enabled. Defaults to `true`.", true),
							"membership_calculation_optimization_enabled": defaultedBool("Whether membership-calculation optimization is enabled. Defaults to `false`.", false),
							"keystore": schema.SingleNestedAttribute{
								MarkdownDescription: "Google LDAP client certificate (keystore). Required: Google Secure LDAP authenticates with a client certificate. Supply `file` (base64 of the `.p12`) and `password`; both are `WriteOnly`. Bump `version` to re-upload (rotate) the certificate on a later apply.",
								Required:            true,
								Attributes: map[string]schema.Attribute{
									"file": schema.StringAttribute{
										MarkdownDescription: "Base64-encoded PKCS#12 (`.p12`) client certificate. `WriteOnly`: sent to Jamf Pro on writes but **never persisted in Terraform state**. Idiomatic usage: `file = filebase64(\"google-ldap.p12\")`. Must be supplied together with `password`.",
										Optional:            true,
										Sensitive:           true,
										WriteOnly:           true,
										Validators: []validator.String{
											stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("password")),
										},
									},
									"password": schema.StringAttribute{
										MarkdownDescription: "Password protecting the PKCS#12 keystore. `WriteOnly`: sent to Jamf Pro on writes but **never persisted in Terraform state**. Must be supplied together with `file`.",
										Optional:            true,
										Sensitive:           true,
										WriteOnly:           true,
										Validators: []validator.String{
											stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("file")),
										},
									},
									"wo_version": schema.Int64Attribute{
										MarkdownDescription: "Rotation trigger for the `WriteOnly` keystore (`file` and `password`). Bump this integer (any change) to force the next apply to re-upload the keystore. Initial create should set `wo_version = 1`. Leaving it unset or unchanged leaves the stored keystore alone: the provider omits the keystore from the next update, so Jamf Pro retains the existing certificate.",
										Optional:            true,
									},
									"file_name":       nestedOptString("File name recorded for the uploaded keystore. Returned by Jamf Pro when omitted."),
									"type":            computedString("Keystore type (e.g. `PKCS12`). Read-only."),
									"subject":         computedString("Certificate subject distinguished name. Read-only."),
									"expiration_date": computedString("Certificate expiration date (ISO-8601). Read-only."),
								},
							},
						},
					},
					"mappings": schema.SingleNestedAttribute{
						MarkdownDescription: "Attribute mappings for users and groups. The block is all-or-nothing. Omit it to let Jamf Pro generate the standard Google defaults, or supply it and specify every field across all three sub-blocks. A partial block sends empty values for the fields you leave out; it does not merge with the defaults.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"user_mappings": schema.SingleNestedAttribute{
								MarkdownDescription: "User attribute mappings.",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"object_class_limitation": nestedOptString("Object-class limitation (e.g. `ANY_OBJECT_CLASSES`)."),
									"object_classes":          nestedOptString("Object classes (e.g. `inetOrgPerson`)."),
									"search_base":             nestedOptString("User search base (e.g. `ou=Users`)."),
									"search_scope":            nestedOptString("User search scope (e.g. `ALL_SUBTREES`)."),
									"additional_search_base":  nestedOptString("Additional user search base. When `mappings` is supplied, this must be a valid LDAP distinguished name (e.g. `ou=Users`); Jamf Pro rejects an empty value."),
									"user_id":                 nestedOptString("Attribute mapped to user ID."),
									"username":                nestedOptString("Attribute mapped to username."),
									"real_name":               nestedOptString("Attribute mapped to real name."),
									"email_address":           nestedOptString("Attribute mapped to email address."),
									"department":              nestedOptString("Attribute mapped to department."),
									"building":                nestedOptString("Attribute mapped to building."),
									"room":                    nestedOptString("Attribute mapped to room."),
									"phone":                   nestedOptString("Attribute mapped to phone."),
									"position":                nestedOptString("Attribute mapped to position."),
									"user_uuid":               nestedOptString("Attribute mapped to user UUID."),
								},
							},
							"group_mappings": schema.SingleNestedAttribute{
								MarkdownDescription: "Group attribute mappings.",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"object_class_limitation": nestedOptString("Object-class limitation (e.g. `ANY_OBJECT_CLASSES`)."),
									"object_classes":          nestedOptString("Object classes (e.g. `groupOfNames`)."),
									"search_base":             nestedOptString("Group search base (e.g. `ou=Groups`)."),
									"search_scope":            nestedOptString("Group search scope (e.g. `ALL_SUBTREES`)."),
									"group_id":                nestedOptString("Attribute mapped to group ID."),
									"group_name":              nestedOptString("Attribute mapped to group name."),
									"group_uuid":              nestedOptString("Attribute mapped to group UUID."),
								},
							},
							"membership_mappings": schema.SingleNestedAttribute{
								MarkdownDescription: "Group-membership attribute mapping.",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"group_membership_mapping": nestedOptString("Attribute mapped to group membership (e.g. `memberOf`)."),
								},
							},
						},
					},
				},
			},

			"entra_id": schema.SingleNestedAttribute{
				MarkdownDescription: "Microsoft Entra ID configuration. Required when `provider_name = \"ENTRA_ID\"`; must be omitted otherwise (a plan-time error fires if it does not match `provider_name`). " +
					"After the first apply, complete the manual \"refresh consent\" step in the Jamf Pro admin UI to authorise the Entra ID connection.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"tenant_id": schema.StringAttribute{
						MarkdownDescription: "The Microsoft Entra ID tenant (directory) ID: the GUID identifying your Entra ID tenant. Take it from the Microsoft Entra admin center; it is not entered in the Jamf Pro UI. Changing it forces replacement, because the connection's tenant cannot be updated in place.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"search_timeout": defaultedInt64("Search timeout in seconds. Defaults to `30`.", 30),
					"enabled":        defaultedBool("Whether the Entra ID connection is enabled. Defaults to `true`.", true),
					"membership_calculation_optimization_enabled": defaultedBool("Whether membership-calculation optimization is enabled. Defaults to `false`.", false),
					"transitive_membership_enabled":               defaultedBool("Whether transitive group membership is enabled. Defaults to `false`.", false),
					"transitive_membership_user_field":            defaultedString("User field used for transitive membership lookups. Defaults to `userPrincipalName`.", "userPrincipalName"),
					"transitive_directory_membership_enabled":     defaultedBool("Whether transitive directory membership is enabled. Defaults to `false`.", false),
					"type":               computedString("Entra connection type (e.g. `PUBLIC`). Read-only."),
					"migrated":           computedBool("Whether the connection has been migrated. Read-only."),
					"deprecated_consent": computedBool("Whether the connection uses a deprecated consent flow. Read-only."),
					"mappings": schema.SingleNestedAttribute{
						MarkdownDescription: "Entra ID attribute mappings. Omit the block to leave the connection's current mappings untouched. Jamf Pro supplies no defaults here, so a connection created without the block has all eleven mappings empty. Declare it and Terraform owns every field: a field you leave out inside the block clears that mapping.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"user_id":    nestedOptString("Attribute mapped to user ID (e.g. `id`)."),
							"user_name":  nestedOptString("Attribute mapped to username (e.g. `userPrincipalName`)."),
							"real_name":  nestedOptString("Attribute mapped to real name (e.g. `displayName`)."),
							"email":      nestedOptString("Attribute mapped to email (e.g. `mail`)."),
							"department": nestedOptString("Attribute mapped to department."),
							"building":   nestedOptString("Attribute mapped to building."),
							"room":       nestedOptString("Attribute mapped to room."),
							"phone":      nestedOptString("Attribute mapped to phone (e.g. `mobilePhone`)."),
							"position":   nestedOptString("Attribute mapped to position (e.g. `jobTitle`)."),
							"group_id":   nestedOptString("Attribute mapped to group ID (e.g. `id`)."),
							"group_name": nestedOptString("Attribute mapped to group name (e.g. `displayName`)."),
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
func (r *CloudIdentityProviderResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		providerBlockConfigValidator{},
	}
}

// Configure wires the Jamf Pro client into the resource.
func (r *CloudIdentityProviderResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_cloud_identity_provider")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Cloud Identity Provider ID.
func (r *CloudIdentityProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// --- schema attribute constructors --------------------------------------
//
// The schema has a large number of nested Optional+Computed mapping fields.
// These tiny constructors keep the schema readable and centralise the
// plan-modifier choice. Per the provider's nested-attribute rule, nested
// Optional+Computed attributes use UseNonNullStateForUnknown (NOT
// UseStateForUnknown) so server-defaulted values persist without a
// perpetual diff and list/object growth does not trip "was null, now …"
// consistency errors.

// requiredString returns a Required string attribute (length >= 1).
func requiredString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Required:            true,
		Validators: []validator.String{
			stringvalidator.LengthAtLeast(1),
		},
	}
}

// nestedOptString returns an Optional+Computed nested string attribute.
func nestedOptString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseNonNullStateForUnknown(),
		},
	}
}

// computedString returns a Computed-only string echo attribute.
//
// Deliberately NO UseStateForUnknown: these echoes (keystore type/subject/
// expiration_date, Azure type) are recomputed server-side from inputs the user
// can change without the echo being in config — most importantly, rotating the
// WriteOnly keystore (bumping wo_version with a new certificate) changes
// expiration_date/subject. UseStateForUnknown would pin the stale prior value
// into the plan, then the post-apply GET would return the new value →
// "provider produced inconsistent result after apply". Planning these as
// "(known after apply)" on changes is correct.
func computedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Computed:            true,
	}
}

// computedBool returns a Computed-only bool echo attribute. No
// UseStateForUnknown — same rationale as computedString (server-recomputed
// echoes: Azure migrated/deprecated_consent).
func computedBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Computed:            true,
	}
}

// computedInt64 returns a Computed-only int64 attribute, for the numeric
// settings the defaults data source reports.
func computedInt64(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: desc,
		Computed:            true,
	}
}

// defaulted* constructors return Optional+Computed attributes with a static
// default. Used for the Google server connection scalars: a static default
// keeps the value known at plan time (so Create sends a concrete value rather
// than an unknown), while the user can still override. No plan modifier is
// needed — a static default means the value is never unknown.

func defaultedString(desc, def string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		Default:             stringdefault.StaticString(def),
	}
}

func defaultedStringOneOf(desc, def string, values []string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		Default:             stringdefault.StaticString(def),
		Validators: []validator.String{
			stringvalidator.OneOf(values...),
		},
	}
}

func defaultedInt64(desc string, def int64) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		Default:             int64default.StaticInt64(def),
		Validators: []validator.Int64{
			int64validator.AtLeast(1),
		},
	}
}

func defaultedBool(desc string, def bool) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc,
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(def),
	}
}
