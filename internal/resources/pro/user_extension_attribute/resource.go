// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package user_extension_attribute implements the
// jamfplatform_pro_user_extension_attribute resource, data source, and list
// resource backed by the Jamf Pro Classic /userextensionattributes API.
package user_extension_attribute

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: consistent with every other Pro/ProClassic resource.
const minJamfProVersion = ""

// UserExtensionAttributeResource implements the Terraform resource for Jamf Pro
// user extension attributes.
type UserExtensionAttributeResource struct {
	client *proclassic.Client
}

var _ resource.Resource = &UserExtensionAttributeResource{}
var _ resource.ResourceWithImportState = &UserExtensionAttributeResource{}
var _ resource.ResourceWithIdentity = &UserExtensionAttributeResource{}
var _ resource.ResourceWithConfigValidators = &UserExtensionAttributeResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewUserExtensionAttributeResource returns a new instance of the resource.
func NewUserExtensionAttributeResource() resource.Resource {
	return &UserExtensionAttributeResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *UserExtensionAttributeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_user_extension_attribute"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *UserExtensionAttributeResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro user extension attribute ID used to uniquely reference the EA.",
				RequiredForImport: true,
			},
		},
	}
}

// ConfigValidators enforces the input-type discriminator cross-field rule at
// plan time.
func (r *UserExtensionAttributeResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{inputTypeConfigValidator{}}
}

// Schema returns the Terraform schema for the user extension attribute.
func (r *UserExtensionAttributeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro user extension attribute, a custom inventory field collected for users. Mirrors the Settings → User management → Extension Attributes screen. User EAs support only `Text Field` and `Pop-up Menu` input types; `popup_menu_choices` is valid only with `Pop-up Menu` (enforced at plan time)." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "User extension attribute ID assigned by Jamf Pro.",
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
				MarkdownDescription: "**\"Description\"** in the Jamf Pro admin UI. Optional free-text description of the extension attribute. Remove the attribute to clear the stored text on the next update.",
				Optional:            true,
			},
			"data_type": schema.StringAttribute{
				MarkdownDescription: "**\"Data Type\"** in the Jamf Pro admin UI. Type of data collected: `String`, `Integer`, or `Date` (date values use the `YYYY-MM-DD hh:mm:ss` format).",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validDataTypes...),
				},
			},
			"input_type": schema.StringAttribute{
				MarkdownDescription: "**\"Input Type\"** in the Jamf Pro admin UI. How the attribute value is populated: `Text Field` or `Pop-up Menu` (of `popup_menu_choices`). User EAs do not support script or directory-service-mapped input types.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validInputTypes...),
				},
			},
			"popup_menu_choices": schema.ListAttribute{
				MarkdownDescription: "**\"Pop-up menu choices\"** in the Jamf Pro admin UI. Ordered list of choices presented for a pop-up menu attribute. Valid only when `input_type = \"Pop-up Menu\"` (optional even then). Omit to leave any existing entries untouched (they are not cleared on update); set to `[]` to clear them.",
				Optional:            true,
				ElementType:         types.StringType,
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

// Configure wires the Jamf Pro Classic client into the resource via the shared
// providerdata.ConfigureProClassic helper.
func (r *UserExtensionAttributeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigureProClassic(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_user_extension_attribute")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState imports a user extension attribute by ID.
func (r *UserExtensionAttributeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
