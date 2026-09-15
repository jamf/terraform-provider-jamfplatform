// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package json_web_token_configuration implements the
// jamfplatform_pro_pki_json_web_token_configuration resource, data source, and
// list resource backed by the Jamf ProClassic /jsonwebtokenconfigurations API.
// The construct name mirrors the Jamf Pro admin UI ("JSON Web Token
// Configuration" under Settings → Global → PKI certificates). See
// spike/JSON_WEB_TOKEN_SPIKE.md for the wire-probe behind every field and the
// max-one-per-instance cardinality.
package pki_json_web_token_configuration

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

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required. Empty:
// JSON Web Token configurations predate the provider's overall floor. The
// provider-level advisory still fires through providerdata.ConfigureProClassic
// when the tenant is below the floor.
const minJamfProVersion = ""

// JSONWebTokenConfigurationResource implements the Terraform resource for Jamf
// Pro JSON Web Token configurations.
type JSONWebTokenConfigurationResource struct {
	client *proclassic.Client
}

var (
	_ resource.Resource                = &JSONWebTokenConfigurationResource{}
	_ resource.ResourceWithImportState = &JSONWebTokenConfigurationResource{}
	_ resource.ResourceWithIdentity    = &JSONWebTokenConfigurationResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 90 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewJSONWebTokenConfigurationResource returns a new instance of
// JSONWebTokenConfigurationResource.
func NewJSONWebTokenConfigurationResource() resource.Resource {
	return &JSONWebTokenConfigurationResource{}
}

// Metadata sets the resource type name.
func (r *JSONWebTokenConfigurationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_pki_json_web_token_configuration"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *JSONWebTokenConfigurationResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro JSON Web Token configuration ID used to uniquely reference the record.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema. Attribute names mirror the Jamf Pro
// admin UI labels (STYLE_GUIDE §Attribute names mirror the admin UI).
func (r *JSONWebTokenConfigurationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro JSON Web Token configuration, the \"JSON Web Token Configuration\" tab under Settings → Global → PKI certificates in the Jamf Pro admin UI. The configuration holds the encryption key Jamf Pro uses to issue signed tokens, for example to authenticate apps such as Jamf Setup and Jamf Reset. Jamf Pro allows at most one JSON Web Token configuration per instance; creating a second one fails." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "JSON Web Token configuration ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Display name for the JSON Web Token configuration.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"encryption_key_wo": schema.StringAttribute{
				MarkdownDescription: "**\"Encryption Key\"** in the Jamf Pro admin UI. The key Jamf Pro uses to sign issued tokens. `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**, and never returned by Jamf Pro. Pair with `encryption_key_wo_version` to rotate.",
				Required:            true,
				Sensitive:           true,
				WriteOnly:           true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"encryption_key_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `encryption_key_wo`. Bump this integer to force an update that re-sends `encryption_key_wo`. Initial create should set `encryption_key_wo_version = 1`. Leaving it unset or unchanged signals \"leave the stored key alone\": the provider omits the key from the next update, so Jamf Pro retains the existing value.",
				Optional:            true,
			},
			"token_expiry": schema.Int64Attribute{
				MarkdownDescription: "**\"Token Expiry\"** in the Jamf Pro admin UI. Minutes an issued token remains valid, 1–120. Omit to leave the current value untouched on update; an integer has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Validators:          []validator.Int64{int64validator.Between(1, 120)},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the JSON Web Token configuration is active. Defaults to `true` on create. Omit to leave the current value untouched on update; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
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
func (r *JSONWebTokenConfigurationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_pki_json_web_token_configuration")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro JSON Web Token configuration ID.
func (r *JSONWebTokenConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
