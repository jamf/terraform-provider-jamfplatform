// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package directory_binding implements the jamfplatform_pro_directory_binding
// resource, data source, and list resource backed by the Jamf ProClassic
// /directorybindings API.
package directory_binding

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /directorybindings predates the provider's overall
// floor. The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below
// ProviderMinJamfProVersion.
const minJamfProVersion = ""

// DirectoryBindingResource implements the Terraform resource for Jamf Pro
// directory bindings.
type DirectoryBindingResource struct {
	client *proclassic.Client
	// impact backs the plan-time impact alert reporting how many computers this
	// object reaches through the policies that use it. nil when the provider's
	// impact_alerts attribute is off, which is the default.
	impact *impact.Cache
}

var (
	_ resource.Resource                     = &DirectoryBindingResource{}
	_ resource.ResourceWithImportState      = &DirectoryBindingResource{}
	_ resource.ResourceWithIdentity         = &DirectoryBindingResource{}
	_ resource.ResourceWithConfigValidators = &DirectoryBindingResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewDirectoryBindingResource returns a new instance of
// DirectoryBindingResource.
func NewDirectoryBindingResource() resource.Resource {
	return &DirectoryBindingResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *DirectoryBindingResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_directory_binding"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *DirectoryBindingResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro directory binding ID used to uniquely reference the binding.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the directory binding resource.
func (r *DirectoryBindingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro directory binding. Directory bindings are reusable definitions Jamf Pro policies use to join Mac computers to an Active Directory, Open Directory, PowerBroker, ADmitMac or Centrify directory service. The resource carries flat top-level fields (name, priority, type, domain, username, password, computer_ou) plus exactly one of five per-type nested blocks, selected by `type`. A plan-time cross-field validator checks that the supplied nested block matches `type`. The plaintext `password` is a Terraform `WriteOnly` attribute, sent to Jamf Pro but never persisted in Terraform state. Pair it with `password_wo_version` to rotate: bump the integer to force a new update carrying the current `password` value." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Directory binding ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Directory binding display name. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"priority": schema.Int64Attribute{
				MarkdownDescription: "Binding priority, in the range 1–10. Lower numbers run earlier when Jamf Pro evaluates multiple bindings. " + preserveOnOmitInteger,
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.Between(1, 10),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Directory service type. Selects which nested block is permitted. One of `\"Active Directory\"`, `\"Open Directory\"`, `\"PowerBroker Identity Services\"`, `\"ADmitMac\"`, `\"Centrify\"`. The admin UI labels the Open Directory option \"Apple Open Directory\", but the value Jamf Pro stores is `\"Open Directory\"`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(allDirectoryBindingTypes...),
				},
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "**\"Domain Server\"** in the Jamf Pro admin UI. The interpretation depends on `type`: DNS domain for Active Directory, LDAP host for Open Directory, and bind domain for PowerBroker, ADmitMac and Centrify.",
				Optional:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "**\"Username\"** in the Jamf Pro admin UI. The directory account that performs the bind. May be a domain account name, an LDAP distinguished name, or another type-specific identifier.",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "**\"Password\"** in the Jamf Pro admin UI. Plaintext bind password. `WriteOnly`: the value is sent to Jamf Pro on writes but **never persisted in Terraform state**. Jamf Pro also never returns the plaintext on read, so the only signal Terraform can use to rotate the stored password is the companion `password_wo_version` integer. Bump it to trigger a new update carrying the current `password`.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `password`. Bump this integer (any change) to force a new update that re-sends `password` to Jamf Pro. Initial create should set `password_wo_version = 1`. Leaving the attribute unset or unchanged leaves the stored password alone: the provider omits the password from the next update, so Jamf Pro retains the existing value.",
				Optional:            true,
			},
			"computer_ou": schema.StringAttribute{
				MarkdownDescription: "Computer object's organisational unit (OU) within the directory. Free text. The format is type-specific (e.g. an LDAP-style `OU=...` path for Active Directory).",
				Optional:            true,
			},

			"active_directory": schema.SingleNestedAttribute{
				MarkdownDescription: "Active Directory–specific configuration. May only be set when `type = \"Active Directory\"`; setting it for any other type is a plan-time error. " + preserveOnOmitBlock + " When you supply the block, Jamf Pro applies a default for any inner field you omit, and Terraform records the value it applied.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"forest":                     optString("Active Directory forest."),
					"create_mobile_account":      optBool("**\"Create Mobile Account\"** in the Jamf Pro admin UI. Cache the directory user's account on the bound Mac for offline login."),
					"require_confirmation":       optBool("**\"Require confirmation before creating a mobile account\"** in the Jamf Pro admin UI."),
					"force_local_home_directory": optBool("**\"Force local home directory on startup disk\"** in the Jamf Pro admin UI."),
					"use_unc_path":               optBool("**\"Use UNC path from Active Directory to derive network home location\"** in the Jamf Pro admin UI."),
					"network_protocol":           optString("**\"Network Protocol\"** in the Jamf Pro admin UI. Network protocol used to mount the user's home (e.g. `smb` or `afp`)."),
					"default_shell":              optString("**\"Default User Shell\"** in the Jamf Pro admin UI. Login shell assigned to bound directory users (e.g. `/bin/bash`)."),
					"uid_attribute_mapping":      optString("**\"Map UID to attribute\"** in the Jamf Pro admin UI. Name of the AD attribute that supplies the POSIX UID."),
					"user_gid_attribute_mapping": optString("**\"Map User GID to attribute\"** in the Jamf Pro admin UI. Name of the AD attribute that supplies the per-user primary group GID."),
					"gid_attribute_mapping":      optString("**\"Map Group GID to attribute\"** in the Jamf Pro admin UI. Name of the AD attribute that supplies the group GID."),
					"multiple_domains":           optBool("**\"Allow authentication from any domain in the forest\"** in the Jamf Pro admin UI."),
					"preferred_domain":           optString("**\"Preferred Domain Server\"** in the Jamf Pro admin UI. Preferred AD domain controller hostname."),
					"admin_groups":               optString("**\"Allow administration by\"** in the Jamf Pro admin UI. Comma-separated list of AD groups whose members are granted local admin rights on bound Macs."),
				},
			},

			"open_directory": schema.SingleNestedAttribute{
				MarkdownDescription: "Open Directory–specific configuration. May only be set when `type = \"Open Directory\"`; setting it for any other type is a plan-time error. " + preserveOnOmitBlock + " When you supply the block, Jamf Pro applies a default for any inner field you omit, and Terraform records the value it applied.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"encrypt_using_ssl":      optBool("**\"Encrypt using SSL\"** in the Jamf Pro admin UI. Encrypt the LDAP connection to the directory."),
					"perform_secure_bind":    optBool("**\"Perform secure bind\"** in the Jamf Pro admin UI. Use a secure (authenticated) bind operation."),
					"use_for_authentication": optBool("**\"Use for Authentication\"** in the Jamf Pro admin UI."),
					"use_for_contacts":       optBool("**\"Use for Contacts\"** in the Jamf Pro admin UI."),
				},
			},

			"admitmac": schema.SingleNestedAttribute{
				MarkdownDescription: "ADmitMac–specific configuration. May only be set when `type = \"ADmitMac\"`; setting it for any other type is a plan-time error. " + preserveOnOmitBlock + " When you supply the block, Jamf Pro applies a default for any inner field you omit, and Terraform records the value it applied.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"require_confirmation":       optBool("**\"Require confirmation\"** in the Jamf Pro admin UI. Require admin confirmation when binding new computers to the directory."),
					"home_location":              optString("**\"Home Location\"** in the Jamf Pro admin UI. Where to create the user's home folder (e.g. `\"Local\"`). Free text."),
					"network_protocol":           optString("**\"Network Protocol\"** in the Jamf Pro admin UI. Network protocol used to mount the user's home (e.g. `smb` or `afp`)."),
					"default_shell":              optString("**\"Default User Shell\"** in the Jamf Pro admin UI."),
					"mount_network_home":         optBool("**\"Mount network home as sharepoint\"** in the Jamf Pro admin UI."),
					"place_home_folders":         optString("**\"Place home folders in\"** in the Jamf Pro admin UI. Filesystem path under which local home folders are placed."),
					"uid_attribute_mapping":      optString("**\"Map UID to attribute\"** in the Jamf Pro admin UI. Name of the directory attribute that supplies the POSIX UID."),
					"user_gid_attribute_mapping": optString("**\"Map User GID to attribute\"** in the Jamf Pro admin UI. Name of the directory attribute that supplies the per-user primary group GID."),
					"gid_attribute_mapping":      optString("**\"Map Group GID to attribute\"** in the Jamf Pro admin UI. Name of the directory attribute that supplies the group GID."),
					"admin_group":                optString("**\"Allow administration by\"** in the Jamf Pro admin UI. Directory group whose members are granted local admin rights on bound Macs."),
					"cached_credentials": schema.Int64Attribute{
						MarkdownDescription: "**\"Cached credentials\"** in the Jamf Pro admin UI. Number of users whose credentials are cached for offline login. " + preserveOnOmitInteger,
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
					},
					"add_user_to_local": optBool("**\"Add user to local administrators group\"** in the Jamf Pro admin UI."),
					"users_ou":          optString("**\"Users OU\"** in the Jamf Pro admin UI."),
					"groups_ou":         optString("**\"Groups OU\"** in the Jamf Pro admin UI."),
					"printers_ou":       optString("**\"Printers OU\"** in the Jamf Pro admin UI."),
					"shared_folders_ou": optString("**\"Shared Folders OU\"** in the Jamf Pro admin UI."),
				},
			},

			"centrify": schema.SingleNestedAttribute{
				MarkdownDescription: "Centrify–specific configuration. May only be set when `type = \"Centrify\"`; setting it for any other type is a plan-time error. " + preserveOnOmitBlock + " When you supply the block, Jamf Pro applies a default for any inner field you omit, and Terraform records the value it applied.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"workstation_mode":        optBool("Bind in Workstation mode (versus joined mode)."),
					"overwrite_existing":      optBool("Overwrite an existing Centrify configuration on the target Mac."),
					"update_pam":              optBool("Update PAM configuration to integrate Centrify authentication."),
					"zone":                    optString("Centrify zone name."),
					"preferred_domain_server": optString("Preferred Centrify domain server hostname."),
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

// ConfigValidators returns the cross-field validators evaluated against the
// user's config at plan time.
func (r *DirectoryBindingResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		typeBlockConfigValidator{},
	}
}

// Configure wires the Jamf ProClassic client into the resource via the
// shared providerdata.ConfigureProClassic helper.
func (r *DirectoryBindingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_directory_binding")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.impact = providerdata.ConfigureImpact(req.ProviderData)
	r.client = client
}

// ImportState handles import by the Jamf Pro directory binding ID.
func (r *DirectoryBindingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// preserveOnOmitString, preserveOnOmitBool, preserveOnOmitInteger and
// preserveOnOmitBlock are the house sentences every preserve-on-omit
// description ends with, per STYLE_GUIDE.md §Full-replace endpoints & shared
// backing stores ("Description convention"). The classic /directorybindings
// update merges field by field (see crud.go), so omitting an attribute — or a
// whole per-type block — leaves the stored Jamf Pro value alone. The wording
// differs by type because only a string has a blank that clears.
const (
	preserveOnOmitString  = "Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it."
	preserveOnOmitBool    = "Omit to leave the current value untouched; set `true`/`false` to change it."
	preserveOnOmitInteger = "Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it."
	preserveOnOmitBlock   = "Omit the block to leave any existing values untouched (they are not cleared on update)."
)

// optString is a tiny shorthand for an Optional+Computed schema.StringAttribute
// with the canonical UseStateForUnknown plan modifier and a
// MarkdownDescription, to which it appends preserveOnOmitString — every field
// built through it reaches the wire as a drop-on-null pointer, so omitting it
// preserves the stored value. The nested per-type blocks have many fields and
// repeating the full struct literal made the schema unreadable.
//
// Why Optional+Computed: the Jamf Pro server populates every per-type
// nested field with a default when the user omits it (e.g. a Centrify
// binding created with only `zone` set comes back with
// `workstation_mode=false`, `update_pam=true`, etc.). Per STYLE_GUIDE
// §Server-derived computed fields, attributes the server can fill in
// when omitted must be Optional+Computed. UseStateForUnknown keeps the
// prior state value across plans so the framework does not surface
// every refresh as a transient diff. Used only inside this file.
func optString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc + " " + preserveOnOmitString,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

// optBool is the bool counterpart of optString — same Optional+Computed
// + UseStateForUnknown rationale, same scope, with preserveOnOmitBool
// appended instead.
func optBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: desc + " " + preserveOnOmitBool,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.UseStateForUnknown(),
		},
	}
}
