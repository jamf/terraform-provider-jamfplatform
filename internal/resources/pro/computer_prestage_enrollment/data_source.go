// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetComputerPrestageV3
//   pro.ResolveComputerPrestageV3IDByName
//   pro.ResolveComputerPrestageV3ByName
//
// Status: current. Last reviewed 2026-05-28.

package computer_prestage_enrollment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/providerdata"
)

// ComputerPrestageEnrollmentDataSource implements the Terraform data source
// for Jamf Pro Computer PreStage Enrollments. Lookup is by `id` OR `name`
// (ExactlyOneOf). Write-only secrets (`recovery_lock_password`,
// `account_settings.admin_password`) are omitted; Jamf Pro never echoes them.
type ComputerPrestageEnrollmentDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &ComputerPrestageEnrollmentDataSource{}
	_ datasource.DataSourceWithConfigure        = &ComputerPrestageEnrollmentDataSource{}
	_ datasource.DataSourceWithConfigValidators = &ComputerPrestageEnrollmentDataSource{}
)

// NewComputerPrestageEnrollmentDataSource returns a new data source instance.
func NewComputerPrestageEnrollmentDataSource() datasource.DataSource {
	return &ComputerPrestageEnrollmentDataSource{}
}

// Metadata sets the data source type name.
func (d *ComputerPrestageEnrollmentDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_computer_prestage_enrollment"
}

// Schema returns the data source schema.
func (d *ComputerPrestageEnrollmentDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro Computer PreStage Enrollment by ID or by exact display name. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Computer PreStage enrollment ID. Mutually exclusive with `name`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name (exact match). Mutually exclusive with `id`.",
				Optional:            true,
				Computed:            true,
			},
			"display_name": schema.StringAttribute{
				MarkdownDescription: "Display name returned by Jamf Pro.",
				Computed:            true,
			},
			"mandatory":                               schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether MDM enrolment is mandatory."},
			"mdm_removable":                           schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the user can remove the MDM profile."},
			"default_prestage":                        schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is the tenant default PreStage."},
			"support_phone_number":                    schema.StringAttribute{Computed: true, MarkdownDescription: "Support phone number shown during Setup Assistant."},
			"support_email_address":                   schema.StringAttribute{Computed: true, MarkdownDescription: "Support email address shown during Setup Assistant."},
			"department":                              schema.StringAttribute{Computed: true, MarkdownDescription: "Department label shown during Setup Assistant."},
			"require_authentication":                  schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether users must authenticate during Setup Assistant."},
			"authentication_prompt":                   schema.StringAttribute{Computed: true, MarkdownDescription: "Prompt shown when authentication is required."},
			"device_enrollment_program_instance_id":   schema.StringAttribute{Computed: true, MarkdownDescription: "ADE instance ID backing this PreStage."},
			"site_id":                                 schema.StringAttribute{Computed: true, MarkdownDescription: "Site ID, or `\"-1\"` when no site is set."},
			"enrollment_site_id":                      schema.StringAttribute{Computed: true, MarkdownDescription: "Site ID applied to enrolled devices, or `\"-1\"` when no site."},
			"enrollment_customization_id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Enrollment customization ID, or `\"0\"` when no customization."},
			"profile_uuid":                            schema.StringAttribute{Computed: true, MarkdownDescription: "MDM profile UUID assigned by Jamf Pro."},
			"prestage_minimum_os_target_version_type": schema.StringAttribute{Computed: true, MarkdownDescription: "Minimum-OS enforcement mode."},
			"minimum_os_specific_version":             schema.StringAttribute{Computed: true, MarkdownDescription: "Specific minimum macOS version (used only when target type is `MINIMUM_OS_SPECIFIC_VERSION`)."},
			"psso_enabled":                            schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Platform SSO is enabled."},
			"timeouts":                                timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces ExactlyOneOf(id, name).
func (d *ComputerPrestageEnrollmentDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *ComputerPrestageEnrollmentDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_computer_prestage_enrollment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a single prestage by id or by display-name resolution.
func (d *ComputerPrestageEnrollmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data ComputerPrestageEnrollmentDataSourceModel
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

	var id string
	switch {
	case !data.ID.IsNull() && !data.ID.IsUnknown() && data.ID.ValueString() != "":
		id = data.ID.ValueString()
	case !data.Name.IsNull() && !data.Name.IsUnknown() && data.Name.ValueString() != "":
		var err error
		id, err = d.client.ResolveComputerPrestageV3IDByName(readCtx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error resolving Jamf Pro computer prestage by name", helpers.APIErrorDetail(err))
			return
		}
	default:
		resp.Diagnostics.AddError("Missing lookup attribute", "Supply either `id` or `name`.")
		return
	}

	got, err := d.client.GetComputerPrestageV3(readCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro computer prestage enrollment", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(got.ID)
	data.Name = types.StringValue(got.DisplayName)
	data.DisplayName = types.StringValue(got.DisplayName)
	data.Mandatory = types.BoolValue(got.Mandatory)
	data.MdmRemovable = types.BoolValue(got.MDMRemovable)
	data.DefaultPrestage = types.BoolValue(got.DefaultPrestage)
	data.SupportPhoneNumber = types.StringValue(got.SupportPhoneNumber)
	data.SupportEmailAddress = types.StringValue(got.SupportEmailAddress)
	data.Department = types.StringValue(got.Department)
	data.RequireAuthentication = types.BoolValue(got.RequireAuthentication)
	data.AuthenticationPrompt = types.StringValue(got.AuthenticationPrompt)
	data.DeviceEnrollmentProgramInstanceID = types.StringValue(got.DeviceEnrollmentProgramInstanceID)
	data.SiteID = types.StringValue(got.SiteID)
	data.EnrollmentSiteID = types.StringValue(got.EnrollmentSiteID)
	data.EnrollmentCustomizationID = types.StringValue(got.EnrollmentCustomizationID)
	data.ProfileUUID = types.StringValue(got.ProfileUUID)
	data.PrestageMinimumOsTargetVersionType = types.StringValue(got.PrestageMinimumOsTargetVersionType)
	data.MinimumOsSpecificVersion = types.StringValue(got.MinimumOsSpecificVersion)
	data.PssoEnabled = types.BoolValue(got.PssoEnabled)

	tflog.Trace(ctx, "read Jamf Pro computer prestage enrollment data source", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
