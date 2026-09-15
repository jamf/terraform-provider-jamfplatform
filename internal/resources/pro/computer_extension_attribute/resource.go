// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package computer_extension_attribute implements the
// jamfplatform_pro_computer_extension_attribute resource, data source, and list
// resource backed by the Jamf Pro /v1/computer-extension-attributes API.
package computer_extension_attribute

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
// resource. Empty: consistent with every other Pro resource (the provider's
// overall floor governs).
const minJamfProVersion = ""

// ComputerExtensionAttributeResource implements the Terraform resource for Jamf
// Pro computer extension attributes.
type ComputerExtensionAttributeResource struct {
	client *pro.Client
}

var _ resource.Resource = &ComputerExtensionAttributeResource{}
var _ resource.ResourceWithImportState = &ComputerExtensionAttributeResource{}
var _ resource.ResourceWithIdentity = &ComputerExtensionAttributeResource{}
var _ resource.ResourceWithConfigValidators = &ComputerExtensionAttributeResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewComputerExtensionAttributeResource returns a new instance of the resource.
func NewComputerExtensionAttributeResource() resource.Resource {
	return &ComputerExtensionAttributeResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *ComputerExtensionAttributeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_computer_extension_attribute"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *ComputerExtensionAttributeResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro computer extension attribute ID used to uniquely reference the EA.",
				RequiredForImport: true,
			},
		},
	}
}

// ConfigValidators enforces the input-type discriminator cross-field rules at
// plan time (mirrors the server's FIELD_REQUIRED / INVALID_CONTENT 400s).
func (r *ComputerExtensionAttributeResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{inputTypeConfigValidator{}}
}

// Schema returns the Terraform schema for the computer extension attribute.
func (r *ComputerExtensionAttributeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro computer extension attribute: a custom inventory field collected from managed computers. Mirrors the Settings → Computer management → Extension Attributes UI. `input_type` acts as a discriminator. `script` is required with `SCRIPT` and valid only there; `popup_menu_choices` only with `POPUP`; `directory_service_attribute` and `allow_multiple_values` only with `DIRECTORY_SERVICE_ATTRIBUTE_MAPPING`. Only `SCRIPT` extension attributes may be disabled. A plan-time validator enforces these rules before apply." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Computer extension attribute ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "**\"Display Name\"** in the Jamf Pro admin UI. Must be unique within the tenant.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "**\"Description\"** in the Jamf Pro admin UI. Free-text description of the extension attribute. Omit it to leave any existing value untouched; it is not cleared on update. Set it to `\"\"` to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"data_type": schema.StringAttribute{
				MarkdownDescription: "**\"Data type\"** in the Jamf Pro admin UI. Type of data collected: `STRING`, `INTEGER`, or `DATE`. Date values use the `YYYY-MM-DD hh:mm:ss` format.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validDataTypes...),
				},
			},
			"input_type": schema.StringAttribute{
				MarkdownDescription: "**\"Input type\"** in the Jamf Pro admin UI. How the attribute value is populated: `TEXT` (text field), `POPUP` (pop-up menu of `popup_menu_choices`), `SCRIPT` (collected by a script), or `DIRECTORY_SERVICE_ATTRIBUTE_MAPPING` (mapped from a directory service attribute). The modern admin UI offers only Text field, Pop-up menu and Script for new computer extension attributes. Jamf Pro still accepts `DIRECTORY_SERVICE_ATTRIBUTE_MAPPING`, which is kept here so existing LDAP-mapped extension attributes can be imported and managed.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validInputTypes...),
				},
			},
			"inventory_display": schema.StringAttribute{
				MarkdownDescription: "**\"Inventory display\"** in the Jamf Pro admin UI. Inventory category the attribute is shown under: `GENERAL`, `HARDWARE`, `OPERATING_SYSTEM`, `USER_AND_LOCATION`, `PURCHASING`, or `EXTENSION_ATTRIBUTES`.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validInventoryDisplays...),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "**\"Enable\"** in the Jamf Pro admin UI. Only `SCRIPT` extension attributes may be disabled; for every other input type Jamf Pro forces this to `true`. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"script": schema.StringAttribute{
				MarkdownDescription: "**\"Script\"** in the Jamf Pro admin UI. Script contents collected as the attribute value. Required when `input_type = SCRIPT`, and must be omitted for every other input type. Jamf Pro normalises the stored script, notably by appending a trailing newline; the provider tolerates that, so it does not show as a perpetual diff.",
				Optional:            true,
			},
			"popup_menu_choices": schema.SetAttribute{
				MarkdownDescription: "**\"Pop-up menu choices\"** in the Jamf Pro admin UI. The choices presented for a pop-up menu attribute. Valid only when `input_type = POPUP`, and optional even then. Modelled as a set, because Jamf Pro returns the choices sorted alphabetically rather than in the submitted order. Omit the attribute to leave any existing choices untouched; they are not cleared on an unrelated update. Set it to `[]` to clear them. Changing `input_type` away from `POPUP` also clears them.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.Set{
					popupChoicesPlanModifier{},
				},
			},
			"directory_service_attribute": schema.StringAttribute{
				MarkdownDescription: "**\"Directory Service Attribute\"** in the Jamf Pro admin UI. The directory-service attribute name mapped to this extension attribute. Required when `input_type = DIRECTORY_SERVICE_ATTRIBUTE_MAPPING`, and must be omitted for every other input type.",
				Optional:            true,
			},
			"allow_multiple_values": schema.BoolAttribute{
				MarkdownDescription: "**\"Allow Multiple Values\"** in the Jamf Pro admin UI. Collect multiple values for a directory-service-mapped attribute. Doing so limits the operators available when the attribute is used in smart group or advanced search criteria. Meaningful only when `input_type = DIRECTORY_SERVICE_ATTRIBUTE_MAPPING`. Defaults to `false`. Jamf Pro does not allow the flag to change after creation, so changing it forces the extension attribute to be replaced.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"manage_existing_data": schema.StringAttribute{
				MarkdownDescription: "Controls what Jamf Pro does with the inventory data already collected by a `SCRIPT` extension attribute when that attribute is disabled: `RETAIN` keeps the existing values, `DELETE` clears them. There is no matching admin-UI field. Valid only with `input_type = SCRIPT` and `enabled = false`; Jamf Pro rejects it on create and on any other update. Jamf Pro requires the instruction when an enabled `SCRIPT` extension attribute is being disabled, so `RETAIN` is sent when you omit it. This is a Terraform `WriteOnly` attribute: it is sent to Jamf Pro on update but never stored in state, because Jamf Pro never returns it and it is an instruction rather than a persistent property. Changing only this value does not trigger an update; it takes effect alongside other changes to the extension attribute.",
				Optional:            true,
				WriteOnly:           true,
				Validators: []validator.String{
					stringvalidator.OneOf(validManageExistingData...),
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
func (r *ComputerExtensionAttributeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_computer_extension_attribute")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState imports a computer extension attribute by ID.
func (r *ComputerExtensionAttributeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
