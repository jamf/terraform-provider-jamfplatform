// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package mobile_device_provisioning_profile implements the
// jamfplatform_pro_mobile_device_provisioning_profile resource, data source,
// and list resource backed by the Jamf ProClassic mobile device provisioning
// profiles API.
//
// Server invariant (wire-probed): once a profile carries an uploaded
// `.mobileprovision` blob, EVERY PUT — including a general-only rename — returns
// HTTP 500. The blob is therefore create-only and the resource is immutable in
// place: `name`, `display_name`, and `profile_data` are all RequiresReplace, and
// Update never issues an SDK write (it only refreshes computed fields).
package mobile_device_provisioning_profile

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
// Empty: the classic /mobiledeviceprovisioningprofiles endpoint predates the
// provider's overall floor. The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below the provider minimum.
const minJamfProVersion = ""

// ProvisioningProfileResource implements the Terraform resource for Jamf
// ProClassic mobile device provisioning profiles.
type ProvisioningProfileResource struct {
	client *proclassic.Client
}

var _ resource.Resource = &ProvisioningProfileResource{}
var _ resource.ResourceWithImportState = &ProvisioningProfileResource{}
var _ resource.ResourceWithIdentity = &ProvisioningProfileResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewProvisioningProfileResource returns a new instance of ProvisioningProfileResource.
func NewProvisioningProfileResource() resource.Resource {
	return &ProvisioningProfileResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ProvisioningProfileResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_provisioning_profile"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *ProvisioningProfileResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro mobile device provisioning profile ID used to uniquely reference the profile.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the provisioning profile resource.
func (r *ProvisioningProfileResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro mobile device provisioning profile: the signed `.mobileprovision` profile that authorises in-house (enterprise) apps to run on managed devices.\n\n" +
			"An uploaded profile cannot be modified in place: changing `name` or `profile_data` replaces the profile (Terraform deletes and recreates it)." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Provisioning profile ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Profile name. Must not be empty. Changing it replaces the profile.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// Jamf Pro forces display_name to equal name (and the wire is field-order
			// sensitive — see input_builders.go), so it is never sent and always
			// mirrors name.
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display name shown in the Jamf Pro admin UI. Jamf Pro sets this to match the profile name.",
				Computed:            true,
			},
			"profile_data": schema.StringAttribute{
				MarkdownDescription: "Base64-encoded signed `.mobileprovision` profile. Jamf Pro parses the UUID and expiration from it when the profile is created. Changing or removing it replaces the profile.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "UUID parsed from the uploaded profile. Empty until a profile is uploaded.",
				Computed:            true,
			},
			"creation_date_utc": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp (UTC) reported by Jamf Pro.",
				Computed:            true,
			},
			"creation_date_epoch": schema.StringAttribute{
				MarkdownDescription: "Creation timestamp as epoch milliseconds reported by Jamf Pro.",
				Computed:            true,
			},
			"expiration_date_utc": schema.StringAttribute{
				MarkdownDescription: "Expiration timestamp (UTC) parsed from the uploaded profile.",
				Computed:            true,
			},
			"expiration_date_epoch": schema.StringAttribute{
				MarkdownDescription: "Expiration timestamp as epoch milliseconds parsed from the uploaded profile.",
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

// Configure wires the Jamf ProClassic client into the resource via the shared
// providerdata.ConfigureProClassic helper.
func (r *ProvisioningProfileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_provisioning_profile")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro provisioning profile ID.
func (r *ProvisioningProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
