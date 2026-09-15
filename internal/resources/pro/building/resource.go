// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package building implements the jamfplatform_pro_building resource, data source, and
// list resource backed by the Jamf Pro buildings API.
package building

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
// Empty string skips the per-resource version check — the buildings endpoint has been
// stable since well before the provider's overall floor (11.0.0), so no per-resource
// gate is needed.
const minJamfProVersion = ""

// BuildingResource implements the Terraform resource for Jamf Pro buildings.
type BuildingResource struct {
	client *pro.Client
}

var _ resource.Resource = &BuildingResource{}
var _ resource.ResourceWithImportState = &BuildingResource{}
var _ resource.ResourceWithIdentity = &BuildingResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewBuildingResource returns a new instance of BuildingResource.
func NewBuildingResource() resource.Resource {
	return &BuildingResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *BuildingResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_building"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *BuildingResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro building ID used to uniquely reference the building.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the building resource.
func (r *BuildingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro building. Buildings represent physical locations and can be assigned to inventory records for reporting and scoping." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Building ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Building display name. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			// The buildings PUT is full-replace (wire-probed 2026-06-09): a request
			// omitting an address field resets it to null server-side. All six
			// address scalars are therefore Optional+Computed with UseStateForUnknown
			// so an omitted field is PRESERVED, not wiped — the omit-then-unrelated-
			// -apply footgun documented in STYLE_GUIDE.md §Full-replace endpoints.
			// Set a field to "" to clear it (round-trips as empty string); omitting
			// it carries the prior state value forward via the plan modifier.
			// Caveat: a UI edit landing between refresh and apply is clobbered by the
			// re-PUT (the inherent refresh→apply race on full-replace endpoints).
			"city": schema.StringAttribute{
				MarkdownDescription: "City the building is located in. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"country": schema.StringAttribute{
				MarkdownDescription: "Country the building is located in. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"state_province": schema.StringAttribute{
				MarkdownDescription: "State, province, or administrative region the building is located in. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"street_address_1": schema.StringAttribute{
				MarkdownDescription: "Primary street address line. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"street_address_2": schema.StringAttribute{
				MarkdownDescription: "Secondary street address line (suite, unit, floor). Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"zip_postal_code": schema.StringAttribute{
				MarkdownDescription: "Zip or postal code for the building location. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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
func (r *BuildingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_building")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro building ID.
func (r *BuildingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
