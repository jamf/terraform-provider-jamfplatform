// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package licensed_software implements the jamfplatform_pro_licensed_software
// resource, data source, and list resource backed by the Jamf ProClassic
// /licensedsoftware API. The construct name mirrors the Jamf Pro admin UI
// ("Licensed software" under the Computers sidebar).
//
// Two endpoint quirks shape the whole package (wire-probed — see
// LICENSED_SOFTWARE_SPIKE.md):
//   - Licenses and software definitions carry NO server-readable id; GET-by-id
//     returns idless elements and preserves send-order, so both nested lists
//     reconcile POSITIONALLY (ordered List, not Set).
//   - The legacy font_definitions / plugin_definitions buckets are silently
//     dropped by the server on write; only software_definitions round-trips, so
//     they are intentionally not modeled.
package licensed_software

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: classic /licensedsoftware predates the provider's overall
// floor. The provider-level advisory still fires through
// providerdata.ConfigureProClassic when the tenant is below the floor.
const minJamfProVersion = ""

// LicensedSoftwareResource implements the Terraform resource for Jamf Pro
// licensed software records.
type LicensedSoftwareResource struct {
	client *proclassic.Client
}

var _ resource.Resource = &LicensedSoftwareResource{}
var _ resource.ResourceWithImportState = &LicensedSoftwareResource{}
var _ resource.ResourceWithIdentity = &LicensedSoftwareResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 90 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewLicensedSoftwareResource returns a new instance of LicensedSoftwareResource.
func NewLicensedSoftwareResource() resource.Resource {
	return &LicensedSoftwareResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *LicensedSoftwareResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_licensed_software"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *LicensedSoftwareResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro licensed software ID used to uniquely reference the record.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the resource. Attribute names mirror
// the Jamf Pro admin UI labels (STYLE_GUIDE §Attribute names mirror the admin
// UI); differing wire element names are noted in the attribute descriptions.
func (r *LicensedSoftwareResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro licensed software record: the \"Licensed software\" entry under the Computers sidebar in the Jamf Pro admin UI. Tracks software licences and matches installed copies against software definitions. `software_definitions` and `licenses` are ordered lists matched by position, so keep their ordering stable across changes. Only software definitions are supported; legacy font and plug-in definitions are not exposed because Jamf Pro does not retain them." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Licensed software ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Display name for the licensed software record; must be unique within the tenant.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"publisher": schema.StringAttribute{
				MarkdownDescription: "**\"Publisher\"** in the Jamf Pro admin UI. Name of the licensed software publisher. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
			},
			"platform": schema.StringAttribute{
				MarkdownDescription: "**\"Platform\"** in the Jamf Pro admin UI. Platform the software is for. One of `Any`, `Mac`, or `Windows`. Defaults to `Any`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("Any"),
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
				Validators:          []validator.String{stringvalidator.OneOf("Any", "Mac", "Windows")},
			},
			"notes": schema.StringAttribute{
				MarkdownDescription: "**\"Notes\"** in the Jamf Pro admin UI. Notes about the licensed software record. Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
			},
			"send_email_on_violation": schema.BoolAttribute{
				MarkdownDescription: "**\"Send email notification on violation\"** in the Jamf Pro admin UI. Email Jamf Pro users with notifications enabled when the licence count is exceeded (an SMTP server must be configured). Jamf Pro defaults it to `false` on create. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
			},
			"remove_titles_from_inventory_reports": schema.BoolAttribute{
				MarkdownDescription: "**\"Remove titles from inventory reports\"** in the Jamf Pro admin UI. Exclude the matched titles from inventory reports if they are specified in software definitions. Jamf Pro defaults it to `false` on create. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
			},
			"exclude_titles_purchased_from_app_store": schema.BoolAttribute{
				MarkdownDescription: "**\"Exclude titles purchased from the App Store\"** in the Jamf Pro admin UI. Do not count copies of the title purchased from the Mac App Store against the licence count. Jamf Pro defaults it to `false` on create. Omit to leave the current value untouched; set `true`/`false` to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseNonNullStateForUnknown()},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "**\"Site\"** in the Jamf Pro admin UI. Jamf Pro site ID scoping the record. Jamf Pro defaults it to `-1` (\"None\") on create. Omit to leave the current value untouched; set `-1` to clear the site.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
			},
			"site_name": schema.StringAttribute{
				// No UseStateForUnknown: site_name is derived from site_id, so it
				// must go Unknown (not pin a stale value) whenever site_id
				// changes, or it trips the post-apply consistency check.
				MarkdownDescription: "Site display name. Returned by Jamf Pro; not user-settable.",
				Computed:            true,
			},
			"software_definitions": schema.ListNestedAttribute{
				MarkdownDescription: "**\"Software Definitions\"** tab in the Jamf Pro admin UI. Ordered list of definitions used to match installed software, matched by position, so keep their ordering stable across changes. Omit the attribute to leave any existing definitions unmanaged (Jamf Pro keeps them); set it to `[]` to clear all definitions; otherwise the list fully replaces what is stored. Legacy font and plug-in definitions are not supported.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "**\"App Name\"** in the Jamf Pro admin UI. The application or software title to match.",
							Required:            true,
							Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
						},
						"version": schema.StringAttribute{
							MarkdownDescription: "The software version to match. Leave unset to match any version.",
							Optional:            true,
						},
						"compare_type": schema.StringAttribute{
							MarkdownDescription: "How `version` is compared. One of `is` or `like`. Defaults to `like`; Jamf Pro coerces any other value to `like`.",
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString(proclassic.LicensedSoftwareDefintionCompareTypeLike),
							PlanModifiers:       []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
							Validators:          []validator.String{stringvalidator.OneOf(proclassic.LicensedSoftwareDefintionCompareTypeValues()...)},
						},
					},
				},
			},
			"licenses": schema.ListNestedAttribute{
				MarkdownDescription: "**\"Licenses\"** tab in the Jamf Pro admin UI. Ordered list of licence entries matched by position, so keep their ordering stable across changes. Omit the attribute to leave any existing licences unmanaged (Jamf Pro keeps them); set it to `[]` to clear all licences; otherwise the list fully replaces what is stored.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"serial_number_1": schema.StringAttribute{
							MarkdownDescription: "**\"Serial Number 1\"** in the Jamf Pro admin UI.",
							Optional:            true,
						},
						"serial_number_2": schema.StringAttribute{
							MarkdownDescription: "**\"Serial Number 2\"** in the Jamf Pro admin UI.",
							Optional:            true,
						},
						"organization_name": schema.StringAttribute{
							MarkdownDescription: "**\"Organization Name\"** in the Jamf Pro admin UI. Name of the organization the licence is registered to.",
							Optional:            true,
						},
						// Wire/attribute name: registered_to. UI label is "License".
						"registered_to": schema.StringAttribute{
							MarkdownDescription: "**\"License\"** in the Jamf Pro admin UI. Name of the person the licence is registered to.",
							Optional:            true,
						},
						"license_type": schema.StringAttribute{
							MarkdownDescription: "**\"License Type\"** in the Jamf Pro admin UI. Type of licence obtained for the software, e.g. `Standard`, `Concurrent`, or `Site License`.",
							Optional:            true,
						},
						"license_count": schema.Int64Attribute{
							MarkdownDescription: "**\"License Count\"** in the Jamf Pro admin UI. Number of licences owned. Defaults to `0` (unlimited).",
							Optional:            true,
							Computed:            true,
							Default:             int64default.StaticInt64(0),
							PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseNonNullStateForUnknown()},
						},
						"notes": schema.StringAttribute{
							MarkdownDescription: "Notes about this licence.",
							Optional:            true,
						},
						"purchasing": schema.SingleNestedAttribute{
							MarkdownDescription: "**\"Purchasing Information\"** tab for the licence in the Jamf Pro admin UI.",
							Optional:            true,
							Attributes: map[string]schema.Attribute{
								"license_term": schema.StringAttribute{
									MarkdownDescription: "**\"License Term\"** in the Jamf Pro admin UI. One of `perpetual` or `annual`. Required when the `purchasing` block is set.",
									Required:            true,
									Validators:          []validator.String{stringvalidator.OneOf("perpetual", "annual")},
								},
								"po_number": schema.StringAttribute{
									MarkdownDescription: "**\"Purchase Order Number\"** in the Jamf Pro admin UI.",
									Optional:            true,
								},
								"po_date": schema.StringAttribute{
									MarkdownDescription: "**\"Purchase Order Date\"** in the Jamf Pro admin UI. Format `YYYY-MM-DD`.",
									Optional:            true,
								},
								"po_date_epoch": schema.Int64Attribute{
									MarkdownDescription: "Unix-epoch (milliseconds) form of `po_date`. Returned by Jamf Pro; not user-settable.",
									Computed:            true,
								},
								"po_date_utc": schema.StringAttribute{
									MarkdownDescription: "ISO-8601 UTC form of `po_date`. Returned by Jamf Pro; not user-settable.",
									Computed:            true,
								},
								"vendor": schema.StringAttribute{
									MarkdownDescription: "**\"Vendor\"** in the Jamf Pro admin UI.",
									Optional:            true,
								},
								"license_expires": schema.StringAttribute{
									MarkdownDescription: "**\"Expiration Date\"** in the Jamf Pro admin UI. Format `YYYY-MM-DD`.",
									Optional:            true,
								},
								"license_expires_epoch": schema.Int64Attribute{
									MarkdownDescription: "Unix-epoch (milliseconds) form of `license_expires`. Returned by Jamf Pro; not user-settable.",
									Computed:            true,
								},
								"license_expires_utc": schema.StringAttribute{
									MarkdownDescription: "ISO-8601 UTC form of `license_expires`. Returned by Jamf Pro; not user-settable.",
									Computed:            true,
								},
								"purchase_price": schema.StringAttribute{
									MarkdownDescription: "**\"Purchase Price\"** in the Jamf Pro admin UI. Entered as a free-text string (e.g. `1999.00`).",
									Optional:            true,
								},
								"life_expectancy": schema.Int64Attribute{
									MarkdownDescription: "**\"Life Expectancy\"** in the Jamf Pro admin UI. Expected life of the licence in years (1–5). Leave unset for none.",
									Optional:            true,
									Validators:          []validator.Int64{int64validator.Between(1, 5)},
								},
								"purchasing_account": schema.StringAttribute{
									MarkdownDescription: "**\"Purchasing Account\"** in the Jamf Pro admin UI.",
									Optional:            true,
								},
								"purchasing_contact": schema.StringAttribute{
									MarkdownDescription: "**\"Purchasing Contact\"** in the Jamf Pro admin UI.",
									Optional:            true,
								},
							},
						},
						"attachments": schema.ListNestedAttribute{
							MarkdownDescription: "Read-only licence attachments. Uploaded via the Jamf Pro admin UI \"Attachments\" tab; surfaced here for reference only.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"id":       schema.StringAttribute{MarkdownDescription: "Attachment ID.", Computed: true},
									"filename": schema.StringAttribute{MarkdownDescription: "Attachment file name.", Computed: true},
									"uri":      schema.StringAttribute{MarkdownDescription: "Attachment download URI.", Computed: true},
								},
							},
						},
					},
				},
			},
			"computers": schema.ListNestedAttribute{
				MarkdownDescription: "Read-only set of computers Jamf Pro matched against the software definitions (the admin UI usage view). Populated by Jamf Pro; not user-settable.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":   schema.StringAttribute{MarkdownDescription: "Computer ID.", Computed: true},
						"name": schema.StringAttribute{MarkdownDescription: "Computer name.", Computed: true},
						"udid": schema.StringAttribute{MarkdownDescription: "Computer UDID.", Computed: true},
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

// Configure wires the Jamf ProClassic client into the resource via the shared
// providerdata.ConfigureProClassic helper.
func (r *LicensedSoftwareResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_licensed_software")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro licensed software ID.
func (r *LicensedSoftwareResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
