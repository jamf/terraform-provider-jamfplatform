// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package advanced_mobile_device_search implements the
// jamfplatform_pro_advanced_mobile_device_search resource, data source, and list
// resource backed by the Jamf Pro /v1/advanced-mobile-device-searches API.
package advanced_mobile_device_search

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this
// resource. Empty: consistent with every other Pro resource (the provider's
// overall floor governs); the /v1/advanced-mobile-device-searches endpoint
// predates any meaningful per-resource floor.
const minJamfProVersion = ""

// AdvancedMobileDeviceSearchResource implements the Terraform resource for Jamf
// Pro advanced mobile device searches.
type AdvancedMobileDeviceSearchResource struct {
	client *pro.Client
}

var _ resource.Resource = &AdvancedMobileDeviceSearchResource{}
var _ resource.ResourceWithImportState = &AdvancedMobileDeviceSearchResource{}
var _ resource.ResourceWithIdentity = &AdvancedMobileDeviceSearchResource{}
var _ resource.ResourceWithModifyPlan = &AdvancedMobileDeviceSearchResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewAdvancedMobileDeviceSearchResource returns a new instance of the resource.
func NewAdvancedMobileDeviceSearchResource() resource.Resource {
	return &AdvancedMobileDeviceSearchResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *AdvancedMobileDeviceSearchResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_advanced_mobile_device_search"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *AdvancedMobileDeviceSearchResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro advanced mobile device search ID used to uniquely reference the search.",
				RequiredForImport: true,
			},
		},
	}
}

// Schema returns the Terraform schema for the advanced mobile device search
// resource.
func (r *AdvancedMobileDeviceSearchResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Jamf Pro advanced mobile device search, a saved criteria-driven inventory query with a configurable set of display columns. The matched-device result set and the Reports tab (file format, scheduled email) are report concerns Jamf Pro computes, and are intentionally not modelled. Mirrors the Devices → Search Inventory → Advanced Mobile Device Search UI." + resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Advanced mobile device search ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Advanced mobile device search display name. Must be unique within the tenant.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Optional Jamf Pro site ID to scope the search. Omit to leave unscoped (Jamf Pro reports the `NONE` site, id `-1`).",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(noSiteID),
			},
			"criteria": schema.ListNestedAttribute{
				MarkdownDescription: "Ordered list of criteria Jamf Pro evaluates to populate the search. Order matters: Jamf Pro reads left to right, applying the supplied `and_or` joins and parentheses. Omitting the attribute leaves any existing criteria untouched; they are not cleared on an unrelated update. Set it to `[]` to remove all criteria.",
				Optional:            true,
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: criteria.CriterionAttributes(ValidOperators),
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"display_fields": schema.SetAttribute{
				MarkdownDescription: "Set of inventory column names shown in the search results, for example `Display Name`, `Serial Number`, `Last Inventory Update`. Order is not significant, because Jamf Pro returns the columns in its own canonical order. Omitting the attribute leaves any existing columns untouched; they are not cleared on an unrelated update. Set it to `[]` to remove all display columns.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
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
func (r *AdvancedMobileDeviceSearchResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_advanced_mobile_device_search")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState imports an advanced mobile device search by ID.
func (r *AdvancedMobileDeviceSearchResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
