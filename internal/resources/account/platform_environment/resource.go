// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package platform_environment implements the
// jamfplatform_account_platform_environment resource, data sources and list
// resource backed by the Jamf Account Organization API.
//
// A platform environment is a named group of an organization's tenants that
// together make up one deployment of Jamf. It is the unit a platform
// environment-scoped API integration is created against, which is why every
// Blueprints, Compliance Benchmarks and AI Governance construct depends on one.
// The Jamf Account console lists them under Platform Environments.
//
// Family: Jamf Account / organization-level. Configure goes through
// providerdata.ConfigureAccount, which gates on ScopeOrganization alone. The
// namespace is served only from the US gateway.
//
// # Shape
//
// Create, read, update and delete, wire-probed 2026-10-22 against an
// organization-scoped integration. Four facts run through the package:
//
//   - There is no read by identifier. An environment is found by scanning the
//     organization's collection and matching on id.
//   - An environment is created empty. Tenants join it afterwards, through the
//     jamfplatform_account_platform_environment_assign_tenants action, and the
//     environment object carries no list of them: membership is recorded on the
//     tenant. That is why this resource has no tenant attribute, and why Delete
//     reads the tenant list.
//   - Names are not unique, and Jamf Account trims them. Import and the
//     data-source id lookup use the identifier; the name lookup refuses an
//     ambiguous match.
//   - Every request that names an environment which no longer exists answers 500,
//     where the specification declares 404. Update and Delete therefore re-read
//     the collection after any failure, and treat absence as the answer.
//
// # Attribute names
//
// Attribute names follow the Jamf Account console rather than the payload, per
// STYLE_GUIDE §Attribute names mirror the Jamf Pro admin UI. Because the guide
// also forbids comments inside function bodies, the mapping lives here:
//
//	Terraform attribute   Payload field
//	-------------------   -------------
//	type                  label
//	region                metaRegion
//
// The console labels the field "Type" and offers Production and Sandbox in its
// dialogs, while the payload calls it `label` and accepts five values. The values
// keep Jamf's spelling, which differs from the console's only in case. The
// payload's `href` is the environment's own address relative to the API and is
// not exposed.
package platform_environment

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// PlatformEnvironmentResource implements the Terraform resource for Jamf Account
// platform environments.
type PlatformEnvironmentResource struct {
	client *account.Client
}

var (
	_ resource.Resource                = &PlatformEnvironmentResource{}
	_ resource.ResourceWithImportState = &PlatformEnvironmentResource{}
	_ resource.ResourceWithIdentity    = &PlatformEnvironmentResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewPlatformEnvironmentResource returns a new instance of
// PlatformEnvironmentResource.
func NewPlatformEnvironmentResource() resource.Resource {
	return &PlatformEnvironmentResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *PlatformEnvironmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_account_platform_environment"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *PlatformEnvironmentResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Identifier Jamf Account assigned to the platform environment.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the platform environment resource.
func (r *PlatformEnvironmentResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a platform environment in Jamf Account: a named group of your organization's " +
			"tenants that together make up one deployment of Jamf. The Jamf Account console lists them under " +
			"**Platform Environments**.\n\nA new platform environment holds no tenants. Add tenants with the " +
			"`jamfplatform_account_platform_environment_assign_tenants` action. You cannot undo an assignment, and " +
			"this resource reports no tenants. Read them with the `jamfplatform_account_tenants` data source, " +
			"which gives each tenant's platform environment.\n\nTerraform refuses to destroy a platform " +
			"environment that holds tenants and names them. Run `terraform state rm` to stop managing it.\n\n" +
			"`name` does not have to be unique. Changing `region` replaces the platform environment, because " +
			"Jamf Account fixes it at creation.\n\nNeeds an organization-scoped API integration, created " +
			"against neither a platform environment nor a tenant. Available only in the United States region." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier Jamf Account assigns to the platform environment. It stays the same " +
					"for the life of the platform environment.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Name\"** in the Jamf Account console: the display name administrators " +
					"see. Leading and trailing whitespace is not allowed, because Jamf Account trims it.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					NoSurroundingWhitespace(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "**\"Type\"** in the Jamf Account console: what the platform environment is " +
					"for. One of " + valuesDocs(typeValues()) + ". The console shows `PRODUCTION` as " +
					"\"Production\", `SANDBOX` as \"Sandbox\", and so on. The type describes the platform " +
					"environment and does not change which tenants can join.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(typeValues()...),
				},
			},
			"region": schema.StringAttribute{
				MarkdownDescription: "Region that every tenant in this platform environment must be in. One " +
					"of " + valuesDocs(regionValues()) + ". Omit it for a global platform environment, " +
					"which takes tenants from any region. Changing it replaces the platform environment.",
				Optional: true,
				Validators: []validator.String{
					stringvalidator.OneOf(regionValues()...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
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

// Configure wires the Jamf Account client into the resource via the shared
// providerdata.ConfigureAccount helper.
func (r *PlatformEnvironmentResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureAccount(ctx, req.ProviderData, "jamfplatform_account_platform_environment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the identifier Jamf Account assigned, which is
// stable for the life of the platform environment and is shown in the console.
func (r *PlatformEnvironmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
