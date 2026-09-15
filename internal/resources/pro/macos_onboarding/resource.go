// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package macos_onboarding implements the jamfplatform_pro_macos_onboarding
// singleton resource and data source backed by the Jamf Pro macOS Onboarding
// settings API (Settings > Self Service > macOS Onboarding), plus a parameterised
// eligible-items discovery data source.
package macos_onboarding

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty to match every other settings singleton — the /v1/onboarding endpoint is
// present at the provider's overall floor, so no per-resource gate is applied.
const minJamfProVersion = ""

// OnboardingResource implements the singleton resource for the Jamf Pro macOS
// Onboarding settings. Backed by an Update-only API (no Create/Delete on the
// remote): Create funnels into Update; Delete is a no-op that only removes the
// object from Terraform state.
type OnboardingResource struct {
	client *pro.Client
}

var _ resource.Resource = &OnboardingResource{}
var _ resource.ResourceWithImportState = &OnboardingResource{}
var _ resource.ResourceWithIdentity = &OnboardingResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewOnboardingResource returns a new instance of the resource.
func NewOnboardingResource() resource.Resource {
	return &OnboardingResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *OnboardingResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_macos_onboarding"
}

// IdentitySchema defines the identifier used for import. Singleton resources accept
// only the fixed helpers.SingletonID value.
func (r *OnboardingResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". macOS Onboarding settings are one per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the macOS Onboarding settings resource.
func (r *OnboardingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Pro macOS Onboarding configuration (Settings > Self Service > macOS Onboarding). " +
			"One record per tenant. " +
			"Onboarding presents users with a curated, ordered list of Self Service items during macOS onboarding: policies, configuration profiles and apps. " +
			"The `onboarding_items` list fully replaces what is stored. Declare the complete set in the order users should see them. An item you remove is removed from onboarding, and `onboarding_items = []` clears every item. " +
			"`priority` follows the list order automatically. " +
			"Import with `terraform import jamfplatform_pro_macos_onboarding.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether macOS Onboarding is enabled for the tenant (the top-right \"Enabled\" toggle). " +
					"Items may be staged while disabled: `enabled = false` with a populated `onboarding_items` list is accepted.",
				Required: true,
			},
			"onboarding_items": schema.ListNestedAttribute{
				MarkdownDescription: "Ordered list of Self Service items presented during macOS onboarding. " +
					"Order is significant: items appear to users in this order, and `priority` follows it (1-based). " +
					"This list fully replaces what is stored, so declare the complete set. An item removed here is removed from onboarding, and `[]` clears every item.",
				Required: true,
				// The server-derived nested fields (priority, id, entity_name,
				// scope_description, site_description) are modelled as plain Computed with
				// NO plan modifier. This is deliberate and safe-by-construction against two
				// wire-probed facts: (1) the server reminds each item's id on every write,
				// and (2) array order != priority order so a list edit/reorder changes
				// several elements at once. With no UseStateForUnknown, a changed/reordered
				// element's Computed children go Unknown and are filled from the post-write
				// GET — never trips "inconsistent result after apply". A true no-op preserves
				// them (the framework keeps prior Computed values when nothing changes).
				// §240's UseNonNullStateForUnknown rule is for Optional+Computed nested
				// fields (the append-null trap), not Computed-only echoes. Precedent:
				// jamfplatform_pro_licensed_software's Computed-only po_date_epoch/utc and
				// license_expires_epoch/utc nested fields ship modifier-free and acc-green.
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"entity_id": schema.StringAttribute{
							MarkdownDescription: "ID of the Jamf Pro object to present, paired with `self_service_entity_type`. " +
								"Source per type: `OS_X_POLICY` → `jamfplatform_pro_policy`; `OS_X_CONFIG_PROFILE` → `jamfplatform_pro_macos_configuration_profile`; " +
								"`OS_X_MAC_APP` → `jamfplatform_pro_mac_app_store_app`; `OS_X_APP_INSTALLER` → `jamfplatform_pro_app_installer`. " +
								"The `jamfplatform_pro_macos_onboarding_eligible_items` data source lists the eligible IDs for every type. " +
								"The referenced object must be enabled and available in Self Service. Jamf Pro rejects an ineligible item with a clear error.",
							Required: true,
						},
						"self_service_entity_type": schema.StringAttribute{
							MarkdownDescription: "Type of the referenced object. One of `OS_X_POLICY`, `OS_X_CONFIG_PROFILE`, `OS_X_MAC_APP`, `OS_X_APP_INSTALLER`.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf(validEntityTypes...),
							},
						},
						"priority": schema.Int64Attribute{
							MarkdownDescription: "Presentation order, derived automatically from the `onboarding_items` list order (1-based). Returned by Jamf Pro; not user-settable.",
							Computed:            true,
						},
						"id": schema.StringAttribute{
							MarkdownDescription: "Identifier for this onboarding item. Returned by Jamf Pro; not user-settable. It changes whenever the list is updated, so do not reference it.",
							Computed:            true,
						},
						"entity_name": schema.StringAttribute{
							MarkdownDescription: "Display name of the referenced object. Returned by Jamf Pro; not user-settable.",
							Computed:            true,
						},
						"scope_description": schema.StringAttribute{
							MarkdownDescription: "Scope summary of the referenced object (the \"Scope\" column in the admin UI). Returned by Jamf Pro; not user-settable.",
							Computed:            true,
						},
						"site_description": schema.StringAttribute{
							MarkdownDescription: "Site summary of the referenced object (the \"Site\" column in the admin UI). Returned by Jamf Pro; not user-settable.",
							Computed:            true,
						},
					},
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
func (r *OnboardingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_macos_onboarding")
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
//	terraform import jamfplatform_pro_macos_onboarding.<name> singleton
func (r *OnboardingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_macos_onboarding")
}
