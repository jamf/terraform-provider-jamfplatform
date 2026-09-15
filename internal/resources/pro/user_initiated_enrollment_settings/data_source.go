// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_initiated_enrollment_settings

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

// UserInitiatedEnrollmentSettingsDataSource is the read-only mirror of the
// resource.
type UserInitiatedEnrollmentSettingsDataSource struct {
	client *pro.Client
}

var _ datasource.DataSource = &UserInitiatedEnrollmentSettingsDataSource{}

// NewUserInitiatedEnrollmentSettingsDataSource constructs the data source.
func NewUserInitiatedEnrollmentSettingsDataSource() datasource.DataSource {
	return &UserInitiatedEnrollmentSettingsDataSource{}
}

// Metadata sets the data source type name.
func (d *UserInitiatedEnrollmentSettingsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pro_user_initiated_enrollment_settings"
}

// Schema returns the data source schema. Every attribute is Computed. Per the
// style guide it is kept inline and flat — no attribute-returning helpers.
func (d *UserInitiatedEnrollmentSettingsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read the current Jamf Pro User-Initiated Enrollment settings (UI: Settings → Global → User-initiated enrollment), including the directory-service Access Groups. One record per tenant." + dataSourcePrivileges,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Fixed singleton identifier. Always `singleton`."},

			"skip_certificate_installation": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether certificate installation is skipped during enrollment."},
			"restrict_reenrollment":         schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether re-enrollment is restricted to authorized users only."},
			"signing_mdm_profile_enabled":   schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a third-party signing certificate signs the MDM profile."},

			"enable_computer_enrollment":             schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether user-initiated enrollment is enabled for computers."},
			"create_management_account":              schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a managed local administrator account is created on enrolled computers."},
			"management_username":                    schema.StringAttribute{Computed: true, MarkdownDescription: "Username for the managed local administrator account."},
			"hide_management_account":                schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the managed local administrator account is hidden."},
			"allow_ssh_only_management_account":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the managed local administrator account has SSH access only."},
			"ensure_ssh_running":                     schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether SSH (Remote Login) is ensured enabled on enrolled computers."},
			"launch_self_service":                    schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Self Service launches after a computer completes enrollment."},
			"sign_quickadd_package":                  schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the QuickAdd package is signed with a developer certificate."},
			"account_driven_device_enrollment_macos": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Account-Driven Device Enrollment is enabled for institutionally owned computers."},

			"profile_driven_enrollment_via_url_institutional": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Profile-Driven Enrollment via URL is enabled for institutionally owned mobile devices."},
			"profile_driven_enrollment_via_url_personal":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Profile-Driven Enrollment via URL is enabled for personally owned mobile devices."},
			"account_driven_user_enrollment":                  schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Account-Driven User Enrollment is enabled for mobile devices."},
			"account_driven_user_enrollment_visionos":         schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Account-Driven User Enrollment is enabled for Apple Vision Pro."},
			"merge_managed_apple_account_usernames":           schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether matching Managed Apple Account usernames are merged during enrollment."},
			"account_driven_device_enrollment_ios":            schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Account-Driven Device Enrollment is enabled for institutionally owned mobile devices."},
			"account_driven_device_enrollment_visionos":       schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Account-Driven Device Enrollment is enabled for Apple Vision Pro."},

			"personal_device_enrollment_type": schema.StringAttribute{Computed: true, MarkdownDescription: "Personal-device enrollment type (deprecated; always `USERENROLLMENT`)."},

			"mdm_signing_certificate": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Third-party MDM profile signing certificate details.",
				Attributes: map[string]schema.Attribute{
					"keystore_file_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Display filename for the uploaded keystore."},
					"subject":            schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate subject DN."},
					"serial_number":      schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate serial number."},
				},
			},
			"developer_certificate": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "QuickAdd developer signing certificate details.",
				Attributes: map[string]schema.Attribute{
					"keystore_file_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Display filename for the uploaded keystore."},
					"subject":            schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate subject DN."},
					"serial_number":      schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate serial number."},
				},
			},

			"access_group": schema.SetNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Directory-service Access Groups permitted to perform user-initiated enrollment.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                                     schema.StringAttribute{Computed: true, MarkdownDescription: "Access Group identifier assigned by Jamf Pro."},
						"directory_service_group_id":             schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier of the directory-service group."},
						"ldap_server_id":                         schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier of the LDAP / directory-service server."},
						"name":                                   schema.StringAttribute{Computed: true, MarkdownDescription: "Display name of the Access Group."},
						"site_id":                                schema.StringAttribute{Computed: true, MarkdownDescription: "Site assigned to devices enrolled through this group."},
						"enterprise_enrollment_enabled":          schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether institutional enrollment is allowed for this group."},
						"personal_enrollment_enabled":            schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether personal-device enrollment is allowed for this group."},
						"account_driven_user_enrollment_enabled": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether Account-Driven User Enrollment is allowed for this group."},
						"require_eula":                           schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether members must accept the EULA during enrollment."},
					},
				},
			},

			"messaging_languages": schema.MapNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Per-language enrollment messaging configured on the tenant (UI: Messaging tab), keyed by ISO 639-1 language code.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":       schema.StringAttribute{Computed: true, MarkdownDescription: "Display name of the language."},
						"page_title": schema.StringAttribute{Computed: true, MarkdownDescription: "Title shown on all enrollment pages."},

						"login_page_text":   schema.StringAttribute{Computed: true, MarkdownDescription: "Text shown below the title on the login page during enrollment."},
						"username_text":     schema.StringAttribute{Computed: true, MarkdownDescription: "Text for the username field on the login page."},
						"password_text":     schema.StringAttribute{Computed: true, MarkdownDescription: "Text for the password field on the login page."},
						"login_button_text": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the log-in button."},

						"device_ownership_page_text":                  schema.StringAttribute{Computed: true, MarkdownDescription: "Text prompting the user to specify device ownership."},
						"personal_device_button_name":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the enroll-personal-device button."},
						"institutional_ownership_button_name":         schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the enroll-institutional-device button."},
						"personal_device_management_description":      schema.StringAttribute{Computed: true, MarkdownDescription: "Description shown for personal device management."},
						"institutional_device_management_description": schema.StringAttribute{Computed: true, MarkdownDescription: "Description shown for institutional device management."},
						"enroll_device_button_name":                   schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the start-enrollment button."},

						"personal_eula":           schema.StringAttribute{Computed: true, MarkdownDescription: "EULA shown for personally owned devices."},
						"institutional_eula":      schema.StringAttribute{Computed: true, MarkdownDescription: "EULA shown for institutionally owned devices and computers."},
						"eula_accept_button_text": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the EULA accept button."},

						"site_selection_text": schema.StringAttribute{Computed: true, MarkdownDescription: "Text prompting site selection during enrollment."},

						"ca_certificate_installation_text":   schema.StringAttribute{Computed: true, MarkdownDescription: "Text shown when installing the CA certificate."},
						"ca_certificate_name":                schema.StringAttribute{Computed: true, MarkdownDescription: "Display name for the CA certificate."},
						"ca_certificate_description":         schema.StringAttribute{Computed: true, MarkdownDescription: "Description for the CA certificate."},
						"ca_certificate_install_button_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the CA certificate install button."},

						"institutional_mdm_installation_text":   schema.StringAttribute{Computed: true, MarkdownDescription: "Text shown when installing the MDM profile (institutional)."},
						"institutional_mdm_profile_name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Display name for the MDM profile (institutional)."},
						"institutional_mdm_profile_description": schema.StringAttribute{Computed: true, MarkdownDescription: "Description for the MDM profile (institutional)."},
						"institutional_mdm_pending_text":        schema.StringAttribute{Computed: true, MarkdownDescription: "Text shown while the MDM profile installs (institutional)."},
						"institutional_mdm_install_button_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the MDM profile install button (institutional)."},

						"user_enrollment_mdm_installation_text":   schema.StringAttribute{Computed: true, MarkdownDescription: "Text shown when prompting to install the MDM profile (user enrollment)."},
						"user_enrollment_mdm_profile_name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Display name for the MDM profile (user enrollment)."},
						"user_enrollment_mdm_profile_description": schema.StringAttribute{Computed: true, MarkdownDescription: "Description for the MDM profile (user enrollment)."},
						"user_enrollment_mdm_install_button_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the MDM profile install button (user enrollment)."},

						"quickadd_installation_text":   schema.StringAttribute{Computed: true, MarkdownDescription: "Text shown when installing the QuickAdd package."},
						"quickadd_name":                schema.StringAttribute{Computed: true, MarkdownDescription: "Display name for the QuickAdd package."},
						"quickadd_progress_text":       schema.StringAttribute{Computed: true, MarkdownDescription: "Text shown while the QuickAdd package downloads."},
						"quickadd_install_button_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the QuickAdd package install button."},

						"enrollment_complete_text":           schema.StringAttribute{Computed: true, MarkdownDescription: "Text shown when enrollment is complete."},
						"enrollment_failed_text":             schema.StringAttribute{Computed: true, MarkdownDescription: "Text shown when enrollment fails."},
						"try_again_button_name":              schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the try-again button."},
						"view_enrollment_status_button_name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the view-enrollment-status button."},
						"view_enrollment_status_text":        schema.StringAttribute{Computed: true, MarkdownDescription: "Text prompting the user to view enrollment status."},
						"log_out_button_name":                schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the log-out button."},
					},
				},
			},

			"timeouts": timeouts.Attributes(ctx),
		},
	}
}

// Configure wires the Jamf Pro client.
func (d *UserInitiatedEnrollmentSettingsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	client, diags := providerdata.ConfigurePro(ctx, req.ProviderData, minJamfProVersion, "jamfplatform_pro_user_initiated_enrollment_settings")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	d.client = client
}

// Read populates state from the settings + access-group endpoints.
func (d *UserInitiatedEnrollmentSettingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError(
			"Provider not configured",
			"The provider client was not configured. Please ensure the provider block is set up correctly.",
		)
		return
	}

	var data UserInitiatedEnrollmentSettingsDataSourceModel
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

	got, err := d.client.GetEnrollmentSettingsV4(readCtx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro User-Initiated Enrollment settings", helpers.APIErrorDetail(err))
		return
	}
	assignSettingsDataSourceModel(&data, got)

	groups, err := d.client.ListEnrollmentAccessGroupsV3(readCtx, nil, true)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro enrollment Access Groups", helpers.APIErrorDetail(err))
		return
	}
	set, d2 := assignAccessGroupsState(readCtx, groups)
	resp.Diagnostics.Append(d2...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.AccessGroups = set

	langs, err := d.client.ListEnrollmentLanguagesV3(readCtx, nil)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Jamf Pro enrollment languages", helpers.APIErrorDetail(err))
		return
	}
	langMap, d3 := messagingLanguagesToMap(readCtx, langs)
	resp.Diagnostics.Append(d3...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.MessagingLanguages = langMap

	data.ID = types.StringValue(helpers.SingletonID)

	tflog.Trace(ctx, "read Jamf Pro User-Initiated Enrollment settings data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
