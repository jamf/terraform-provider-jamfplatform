// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package app_request_form_field implements the jamfplatform_pro_app_request_form_field
// resource, data source, and list resource backed by the Jamf Pro App Request form input
// fields API (Settings → Self Service → App Request → App Request Form).
package app_request_form_field

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the App Request endpoints are present at the provider's overall floor, so no
// per-resource gate is needed. The provider-level advisory still fires through
// providerdata.ConfigurePro when the tenant is below ProviderMinJamfProVersion.
const minJamfProVersion = ""

// AppRequestFormFieldResource implements the Terraform resource for Jamf Pro App Request
// form fields.
type AppRequestFormFieldResource struct {
	client *pro.Client
}

var _ resource.Resource = &AppRequestFormFieldResource{}
var _ resource.ResourceWithImportState = &AppRequestFormFieldResource{}
var _ resource.ResourceWithIdentity = &AppRequestFormFieldResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewAppRequestFormFieldResource returns a new instance of AppRequestFormFieldResource.
func NewAppRequestFormFieldResource() resource.Resource {
	return &AppRequestFormFieldResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *AppRequestFormFieldResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_app_request_form_field"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *AppRequestFormFieldResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro App Request form field ID used to uniquely reference the record.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the App Request form field resource.
func (r *AppRequestFormFieldResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro App Request form field (Settings → Self Service → App Request → App Request Form). " +
			"Form fields are the custom input prompts shown to users on the App Request form in Self Service. Each field is an independent record ordered by `priority`. " +
			"Titles are not required to be unique. " +
			"A tenant must hold at least one form field before App Requests can be enabled (`jamfplatform_pro_app_request_settings`). Jamf Pro checks that only on the settings write, so removing the last field while App Requests are enabled succeeds and leaves the form empty." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "App Request form field ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"title": schema.StringAttribute{
				MarkdownDescription: "Title of the form field shown to the user on the App Request form. Titles are not required to be unique across fields.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional helper text shown beneath the field title to prompt the requester. Omit to leave it unset.",
				Optional:            true,
			},
			"priority": schema.Int64Attribute{
				MarkdownDescription: "Display order of the field on the App Request form. Fields are shown in ascending priority (lower numbers appear first). Values need not be unique or contiguous.",
				Required:            true,
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

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *AppRequestFormFieldResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_app_request_form_field")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro App Request form field ID.
func (r *AppRequestFormFieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
