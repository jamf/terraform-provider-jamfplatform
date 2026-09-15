// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package mobile_device_extension_attribute implements the
// jamfplatform_pro_mobile_device_extension_attribute resource, data source, and
// list resource backed by the Jamf Pro /v1/mobile-device-extension-attributes API.
package mobile_device_extension_attribute

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: consistent with every other Pro resource.
const minJamfProVersion = ""

// MobileDeviceExtensionAttributeResource implements the Terraform resource for
// Jamf Pro mobile device extension attributes.
type MobileDeviceExtensionAttributeResource struct {
	client *pro.Client
}

var _ resource.Resource = &MobileDeviceExtensionAttributeResource{}
var _ resource.ResourceWithImportState = &MobileDeviceExtensionAttributeResource{}
var _ resource.ResourceWithIdentity = &MobileDeviceExtensionAttributeResource{}
var _ resource.ResourceWithConfigValidators = &MobileDeviceExtensionAttributeResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewMobileDeviceExtensionAttributeResource returns a new instance of the resource.
func NewMobileDeviceExtensionAttributeResource() resource.Resource {
	return &MobileDeviceExtensionAttributeResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *MobileDeviceExtensionAttributeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_extension_attribute"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *MobileDeviceExtensionAttributeResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro mobile device extension attribute ID used to uniquely reference the EA.",
				RequiredForImport: true,
			},
		},
	}
}

// ConfigValidators enforces the input-type discriminator cross-field rules at
// plan time.
func (r *MobileDeviceExtensionAttributeResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{inputTypeConfigValidator{}}
}

// Schema returns the Terraform schema for the mobile device extension attribute.
func (r *MobileDeviceExtensionAttributeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro mobile device extension attribute, a custom inventory field collected from managed mobile devices (Settings → Mobile device management → Extension Attributes). Mobile device extension attributes cannot run scripts, so there is no script or enabled field. `input_type` acts as a discriminator: `popup_menu_choices` is valid only with `POPUP`, and `directory_service_attribute` (together with `allow_multiple_values`) only with `DIRECTORY_SERVICE_ATTRIBUTE_MAPPING`. A plan-time validator enforces both rules before apply." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Mobile device extension attribute ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Display name for the extension attribute. Must be unique within the tenant.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "**\"Description\"** in the Jamf Pro admin UI. Free-text description of the extension attribute. Omit to leave any existing value untouched; it is not cleared on update. Set `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"data_type": schema.StringAttribute{
				MarkdownDescription: "**\"Data type\"** in the Jamf Pro admin UI. Type of data collected: `STRING`, `INTEGER`, or `DATE` (date values use the `YYYY-MM-DD hh:mm:ss` format).",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validDataTypes...),
				},
			},
			"input_type": schema.StringAttribute{
				MarkdownDescription: "**\"Input type\"** in the Jamf Pro admin UI. How the attribute value is populated: `TEXT` (text field), `POPUP` (pop-up menu of `popup_menu_choices`), or `DIRECTORY_SERVICE_ATTRIBUTE_MAPPING` (mapped from a directory service attribute). Mobile-device EAs cannot use `SCRIPT`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validInputTypes...),
				},
			},
			"inventory_display": schema.StringAttribute{
				MarkdownDescription: "**\"Inventory display\"** in the Jamf Pro admin UI. Inventory category under which the attribute is shown: `GENERAL`, `HARDWARE`, `USER_AND_LOCATION`, `PURCHASING`, or `EXTENSION_ATTRIBUTES`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validInventoryDisplays...),
				},
			},
			"popup_menu_choices": schema.SetAttribute{
				MarkdownDescription: "**\"Pop-up menu choices\"** in the Jamf Pro admin UI. The set of choices presented for a pop-up menu attribute. Valid only when `input_type = POPUP`, and optional even then. Modelled as a set because Jamf Pro returns the choices sorted alphabetically rather than in the submitted order. Omit to leave any existing choices untouched; they are not cleared on an unrelated update. Set `[]` to clear them. Changing `input_type` away from `POPUP` clears the choices.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.Set{
					popupChoicesPlanModifier{},
				},
			},
			"directory_service_attribute": schema.StringAttribute{
				MarkdownDescription: "**\"Directory Service Attribute\"** in the Jamf Pro admin UI. The directory-service attribute name mapped to this EA. Required when `input_type = DIRECTORY_SERVICE_ATTRIBUTE_MAPPING`; must be omitted for every other input type.",
				Optional:            true,
			},
			"allow_multiple_values": schema.BoolAttribute{
				MarkdownDescription: "**\"Allow Multiple Values\"** in the Jamf Pro admin UI. Collect multiple values for a directory-service-mapped attribute (results in a limited choice of operators when used in smart-group / advanced-search criteria). Meaningful only when `input_type = DIRECTORY_SERVICE_ATTRIBUTE_MAPPING`. Defaults to `false`. Jamf Pro does not allow this flag to change after creation, so changing it forces the extension attribute to be replaced.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
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
func (r *MobileDeviceExtensionAttributeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_extension_attribute")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState imports a mobile device extension attribute by ID.
func (r *MobileDeviceExtensionAttributeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
