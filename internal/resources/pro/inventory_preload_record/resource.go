// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// Package inventory_preload_record implements the jamfplatform_pro_inventory_preload_record
// resource, data source, and list resource backed by the Jamf Pro Inventory Preload
// records API.
package inventory_preload_record

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/validators"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// minJamfProVersion is the minimum Jamf Pro tenant version required by this resource.
// Empty string skips the per-resource version check — the v2 Inventory Preload
// endpoint has been stable since well before the provider's overall floor (the v1
// endpoint was deprecated in 2020), so no per-resource gate is needed.
const minJamfProVersion = ""

// validDeviceTypes is the accepted device_type set. Deliberately narrower than
// pro.InventoryPreloadRecordV2DeviceTypeValues(), which also generates Unknown:
// Unknown is what the field reads back as for a record whose type the server
// cannot classify, not a type a caller can ask for. The set is curated; the
// spellings are the SDK's.
var validDeviceTypes = []string{
	pro.InventoryPreloadRecordV2DeviceTypeComputer,
	pro.InventoryPreloadRecordV2DeviceTypeMobileDevice,
}

// InventoryPreloadRecordResource implements the Terraform resource for Jamf Pro
// Inventory Preload records.
type InventoryPreloadRecordResource struct {
	client *pro.Client
}

var _ resource.Resource = &InventoryPreloadRecordResource{}
var _ resource.ResourceWithImportState = &InventoryPreloadRecordResource{}
var _ resource.ResourceWithIdentity = &InventoryPreloadRecordResource{}

const (
	defaultCreateTimeout = 60 * time.Second
	defaultReadTimeout   = 60 * time.Second
	defaultUpdateTimeout = 60 * time.Second
	defaultDeleteTimeout = 60 * time.Second
)

// NewInventoryPreloadRecordResource returns a new instance of InventoryPreloadRecordResource.
func NewInventoryPreloadRecordResource() resource.Resource {
	return &InventoryPreloadRecordResource{}
}

// Metadata sets the resource type name for the Terraform provider.
func (r *InventoryPreloadRecordResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_inventory_preload_record"
}

// IdentitySchema defines the identifier used for import and list results.
func (r *InventoryPreloadRecordResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       "Jamf Pro Inventory Preload record ID used to uniquely reference the record.",
				RequiredForImport: true,
			},
		},
	}
}

// fullReplaceStringAttribute returns the Optional+Computed+UseStateForUnknown string
// attribute every user-settable optional scalar on this resource uses. The records PUT
// is full-replace (wire-probed 2026-06-10): omitting a field clears it server-side. The
// UseStateForUnknown modifier carries the prior state value into the plan when the field
// is omitted, so the input builder re-emits it and omit=preserve holds — see
// STYLE_GUIDE.md §Full-replace endpoints. The standard description sentence is appended
// so every field documents the same omit/clear contract.
func fullReplaceStringAttribute(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: desc + " Omit to leave any existing value untouched (it is not cleared on update); set to `\"\"` to clear it.",
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

// Schema returns the Terraform schema for the inventory preload record resource.
func (r *InventoryPreloadRecordResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a single Jamf Pro Inventory Preload record. " +
			"The Jamf Pro admin UI (**Settings > Global > Inventory Preload**) manages these records by CSV upload; " +
			"this resource manages them one at a time instead. " +
			"Preloaded data is applied at every inventory collection by matching the device serial number, " +
			"and overwrites manual inventory edits each time it is applied. " +
			"Records persist after a device enrolls; applying one does not remove it." +
			resourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Inventory Preload record ID assigned by Jamf Pro.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"serial_number": schema.StringAttribute{
				MarkdownDescription: "Serial number of the device the record applies to. " +
					"Jamf Pro enforces case-insensitive uniqueness across records, so creating a second record whose serial number differs only in case fails. " +
					"The value itself is stored and returned exactly as entered. Can be changed in place.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"device_type": schema.StringAttribute{
				MarkdownDescription: "Type of device the record applies to. Valid values: `Computer`, `Mobile Device`. Can be changed in place.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(validDeviceTypes...),
				},
			},
			"username":      fullReplaceStringAttribute("Username assigned to the device."),
			"full_name":     fullReplaceStringAttribute("Full name of the user assigned to the device."),
			"email_address": fullReplaceStringAttribute("Email address of the user assigned to the device."),
			"phone_number":  fullReplaceStringAttribute("Phone number of the user assigned to the device."),
			"position":      fullReplaceStringAttribute("Position (job title) of the user assigned to the device."),
			"department": fullReplaceStringAttribute("Department the device is assigned to. Free text — " +
				"Jamf Pro matches it against existing Department names at inventory-collection time and does not validate it when the record is written."),
			"building": fullReplaceStringAttribute("Building the device is assigned to. Free text — " +
				"Jamf Pro matches it against existing Building names at inventory-collection time and does not validate it when the record is written."),
			"room":      fullReplaceStringAttribute("Room the device is located in."),
			"po_number": fullReplaceStringAttribute("Purchase order number."),
			"po_date": fullReplaceStringAttribute("Purchase order date. Free text stored exactly as entered — " +
				"no date format is enforced."),
			"warranty_expiration": fullReplaceStringAttribute("Warranty expiration date. Free text stored exactly as entered — " +
				"no date format is enforced."),
			"lease_expiration": fullReplaceStringAttribute("Lease expiration date. Free text stored exactly as entered — " +
				"no date format is enforced."),
			"apple_care_id":      fullReplaceStringAttribute("AppleCare ID for the device."),
			"life_expectancy":    fullReplaceStringAttribute("Life expectancy of the device, in years."),
			"purchase_price":     fullReplaceStringAttribute("Purchase price of the device."),
			"purchasing_contact": fullReplaceStringAttribute("Purchasing contact for the device."),
			"purchasing_account": fullReplaceStringAttribute("Purchasing account for the device."),
			"bar_code_1": fullReplaceStringAttribute("Bar code 1 for the device. Documented as applying to computers only; " +
				"the API does not enforce this."),
			"bar_code_2": fullReplaceStringAttribute("Bar code 2 for the device. Documented as applying to computers only; " +
				"the API does not enforce this."),
			"asset_tag": fullReplaceStringAttribute("Asset tag for the device."),
			"vendor":    fullReplaceStringAttribute("Vendor the device was purchased from."),
			"extension_attributes": schema.SetNestedAttribute{
				MarkdownDescription: "Extension attribute values applied to the device's inventory record. " +
					"Each entry's `name` is matched against existing extension attribute display names at inventory-collection time; " +
					"Jamf Pro does not validate the names when the record is written, so entries with non-matching names are stored on the record but have no effect. " +
					"Omit to leave any existing entries untouched (they are not cleared on update); set to `[]` to clear them.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
				// Jamf Pro accepts duplicate names on the wire (both entries are
				// stored against the same extension attribute), but the UI/CSV
				// reality is one value per attribute, and the Set type alone only
				// dedupes identical name+value pairs.
				Validators: []validator.Set{
					validators.UniqueStringFieldSet("name"),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Display name of the extension attribute the value applies to.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
						"value": schema.StringAttribute{
							MarkdownDescription: "Value applied to the extension attribute. An omitted value and an explicit empty string are stored distinctly.",
							Optional:            true,
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
func (r *InventoryPreloadRecordResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_inventory_preload_record")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.client = client
}

// ImportState handles import by the Jamf Pro Inventory Preload record ID.
func (r *InventoryPreloadRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
