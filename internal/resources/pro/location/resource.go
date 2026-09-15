// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package location implements the jamfplatform_pro_volume_purchasing_location
// resource backed by the Jamf Pro /api/v1/volume-purchasing-locations API.
package location

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: defer to the provider-wide floor via
// providerdata.ConfigurePro — the volume-purchasing-locations endpoint
// predates the provider's overall minimum.
const minJamfProVersion = ""

// VolumePurchasingLocationResource implements the Terraform resource for a
// Jamf Pro Volume Purchasing (VPP) location.
type VolumePurchasingLocationResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                = &VolumePurchasingLocationResource{}
	_ resource.ResourceWithImportState = &VolumePurchasingLocationResource{}
	_ resource.ResourceWithIdentity    = &VolumePurchasingLocationResource{}
)

const (
	// VPP Create can stall behind a real Apple-side content sync. On large
	// catalogs (thousands of titles) Apple has been observed to take several
	// minutes to populate `lastSyncTime`, so the default Create budget is
	// 30 minutes. Users with larger tenants should override via
	// `timeouts { create = "60m" }`.
	defaultCreateTimeout = 30 * time.Minute
	defaultReadTimeout   = 60 * time.Second
	// Update mirrors Create: token rotation re-triggers Apple's sync flow.
	defaultUpdateTimeout = 30 * time.Minute
	defaultDeleteTimeout = 60 * time.Second
	// syncPollInterval is the gap between successive GETs during the
	// post-Create / post-rotation sync convergence loop.
	syncPollInterval = 15 * time.Second
)

// NewVolumePurchasingLocationResource returns a new instance of
// VolumePurchasingLocationResource.
func NewVolumePurchasingLocationResource() resource.Resource {
	return &VolumePurchasingLocationResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *VolumePurchasingLocationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_volume_purchasing_location"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *VolumePurchasingLocationResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro Volume Purchasing (VPP) location ID used to uniquely reference the location.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the volume purchasing location
// resource.
func (r *VolumePurchasingLocationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro Volume Purchasing (VPP) location. " +
			"A VPP location binds a Jamf Pro tenant to an Apple Business Manager / Apple " +
			"School Manager Volume Purchasing account using a `.vpptoken` file (already " +
			"base64-encoded by Apple, so supply the file contents directly via " +
			"`file(\"/path/to/vpp.vpptoken\")`). On create the provider registers the " +
			"location, immediately reclaims licenses to clear any client-context mismatch " +
			"inherited from a previously shared token, then polls until Apple's content " +
			"sync populates `last_sync_time` before committing the resource. The default " +
			"create timeout is 30 minutes; raise it with `timeouts { create = \"60m\" }` " +
			"if your tenant has a large catalog." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Volume Purchasing location ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name for the VPP location in the Jamf Pro admin UI. Must not be empty.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"service_token": schema.StringAttribute{
				MarkdownDescription: "Base64-encoded contents of the `.vpptoken` file downloaded from Apple " +
					"Business Manager / Apple School Manager. The `.vpptoken` file is already a base64 string " +
					"on disk, so supply it directly via `file(\"/path/to/vpp.vpptoken\")`. Do not base64-encode " +
					"it again. `WriteOnly`: the value is sent to Jamf Pro on create and on token-rotating " +
					"updates, but is **never persisted in Terraform state**. Jamf Pro never returns the token on " +
					"reads, so the only signal Terraform can use to rotate the stored token is the companion " +
					"`service_token_wo_version` integer. The provider trims surrounding whitespace from the " +
					"supplied string before sending (Apple's downloaded `.vpptoken` files often carry a " +
					"trailing newline that Jamf Pro rejects).",
				Required:  true,
				Sensitive: true,
				WriteOnly: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"service_token_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Rotation trigger for the `WriteOnly` `service_token`. Bump this integer " +
					"(any change) to force an update that re-sends the current `service_token` to Jamf Pro. " +
					"Initial create should set `service_token_wo_version = 1`. Required because " +
					"`service_token` is itself Required: keeping the companion Required keeps the rotation " +
					"signal explicit in config.",
				Required: true,
			},
			"automatically_populate_purchased_content": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Pro should automatically populate purchased content from " +
					"Apple after every sync. Jamf Pro decides the default on create. Omit to leave the " +
					"current value untouched; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"send_notification_when_no_longer_assigned": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Pro should send a notification when a previously-assigned " +
					"content item is no longer assigned to the location. Jamf Pro decides the default on " +
					"create. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"auto_register_managed_users": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Pro should auto-register managed users associated with this " +
					"location. Jamf Pro decides the default on create. Omit to leave the current value " +
					"untouched; set `true`/`false` to change it.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Optional Jamf Pro site ID to associate with this VPP location. Jamf Pro " +
					"reports the sentinel `\"-1\"` when no site is set; the provider mirrors whatever Jamf Pro " +
					"reports into state and does not apply a default. Omit to leave the current value " +
					"untouched; set `-1` to clear the site association.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"site_name": schema.StringAttribute{
				// No UseStateForUnknown: derived from the mutable site_id, so it
				// must go Unknown when site_id changes. See STYLE_GUIDE §886.
				MarkdownDescription: "Site display name for the associated `site_id`. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
			},
			"apple_id": schema.StringAttribute{
				MarkdownDescription: "Apple ID associated with the uploaded service token.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"organization_name": schema.StringAttribute{
				MarkdownDescription: "Organization name parsed from the uploaded service token. Apple may " +
					"return values containing trailing whitespace; the provider preserves the exact value " +
					"Jamf Pro reports.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"location_name": schema.StringAttribute{
				MarkdownDescription: "Apple-returned location name (distinct from the user-supplied `name`). " +
					"Apple may return values containing trailing whitespace; the provider preserves the " +
					"exact value Jamf Pro reports.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"country_code": schema.StringAttribute{
				MarkdownDescription: "Apple-returned country code for the location.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "Apple-returned contact email for the location.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"token_expiration": schema.StringAttribute{
				MarkdownDescription: "ISO 8601 expiration timestamp for the uploaded service token.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"total_purchased_licenses": schema.Int64Attribute{
				MarkdownDescription: "Total number of licenses purchased across all content items for this location. " +
					"Returned by Jamf Pro; not user-settable. Apple may resync this value between Terraform " +
					"applies, so the attribute is shown as `(known after apply)` on every update to avoid " +
					"`inconsistent result after apply` errors.",
				Computed: true,
			},
			"total_used_licenses": schema.Int64Attribute{
				MarkdownDescription: "Total number of licenses currently in use across all content items for this location. " +
					"Returned by Jamf Pro; not user-settable. See `total_purchased_licenses` for the rationale " +
					"on why this attribute does not reuse the prior state on update.",
				Computed: true,
			},
			"last_sync_time": schema.StringAttribute{
				MarkdownDescription: "ISO 8601 timestamp of the most recent Apple content sync for this location. " +
					"Empty until Apple completes the initial sync after a create or token rotation. " +
					"Returned by Jamf Pro; not user-settable. Apple may update this value between Terraform " +
					"applies, so the attribute is shown as `(known after apply)` on every update to avoid " +
					"`inconsistent result after apply` errors when Apple syncs between plan and apply.",
				Computed: true,
			},
			"client_context_mismatch": schema.BoolAttribute{
				MarkdownDescription: "Whether Jamf Pro detected a client-context mismatch for this location. " +
					"Should be `false` after a successful create, because the provider reclaims licenses " +
					"immediately after registering the location to clear residual mismatches inherited from a " +
					"previously shared service token. Returned by Jamf Pro; not user-settable. Jamf Pro " +
					"recomputes this value on every read so the attribute is shown as `(known after apply)` " +
					"on update.",
				Computed: true,
			},
			"content": schema.ListNestedAttribute{
				MarkdownDescription: "Apple-returned purchased-content catalog for this location, one row per " +
					"`adam_id`. Returned by Jamf Pro; not user-settable. Consumers such as the mobile device app " +
					"and Mac app resources can look up `license_count_total` / `license_count_in_use` for a given " +
					"`adam_id` to verify a license is available before assigning. The catalog mirrors the most " +
					"recent Apple sync, which can update independently of Terraform applies, so the attribute " +
					"is shown as `(known after apply)` on every update.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"adam_id": schema.StringAttribute{
							MarkdownDescription: "Apple App Store / iTunes adam_id for this content item.",
							Computed:            true,
						},
						"content_type": schema.StringAttribute{
							MarkdownDescription: "Apple-reported content type (e.g. `App`, `Book`).",
							Computed:            true,
						},
						"device_types": schema.ListAttribute{
							MarkdownDescription: "Device types this content item targets (e.g. `[\"iphone\", \"ipad\"]`).",
							Computed:            true,
							ElementType:         types.StringType,
						},
						"icon_url": schema.StringAttribute{
							MarkdownDescription: "URL of the App Store / iTunes icon artwork for this content item.",
							Computed:            true,
						},
						"license_count_in_use": schema.Int64Attribute{
							MarkdownDescription: "Number of licenses Jamf Pro reports as currently assigned for this content item.",
							Computed:            true,
						},
						"license_count_reported": schema.Int64Attribute{
							MarkdownDescription: "Number of licenses Apple last reported for this content item.",
							Computed:            true,
						},
						"license_count_total": schema.Int64Attribute{
							MarkdownDescription: "Total number of licenses purchased for this content item.",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Apple-reported display name for this content item.",
							Computed:            true,
						},
						"pricing_param": schema.StringAttribute{
							MarkdownDescription: "Apple-reported pricing parameter for this content item (e.g. `STDQ`).",
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
func (r *VolumePurchasingLocationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_volume_purchasing_location")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro VPP location ID.
func (r *VolumePurchasingLocationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
