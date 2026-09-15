// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package return_to_service implements the jamfplatform_pro_return_to_service
// resource, data source, and list resource. A Return to Service configuration
// pairs a display name with a Wi-Fi configuration profile so a wiped device can
// automatically rejoin a network and re-enrol without manual setup.
package return_to_service

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

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: consistent with every other Pro resource (the provider's
// overall floor governs). The standalone Return to Service interface is
// documented as a Jamf Pro 11.18 feature, but that is the UI availability date;
// the underlying configuration endpoint predates the UI and is reachable on
// tenants below it, so no per-resource floor is asserted.
const minJamfProVersion = ""

// ReturnToServiceResource implements the Terraform resource for Jamf Pro Return
// to Service configurations.
type ReturnToServiceResource struct {
	client *pro.Client
}

var _ resource.Resource = &ReturnToServiceResource{}
var _ resource.ResourceWithImportState = &ReturnToServiceResource{}
var _ resource.ResourceWithIdentity = &ReturnToServiceResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewReturnToServiceResource returns a new instance of the resource.
func NewReturnToServiceResource() resource.Resource {
	return &ReturnToServiceResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ReturnToServiceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_return_to_service"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *ReturnToServiceResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro Return to Service configuration ID used to uniquely reference the configuration.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the Return to Service resource.
func (r *ReturnToServiceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro Return to Service configuration, which pairs a display name with a Wi-Fi configuration profile. When a device is erased with Return to Service, the referenced Wi-Fi profile lets it rejoin a network and re-enrol into Jamf Pro without manual setup." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Return to Service configuration ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display name of the Return to Service configuration.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"wifi_profile_id": schema.StringAttribute{
				MarkdownDescription: "ID of the Wi-Fi configuration profile a device rejoins during Return to Service. Must reference a mobile device configuration profile that carries a Wi-Fi payload.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(wholeNumberRegex, "must be a whole number greater than 0"),
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

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *ReturnToServiceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_return_to_service")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState imports a Return to Service configuration by ID.
func (r *ReturnToServiceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
