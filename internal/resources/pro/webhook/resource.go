// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package webhook implements the jamfplatform_pro_webhook resource, data
// source, and list resource backed by the Jamf ProClassic /webhooks API. The
// construct name mirrors the Jamf Pro admin UI ("Webhooks" under Settings →
// Global). The wire envelope is flat (no <general> wrapper). See
// WEBHOOK_SPIKE.md for the wire-probe behind every field, enum, and validator.
package webhook

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required. Empty:
// classic /webhooks predates the provider's overall floor. The provider-level
// advisory still fires through providerdata.ConfigureProClassic when the tenant
// is below the floor.
const minJamfProVersion = ""

// WebhookResource implements the Terraform resource for Jamf Pro webhooks.
type WebhookResource struct {
	client *proclassic.Client
}

var (
	_ resource.Resource                     = &WebhookResource{}
	_ resource.ResourceWithImportState      = &WebhookResource{}
	_ resource.ResourceWithIdentity         = &WebhookResource{}
	_ resource.ResourceWithConfigValidators = &WebhookResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 90 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewWebhookResource returns a new instance of WebhookResource.
func NewWebhookResource() resource.Resource {
	return &WebhookResource{}
}

// Metadata sets the resource type name.
func (r *WebhookResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_webhook"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *WebhookResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro webhook ID used to uniquely reference the record.",
				RequiredForImport: true,
			},
		},
	}
}

// ConfigValidators returns the plan-time cross-field validators.
func (r *WebhookResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		usernameRequiresBasicValidator{},
		passwordRequiresBasicOrHashValidator{},
		headerRequiresHeaderAuthValidator{},
		basicRequiresUsernameValidator{},
		headerAuthRequiresHeaderValidator{},
		smartGroupIDRequiresSmartEventValidator{},
	}
}

// Schema returns the Terraform schema. Attribute names mirror the Jamf Pro
// admin UI labels (STYLE_GUIDE §Attribute names mirror the admin UI).
func (r *WebhookResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro webhook (Settings → Global → Webhooks in the Jamf Pro admin UI). A webhook posts an event payload to an external URL when the selected Jamf Pro event fires. \"Mutual TLS Authentication\" is deliberately unsupported: its certificate material is settable only through the legacy admin web UI, and no API reaches it." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Webhook ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Must not be empty.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "**\"Enabled\"** in the Jamf Pro admin UI. Whether the webhook is active. Defaults to `true`. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "**\"Webhook URL\"** in the Jamf Pro admin UI. The URL the webhook payload is posted to.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"authentication_type": schema.StringAttribute{
				MarkdownDescription: "**\"Authentication type\"** in the Jamf Pro admin UI. One of " + markdownValueList(webhookAuthTypes) + ": `BASIC` (then set `username`/`password`), `HEADER` (then set `header` to a JSON object), `HASH_SIGNATURE` (then set `password` as the signing secret and optionally `hash_algorithm`), or `MTLS` (\"Mutual TLS Authentication\"). `MTLS` is accepted so existing webhooks import, but the client certificate it needs can only be supplied through the Jamf Pro admin UI, so an MTLS webhook created here is inert until that certificate is added. Defaults to `NONE`. This attribute is computed, so switching authentication off again requires setting `authentication_type = \"NONE\"` explicitly; removing the attribute retains the last applied value.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators:          []validator.String{stringvalidator.OneOf(webhookAuthTypes...)},
			},
			"connection_timeout": schema.Int64Attribute{
				MarkdownDescription: "**\"Connection Timeout\"** in the Jamf Pro admin UI. Seconds to wait when establishing the connection to the webhook host. Defaults to `5`. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"read_timeout": schema.Int64Attribute{
				MarkdownDescription: "**\"Read Timeout\"** in the Jamf Pro admin UI. Seconds to wait for a response after sending the request. Defaults to `2`. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"content_type": schema.StringAttribute{
				MarkdownDescription: "**\"Content Type\"** in the Jamf Pro admin UI (the XML/JSON radio). Format of the webhook payload: `application/json` or `text/xml`. Defaults to `text/xml`. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators:          []validator.String{stringvalidator.OneOf(webhookContentTypes...)},
			},
			"event": schema.StringAttribute{
				MarkdownDescription: "**\"Webhook Event\"** in the Jamf Pro admin UI. The Jamf Pro event that triggers the webhook. Changing this is an in-place update.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf(webhookEvents...)},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "**\"Username\"** in the Jamf Pro admin UI (BASIC authentication). Required when `authentication_type = \"BASIC\"` and not allowed otherwise.",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "**\"Password\"** (BASIC) / **\"Signing Secret\"** (HASH_SIGNATURE) in the Jamf Pro admin UI. `WriteOnly`: sent to Jamf Pro on writes and never persisted in Terraform state; Jamf Pro returns a redaction sentinel on read. Pair with `password_wo_version` to rotate. For HASH_SIGNATURE the password must be at least 16 characters.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `password`. Bump this integer to force a new update that re-sends `password`. Set `password_wo_version = 1` on create. Leave it unset or unchanged to keep the stored secret: the provider omits the password from the next update, so Jamf Pro retains the existing value.",
				Optional:            true,
			},
			"header": schema.StringAttribute{
				MarkdownDescription: "**\"Header Authentication\"** metadata in the Jamf Pro admin UI (HEADER authentication). Must be a JSON object of header name/value pairs, e.g. `{\"Authorization\":\"Bearer …\"}`. Required when `authentication_type = \"HEADER\"` and not allowed otherwise. `Sensitive` (it carries credentials) but tracked in state because Jamf echoes it back.",
				Optional:            true,
				Sensitive:           true,
			},
			"hash_algorithm": schema.StringAttribute{
				MarkdownDescription: "**\"Algorithm\"** in the Jamf Pro admin UI (HASH_SIGNATURE authentication). Signature hash algorithm: `SHA256` or `SHA512`. Always returned by Jamf Pro; only meaningful for HASH_SIGNATURE. Defaults to `SHA256`. Omit to leave the current value untouched; an enum has no blank-clear, so set a concrete value to change it. The value is only left untouched while `authentication_type` stays the same: Jamf Pro resets it to `SHA256` on any update whose authentication type is not `HASH_SIGNATURE`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{hashAlgorithmAuthResetPlanModifier{}},
				Validators:          []validator.String{stringvalidator.OneOf(webhookHashAlgorithms...)},
			},
			"smart_group_id": schema.Int64Attribute{
				// No plan modifier: smart_group_id is meaningful only for the
				// SmartGroup* events, so it must fall to null (not pin a stale
				// value) whenever `event` changes to a non-smart event.
				MarkdownDescription: "Jamf Pro smart group ID for the SmartGroup membership-change events. Only valid when `event` is `SmartGroupComputerMembershipChange`, `SmartGroupMobileDeviceMembershipChange`, or `SmartGroupUserMembershipChange`. Interpolate a Jamf Pro smart group ID (e.g. `jamfplatform_device_group.<x>.jamf_pro_id`). Omit for \"any\" group.",
				Optional:            true,
			},
			"enable_display_fields_for_group_object": schema.BoolAttribute{
				MarkdownDescription: "**\"Include Display Fields for the Group Object\"** in the Jamf Pro admin UI. Whether to include the smart group's display fields in the payload. Defaults to `false`. Omit to leave the current value untouched; set `true`/`false` to change it. The display field list itself cannot be set here; see `display_fields`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"display_fields": schema.SetAttribute{
				MarkdownDescription: "Read-only set of display field names included in the group-object payload. Terraform cannot set them: Jamf Pro rejects any populated display field, because the list is managed in the admin UI. This attribute reflects whatever the Jamf Pro UI configured.",
				ElementType:         types.StringType,
				Computed:            true,
				PlanModifiers:       []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
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

// Configure wires the Jamf ProClassic client into the resource.
func (r *WebhookResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_webhook")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro webhook ID.
func (r *WebhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
