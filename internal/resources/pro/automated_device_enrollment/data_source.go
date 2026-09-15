// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetDeviceEnrollmentV1
//   pro.ResolveDeviceEnrollmentV1ByName
// Status: current. Last reviewed 2026-05-25.

package automated_device_enrollment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// AutomatedDeviceEnrollmentDataSource implements the Terraform data source for
// Jamf Pro Automated Device Enrollment (ADE) instances. The singular data
// source supports lookup by ID OR by name — exactly one of the two must be
// supplied. WriteOnly / upload-only resource attributes (`server_token`,
// `server_token_wo_version`, `token_file_name`) are omitted from the data
// source schema because the Jamf Pro GET response never echoes them back.
type AutomatedDeviceEnrollmentDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &AutomatedDeviceEnrollmentDataSource{}
	_ datasource.DataSourceWithConfigure        = &AutomatedDeviceEnrollmentDataSource{}
	_ datasource.DataSourceWithConfigValidators = &AutomatedDeviceEnrollmentDataSource{}
)

// NewAutomatedDeviceEnrollmentDataSource returns a new instance of the data
// source.
func NewAutomatedDeviceEnrollmentDataSource() datasource.DataSource {
	return &AutomatedDeviceEnrollmentDataSource{}
}

// Metadata sets the data source type name for the Terraform provider.
func (d *AutomatedDeviceEnrollmentDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_automated_device_enrollment"
}

// Schema returns the data source schema.
func (d *AutomatedDeviceEnrollmentDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro Automated Device Enrollment (ADE) instance by ID or by exact name. Exactly one of `id` or `name` must be supplied. The data source never returns the uploaded server token, because Jamf Pro never returns it on reads. Use the `jamfplatform_pro_automated_device_enrollment` resource to manage the token." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Automated Device Enrollment instance ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "ADE instance display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro site ID associated with this ADE instance, or the sentinel `\"-1\"` when no site is set.",
				Computed:            true,
			},
			"supervision_identity_id": schema.StringAttribute{
				MarkdownDescription: "Jamf Pro supervision identity ID associated with this ADE instance, or the sentinel `\"-1\"` when no supervision identity is set.",
				Computed:            true,
			},
			"admin_id": schema.StringAttribute{
				MarkdownDescription: "Apple administrator ID parsed from the uploaded server token.",
				Computed:            true,
			},
			"org_name": schema.StringAttribute{
				MarkdownDescription: "Organization name parsed from the uploaded server token.",
				Computed:            true,
			},
			"org_email": schema.StringAttribute{
				MarkdownDescription: "Organization email address parsed from the uploaded server token.",
				Computed:            true,
			},
			"org_phone": schema.StringAttribute{
				MarkdownDescription: "Organization phone number parsed from the uploaded server token. Apple may return values containing trailing whitespace; the provider preserves the exact value Jamf Pro reports.",
				Computed:            true,
			},
			"org_address": schema.StringAttribute{
				MarkdownDescription: "Organization mailing address parsed from the uploaded server token. Apple may return values containing trailing whitespace; the provider preserves the exact value Jamf Pro reports.",
				Computed:            true,
			},
			"server_name": schema.StringAttribute{
				MarkdownDescription: "MDM server hostname recorded by Apple for this ADE instance.",
				Computed:            true,
			},
			"server_uuid": schema.StringAttribute{
				MarkdownDescription: "MDM server UUID recorded by Apple for this ADE instance.",
				Computed:            true,
			},
			"token_expiration_date": schema.StringAttribute{
				MarkdownDescription: "Expiration date of the uploaded ADE server token, in `YYYY-MM-DD` format.",
				Computed:            true,
			},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces that exactly one of id / name is supplied.
func (d *AutomatedDeviceEnrollmentDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(
			path.MatchRoot("id"),
			path.MatchRoot("name"),
		),
	}
}

// Configure wires the Jamf Pro client into the data source via the shared
// providerdata.ConfigurePro helper.
func (d *AutomatedDeviceEnrollmentDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_automated_device_enrollment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches an ADE instance by ID or by name and populates Terraform state.
func (d *AutomatedDeviceEnrollmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data AutomatedDeviceEnrollmentDataSourceModel
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
		got *pro.DeviceEnrollmentInstance
		err error
	)
	switch {
	case !data.ID.IsNull() && data.ID.ValueString() != "":
		got, err = d.client.GetDeviceEnrollmentV1(readCtx, data.ID.ValueString())
	case !data.Name.IsNull() && data.Name.ValueString() != "":
		got, err = d.client.ResolveDeviceEnrollmentV1ByName(readCtx, data.Name.ValueString())
	default:
		resp.Diagnostics.AddError("Missing Automated Device Enrollment selector", "Exactly one of id or name must be supplied.")
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to find Jamf Pro Automated Device Enrollment instance", helpers.APIErrorDetail(err))
		return
	}

	assignAutomatedDeviceEnrollmentDataSourceModel(&data, got)

	tflog.Trace(ctx, "read Jamf Pro Automated Device Enrollment data source", map[string]any{"id": data.ID.ValueString(), "name": data.Name.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
