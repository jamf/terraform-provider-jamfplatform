// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package sso_settings implements the jamfplatform_pro_sso_settings singleton
// resource and data source. It wraps the Jamf Pro Single Sign-On configuration
// surface (/v3/sso + embedded /v2/sso/cert), the SAML/OIDC sub-blocks, and the
// embedded signing certificate sub-block that drives separate cert CRUD calls.
package sso_settings

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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required.
// Empty: /v3/sso ships at the provider's overall floor.
const minJamfProVersion = ""

// SsoSettingsResource implements the singleton Jamf Pro SSO settings resource.
//
// The resource is backed by an Update-only Jamf Pro API — one SSO object per
// tenant. Create funnels into Update. Delete is state-only by design.
type SsoSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &SsoSettingsResource{}
var _ resource.ResourceWithImportState = &SsoSettingsResource{}
var _ resource.ResourceWithIdentity = &SsoSettingsResource{}

// Default timeouts.
const (
	defaultCreateTimeout = 30 * time.Second
	defaultReadTimeout   = 30 * time.Second
	defaultUpdateTimeout = 30 * time.Second
	defaultDeleteTimeout = 30 * time.Second
)

// NewSsoSettingsResource constructs a new SsoSettingsResource.
func NewSsoSettingsResource() resource.Resource {
	return &SsoSettingsResource{}
}

// Metadata sets the resource type name.
func (r *SsoSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_sso_settings"
}

// IdentitySchema defines the import identifier — singleton id only.
func (r *SsoSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\".",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the resource schema.
func (r *SsoSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro **Single Sign-On (SSO)** settings (UI: Settings → System → Single Sign-On). One record per tenant. Combines the SSO configuration with an embedded `signing_certificate` sub-block that manages the SAML signing keystore as a single resource.\n\n" +
			"### What Terraform owns\n\n" +
			"Terraform owns every option you declare: change one in the admin console and the next plan reports drift. An option you leave out keeps whatever the tenant already holds, on the first apply as well as later ones, so you can hand SSO over without transcribing every setting you want to keep. Set an empty string to clear a field.\n\n" +
			"### Cross-field requirements\n\n" +
			"All of these are enforced at plan time.\n\n" +
			"- `configuration_type = \"SAML\"` requires the `saml_settings` block.\n" +
			"- `configuration_type = \"OIDC\"` requires the `oidc_settings` block and forbids `saml_settings` (Jamf Pro ignores SAML configuration in pure OIDC mode).\n" +
			"- `configuration_type = \"OIDC_WITH_SAML\"` requires both `saml_settings` and `oidc_settings`.\n" +
			"- `saml_settings.entity_id` and `saml_settings.group_attribute_name` must be non-empty whenever SAML is part of the configuration.\n" +
			"- `saml_settings.metadata_source = \"URL\"` requires `idp_url` and forbids `federation_metadata_file` / `metadata_file_name`; `= \"FILE\"` is the inverse.\n" +
			"- `saml_settings.idp_provider_type = \"OTHER\"` requires `other_provider_type_name`.\n" +
			"- `saml_settings.user_attribute_enabled = true` requires `user_attribute_name`.\n" +
			"- `group_enrollment_access_enabled = true` together with `sso_for_enrollment_enabled = true` requires `group_enrollment_access_name`.\n" +
			"- `signing_certificate.setup_type = \"UPLOADED\"` requires `type`, `key`, `keystore_file`, `keystore_file_name`, `keystore_password`, and `password`.\n\n" +
			"### Account-Driven Enrollment dependency\n\n" +
			"`enrollment_sso_for_account_driven_enrollment_enabled = true` requires Account-Driven Device Enrollment to be enabled on the tenant. Jamf Pro rejects the apply with a field-named error if the prerequisite is missing.\n\n" +
			"### Concurrency\n\n" +
			"Jamf Pro applies SSO changes last-writer-wins with no conflict detection. Use Terraform state-locking to serialise applies that touch this resource.\n\n" +
			"### Destroy\n\n" +
			"`terraform destroy` removes the resource from Terraform state only. The SSO configuration is left intact on the tenant. To actually disable SSO, set `sso_enabled = false` explicitly and apply before destroy. This protects shared tenants where the Platform API depends on SSO remaining enabled.\n\n" +
			"Import with `terraform import jamfplatform_pro_sso_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},

			"sso_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether SSO is enabled on the tenant. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"sso_bypass_allowed": schema.BoolAttribute{
				MarkdownDescription: "Allow administrators to bypass SSO when signing in. Only honored when `configuration_type` includes SAML (`SAML` or `OIDC_WITH_SAML`); Jamf Pro silently coerces the value to `false` in pure OIDC mode. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				Validators: []validator.Bool{
					SsoBypassAllowedValidator(),
				},
			},
			"sso_for_enrollment_enabled": schema.BoolAttribute{
				MarkdownDescription: "Enable SSO for user-initiated enrollment. Only honored when `configuration_type` includes SAML (`SAML` or `OIDC_WITH_SAML`); Jamf Pro silently coerces the value to `false` in pure OIDC mode. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				Validators: []validator.Bool{
					RequiresSamlBoolValidator("sso_for_enrollment_enabled"),
				},
			},
			"sso_for_macos_self_service_enabled": schema.BoolAttribute{
				MarkdownDescription: "Enable SSO for the macOS Self Service app. Only honored when `configuration_type` includes SAML (`SAML` or `OIDC_WITH_SAML`); Jamf Pro silently coerces the value to `false` in pure OIDC mode. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				Validators: []validator.Bool{
					RequiresSamlBoolValidator("sso_for_macos_self_service_enabled"),
				},
			},
			"enrollment_sso_for_account_driven_enrollment_enabled": schema.BoolAttribute{
				MarkdownDescription: "Enable SSO for Account-Driven Enrollment (both User and Device variants). Only honored when `configuration_type` includes SAML (`SAML` or `OIDC_WITH_SAML`); Jamf Pro silently coerces the value to `false` in pure OIDC mode. Also requires Account-Driven Device Enrollment to be enabled on the tenant. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				Validators: []validator.Bool{
					RequiresSamlBoolValidator("enrollment_sso_for_account_driven_enrollment_enabled"),
				},
			},
			"group_enrollment_access_enabled": schema.BoolAttribute{
				MarkdownDescription: "Restrict enrollment SSO to a single LDAP/IdP group. Only honored when `configuration_type` includes SAML (`SAML` or `OIDC_WITH_SAML`); Jamf Pro silently coerces the value to `false` in pure OIDC mode. When set together with `sso_for_enrollment_enabled = true`, `group_enrollment_access_name` must also be supplied. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
				Validators: []validator.Bool{
					RequiresSamlBoolValidator("group_enrollment_access_enabled"),
					GroupEnrollmentAccessEnabledValidator(),
				},
			},
			"group_enrollment_access_name": schema.StringAttribute{
				MarkdownDescription: "Name of the LDAP/IdP group allowed to enroll. Required when `group_enrollment_access_enabled` and `sso_for_enrollment_enabled` are both `true`. Omit to leave any existing value untouched; set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},

			"configuration_type": schema.StringAttribute{
				MarkdownDescription: "SSO configuration type. One of `SAML`, `OIDC`, or `OIDC_WITH_SAML`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validConfigurationTypes...),
					ConfigurationTypeBlockValidator(),
				},
			},

			"oidc_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "OIDC configuration. Required when `configuration_type` is `OIDC` or `OIDC_WITH_SAML`. May be omitted in pure SAML mode.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"user_mapping": schema.StringAttribute{
						MarkdownDescription: "How OIDC claims map to Jamf Pro users. One of `USERNAME` or `EMAIL`.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.OneOf(validUserMappings...),
						},
					},
					"jamf_id_authentication_enabled": schema.BoolAttribute{
						MarkdownDescription: "Allow Jamf ID authentication alongside the configured OIDC provider. Omit to leave the current value untouched; set `true`/`false` to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
					},
					"username_attribute_claim_mapping": schema.StringAttribute{
						MarkdownDescription: "OIDC claim used as the username attribute. One of `USERNAME` or `EMAIL`. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(validUserMappings...),
						},
					},
				},
			},

			"saml_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "SAML configuration. Required when `configuration_type` is `SAML` or `OIDC_WITH_SAML`. Must be omitted in pure OIDC mode.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"idp_provider_type": schema.StringAttribute{
						MarkdownDescription: "SAML IdP type. One of `ADFS`, `OKTA`, `GOOGLE`, `SHIBBOLETH`, `ONELOGIN`, `PING`, `CENTRIFY`, `AZURE`, or `OTHER`. When `OTHER`, `other_provider_type_name` must also be set. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(validIdpProviderTypes...),
							IdpProviderTypeOtherValidator(),
						},
					},
					"other_provider_type_name": schema.StringAttribute{
						MarkdownDescription: "Display name for the IdP when `idp_provider_type = \"OTHER\"`. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"entity_id": schema.StringAttribute{
						MarkdownDescription: "SAML EntityID. Required (non-empty) when `configuration_type` includes SAML. Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							SamlEntityIDRequiredValidator(),
						},
					},
					"metadata_source": schema.StringAttribute{
						MarkdownDescription: "How Jamf Pro obtains IdP SAML metadata. `URL` (Jamf Pro fetches metadata from `idp_url`) or `FILE` (raw base64 supplied in `federation_metadata_file`). The two branches are mutually exclusive. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(validMetadataSources...),
							MetadataSourceBranchValidator(),
						},
					},
					"idp_url": schema.StringAttribute{
						MarkdownDescription: "URL Jamf Pro fetches IdP metadata from. Required when `metadata_source = \"URL\"`. Jamf Pro performs a live HTTP fetch when the resource is applied; the URL is not pre-validated for syntax or reachability by the provider. Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"federation_metadata_file": schema.StringAttribute{
						MarkdownDescription: "Raw base64 of the IdP SAML metadata XML. Required when `metadata_source = \"FILE\"`. Idiomatic usage: `filebase64(\"idp-metadata.xml\")`.",
						Optional:            true,
						Sensitive:           true,
					},
					"metadata_file_name": schema.StringAttribute{
						MarkdownDescription: "Display filename for the uploaded metadata. Required when `metadata_source = \"FILE\"`; must be omitted when `metadata_source = \"URL\"`. Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"session_timeout": schema.Int64Attribute{
						MarkdownDescription: "SAML session timeout in minutes. Upper bound: 35,791,393. Jamf Pro keeps the stored value even when `token_expiration_disabled = true`. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
						Validators: []validator.Int64{
							int64validator.AtMost(35791393),
						},
					},
					"token_expiration_disabled": schema.BoolAttribute{
						MarkdownDescription: "Disable SAML token expiration. When `true`, `session_timeout` becomes runtime-inactive but is still stored. Defaults to `true` on the first apply. Jamf Pro requires an explicit boolean here, so the provider always sends one on update. Omit to leave the current value untouched; set `true`/`false` to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
					},
					"user_mapping": schema.StringAttribute{
						MarkdownDescription: "How SAML attributes map to Jamf Pro users. One of `USERNAME` or `EMAIL`. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							stringvalidator.OneOf(validUserMappings...),
						},
					},
					"user_attribute_enabled": schema.BoolAttribute{
						MarkdownDescription: "Use a custom SAML attribute (`user_attribute_name`) for username lookup instead of NameID. Requires `user_attribute_name` when `true`. Defaults to `false` on the first apply. Jamf Pro requires an explicit boolean here, so the provider always sends one on update. Omit to leave the current value untouched; set `true`/`false` to change it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
						Validators: []validator.Bool{
							UserAttributeEnabledValidator(),
						},
					},
					"user_attribute_name": schema.StringAttribute{
						MarkdownDescription: "Name of the SAML attribute carrying the username. Required when `user_attribute_enabled = true`. Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
					"group_attribute_name": schema.StringAttribute{
						MarkdownDescription: "SAML attribute carrying group claims (e.g. `http://schemas.xmlsoap.org/claims/Group`). Required (non-empty) when `configuration_type` includes SAML. Omit to leave the current value untouched.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
						Validators: []validator.String{
							SamlGroupAttributeNameRequiredValidator(),
						},
					},
					"group_rdn_key": schema.StringAttribute{
						MarkdownDescription: "RDN token (e.g. `CN`, `DC`, `OU`) applied when group claims arrive as full distinguished names. Omit to leave any existing value untouched; set to `\"\"` to clear it.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
					},
				},
			},

			"enrollment_sso_config": schema.SingleNestedAttribute{
				MarkdownDescription: "Configuration consumed by the Account-Driven Enrollment SSO flow.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"hosts": schema.SetAttribute{
						MarkdownDescription: "Trusted IdP hosts (e.g. `dev-12324233.okta.com`).",
						Optional:            true,
						ElementType:         types.StringType,
					},
					"management_hint": schema.StringAttribute{
						MarkdownDescription: "Optional management hint surfaced during account-driven enrollment.",
						Optional:            true,
					},
				},
			},

			"signing_certificate": signingCertificateResourceSchema(),

			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// signingCertificateResourceSchema returns the SingleNestedAttribute schema
// for the embedded SSO signing certificate sub-block.
func signingCertificateResourceSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Embedded signing certificate sub-block. Three modes selected by `setup_type`:\n" +
			"- `NONE`: no certificate configured. Setting `setup_type = \"NONE\"` (or removing the block) deletes any existing certificate on the tenant.\n" +
			"- `GENERATED`: Jamf Pro generates a self-signed certificate. Re-applying with the same `setup_type = \"GENERATED\"` is a no-op, since the provider skips the regenerate step so subsequent applies do not churn the certificate.\n" +
			"- `UPLOADED`: user-supplied PKCS12 or JKS keystore. Requires `type`, `key`, `keystore_file`, `keystore_file_name`, `keystore_password`, and `password`. `key` is the case-sensitive alias inside the keystore; enumerate aliases with `keytool -list -keystore foo.p12 -storetype PKCS12 -storepass <pw>`.\n\n" +
			"`keystore_password` and `password` are both `WriteOnly`: sent to Jamf Pro on writes and never persisted in Terraform state. Bump the matching `_wo_version` integer to force the next update to re-send the value.",
		Optional: true,
		Attributes: map[string]schema.Attribute{
			"setup_type": schema.StringAttribute{
				MarkdownDescription: "Certificate setup mode. One of `GENERATED`, `UPLOADED`, or `NONE`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validSetupTypes...),
					SigningCertificateSetupTypeValidator(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Keystore type. `PKCS12` or `JKS`. User-supplied for `setup_type = \"UPLOADED\"`; set by Jamf Pro for `\"GENERATED\"`.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validKeystoreTypes...),
				},
			},
			"key": schema.StringAttribute{
				MarkdownDescription: "Alias inside the keystore that selects the private key and certificate to use. Case-sensitive. Required when `setup_type = \"UPLOADED\"`; set by Jamf Pro for `\"GENERATED\"`. Discover aliases with `keytool -list -keystore foo.p12 -storetype PKCS12 -storepass <pw>`.",
				Optional:            true,
				Computed:            true,
			},
			"keystore_file": schema.StringAttribute{
				MarkdownDescription: "Raw base64 of the keystore file (`.p12` or `.jks`). Idiomatic usage: `filebase64(\"jamf-saml.p12\")`. Required when `setup_type = \"UPLOADED\"`.",
				Optional:            true,
				Sensitive:           true,
			},
			"keystore_file_name": schema.StringAttribute{
				MarkdownDescription: "Display filename for the keystore. Required when `setup_type = \"UPLOADED\"`; set by Jamf Pro for `\"GENERATED\"`.",
				Optional:            true,
				Computed:            true,
			},
			"keystore_password": schema.StringAttribute{
				MarkdownDescription: "Keystore (file) password. `WriteOnly`: sent to Jamf Pro on writes and never persisted in Terraform state. Pair with `keystore_password_wo_version`, the rotation companion; bump that integer to re-send.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("keystore_password_wo_version")),
				},
			},
			"keystore_password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `keystore_password`. Bump this integer to force re-sending the current value on the next Update.",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Private-key password (the password used to encrypt the key entry inside the keystore). `WriteOnly`. Pair with `password_wo_version` (the rotation companion); bump that integer to re-send.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("password_wo_version")),
				},
			},
			"password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `password`. Bump this integer to force re-sending the current value on the next Update.",
				Optional:            true,
			},

			"serial_number": schema.StringAttribute{
				MarkdownDescription: "Certificate serial number. Surfaced as a decimal string so full-precision X.509 BigInts round-trip losslessly.",
				Computed:            true,
			},
			"subject": schema.StringAttribute{
				MarkdownDescription: "Certificate subject DN.",
				Computed:            true,
			},
			"issuer": schema.StringAttribute{
				MarkdownDescription: "Certificate issuer DN.",
				Computed:            true,
			},
			"expiration": schema.StringAttribute{
				MarkdownDescription: "Certificate expiration timestamp.",
				Computed:            true,
			},
			"keys": schema.ListNestedAttribute{
				MarkdownDescription: "Keys (aliases) discovered inside the uploaded keystore.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":    schema.StringAttribute{Computed: true, MarkdownDescription: "Alias name."},
						"valid": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the alias is currently valid."},
					},
				},
			},
		},
	}
}

// Configure wires the Jamf Pro client into the resource.
func (r *SsoSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_sso_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton.
func (r *SsoSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_sso_settings")
}
