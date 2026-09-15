// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package managed_software_updates implements the jamfplatform_pro_managed_software_update
// singleton resource backed by the Jamf Pro Managed Software Updates feature-toggle API.
package managed_software_updates

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the Managed Software Updates feature-toggle endpoint is present at the provider's
// overall floor, so no per-resource gate is needed.
const minJamfProVersion = ""

// ManagedSoftwareUpdateResource implements the singleton resource for the Jamf Pro Managed
// Software Updates feature. Backed by an Update-only API (no Create/Delete on the remote):
// Create funnels into Update; Delete is a no-op that only removes the object from Terraform
// state. The write is asynchronous, so Create/Update poll the toggle to convergence before
// writing state (see crud.go).
type ManagedSoftwareUpdateResource struct {
	client *pro.Client
}

var _ resource.Resource = &ManagedSoftwareUpdateResource{}
var _ resource.ResourceWithImportState = &ManagedSoftwareUpdateResource{}
var _ resource.ResourceWithIdentity = &ManagedSoftwareUpdateResource{}

const (
	defaultCreateTimeout = 5 * time.Minute
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 5 * time.Minute
	defaultDeleteTimeout = 60 * time.Second
)

// NewManagedSoftwareUpdateResource returns a new instance of the resource.
func NewManagedSoftwareUpdateResource() resource.Resource {
	return &ManagedSoftwareUpdateResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ManagedSoftwareUpdateResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_managed_software_update"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept
// only the fixed helpers.SingletonID value.
func (r *ManagedSoftwareUpdateResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". The Managed Software Updates feature is one per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the Managed Software Updates feature.
func (r *ManagedSoftwareUpdateResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Pro Managed Software Updates feature: the \"Use new feature\" toggle under Computers > Software updates and Mobile devices > Software updates. " +
			"One record per tenant. When enabled, Jamf Pro uses Apple's Declarative Device Management to enforce software update plans. When disabled, those plans are turned off. " +
			"Omit `enabled` and the resource adopts the current Jamf Pro value rather than changing it. " +
			"Turning the feature on or off happens in the background, so applying this resource waits for the change to take effect before completing. " +
			"Import with `terraform import jamfplatform_pro_managed_software_update.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the Managed Software Updates feature is enabled. " +
					"Omit to leave the current value untouched (it is not changed on an unrelated apply); set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"dss_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether Declarative Device Management software update enforcement is in effect. Managed by Jamf Pro (derived from the tenant's licence and configuration); read-only.",
				Computed:            true,
			},
			"recipe_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether legacy recipe-based update enforcement is in effect. Managed by Jamf Pro (derived from the tenant's configuration); read-only.",
				Computed:            true,
			},
			"force_install_local_date_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether update plans may enforce a forced-install deadline. Managed by Jamf Pro (derived from the tenant's configuration); read-only.",
				Computed:            true,
			},
			"custom_version_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether update plans may target a custom OS version. Managed by Jamf Pro (derived from the tenant's configuration); read-only.",
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

// Configure wires the Jamf Pro client into the resource via the shared
// providerdata.ConfigurePro helper.
func (r *ManagedSoftwareUpdateResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_managed_software_update")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed helpers.SingletonID
// value is accepted; any other identifier is rejected with a clear error so users do
// not accidentally end up with mis-keyed state that the resource silently normalizes
// on the next Read.
//
//	terraform import jamfplatform_pro_managed_software_update.<name> singleton
func (r *ManagedSoftwareUpdateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_managed_software_update")
}
