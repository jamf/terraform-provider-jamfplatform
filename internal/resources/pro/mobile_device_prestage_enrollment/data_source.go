// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

// SDK endpoints used:
//   pro.GetMobileDevicePrestageV3
//   pro.ResolveMobileDevicePrestageV3IDByName
//   pro.ResolveMobileDevicePrestageV3ByName
//
// Status: current. Last reviewed 2026-05-30.

package mobile_device_prestage_enrollment

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

// MobileDevicePrestageEnrollmentDataSource implements the Terraform data
// source for Jamf Pro Mobile Device PreStage Enrollments. Lookup is by `id` OR
// `name` (ExactlyOneOf).
type MobileDevicePrestageEnrollmentDataSource struct {
	client *pro.Client
}

var (
	_ datasource.DataSource                     = &MobileDevicePrestageEnrollmentDataSource{}
	_ datasource.DataSourceWithConfigure        = &MobileDevicePrestageEnrollmentDataSource{}
	_ datasource.DataSourceWithConfigValidators = &MobileDevicePrestageEnrollmentDataSource{}
)

// NewMobileDevicePrestageEnrollmentDataSource returns a new data source instance.
func NewMobileDevicePrestageEnrollmentDataSource() datasource.DataSource {
	return &MobileDevicePrestageEnrollmentDataSource{}
}

// Metadata sets the data source type name.
func (d *MobileDevicePrestageEnrollmentDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_mobile_device_prestage_enrollment"
}

// Schema returns the data source schema.
func (d *MobileDevicePrestageEnrollmentDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Look up a Jamf Pro Mobile Device PreStage Enrollment by ID or by exact display name. Exactly one of `id` or `name` must be supplied." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Mobile device PreStage enrollment ID. Mutually exclusive with `name`.",
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
			"device_enrollment_program_instance_id": schema.StringAttribute{Computed: true, MarkdownDescription: "ADE instance ID backing this PreStage."},
			"mandatory":                             schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether MDM enrolment is mandatory."},
			"mdm_removable":                         schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the user can remove the MDM profile."},
			"default_prestage":                      schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is the tenant default PreStage."},
			"require_authentication":                schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether users must authenticate during Setup Assistant."},
			"supervised":                            schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether enrolled devices are supervised."},
			"support_phone_number":                  schema.StringAttribute{Computed: true, MarkdownDescription: "Support phone number shown during Setup Assistant."},
			"support_email_address":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Support email address shown during Setup Assistant."},
			"department":                            schema.StringAttribute{Computed: true, MarkdownDescription: "Department label shown during Setup Assistant."},
			"authentication_prompt":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Prompt shown when authentication is required."},
			"site_id":                               schema.StringAttribute{Computed: true, MarkdownDescription: "Site ID, or `\"-1\"` when no site is set."},
			"enrollment_site_id":                    schema.StringAttribute{Computed: true, MarkdownDescription: "Site ID applied to enrolled devices, or `\"-1\"` when no site."},
			"enrollment_customization_id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Enrollment customization ID, or `\"0\"` when no customization."},
			"profile_uuid":                          schema.StringAttribute{Computed: true, MarkdownDescription: "MDM profile UUID assigned by Jamf Pro."},
			"timezone":                              schema.StringAttribute{Computed: true, MarkdownDescription: "Time zone (IANA identifier, e.g. `\"America/Chicago\"`)."},
			"multi_user":                            schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Shared iPad is enabled."},
			"prestage_minimum_os_target_version_type_ios":  schema.StringAttribute{Computed: true, MarkdownDescription: "Minimum-iOS enforcement mode."},
			"prestage_minimum_os_target_version_type_ipad": schema.StringAttribute{Computed: true, MarkdownDescription: "Minimum-iPadOS enforcement mode."},
			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// ConfigValidators enforces ExactlyOneOf(id, name).
func (d *MobileDevicePrestageEnrollmentDataSource) ConfigValidators(ctx context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

// Configure wires the Jamf Pro client into the data source.
func (d *MobileDevicePrestageEnrollmentDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_mobile_device_prestage_enrollment")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read fetches a single prestage by id or by display-name resolution.
func (d *MobileDevicePrestageEnrollmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data MobileDevicePrestageEnrollmentDataSourceModel
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
		id, err = d.client.ResolveMobileDevicePrestageV3IDByName(readCtx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error resolving Jamf Pro mobile device prestage by name", helpers.APIErrorDetail(err))
			return
		}
	default:
		resp.Diagnostics.AddError("Missing lookup attribute", "Supply either `id` or `name`.")
		return
	}

	got, err := d.client.GetMobileDevicePrestageV3(readCtx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Jamf Pro mobile device prestage enrollment", helpers.APIErrorDetail(err))
		return
	}

	data.ID = types.StringValue(got.ID)
	data.Name = types.StringValue(got.DisplayName)
	data.DisplayName = types.StringValue(got.DisplayName)
	data.DeviceEnrollmentProgramInstanceID = types.StringValue(got.DeviceEnrollmentProgramInstanceID)
	data.Mandatory = types.BoolValue(got.Mandatory)
	data.MdmRemovable = types.BoolValue(got.MDMRemovable)
	data.DefaultPrestage = types.BoolValue(got.DefaultPrestage)
	data.RequireAuthentication = types.BoolValue(got.RequireAuthentication)
	data.Supervised = types.BoolValue(got.Supervised)
	data.SupportPhoneNumber = types.StringValue(got.SupportPhoneNumber)
	data.SupportEmailAddress = types.StringValue(got.SupportEmailAddress)
	data.Department = types.StringValue(got.Department)
	data.AuthenticationPrompt = types.StringValue(got.AuthenticationPrompt)
	data.SiteID = types.StringValue(got.SiteID)
	data.EnrollmentSiteID = types.StringValue(got.EnrollmentSiteID)
	data.EnrollmentCustomizationID = types.StringValue(got.EnrollmentCustomizationID)
	data.ProfileUUID = types.StringValue(got.ProfileUUID)
	data.Timezone = types.StringValue(got.Timezone)
	data.MultiUser = types.BoolValue(got.MultiUser)
	data.PrestageMinimumOsTargetVersionTypeIos = types.StringValue(got.PrestageMinimumOsTargetVersionTypeIos)
	data.PrestageMinimumOsTargetVersionTypeIpad = types.StringValue(got.PrestageMinimumOsTargetVersionTypeIpad)

	tflog.Trace(ctx, "read Jamf Pro mobile device prestage enrollment data source", map[string]any{"id": id})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
