// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package enrollment_customization

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// EnrollmentCustomizationResourceModel is the Terraform resource model for a
// Jamf Pro enrollment customization, including the branding settings parent
// record and any combination of text / ldap / sso panes.
type EnrollmentCustomizationResourceModel struct {
	ID               types.String           `tfsdk:"id"`
	DisplayName      types.String           `tfsdk:"display_name"`
	Description      types.String           `tfsdk:"description"`
	SiteID           types.String           `tfsdk:"site_id"`
	IconSource       types.String           `tfsdk:"icon_source"`
	IconSourceHash   types.String           `tfsdk:"icon_source_hash"`
	BrandingSettings *brandingSettingsModel `tfsdk:"branding_settings"`
	TextPanes        []textPaneModel        `tfsdk:"text_panes"`
	LdapPanes        []ldapPaneModel        `tfsdk:"ldap_panes"`
	SsoPanes         []ssoPaneModel         `tfsdk:"sso_panes"`
	Timeouts         resourceTimeouts.Value `tfsdk:"timeouts"`
}

// brandingSettingsModel is the nested branding settings block, holding the
// four palette colours plus the icon URL.
type brandingSettingsModel struct {
	BodyTextColor   types.String `tfsdk:"body_text_color"`
	ButtonColor     types.String `tfsdk:"button_color"`
	ButtonTextColor types.String `tfsdk:"button_text_color"`
	BackgroundColor types.String `tfsdk:"background_color"`
	IconURL         types.String `tfsdk:"icon_url"`
}

// textPaneModel is a single text pane element.
type textPaneModel struct {
	ID                 types.String `tfsdk:"id"`
	DisplayName        types.String `tfsdk:"display_name"`
	Rank               types.Int64  `tfsdk:"rank"`
	Title              types.String `tfsdk:"title"`
	Body               types.String `tfsdk:"body"`
	Subtext            types.String `tfsdk:"subtext"`
	PreviousButtonText types.String `tfsdk:"previous_button_text"`
	NextButtonText     types.String `tfsdk:"next_button_text"`
}

// ldapPaneModel is a single LDAP authentication pane element.
type ldapPaneModel struct {
	ID                     types.String                 `tfsdk:"id"`
	DisplayName            types.String                 `tfsdk:"display_name"`
	Rank                   types.Int64                  `tfsdk:"rank"`
	Title                  types.String                 `tfsdk:"title"`
	UsernameText           types.String                 `tfsdk:"username_text"`
	PasswordText           types.String                 `tfsdk:"password_text"`
	PreviousButtonText     types.String                 `tfsdk:"previous_button_text"`
	LoginButtonText        types.String                 `tfsdk:"login_button_text"`
	DirectoryServiceGroups []directoryServiceGroupModel `tfsdk:"directory_service_groups"`
}

// directoryServiceGroupModel is a single LDAP group access entry.
type directoryServiceGroupModel struct {
	GroupName                types.String `tfsdk:"group_name"`
	DirectoryServiceServerID types.Int64  `tfsdk:"directory_service_server_id"`
}

// ssoPaneModel is a single SSO authentication pane element.
type ssoPaneModel struct {
	ID                        types.String `tfsdk:"id"`
	DisplayName               types.String `tfsdk:"display_name"`
	Rank                      types.Int64  `tfsdk:"rank"`
	EnrollmentAccess          types.String `tfsdk:"enrollment_access"`
	AccessGroupName           types.String `tfsdk:"access_group_name"`
	PassUserInfoToJamfConnect types.Bool   `tfsdk:"pass_user_info_to_jamf_connect"`
	AccountNameAttribute      types.String `tfsdk:"account_name_attribute"`
	AccountFullNameAttribute  types.String `tfsdk:"account_full_name_attribute"`
}

// EnrollmentCustomizationIdentityModel is the framework identity struct used
// for import.
type EnrollmentCustomizationIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// EnrollmentCustomizationDataSourceModel is the data-source-side model. The
// data source returns only the parent record (no panes, no icon_source) — the
// panes belong to the managed resource lifecycle.
type EnrollmentCustomizationDataSourceModel struct {
	ID               types.String             `tfsdk:"id"`
	DisplayName      types.String             `tfsdk:"display_name"`
	Description      types.String             `tfsdk:"description"`
	SiteID           types.String             `tfsdk:"site_id"`
	BrandingSettings *brandingSettingsModel   `tfsdk:"branding_settings"`
	Timeouts         datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// EnrollmentCustomizationListResourceModel is the list resource config model.
// The Pro v2 list endpoint accepts no RSQL filter, so client-side substring
// matching is provided via the shared classic-filter helper.
type EnrollmentCustomizationListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
