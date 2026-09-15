// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package department implements the jamfplatform_pro_department resource, data source, and
// list resource backed by the Jamf Pro departments API.
package department

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
// Empty string skips the per-resource version check — the departments endpoint has been
// stable since well before the provider's overall floor (11.0.0), so no per-resource
// gate is needed.
const minJamfProVersion = ""

// DepartmentResource implements the Terraform resource for Jamf Pro departments.
type DepartmentResource struct {
	client *pro.Client
}

var _ resource.Resource = &DepartmentResource{}
var _ resource.ResourceWithImportState = &DepartmentResource{}
var _ resource.ResourceWithIdentity = &DepartmentResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewDepartmentResource returns a new instance of DepartmentResource.
func NewDepartmentResource() resource.Resource {
	return &DepartmentResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *DepartmentResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_department"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *DepartmentResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro department ID used to uniquely reference the department.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the department resource.
func (r *DepartmentResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro department. Departments represent organisational units and can be assigned to inventory records for reporting and scoping." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Department ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Department display name. Must not be empty.",
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

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *DepartmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_department")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro department ID.
func (r *DepartmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
