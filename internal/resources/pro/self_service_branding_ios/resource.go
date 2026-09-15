// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package self_service_branding_ios implements the
// jamfplatform_pro_self_service_branding_ios singleton resource and data source
// backed by the Jamf Pro Self Service iOS branding API
// (/api/pro/v1/self-service/branding/ios).
package self_service_branding_ios

import (
	"context"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: the Self Service branding endpoints predate the provider's
// overall floor (11.0.0).
const minJamfProVersion = ""

// hexColorRegex matches a 6-digit hex colour with no leading '#', as the wire
// stores them (e.g. "FFFFFF", "007AFF"). Wire-probed: the API returns codes
// without a '#'.
var hexColorRegex = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)

// SelfServiceBrandingIosResource implements the presence-optional singleton
// resource for the Jamf Pro Self Service iOS & iPadOS branding configuration.
// See the macOS sibling for the singleton/CRUD shape — identical, except the
// iOS object requires main_header and the colour fields.
type SelfServiceBrandingIosResource struct {
	client *pro.Client
}

var (
	_ resource.Resource                = &SelfServiceBrandingIosResource{}
	_ resource.ResourceWithImportState = &SelfServiceBrandingIosResource{}
	_ resource.ResourceWithIdentity    = &SelfServiceBrandingIosResource{}
)

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// selfServiceBrandingIosIdentityModel is the identity struct for import.
type selfServiceBrandingIosIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// NewSelfServiceBrandingIosResource returns a new instance of the resource.
func NewSelfServiceBrandingIosResource() resource.Resource {
	return &SelfServiceBrandingIosResource{}
}

// Metadata sets the resource type name.
func (r *SelfServiceBrandingIosResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_self_service_branding_ios"
}

// IdentitySchema defines the import identity. Singleton — only the fixed
// helpers.SingletonID value is accepted.
func (r *SelfServiceBrandingIosResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Fixed singleton identifier. Always \"singleton\". Self Service iOS branding is one record per tenant.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the iOS branding resource.
func (r *SelfServiceBrandingIosResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the Jamf Pro Self Service iOS & iPadOS branding configuration (Settings > Self Service > Branding > iOS & iPadOS Branding). " +
			"One configuration per tenant. Creating this resource adds the iOS branding; destroying it removes it. " +
			"Jamf Pro requires `main_header` and the three colour codes. " +
			"Import with `terraform import jamfplatform_pro_self_service_branding_ios.<name> singleton`." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"main_header": schema.StringAttribute{
				MarkdownDescription: "**\"Main Header\"** in the Jamf Pro admin UI. Title shown at the top of the Self Service iOS app. Required. The Jamf Pro default is `Self Service`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"branding_name_color_code": schema.StringAttribute{
				MarkdownDescription: "Hex colour (6 digits, no `#`, e.g. `000000`) of the Main Header text. Required.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.RegexMatches(hexColorRegex, "must be a 6-digit hex colour with no leading '#', e.g. 000000")},
			},
			"header_background_color_code": schema.StringAttribute{
				MarkdownDescription: "Hex colour (6 digits, no `#`, e.g. `FFFFFF`) of the header background. Required.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.RegexMatches(hexColorRegex, "must be a 6-digit hex colour with no leading '#', e.g. FFFFFF")},
			},
			"menu_icon_color_code": schema.StringAttribute{
				MarkdownDescription: "Hex colour (6 digits, no `#`, e.g. `007AFF`) of the menu icons. Required.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.RegexMatches(hexColorRegex, "must be a 6-digit hex colour with no leading '#', e.g. 007AFF")},
			},
			"status_bar_text_color": schema.StringAttribute{
				MarkdownDescription: "Status bar text appearance. One of `light` or `dark`. Required.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf("light", "dark")},
			},
			"icon_id": schema.Int64Attribute{
				MarkdownDescription: "**\"Icon\"** in the Jamf Pro admin UI. ID of the branding image shown as the Self Service app icon. Use a `jamfplatform_pro_self_service_branding_image` ID, **not** a `jamfplatform_pro_icon` ID; the two stores are separate. Omit to leave unset.",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
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
func (r *SelfServiceBrandingIosResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_self_service_branding_ios")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import for the singleton. Only the fixed
// helpers.SingletonID value is accepted.
func (r *SelfServiceBrandingIosResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	helpers.ImportSingletonState(ctx, req, resp, "jamfplatform_pro_self_service_branding_ios")
}
