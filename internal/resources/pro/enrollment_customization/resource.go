// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package enrollment_customization implements the jamfplatform_pro_enrollment_customization
// resource, data source, and list resource backed by the Jamf Pro enrollment
// customization API.
package enrollment_customization

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: defer to the provider-wide floor via
// providerdata.ConfigurePro — the enrollment-customization v2 endpoints
// predate the provider's overall minimum (matches the sibling
// automated_device_enrollment resource).
const minJamfProVersion = ""

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// EnrollmentCustomizationResource implements the Terraform resource.
type EnrollmentCustomizationResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                = &EnrollmentCustomizationResource{}
	_ resource.ResourceWithImportState = &EnrollmentCustomizationResource{}
	_ resource.ResourceWithIdentity    = &EnrollmentCustomizationResource{}
	_ resource.ResourceWithModifyPlan  = &EnrollmentCustomizationResource{}
)

// NewEnrollmentCustomizationResource returns a new instance of the resource.
func NewEnrollmentCustomizationResource() resource.Resource {
	return &EnrollmentCustomizationResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *EnrollmentCustomizationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_enrollment_customization"
}

// IdentitySchema defines the identifier used for import.
func (r *EnrollmentCustomizationResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro enrollment customization ID used to uniquely reference the customization.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the enrollment customization
// resource.
func (r *EnrollmentCustomizationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro enrollment customization: the parent record carrying the branding palette plus any combination of text, LDAP, and SSO authentication panes shown to users during enrollment. " +
			"At most one authentication pane (either LDAP or SSO) can be configured per customization; the two are mutually exclusive. " +
			"Supply the icon either as a source the provider uploads (`icon_source`) or as a URL you uploaded yourself (`branding_settings.icon_url`); the two are mutually exclusive. " +
			"The provider hashes a local `icon_source` on every plan and re-uploads it when the bytes change. It reads an `http(s)://` one during apply and compares the URL string at plan time, so re-pointing the URL uploads the new image, and the provider will not see a file published behind an unchanged URL." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Enrollment customization ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display name for the customization shown in the Jamf Pro admin UI. Must not be blank. Display names are not enforced unique by Jamf Pro; the data source `ResolveByName` lookup surfaces an error when the name resolves to more than one customization.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Administrator-visible description for the customization. Must not be blank.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Optional Jamf Pro site ID to associate with this customization. Jamf Pro reports the sentinel `\"-1\"` when no site is set; the provider mirrors that value into state.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"icon_source": schema.StringAttribute{
				MarkdownDescription: "Local filesystem path or `http(s)://` URL of the icon image to upload to Jamf Pro. The provider reads a local path on every plan and re-uploads when the bytes change. It reads a URL during apply only, so a plan compares the URL string rather than the bytes it serves. Mutually exclusive with `branding_settings.icon_url`; supply one or the other. When neither is set the customization is created without an icon.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ConflictsWith(path.MatchRoot("branding_settings").AtName("icon_url")),
				},
			},
			"icon_source_hash": schema.StringAttribute{
				MarkdownDescription: "SHA-256 of the most recently uploaded icon bytes, prefixed `sha256:`. The provider computes it during apply from the bytes it sends, so it reads `(known after apply)` on any plan that uploads an icon. An imported customization carries no hash, because nothing in a read recovers one. The first apply that gives it an `icon_source` therefore uploads the image once. Read-only.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"branding_settings": schema.SingleNestedAttribute{
				MarkdownDescription: "Palette of colours plus the icon URL shown to users during enrollment. All four colour attributes are required and must be six-digit hexadecimal RGB without the leading `#`.",
				Required:            true,
				Attributes: map[string]schema.Attribute{
					"body_text_color": schema.StringAttribute{
						MarkdownDescription: "**\"Body Text Color\"** in the Jamf Pro admin UI. Six-digit hex RGB without the leading `#` (e.g. `333333`).",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(hexColorPattern, "must be a six-digit hexadecimal RGB value without the leading '#'"),
						},
					},
					"button_color": schema.StringAttribute{
						MarkdownDescription: "**\"Button Color\"** in the Jamf Pro admin UI. Six-digit hex RGB without the leading `#`.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(hexColorPattern, "must be a six-digit hexadecimal RGB value without the leading '#'"),
						},
					},
					"button_text_color": schema.StringAttribute{
						MarkdownDescription: "**\"Button Text Color\"** in the Jamf Pro admin UI. Six-digit hex RGB without the leading `#`.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(hexColorPattern, "must be a six-digit hexadecimal RGB value without the leading '#'"),
						},
					},
					"background_color": schema.StringAttribute{
						MarkdownDescription: "**\"Background Color\"** in the Jamf Pro admin UI. Six-digit hex RGB without the leading `#`.",
						Required:            true,
						Validators: []validator.String{
							stringvalidator.RegexMatches(hexColorPattern, "must be a six-digit hexadecimal RGB value without the leading '#'"),
						},
					},
					"icon_url": schema.StringAttribute{
						MarkdownDescription: "Pre-uploaded icon URL returned by Jamf Pro after a prior image upload. Supply this when you have already uploaded the image out-of-band; otherwise leave unset and use `icon_source` to have the provider manage the upload. Mutually exclusive with `icon_source`.",
						Optional:            true,
						Computed:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
						Validators: []validator.String{
							stringvalidator.ConflictsWith(path.MatchRoot("icon_source")),
						},
					},
				},
			},
			"text_panes": schema.ListNestedAttribute{
				MarkdownDescription: "Ordered list of text panes shown during enrollment. Jamf Pro assigns each pane an `id` after creation, and the framework reconciles panes by list position, so reordering elements in the middle of the list will trigger a churn of create + delete operations. Append new panes to the end of the list to avoid churn.",
				Optional:            true,
				// Jamf Pro tolerates duplicate display names server-side, but
				// duplicates make admin-UI navigation ambiguous.
				Validators: []validator.List{
					validators.UniqueStringFieldList("display_name"),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Panel ID assigned by Jamf Pro on first save. Returned by Jamf Pro; not user-settable.",
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseNonNullStateForUnknown(),
							},
						},
						"display_name": schema.StringAttribute{
							MarkdownDescription: "Display name for the pane shown in the Jamf Pro admin UI. Must be unique across all text panes within this customization.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
						"rank": schema.Int64Attribute{
							MarkdownDescription: "Ordering rank for the pane (lower ranks shown first). Jamf Pro does not enforce uniqueness on `rank`; ties are tolerated.",
							Required:            true,
							Validators: []validator.Int64{
								int64validator.AtLeast(0),
							},
						},
						"title": schema.StringAttribute{
							MarkdownDescription: "Title shown at the top of the pane.",
							Required:            true,
						},
						"body": schema.StringAttribute{
							MarkdownDescription: "Body text shown in the pane. Markdown is supported by Jamf Pro and rendered at enrollment time.",
							Required:            true,
						},
						"subtext": schema.StringAttribute{
							MarkdownDescription: "Optional subtitle shown beneath the title. Jamf Pro returns an empty string when omitted.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseNonNullStateForUnknown(),
							},
						},
						"previous_button_text": schema.StringAttribute{
							MarkdownDescription: "**\"Previous Button Text\"** in the Jamf Pro admin UI. Label for the back-navigation button.",
							Required:            true,
						},
						"next_button_text": schema.StringAttribute{
							MarkdownDescription: "**\"Next Button Text\"** in the Jamf Pro admin UI. Label for the forward-navigation button.",
							Required:            true,
						},
					},
				},
			},
			"ldap_panes": schema.ListNestedAttribute{
				MarkdownDescription: "Optional LDAP authentication pane. At most one authentication pane (LDAP or SSO) may be configured per customization; supplying both `ldap_panes` and `sso_panes` is rejected at plan time.",
				Optional:            true,
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
					listvalidator.ConflictsWith(path.MatchRoot("sso_panes")),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Panel ID assigned by Jamf Pro on first save. Returned by Jamf Pro; not user-settable.",
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseNonNullStateForUnknown(),
							},
						},
						"display_name": schema.StringAttribute{
							MarkdownDescription: "Display name for the LDAP pane shown in the Jamf Pro admin UI.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
						"rank": schema.Int64Attribute{
							MarkdownDescription: "Ordering rank for the pane (lower ranks shown first).",
							Required:            true,
							Validators: []validator.Int64{
								int64validator.AtLeast(0),
							},
						},
						"title": schema.StringAttribute{
							MarkdownDescription: "Title shown at the top of the LDAP authentication pane.",
							Required:            true,
						},
						"username_text": schema.StringAttribute{
							MarkdownDescription: "**\"Username Text\"** in the Jamf Pro admin UI. Label shown above the username input field.",
							Required:            true,
						},
						"password_text": schema.StringAttribute{
							MarkdownDescription: "**\"Password Text\"** in the Jamf Pro admin UI. Label shown above the password input field.",
							Required:            true,
						},
						"previous_button_text": schema.StringAttribute{
							MarkdownDescription: "**\"Previous Button Text\"** in the Jamf Pro admin UI. Label for the back-navigation button.",
							Required:            true,
						},
						"login_button_text": schema.StringAttribute{
							MarkdownDescription: "**\"Login Button Text\"** in the Jamf Pro admin UI. Label for the submit button.",
							Required:            true,
						},
						"directory_service_groups": schema.ListNestedAttribute{
							MarkdownDescription: "**\"Directory Service Groups\"** in the Jamf Pro admin UI. Optional allow-list restricting enrollment to members of specific directory-service groups. Jamf Pro de-duplicates entries by `(group_name, directory_service_server_id)` and does not validate that the supplied server ID exists.",
							Optional:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"group_name": schema.StringAttribute{
										MarkdownDescription: "Directory service group name (exact match).",
										Required:            true,
										Validators: []validator.String{
											stringvalidator.LengthAtLeast(1),
										},
									},
									"directory_service_server_id": schema.Int64Attribute{
										MarkdownDescription: "**\"Server Name\"** in the Jamf Pro admin UI. ID of the directory service server hosting the group; the admin UI shows the server name but the value stored is the server ID.",
										Required:            true,
										Validators: []validator.Int64{
											int64validator.AtLeast(0),
										},
									},
								},
							},
						},
					},
				},
			},
			"sso_panes": schema.ListNestedAttribute{
				MarkdownDescription: "Optional SSO authentication pane. At most one authentication pane (LDAP or SSO) may be configured per customization; supplying both `sso_panes` and `ldap_panes` is rejected at plan time.",
				Optional:            true,
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
					listvalidator.ConflictsWith(path.MatchRoot("ldap_panes")),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Panel ID assigned by Jamf Pro on first save. Returned by Jamf Pro; not user-settable.",
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseNonNullStateForUnknown(),
							},
						},
						"display_name": schema.StringAttribute{
							MarkdownDescription: "Display name for the SSO pane shown in the Jamf Pro admin UI.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
						"rank": schema.Int64Attribute{
							MarkdownDescription: "Ordering rank for the pane (lower ranks shown first).",
							Required:            true,
							Validators: []validator.Int64{
								int64validator.AtLeast(0),
							},
						},
						"enrollment_access": schema.StringAttribute{
							MarkdownDescription: "**\"Enrollment Access\"** in the Jamf Pro admin UI. One of `any_idp_user` (any IdP user may enrol) or `specific_group` (restrict enrolment to members of the IdP group named in `access_group_name`).",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf(enrollmentAccessAnyIdpUser, enrollmentAccessSpecificGroup),
							},
						},
						"access_group_name": schema.StringAttribute{
							MarkdownDescription: "IdP group name allowed to enrol. Required when `enrollment_access = \"specific_group\"`; ignored otherwise.",
							Optional:            true,
							Validators: []validator.String{
								AccessGroupNameValidator(),
							},
						},
						"pass_user_info_to_jamf_connect": schema.BoolAttribute{
							MarkdownDescription: "**\"Enable Jamf Pro to pass user information to Jamf Connect\"** in the Jamf Pro admin UI. Whether enrolment user info is forwarded to Jamf Connect.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseNonNullStateForUnknown(),
							},
						},
						"account_name_attribute": schema.StringAttribute{
							MarkdownDescription: "**\"Account Name\"** in the Jamf Pro admin UI. SSO claim used as the account short name on the enrolled device.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseNonNullStateForUnknown(),
							},
						},
						"account_full_name_attribute": schema.StringAttribute{
							MarkdownDescription: "**\"Account Full Name\"** in the Jamf Pro admin UI. SSO claim used as the account full name on the enrolled device.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseNonNullStateForUnknown(),
							},
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
func (r *EnrollmentCustomizationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_enrollment_customization")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro enrollment customization ID.
func (r *EnrollmentCustomizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
