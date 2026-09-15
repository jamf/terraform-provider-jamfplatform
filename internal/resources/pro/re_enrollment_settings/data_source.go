// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package re_enrollment_settings

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// ReEnrollmentSettingsDataSource is the read-only mirror of the Re-enrollment
// settings resource.
type ReEnrollmentSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &ReEnrollmentSettingsDataSource{}

// NewReEnrollmentSettingsDataSource constructs a new ReEnrollmentSettingsDataSource.
func NewReEnrollmentSettingsDataSource() datasource.DataSource {
	return &ReEnrollmentSettingsDataSource{}
}

// Metadata sets the data source type name.
func (d *ReEnrollmentSettingsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_re_enrollment_settings"
}

// Schema returns the data source schema. Every attribute is Computed.
func (d *ReEnrollmentSettingsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro Re-enrollment settings (Settings → Global → Re-enrollment). One record per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id":                                 schema.StringAttribute{Computed: true, MarkdownDescription: "Fixed singleton identifier. Always `singleton`."},
			"clear_policy_logs":                  schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether policy logs on computers are cleared when a device re-enrolls."},
			"clear_location_information":         schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether user and location information on mobile devices and computers is cleared when a device re-enrolls."},
			"clear_location_information_history": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether user and location information history on mobile devices and computers is cleared when a device re-enrolls."},
			"clear_extension_attributes":         schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether extension attribute values on computers and mobile devices are cleared when a device re-enrolls."},
			"clear_software_update_plans":        schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether software update plans on mobile devices and computers are cleared when a device re-enrolls."},
			"clear_management_history":           schema.StringAttribute{Computed: true, MarkdownDescription: "How much of a device's management command history is cleared when it re-enrolls."},
			"timeouts":                           timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client.
func (d *ReEnrollmentSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_re_enrollment_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read populates state from the Re-enrollment settings endpoint.
func (d *ReEnrollmentSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ReEnrollmentSettingsDataSourceModel
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

	got, err := d.client.GetReenrollmentSettingsV1(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro Re-enrollment settings", helpers.APIErrorDetail(err))
		return
	}

	assignReEnrollmentSettingsDataSourceModel(&data, got)
	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro Re-enrollment settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
