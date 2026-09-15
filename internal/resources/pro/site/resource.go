// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package site implements the jamfplatform_pro_site resource, data source, and
// list resource backed by the Jamf ProClassic sites API.
package site

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: classic /sites endpoint predates the provider's overall floor (11.0.0). The
// provider-level advisory still fires through providerdata.ConfigureProClassic when the
// tenant is below ProviderMinJamfProVersion.
const minJamfProVersion = ""

// SiteResource implements the Terraform resource for Jamf ProClassic sites.
type SiteResource struct {
	client *proclassic.Client
}

var _ resource.Resource = &SiteResource{}
var _ resource.ResourceWithImportState = &SiteResource{}
var _ resource.ResourceWithIdentity = &SiteResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewSiteResource returns a new instance of SiteResource.
func NewSiteResource() resource.Resource {
	return &SiteResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *SiteResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_site"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *SiteResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro site ID used to uniquely reference the site.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the site resource.
func (r *SiteResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro site. Sites partition inventory: most Jamf Pro objects can be scoped to a site for access control and reporting." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Site ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Site display name. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
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

// Configure wires the Jamf ProClassic client into the resource via the shared
// providerdata.ConfigureProClassic helper.
func (r *SiteResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_site")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro site ID.
func (r *SiteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
