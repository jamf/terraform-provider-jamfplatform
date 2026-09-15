// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package local_admin_password_settings implements the
// jamfplatform_pro_local_admin_password_settings singleton resource. It wraps the
// LAPS (local administrator password) section of the Jamf Pro Security page —
// the PreStage-accounts toggle and the two rotation-interval dropdowns that
// govern how passwords for managed local administrator accounts are rotated.
package local_admin_password_settings

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required.
// Empty: the LAPS settings endpoint ships at the provider's overall floor,
// matching the other settings singletons. LAPS is a more recent feature, but a
// version floor was not warranted on the tenants available during the build;
// revisit if an older tenant is found to 404 the endpoint.
const minJamfProVersion = ""

// LocalAdminPasswordSettingsResource implements the singleton Jamf Pro LAPS
// settings resource.
//
// The resource is backed by an Update-only Jamf Pro API — one LAPS settings
// object per tenant. Create funnels into a full-replace write. Delete is
// state-only by design.
type LocalAdminPasswordSettingsResource struct {
	client *pro.Client
}

var _ resource.Resource = &LocalAdminPasswordSettingsResource{}
var _ resource.ResourceWithImportState = &LocalAdminPasswordSettingsResource{}
var _ resource.ResourceWithIdentity = &LocalAdminPasswordSettingsResource{}

// Default timeouts.
const (
	defaultCreateTimeout = 30 * time.Second
	defaultReadTimeout   = 30 * time.Second
	defaultUpdateTimeout = 30 * time.Second
	defaultDeleteTimeout = 30 * time.Second
)

// NewLocalAdminPasswordSettingsResource constructs a new resource.
func NewLocalAdminPasswordSettingsResource() resource.Resource {
	return &LocalAdminPasswordSettingsResource{}
}

// Metadata sets the resource type name.
func (r *LocalAdminPasswordSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_local_admin_password_settings"
}

// IdentitySchema defines the import identifier — singleton id only.
func (r *LocalAdminPasswordSettingsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\".",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the resource schema.
func (r *LocalAdminPasswordSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Pro **local administrator password (LAPS)** settings (UI: Settings → Computer Management → Security → \"Password settings for managed local administrator accounts\"). One record per tenant. These settings apply to all managed local administrator accounts configured in User-initiated enrollment settings and computer PreStage enrollments.\n\n" +
			"A control you omit keeps its current Jamf Pro value, including on the first apply: the resource adopts the existing settings and changes only the controls you declare. A control you do set is managed by Terraform, and is restored if someone edits it in the Jamf Pro UI. Manage a subset and leave the rest as configured in the admin console.\n\n" +
			"`terraform destroy` removes the resource from Terraform state only. The LAPS settings stay intact on the tenant; they cannot be deleted.\n\n" +
			"Import with `terraform import jamfplatform_pro_local_admin_password_settings.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},

			// All three controls are Optional+Computed with UseStateForUnknown:
			// the write is full-replace, but omitting a control carries its prior
			// value forward (plan Unknown -> USFU -> prior state -> re-emitted ->
			// preserved). On first create there is no prior state, so Create reads
			// the live settings and merges them in (see crud.go) — the singleton is
			// adopted, not reset.
			"laps_for_prestage_accounts_enabled": schema.BoolAttribute{
				MarkdownDescription: "Enable LAPS for PreStage accounts. Enables LAPS for all new and existing managed local administrator accounts created via PreStage enrollment. Disabling it prevents LAPS on new accounts but does not disable LAPS on existing accounts. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"rotation_interval": schema.StringAttribute{
				MarkdownDescription: "How often passwords for managed local administrator accounts are automatically rotated. Matches the \"Rotation interval\" dropdown. `Never` turns automatic rotation off. One of: " +
					markdownValueList(validRotationInterval) + ". Omit to leave the current value untouched.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators: []validator.String{
					stringvalidator.OneOf(validRotationInterval...),
				},
			},
			"rotation_after_viewing_interval": schema.StringAttribute{
				MarkdownDescription: "How long after a password is viewed in the inventory record before it is rotated. Matches the \"Rotation after viewing interval\" dropdown. One of: " +
					markdownValueList(validRotationAfterViewingInterval) + ". Omit to leave the current value untouched.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators: []validator.String{
					stringvalidator.OneOf(validRotationAfterViewingInterval...),
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

// Configure wires the Jamf Pro client into the resource.
func (r *LocalAdminPasswordSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_local_admin_password_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton.
func (r *LocalAdminPasswordSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_local_admin_password_settings")
}
