// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// dsStr is a Computed-only data-source string attribute shorthand.
func dsStr(desc string) dsschema.StringAttribute {
	return dsschema.StringAttribute{Computed: true, MarkdownDescription: desc}
}

// InventoryPreloadRecordDataSource implements the Terraform data source for Jamf Pro
// Inventory Preload records. Lookup by ID OR by serial number — exactly one of the two.
type InventoryPreloadRecordDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &InventoryPreloadRecordDataSource{}
	_ datasource.DataSourceWithConfigure        = &InventoryPreloadRecordDataSource{}
	_ datasource.DataSourceWithConfigValidators = &InventoryPreloadRecordDataSource{}
)

// NewInventoryPreloadRecordDataSource returns a new instance of InventoryPreloadRecordDataSource.
func NewInventoryPreloadRecordDataSource() datasource.DataSource {
	return &InventoryPreloadRecordDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *InventoryPreloadRecordDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_inventory_preload_record"
}

// Schema returns the data source schema.
func (d *InventoryPreloadRecordDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: "Look up a Jamf Pro Inventory Preload record by ID or by serial number. Exactly one of `id` or `serial_number` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]dsschema.Attribute{
			"id": dsschema.StringAttribute{
				MarkdownDescription: "Inventory Preload record ID. Mutually exclusive with `serial_number`.",
				Optional:            true,
				Computed:            true,
			},
			"serial_number": dsschema.StringAttribute{
				MarkdownDescription: "Serial number of the device the record applies to (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"device_type":         dsStr("Type of device the record applies to (`Computer` or `Mobile Device`)."),
			"username":            dsStr("Username assigned to the device."),
			"full_name":           dsStr("Full name of the user assigned to the device."),
			"email_address":       dsStr("Email address of the user assigned to the device."),
			"phone_number":        dsStr("Phone number of the user assigned to the device."),
			"position":            dsStr("Position (job title) of the user assigned to the device."),
			"department":          dsStr("Department the device is assigned to."),
			"building":            dsStr("Building the device is assigned to."),
			"room":                dsStr("Room the device is located in."),
			"po_number":           dsStr("Purchase order number."),
			"po_date":             dsStr("Purchase order date."),
			"warranty_expiration": dsStr("Warranty expiration date."),
			"lease_expiration":    dsStr("Lease expiration date."),
			"apple_care_id":       dsStr("AppleCare ID for the device."),
			"life_expectancy":     dsStr("Life expectancy of the device, in years."),
			"purchase_price":      dsStr("Purchase price of the device."),
			"purchasing_contact":  dsStr("Purchasing contact for the device."),
			"purchasing_account":  dsStr("Purchasing account for the device."),
			"bar_code_1":          dsStr("Bar code 1 for the device."),
			"bar_code_2":          dsStr("Bar code 2 for the device."),
			"asset_tag":           dsStr("Asset tag for the device."),
			"vendor":              dsStr("Vendor the device was purchased from."),
			"extension_attributes": dsschema.ListNestedAttribute{
				MarkdownDescription: "Extension attribute values stored on the record.",
				Computed:            true,
				NestedObject: dsschema.NestedAttributeObject{
					Attributes: map[string]dsschema.Attribute{
						"name":  dsStr("Display name of the extension attribute the value applies to."),
						"value": dsStr("Value applied to the extension attribute."),
					},
				},
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / serial_number is supplied.
func (d *InventoryPreloadRecordDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("serial_number"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *InventoryPreloadRecordDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_inventory_preload_record")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an inventory preload record by ID or serial number and populates state.
func (d *InventoryPreloadRecordDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data InventoryPreloadRecordDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, timeoutDiags := helpers.ResolveTimeout(ctx, data.Timeouts.IsNull(), data.Timeouts.IsUnknown(), defaultReadTimeout, data.Timeouts.Read)
	resp.Diagnostics.Append(timeoutDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	var (
		got *pro.InventoryPreloadRecordV2
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetInventoryPreloadRecordV2(readCtx, data.ID.ValueString())
	case !data.SerialNumber.IsNull() && data.SerialNumber.ValueString() != "":
		got, err = d.client.ResolveInventoryPreloadRecordV2BySerialNumber(readCtx, data.SerialNumber.ValueString())
	default:
		resp.Diagnostics.AddError("Missing inventory preload record selector", "Exactly one of id or serial_number must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro inventory preload record", helpers.APIErrorDetail(err))
		return
	}

	resp.Diagnostics.Append(assignInventoryPreloadRecordDataSourceModel(ctx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "read Jamf Pro inventory preload record data source", map[string]any{"id": data.ID.ValueString(), "serial_number": data.SerialNumber.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
