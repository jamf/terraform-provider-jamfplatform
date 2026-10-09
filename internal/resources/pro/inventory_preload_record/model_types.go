// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// InventoryPreloadRecordResourceModel represents the Terraform resource model for a
// Jamf Pro Inventory Preload record. ExtensionAttributes is types.Set (not a Go typed
// slice) because the attribute is Optional+Computed — a Computed nested collection is
// Unknown at plan time and only types.Set can carry Unknown.
type InventoryPreloadRecordResourceModel struct {
	ID                  types.String           `tfsdk:"id"`
	SerialNumber        types.String           `tfsdk:"serial_number"`
	DeviceType          types.String           `tfsdk:"device_type"`
	Username            types.String           `tfsdk:"username"`
	FullName            types.String           `tfsdk:"full_name"`
	EmailAddress        types.String           `tfsdk:"email_address"`
	PhoneNumber         types.String           `tfsdk:"phone_number"`
	Position            types.String           `tfsdk:"position"`
	Department          types.String           `tfsdk:"department"`
	Building            types.String           `tfsdk:"building"`
	Room                types.String           `tfsdk:"room"`
	PoNumber            types.String           `tfsdk:"po_number"`
	PoDate              types.String           `tfsdk:"po_date"`
	WarrantyExpiration  types.String           `tfsdk:"warranty_expiration"`
	LeaseExpiration     types.String           `tfsdk:"lease_expiration"`
	AppleCareID         types.String           `tfsdk:"apple_care_id"`
	LifeExpectancy      types.String           `tfsdk:"life_expectancy"`
	PurchasePrice       types.String           `tfsdk:"purchase_price"`
	PurchasingContact   types.String           `tfsdk:"purchasing_contact"`
	PurchasingAccount   types.String           `tfsdk:"purchasing_account"`
	BarCode1            types.String           `tfsdk:"bar_code_1"`
	BarCode2            types.String           `tfsdk:"bar_code_2"`
	AssetTag            types.String           `tfsdk:"asset_tag"`
	Vendor              types.String           `tfsdk:"vendor"`
	ExtensionAttributes types.Set              `tfsdk:"extension_attributes"`
	Timeouts            resourceTimeouts.Value `tfsdk:"timeouts"`
}

// InventoryPreloadRecordDataSourceModel represents the Terraform data source model for
// a Jamf Pro Inventory Preload record. ExtensionAttributes is types.List per the
// data-source convention that computed API collections are lists.
type InventoryPreloadRecordDataSourceModel struct {
	ID                  types.String             `tfsdk:"id"`
	SerialNumber        types.String             `tfsdk:"serial_number"`
	DeviceType          types.String             `tfsdk:"device_type"`
	Username            types.String             `tfsdk:"username"`
	FullName            types.String             `tfsdk:"full_name"`
	EmailAddress        types.String             `tfsdk:"email_address"`
	PhoneNumber         types.String             `tfsdk:"phone_number"`
	Position            types.String             `tfsdk:"position"`
	Department          types.String             `tfsdk:"department"`
	Building            types.String             `tfsdk:"building"`
	Room                types.String             `tfsdk:"room"`
	PoNumber            types.String             `tfsdk:"po_number"`
	PoDate              types.String             `tfsdk:"po_date"`
	WarrantyExpiration  types.String             `tfsdk:"warranty_expiration"`
	LeaseExpiration     types.String             `tfsdk:"lease_expiration"`
	AppleCareID         types.String             `tfsdk:"apple_care_id"`
	LifeExpectancy      types.String             `tfsdk:"life_expectancy"`
	PurchasePrice       types.String             `tfsdk:"purchase_price"`
	PurchasingContact   types.String             `tfsdk:"purchasing_contact"`
	PurchasingAccount   types.String             `tfsdk:"purchasing_account"`
	BarCode1            types.String             `tfsdk:"bar_code_1"`
	BarCode2            types.String             `tfsdk:"bar_code_2"`
	AssetTag            types.String             `tfsdk:"asset_tag"`
	Vendor              types.String             `tfsdk:"vendor"`
	ExtensionAttributes types.List               `tfsdk:"extension_attributes"`
	Timeouts            datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// extensionAttributeModel represents one extension_attributes element.
type extensionAttributeModel struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

// inventoryPreloadRecordIdentityModel represents the identity object for inventory
// preload record resources and list results.
type inventoryPreloadRecordIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// InventoryPreloadRecordListResourceModel represents the config model for inventory
// preload record list queries.
type InventoryPreloadRecordListResourceModel struct {
	Filters  []filters.FilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value    `tfsdk:"timeouts"`
}
