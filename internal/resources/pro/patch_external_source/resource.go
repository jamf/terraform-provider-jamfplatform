// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package patch_external_source implements the jamfplatform_pro_patch_external_source
// resource, data source, and list resource backed by the Jamf ProClassic
// patch external sources API.
package patch_external_source

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty: the classic /patchexternalsources endpoint predates the provider's overall
// floor (11.0.0). The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below ProviderMinJamfProVersion.
const minJamfProVersion = ""

// PatchExternalSourceResource implements the Terraform resource for Jamf ProClassic
// patch external sources.
type PatchExternalSourceResource struct {
	client *proclassic.Client
}

var _ resource.Resource = &PatchExternalSourceResource{}
var _ resource.ResourceWithImportState = &PatchExternalSourceResource{}
var _ resource.ResourceWithIdentity = &PatchExternalSourceResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewPatchExternalSourceResource returns a new instance of PatchExternalSourceResource.
func NewPatchExternalSourceResource() resource.Resource {
	return &PatchExternalSourceResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *PatchExternalSourceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_patch_external_source"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *PatchExternalSourceResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro patch external source ID used to uniquely reference the source.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the patch external source resource.
func (r *PatchExternalSourceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro patch external source, configured in the UI under **Settings → Computer management → Patch management**, in the **Patch External Source** section (the **New External Patch Source** form). An external patch source hosts third-party software title definitions for patch management to consume." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Patch external source ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name of the patch external source. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the patch external source is enabled. Jamf Pro applies its own default on create. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"host_name": schema.StringAttribute{
				MarkdownDescription: "Server host name of the external patch source (the server portion of the UI \"Server and Port\" field). Required and must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "TCP port of the external patch source (the port portion of the UI \"Server and Port\" field). May be left unset, and an empty value is treated as unset; removing a previously set port clears it. Must be at least 1 when set. Jamf Pro reports an unset port as empty, which the provider stores as null, so an explicit `0` is rejected at plan time to keep that mapping consistent.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"ssl_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the source is contacted over SSL (UI \"Use SSL\"). Jamf Pro applies its own default on create. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"certificate_validation_enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether software title definitions must be signed by a publicly trusted certificate before being downloaded from the source (UI \"Validate Software Title Definitions\"); unsigned definitions are not downloaded. Jamf Pro applies its own default on create. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
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
func (r *PatchExternalSourceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_patch_external_source")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro patch external source ID.
func (r *PatchExternalSourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
