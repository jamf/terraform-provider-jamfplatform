// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package jamf_protect implements the jamfplatform_pro_jamf_protect
// registration singleton resource and the jamfplatform_pro_jamf_protect_plans
// plural data source, backed by the Jamf Pro Jamf Protect integration API
// (/api/pro/v1/jamf-protect).
package jamf_protect

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

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: the Jamf Protect integration endpoints are a long-standing
// part of the Pro API, present at the provider's overall floor.
const minJamfProVersion = ""

// JamfProtectResource implements the presence-optional registration singleton
// for the Jamf Pro ↔ Jamf Protect integration.
//
// Unlike pure settings singletons (e.g. self_service_plus_settings), the
// resource's existence means "this Jamf Pro tenant is registered to a Jamf
// Protect instance":
//
//	Create = POST /register (201; credentials validated live against Protect).
//	Read   = GET (404 ⇒ unregistered ⇒ RemoveResource).
//	Update = re-register POST in place (credential change) and/or PUT (auto_install).
//	Delete = DELETE (unregister; idempotent 204).
type JamfProtectResource struct {
	client *pro.Client
}

var _ resource.Resource = &JamfProtectResource{}
var _ resource.ResourceWithImportState = &JamfProtectResource{}
var _ resource.ResourceWithIdentity = &JamfProtectResource{}

const (
	// defaultCreateTimeout is longer than the other operations: Create both
	// registers and fires a plans sync, and a Protect instance with many
	// plans can take a while to respond.
	defaultCreateTimeout = 90 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewJamfProtectResource returns a new instance of the resource.
func NewJamfProtectResource() resource.Resource {
	return &JamfProtectResource{}
}

// Metadata sets the resource type name.
func (r *JamfProtectResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_jamf_protect"
}

// IdentitySchema defines the import identity. Singleton — only the fixed
// helpers.SingletonID value is accepted.
func (r *JamfProtectResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". A Jamf Pro tenant holds at most one Jamf Protect registration.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the Jamf Protect registration resource.
func (r *JamfProtectResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Pro ↔ Jamf Protect registration (Settings → Jamf apps → Jamf Protect). One registration per tenant. Creating this resource registers Jamf Pro with the Jamf Protect instance (credentials are validated live against Protect) and triggers an initial plans sync; destroying it unregisters. " +
			"Changing `api_url`, `client_id`, or bumping `password_wo_version` re-registers in place: Jamf Pro overwrites the existing registration without unregistering first, and a failed credential check leaves the old registration intact. " +
			"Destroying this resource removes the registration only. Configuration profiles already created from Protect plans remain in Jamf Pro, and the synced plans catalog persists (see the `jamfplatform_pro_jamf_protect_plans` data source). " +
			"Import with `terraform import jamfplatform_pro_jamf_protect.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"api_url": schema.StringAttribute{
				MarkdownDescription: "**\"Jamf Protect API URL\"** in the Jamf Pro admin UI. The Jamf Protect tenant's GraphQL API address, e.g. `https://instance.protect.jamfcloud.com/graphql`. Jamf Pro echoes it verbatim, so normal drift detection applies. Changing it triggers an in-place re-register.",
				Required:            true,
			},
			"client_id": schema.StringAttribute{
				MarkdownDescription: "**\"Client ID\"** in the Jamf Pro admin UI. Jamf Protect API client identifier. Create an API client in the Jamf Protect web console to obtain it. Changing it triggers an in-place re-register.",
				Required:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "**\"Password\"** in the Jamf Pro admin UI. Jamf Protect API client password. `WriteOnly`: sent to Jamf Pro on writes, never persisted in Terraform state, and never returned on read. Rotate the stored password by bumping the companion `password_wo_version` integer, which triggers an in-place re-register carrying the current `password`.",
				Required:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `password`. Bump this integer (any change) to force an in-place re-register that re-sends `password` to Jamf Pro. Initial create should set `password_wo_version = 1`.",
				Required:            true,
			},
			"auto_install": schema.BoolAttribute{
				MarkdownDescription: "**\"Automatically deploy the Jamf Protect PKG with plans\"** in the Jamf Pro admin UI. Server default `false`. The only field that can change without re-registering. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"registration_id": schema.StringAttribute{
				MarkdownDescription: "Registration identifier assigned by Jamf Pro; not user-settable. Not stable: a new identifier is minted on every re-register.",
				Computed:            true,
			},
			"api_client_name": schema.StringAttribute{
				MarkdownDescription: "Display name of the API client, as configured in the Jamf Protect web console. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
			},
			"platform_plan_sync": schema.BoolAttribute{
				MarkdownDescription: "Whether platform plan sync is enabled. Read-only: Jamf Pro rejects writes to this field and the admin UI has no control for it.",
				Computed:            true,
			},
			"last_sync_time": schema.StringAttribute{
				MarkdownDescription: "Timestamp of the most recent plans sync. Returned by Jamf Pro; volatile, since it changes whenever a sync runs.",
				Computed:            true,
			},
			"sync_status": schema.StringAttribute{
				MarkdownDescription: "Status of the most recent plans sync. One of `UNKNOWN`, `IN_PROGRESS`, `COMPLETED`, `ERROR`. Returned by Jamf Pro; volatile.",
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

// Configure wires the Jamf Pro client into the resource.
func (r *JamfProtectResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_jamf_protect")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed
// helpers.SingletonID value is accepted.
//
//	terraform import jamfplatform_pro_jamf_protect.<name> singleton
func (r *JamfProtectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_jamf_protect")
}
