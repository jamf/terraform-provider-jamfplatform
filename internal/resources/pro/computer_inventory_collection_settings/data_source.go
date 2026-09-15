// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_inventory_collection_settings

import (
	"context"
	"errors"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// diagsToError flattens framework diagnostics into a single error for use inside helpers
// that return error rather than diag.Diagnostics.
func diagsToError(diags diag.Diagnostics) error {
	if !diags.HasError() {
		return nil
	}
	msgs := make([]string, 0, len(diags))
	for _, d := range diags.Errors() {
		msgs = append(msgs, d.Summary()+": "+d.Detail())
	}
	return errors.New(strings.Join(msgs, "; "))
}

// ComputerInventoryCollectionSettingsDataSource implements the Terraform data source.
type ComputerInventoryCollectionSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &ComputerInventoryCollectionSettingsDataSource{}

// NewComputerInventoryCollectionSettingsDataSource returns a new instance of the data source.
func NewComputerInventoryCollectionSettingsDataSource() datasource.DataSource {
	return &ComputerInventoryCollectionSettingsDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *ComputerInventoryCollectionSettingsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_computer_inventory_collection_settings"
}

// computedBool returns a Computed-only bool data source attribute.
func computedBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{MarkdownDescription: desc, Computed: true}
}

// Schema returns the data source schema.
func (d *ComputerInventoryCollectionSettingsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro computer inventory collection settings. One record per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Fixed singleton identifier. Always `singleton`.",
				Computed:            true,
			},

			"collect_local_user_accounts":                      computedBool("Collect local user accounts."),
			"include_home_directory_sizes":                     computedBool("Include home directory sizes when collecting local user accounts."),
			"include_hidden_accounts":                          computedBool("Include hidden accounts when collecting local user accounts."),
			"collect_printers":                                 computedBool("Collect printers."),
			"collect_active_services":                          computedBool("Collect active services."),
			"collect_synced_mobile_device_backup_dates":        computedBool("Collect last backup date/time for managed mobile devices that are synced to computers."),
			"collect_user_and_location_from_directory_service": computedBool("Collect user and location information from Directory Service."),
			"collect_package_receipts":                         computedBool("Collect package receipts."),
			"collect_available_software_updates":               computedBool("Collect available software updates."),
			"collect_unmanaged_certificates":                   computedBool("Collect unmanaged certificates."),
			"monitor_beacon_regions":                           computedBool("Monitor Beacon regions."),
			"allow_jamf_binary_user_and_location_changes":      computedBool("Allow local administrators to use the jamf binary recon verb to change User and Location inventory information in Jamf Pro."),
			"collect_application_usage_information":            computedBool("Collect Application Usage Information."),
			"use_unix_user_paths":                              computedBool("Enable inventory collection of applications in user (UNIX) home-directory paths."),
			"include_software_id":                              computedBool("Whether the inventory submission includes a software identifier."),

			"application_search_paths": schema.SetAttribute{
				MarkdownDescription: "Custom application search paths (built-in paths excluded).",
				ElementType:         types.StringType,
				Computed:            true,
			},

			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *ComputerInventoryCollectionSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_computer_inventory_collection_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches the current settings and populates Terraform state.
func (d *ComputerInventoryCollectionSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ComputerInventoryCollectionSettingsDataSourceModel
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

	got, err := d.client.GetComputerInventoryCollectionSettingsV2(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro computer inventory collection settings", helpers.APIErrorDetail(err))
		return
	}
	resp.Diagnostics.Append(assignComputerInventoryCollectionSettingsDataSourceModel(ctx, &data, got)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro computer inventory collection settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
