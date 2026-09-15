// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package smtp_server implements the jamfplatform_pro_smtp_server singleton
// resource and data source, backed by the Jamf Pro SMTP Server settings API
// (Settings → System → SMTP Server).
package smtp_server

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: the SMTP Server settings endpoint is present at the
// provider's overall floor (matches every other settings singleton). The
// provider-level advisory still fires through providerdata.ConfigurePro.
const minJamfProVersion = ""

// SmtpServerResource implements the SMTP Server settings singleton. Backed by an
// Update-only API (one record per tenant): Create funnels into Update; Delete is
// a state-only no-op.
type SmtpServerResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                     = &SmtpServerResource{}
	_ resource.ResourceWithImportState      = &SmtpServerResource{}
	_ resource.ResourceWithIdentity         = &SmtpServerResource{}
	_ resource.ResourceWithConfigValidators = &SmtpServerResource{}
	_ resource.ResourceWithModifyPlan       = &SmtpServerResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second

	// defaultConnectionTimeout is the Jamf Pro UI default for the SMTP
	// connection timeout (seconds). A static default keeps the value known at
	// plan time so Create sends a concrete value.
	defaultConnectionTimeout int64 = 30
)

// NewSmtpServerResource returns a new instance of the resource.
func NewSmtpServerResource() resource.Resource {
	return &SmtpServerResource{}
}

// Metadata sets the resource type name.
func (r *SmtpServerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_smtp_server"
}

// IdentitySchema defines the identifier used for import. Singleton resources
// accept only the fixed helpers.SingletonID value.
func (r *SmtpServerResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". SMTP Server settings are one record per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource.
func (r *SmtpServerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Jamf Pro SMTP Server settings (Settings → System → SMTP Server), the outbound mail relay Jamf Pro sends notifications, enrollment invitations and other email through. " +
			"One record per tenant. " +
			"`authentication_type` selects the authentication method and which blocks apply: `NONE` and `BASIC` use `connection_settings` (SMTP host/port/encryption); `BASIC` adds `basic_auth_credentials`; `GRAPH_API` uses `graph_api_credentials` (Microsoft Graph); `GOOGLE_MAIL` uses `google_mail_credentials` (Google Workspace). A plan-time validator enforces that the block matching `authentication_type` is present and the others absent. " +
			"Every apply replaces the whole configuration. Omitted scalars are preserved by carrying the current value forward (Optional+Computed). Switching `authentication_type` clears the previous method's credentials. " +
			"Plaintext secrets (`basic_auth_credentials.password`, `graph_api_credentials.client_secret`, `google_mail_credentials.client_secret`) are `WriteOnly`: sent to Jamf Pro on writes but never persisted in state and never returned on read. Pair each with its `*_wo_version` rotation trigger. " +
			"Google Workspace sender accounts are linked through an interactive Google OAuth grant in the Jamf Pro admin UI (\"Add an email address via Google\"). Terraform configures the client credentials but does not drive that grant, so `google_mail_credentials.authentications` is read-only. " +
			"Import with `terraform import jamfplatform_pro_smtp_server.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the SMTP server connection is enabled (\"Use the switch to enable or disable the connection\" in the Jamf Pro admin UI). Omit to preserve the current value (it is adopted on first apply and left untouched on an unrelated apply); set `true`/`false` to change it. Enabling requires `sender_settings.email_address` and `sender_settings.display_name` to both hold a value.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"authentication_type": schema.StringAttribute{
				MarkdownDescription: "**\"Authentication method\"** in the Jamf Pro admin UI. One of `NONE` (no authentication), `BASIC` (Basic Credentials: username and password), `GRAPH_API` (Microsoft Graph API), or `GOOGLE_MAIL` (Google Auth / Workspace). Selects which credential block is required.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(authenticationTypes...),
				},
			},

			"sender_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "**\"Configuration settings\"** in the Jamf Pro admin UI. The sender identity applied to outbound mail. Required for every authentication method.",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"email_address": schema.StringAttribute{
						MarkdownDescription: "**\"Sender email address\"** in the Jamf Pro admin UI. The account email address Jamf Pro sends mail from. " +
							"Jamf Pro requires a real address whenever `enabled` is `true`, and accepts an empty string only while the connection is disabled — which is how a tenant that has never set up mail reads back, so an empty value here is expected on adoption and must be filled in before enabling.",
						Required: true,
					},
					"display_name": schema.StringAttribute{
						MarkdownDescription: "**\"Sender display name\"** in the Jamf Pro admin UI. The sender name shown in messages. Omit to preserve the current value. " +
							"Jamf Pro requires a non-empty name whenever `enabled` is `true`, so a tenant whose stored name is empty needs one supplied here before the connection can be enabled.",
						Optional: true,
						Computed: true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
				},
			},

			"connection_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "**\"Authentication settings\"** (Server and port / Encryption / Connection timeout) in the Jamf Pro admin UI. The SMTP relay connection. Required when `authentication_type` is `NONE` or `BASIC`; must be omitted for `GRAPH_API` and `GOOGLE_MAIL`, which connect over HTTP rather than SMTP.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"host": schema.StringAttribute{
						MarkdownDescription: "**\"Server\"** in the Jamf Pro admin UI. SMTP server hostname or IP address. " +
							"Jamf Pro stores an empty host on a tenant that has never set up mail and returns it on a read, so an empty value here is expected on adoption and must be filled in before the connection is of any use.",
						Required: true,
					},
					"port": schema.Int64Attribute{
						MarkdownDescription: "**\"Port\"** in the Jamf Pro admin UI. SMTP server port (e.g. `25`, `465`, `587`).",
						Required:            true,
						Validators: []validator.Int64{
							int64validator.Between(1, 65535),
						},
					},
					"encryption_type": schema.StringAttribute{
						MarkdownDescription: "**\"Encryption\"** in the Jamf Pro admin UI. Protocol used to encrypt the SMTP connection. One of `NONE`, `SSL`, `TLS_1_2`, `TLS_1_1`, `TLS_1`, `TLS_1_3` (the UI labels these None / SSL / TLSv1.2 / TLSv1.1 / TLSv1 / TLSv1.3).",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.OneOf(encryptionTypes...),
						},
					},
					"connection_timeout": schema.Int64Attribute{
						MarkdownDescription: "**\"Connection timeout\"** in the Jamf Pro admin UI. Seconds to wait before a connection attempt fails. Defaults to `30`.",
						Optional:            true,
						Computed:            true,
						Default:             int64default.StaticInt64(defaultConnectionTimeout),
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
					},
				},
			},

			"basic_auth_credentials": schema.SingleNestedAttribute{
				MarkdownDescription: "Basic SMTP credentials. Required when `authentication_type = \"BASIC\"`; forbidden otherwise.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"username": schema.StringAttribute{
						MarkdownDescription: "**\"Username\"** in the Jamf Pro admin UI. SMTP account username.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"password":            writeOnlySecret("**\"Password\"** in the Jamf Pro admin UI. SMTP account password.", "password_wo_version"),
					"password_wo_version": woVersion("password"),
				},
			},

			"graph_api_credentials": schema.SingleNestedAttribute{
				MarkdownDescription: "Microsoft Graph API credentials. Required when `authentication_type = \"GRAPH_API\"`; forbidden otherwise.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"tenant_id": schema.StringAttribute{
						MarkdownDescription: "**\"Tenant ID\"** in the Jamf Pro admin UI. Microsoft Entra ID tenant (directory) ID.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"client_id": schema.StringAttribute{
						MarkdownDescription: "**\"Client ID\"** in the Jamf Pro admin UI. Microsoft Entra application (client) ID.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"client_secret":            writeOnlySecret("**\"Client secret\"** in the Jamf Pro admin UI. Microsoft Entra application client secret.", "client_secret_wo_version"),
					"client_secret_wo_version": woVersion("client_secret"),
				},
			},

			"google_mail_credentials": schema.SingleNestedAttribute{
				MarkdownDescription: "Google Workspace (Google Auth) credentials. Required when `authentication_type = \"GOOGLE_MAIL\"`; forbidden otherwise. The sender Google accounts are linked out of band through the Jamf Pro admin UI's interactive Google OAuth grant. Terraform configures the client credentials only.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"client_id": schema.StringAttribute{
						MarkdownDescription: "**\"Client ID\"** in the Jamf Pro admin UI. Google OAuth client ID.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
					},
					"client_secret":            writeOnlySecret("**\"Client secret\"** in the Jamf Pro admin UI. Google OAuth client secret.", "client_secret_wo_version"),
					"client_secret_wo_version": woVersion("client_secret"),
					"authentications": schema.ListNestedAttribute{
						MarkdownDescription: "Read-only list of Google sender accounts granted to Jamf Pro via the admin UI's interactive OAuth flow, with their authentication status. Managed out of band; not configurable through Terraform.",
						Computed:            true,
						PlanModifiers: []planmodifier.List{
							listplanmodifier.UseStateForUnknown(),
						},
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"email_address": schema.StringAttribute{
									MarkdownDescription: "The granted Google sender email address.",
									Computed:            true,
								},
								"status": schema.StringAttribute{
									MarkdownDescription: "OAuth grant status. One of `AUTHENTICATED`, `UNAUTHENTICATED`, `FAILED`.",
									Computed:            true,
								},
							},
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

// writeOnlySecret returns the standard Optional + Sensitive + WriteOnly secret
// attribute. Never Computed — Jamf Pro never returns these values. The value is
// sent on writes when the paired `*_wo_version` rotation trigger changes.
//
// AlsoRequires(woName) pairs the secret with its rotation trigger: supplying the
// secret without the version is a plan-time error. Without this, a mode switch
// (e.g. NONE→BASIC) that sets `password` but omits `password_wo_version` would
// compare null==null on Update, send no secret, and silently store an empty
// password. Omitting BOTH (adopt the stored secret) stays valid — the validator
// fires only when the secret is set.
func writeOnlySecret(desc, woName string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc + " `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**, and never returned on read. Bump the companion `*_wo_version` to rotate the stored value.",
		Optional:            true,
		Sensitive:           true,
		WriteOnly:           true,
		Validators: []validator.String{
			stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName(woName)),
		},
	}
}

// woVersion returns the rotation-trigger Int64 attribute for a WriteOnly secret.
func woVersion(secretName string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: fmt.Sprintf("Rotation trigger for the `WriteOnly` `%s`. Bump this integer (any change) to send the current `%s` on the next apply; leaving it unset or unchanged signals \"leave the stored value alone\": the provider omits the secret and Jamf Pro retains the existing one. Set it on create when you supply the secret.", secretName, secretName),
		Optional:            true,
	}
}

// ModifyPlan runs the enable-time preflight against the resolved plan, so an
// enabled connection missing a sender address, a sender display name or an SMTP
// server address fails the plan instead of the apply. It reads the plan rather than the configuration because
// `enabled` is Optional+Computed: the value UseStateForUnknown carries forward is
// the one the write will send. No-op on destroy.
func (r *SmtpServerResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan SmtpServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(validateSenderSettingsWhenEnabled(plan.Enabled, plan.SenderSettings, plan.ConnectionSettings)...)
}

// ConfigValidators returns the cross-field validators evaluated at plan time.
func (r *SmtpServerResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		authBlockConfigValidator{},
	}
}

// Configure wires the Jamf Pro client into the resource.
func (r *SmtpServerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_smtp_server")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed
// helpers.SingletonID value is accepted.
//
//	terraform import jamfplatform_pro_smtp_server.<name> singleton
func (r *SmtpServerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_smtp_server")
}

// authenticationsListType is the Terraform type of the Computed
// google_mail_credentials.authentications list. Centralised so the state builder
// and any null construction agree.
var authenticationsListType = types.ListType{ElemType: types.ObjectType{AttrTypes: googleAuthenticationAttrTypes}}
