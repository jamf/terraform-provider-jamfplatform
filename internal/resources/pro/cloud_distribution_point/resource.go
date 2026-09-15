// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package cloud_distribution_point implements the jamfplatform_pro_cloud_distribution_point
// singleton resource and data source backed by the Jamf Pro cloud distribution
// point API (/api/pro/v1/cloud-distribution-point).
package cloud_distribution_point

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: the cloud distribution point endpoint is a long-standing
// part of the Pro API, present at the provider's overall floor.
const minJamfProVersion = ""

// CloudDistributionPointResource implements the presence-optional singleton
// resource for the Jamf Pro cloud distribution point.
//
// Unlike pure settings singletons (e.g. self_service_plus_settings), this object
// can be present (a real cdn_type) or absent (server cdn_type "NONE"), and the
// API exposes a real POST (enable) and DELETE (disable):
//
//	Create = POST  (None → type; 201). Errors if already configured.
//	Update = PATCH (merge-patch; cdn_type mandatory in every body; 202).
//	Delete = DELETE (→ NONE; 204).
//	Read   = GET   (always 200; cdn_type "NONE" ⇒ RemoveResource).
type CloudDistributionPointResource struct {
	client *pro.Client
}

var _ resource.Resource = &CloudDistributionPointResource{}
var _ resource.ResourceWithImportState = &CloudDistributionPointResource{}
var _ resource.ResourceWithIdentity = &CloudDistributionPointResource{}
var _ resource.ResourceWithConfigValidators = &CloudDistributionPointResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewCloudDistributionPointResource returns a new instance of the resource.
func NewCloudDistributionPointResource() resource.Resource {
	return &CloudDistributionPointResource{}
}

// Metadata sets the resource type name.
func (r *CloudDistributionPointResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_cloud_distribution_point"
}

// IdentitySchema defines the import identity. Singleton — only the fixed
// helpers.SingletonID value is accepted.
func (r *CloudDistributionPointResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". The cloud distribution point is one per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the cloud distribution point resource.
func (r *CloudDistributionPointResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Pro cloud distribution point. One record per tenant. Configuring this resource enables the selected content delivery network (`cdn_type`); destroying it disables the cloud distribution point and resets the tenant to `NONE`. " +
			"Destroying this resource disables the Jamf Cloud distribution point and **permanently deletes all packages, in-house apps, and eBooks hosted in Jamf Cloud**. Changing `cdn_type` forces replacement and has the same effect. Neither can be undone. " +
			"`JAMF_CLOUD` (Jamf Cloud Distribution Service, or JCDS) needs no credentials. The other types (`AMAZON_S3`, `AKAMAI`, `RACKSPACE_CLOUD_FILES`) require the credential and endpoint fields, and the provider does not acceptance-test them. " +
			"Import with `terraform import jamfplatform_pro_cloud_distribution_point.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cdn_type": schema.StringAttribute{
				MarkdownDescription: "Content delivery network type. One of `JAMF_CLOUD` (Jamf Cloud Distribution Service), `AMAZON_S3`, `AKAMAI`, `RACKSPACE_CLOUD_FILES`. Changing this forces replacement: the existing cloud distribution point is deleted, the tenant returns to `NONE`, and a new one is created. `NONE` is the disabled state reached by destroying the resource, not a settable value.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validCdnTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"master": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the master (primary) distribution point. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Connection username. Used by non-JCDS types (`AMAZON_S3`, `AKAMAI`, `RACKSPACE_CLOUD_FILES`); empty for `JAMF_CLOUD`. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Connection password or secret key for the non-JCDS types. `WriteOnly`: sent to Jamf Pro on writes, **never persisted in Terraform state**, and never returned on read. Keep it in configuration, because it is re-sent on every apply. Jamf Pro will not accept a write that omits the value, so there is no `_wo_version` rotation companion. See the resource documentation.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"directory": schema.StringAttribute{
				MarkdownDescription: "Directory / bucket path on the distribution point. Used by non-JCDS types. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"upload_url": schema.StringAttribute{
				MarkdownDescription: "Upload endpoint URL (e.g. an Akamai NetStorage upload host). Used by non-JCDS types; returned by Jamf Pro and empty for `JAMF_CLOUD`. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"download_url": schema.StringAttribute{
				MarkdownDescription: "Download endpoint URL. Used by non-JCDS types; returned by Jamf Pro and empty for `JAMF_CLOUD`. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cdn_url": schema.StringAttribute{
				MarkdownDescription: "CDN URL. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"require_signed_urls": schema.BoolAttribute{
				MarkdownDescription: "Whether downloads require AWS CloudFront signed URLs. Used by `AMAZON_S3`; enabling it makes `private_key` required. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"key_pair_id": schema.StringAttribute{
				MarkdownDescription: "AWS CloudFront key pair identifier used to sign URLs. Used by `AMAZON_S3` when `require_signed_urls` is enabled. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"private_key": schema.StringAttribute{
				MarkdownDescription: "Base64-encoded AWS CloudFront private key (`.pem` or `.der`) used to sign URLs. Required when `cdn_type = \"AMAZON_S3\"` and `require_signed_urls = true`. `WriteOnly`: sent to Jamf Pro on writes but **never persisted in Terraform state**. Idiomatic usage: `private_key = filebase64(\"cloudfront-key.pem\")`. Omit to leave the stored key untouched.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"expiration_seconds": schema.Int64Attribute{
				MarkdownDescription: "AWS CloudFront signed-URL expiration window in seconds. Used by `AMAZON_S3`. Must be at least 1. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"secondary_auth_required": schema.BoolAttribute{
				MarkdownDescription: "Whether secondary authentication is required for downloads. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"secondary_auth_time_to_live": schema.Int64Attribute{
				MarkdownDescription: "Secondary authentication token time-to-live in seconds. Must be at least 1. Omit to leave the current value untouched; an integer has no blank-clear, so set a concrete value to change it.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"secondary_auth_status_code": schema.Int64Attribute{
				MarkdownDescription: "Secondary authentication status code returned by the server. Computed.",
				Computed:            true,
			},
			"has_connection_succeeded": schema.BoolAttribute{
				MarkdownDescription: "Whether the most recent connection test against the distribution point succeeded. Read-only. Reflects live status, and is re-evaluated on every apply.",
				Computed:            true,
			},
			"message": schema.StringAttribute{
				MarkdownDescription: "Human-readable connection status message from the server. Computed.",
				Computed:            true,
			},
			"inventory_id": schema.StringAttribute{
				MarkdownDescription: "Inventory identifier for the distribution point. Returned by Jamf Pro; not user-settable. It is **not stable**: a new identifier is allocated whenever the cloud distribution point is recreated.",
				Computed:            true,
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

// ConfigValidators registers the value-discriminated per-cdn_type required-field
// check. See validators.go.
func (r *CloudDistributionPointResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		cdnTypeRequiredFieldsConfigValidator{},
	}
}

// Configure wires the Jamf Pro client into the resource.
func (r *CloudDistributionPointResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_cloud_distribution_point")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed
// helpers.SingletonID value is accepted.
//
//	terraform import jamfplatform_pro_cloud_distribution_point.<name> singleton
func (r *CloudDistributionPointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_cloud_distribution_point")
}
