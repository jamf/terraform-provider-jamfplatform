// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package jamf_connect implements the jamfplatform_pro_jamf_connect resource
// and data source, backed by the Jamf Pro Jamf Connect deployment settings API
// (/api/pro/v1/jamf-connect). The construct manages how Jamf Connect is
// auto-deployed and updated on an existing, already-Connect-linked macOS
// configuration profile (Settings → Jamf apps → Jamf Connect).
package jamf_connect

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: the Jamf Connect deployment-settings endpoints carry no
// version annotation in the Pro API spec and the sibling Jamf-apps settings
// resources (jamf_protect, managed_software_updates) share the provider's
// overall floor, so no per-resource gate is set.
const minJamfProVersion = ""

// JamfConnectResource implements the update-only adoption resource for Jamf
// Connect deployment-and-update settings on a linked macOS configuration
// profile. See crud.go for the Create=adopt / Read=list-and-match /
// Delete=state-only-no-op lifecycle.
type JamfConnectResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                     = &JamfConnectResource{}
	_ resource.ResourceWithImportState      = &JamfConnectResource{}
	_ resource.ResourceWithIdentity         = &JamfConnectResource{}
	_ resource.ResourceWithConfigValidators = &JamfConnectResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewJamfConnectResource returns a new instance of the resource.
func NewJamfConnectResource() resource.Resource {
	return &JamfConnectResource{}
}

// Metadata sets the resource type name.
func (r *JamfConnectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_jamf_connect"
}

// IdentitySchema defines the import identity — the Jamf Pro configuration
// profile ID (the profile_id adoption key, as a string).
func (r *JamfConnectResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro configuration profile ID (the profile_id adoption key).",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource.
func (r *JamfConnectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Connect deployment and update settings for a single macOS configuration profile (Settings → Jamf apps → Jamf Connect). " +
			"This resource adopts an existing configuration profile, one that already contains a Jamf Connect payload, and controls how Jamf Connect is installed and kept up to date on the computers in that profile's scope. It does not create the configuration profile or the Jamf Connect payload; create those with `jamfplatform_pro_macos_configuration_profile` (or in the Jamf Pro UI) first and reference the profile's `id` as `profile_id`. " +
			"**Adopting a profile applies the configured deployment settings immediately.** A profile left at the default `auto_deployment_type = \"NONE\"` turns automatic deployment off. " +
			"Destroying this resource does not remove Jamf Connect from the configuration profile and does not change the profile itself. It only stops Terraform from managing the deployment and update settings; the settings already applied remain in place. " +
			"Import with `terraform import jamfplatform_pro_jamf_connect.<name> <profile_id>`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Resource identifier. Equals `profile_id`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"profile_id": schema.Int64Attribute{
				MarkdownDescription: "Jamf Pro ID of the configuration profile to manage: the `id` of a `jamfplatform_pro_macos_configuration_profile` that contains a Jamf Connect payload. The profile must already exist and carry a Jamf Connect payload (it then appears automatically under Settings → Jamf apps → Jamf Connect); otherwise apply fails. Changing it manages a different profile and forces replacement.",
				Required:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"config_profile_uuid": schema.StringAttribute{
				MarkdownDescription: "Jamf Connect identifier of the managed configuration profile, resolved from `profile_id`. This is the profile's Jamf Connect UUID and is **not** the same as the configuration profile's own `uuid`.",
				Computed:            true,
			},
			"auto_deployment_type": schema.StringAttribute{
				MarkdownDescription: "How Jamf Connect is deployed and updated on the profile's computers, matching the Jamf Pro \"Update Type\" choices. " +
					"`NONE`: automatic deployment is off (the deploy toggle is No), and `version` is ignored. " +
					"`INITIAL_INSTALLATION_ONLY` (\"Manual\"): deploys the chosen `version` for the initial install only; later updates are manual. " +
					"`PATCH_UPDATES` (\"Maintenance\"): deploys and keeps the app updated with patch releases. " +
					"`MINOR_AND_PATCH_UPDATES` (\"Minor & Maintenance\"): deploys and keeps the app updated with minor and patch releases. " +
					"Defaults to `NONE`.",
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(autoDeploymentNone),
				Validators: []validator.String{
					stringvalidator.OneOf(
						autoDeploymentNone,
						autoDeploymentInitialOnly,
						autoDeploymentPatch,
						autoDeploymentMinorAndPatch,
					),
				},
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Jamf Connect version to deploy (e.g. `2.45.1`), as offered in the Jamf Pro version picker. **Required** when `auto_deployment_type` is anything other than `NONE`, and **must be omitted** when it is `NONE` (Jamf Connect ignores the version in that mode). Must be Jamf Connect 2.3.0 or higher. " +
					"Jamf Pro does not allow lowering an already-deployed version; only the same or a higher version is accepted.",
				Optional: true,
			},
			"profile_name": schema.StringAttribute{
				MarkdownDescription: "Display name of the underlying configuration profile.",
				Computed:            true,
			},
			"scope_description": schema.StringAttribute{
				MarkdownDescription: "Human-readable summary of the configuration profile's scope.",
				Computed:            true,
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "ID of the site the configuration profile belongs to. `-1` means none.",
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

// ConfigValidators returns the plan-time cross-field validators. The single
// validator enforces the version ↔ auto_deployment_type dependency Jamf
// Connect enforces on the wire (version required unless NONE, and rejected
// when NONE). See validators.go.
func (r *JamfConnectResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		versionDeploymentTypeValidator{},
	}
}

// Configure wires the Jamf Pro client into the resource.
func (r *JamfConnectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_jamf_connect")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro configuration profile ID. The id
// passes through to the string `id` attribute; Read derives `profile_id` from
// it (see crud.go resolveProfileID).
//
//	terraform import jamfplatform_pro_jamf_connect.<name> <profile_id>
func (r *JamfConnectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
