// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package restricted_software

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// RestrictedSoftwareResourceModel is the Terraform resource model for a Jamf
// Pro restricted software record (the classic /restrictedsoftware endpoint).
// The optional scope block is pointer-typed so an undeclared block stays null
// in state — the classic server echoes <scope> on every GET, and Read only
// refreshes a block the caller manages (see assignRestrictedSoftwareResourceModel).
type RestrictedSoftwareResourceModel struct {
	ID       types.String                    `tfsdk:"id"`
	General  *RestrictedSoftwareGeneralModel `tfsdk:"general"`
	Scope    *RestrictedSoftwareScopeModel   `tfsdk:"scope"`
	Timeouts resourceTimeouts.Value          `tfsdk:"timeouts"`
}

// RestrictedSoftwareGeneralModel models <restricted_software><general>. The
// user-facing attribute names mirror the Jamf Pro admin UI "Options" tab; the
// differing wire names are recorded in input_builders.go / state_builders.go.
//   - restrict_exact_process_name          ← <match_exact_process_name>
//   - send_email_notification_on_violation ← <send_notification>
//   - delete_application                   ← <delete_executable>
type RestrictedSoftwareGeneralModel struct {
	ID                               types.String `tfsdk:"id"`
	Name                             types.String `tfsdk:"name"`
	ProcessName                      types.String `tfsdk:"process_name"`
	RestrictExactProcessName         types.Bool   `tfsdk:"restrict_exact_process_name"`
	SendEmailNotificationOnViolation types.Bool   `tfsdk:"send_email_notification_on_violation"`
	KillProcess                      types.Bool   `tfsdk:"kill_process"`
	DeleteApplication                types.Bool   `tfsdk:"delete_application"`
	DisplayMessage                   types.String `tfsdk:"display_message"`
	SiteID                           types.String `tfsdk:"site_id"`
	SiteName                         types.String `tfsdk:"site_name"`
}

// RestrictedSoftwareScopeModel models <restricted_software><scope>. This is a
// LIMITED computer-scope subset (wire-probed): targets are computers /
// computer_groups / buildings / departments plus the all_computers flag, with
// NO limitations block and NO target users. It is hand-composed from the shared
// scope primitives (scope.IDSetAttribute / scope.BuildIDSlice / …) rather than
// the full scope.ComputerScopeAttributes factory because the category set
// genuinely differs — see RESTRICTED_SOFTWARE_SPIKE.md §4. The all-flag and
// per-category target ID sets nest under `targets`, mirroring the admin UI's
// Scope > Targets tab; `exclusions` is a sibling (no limitations tab here).
type RestrictedSoftwareScopeModel struct {
	Targets    *RestrictedSoftwareScopeTargetsModel    `tfsdk:"targets"`
	Exclusions *RestrictedSoftwareScopeExclusionsModel `tfsdk:"exclusions"`
}

// RestrictedSoftwareScopeTargetsModel models <scope> targets — the all_computers
// flag plus the per-category ID sets, mirroring the admin UI's Targets tab.
type RestrictedSoftwareScopeTargetsModel struct {
	AllComputers     types.Bool `tfsdk:"all_computers"`
	ComputerIDs      types.Set  `tfsdk:"computer_ids"`
	ComputerGroupIDs types.Set  `tfsdk:"computer_group_ids"`
	BuildingIDs      types.Set  `tfsdk:"building_ids"`
	DepartmentIDs    types.Set  `tfsdk:"department_ids"`
}

// TargetsOrZero returns the targets sub-model, or a zero value with null fields
// when the block was omitted, so input-builders can read target fields without
// a nil-guard. The omission semantics in BuildIDSlice treat null sets as absent.
func (m RestrictedSoftwareScopeModel) TargetsOrZero() RestrictedSoftwareScopeTargetsModel {
	if m.Targets != nil {
		return *m.Targets
	}
	return RestrictedSoftwareScopeTargetsModel{
		AllComputers:     types.BoolNull(),
		ComputerIDs:      types.SetNull(types.StringType),
		ComputerGroupIDs: types.SetNull(types.StringType),
		BuildingIDs:      types.SetNull(types.StringType),
		DepartmentIDs:    types.SetNull(types.StringType),
	}
}

// RestrictedSoftwareScopeExclusionsModel models <scope><exclusions>. The
// endpoint exposes only these five exclusion categories (no network segments,
// user groups, or iBeacons). directory_service_or_local_user_names carries
// free-text local usernames (NAME-keyed <users><user><name>), not Jamf object
// IDs — so there is no directory-service preflight on this resource.
type RestrictedSoftwareScopeExclusionsModel struct {
	ComputerIDs                      types.Set `tfsdk:"computer_ids"`
	ComputerGroupIDs                 types.Set `tfsdk:"computer_group_ids"`
	BuildingIDs                      types.Set `tfsdk:"building_ids"`
	DepartmentIDs                    types.Set `tfsdk:"department_ids"`
	DirectoryServiceOrLocalUserNames types.Set `tfsdk:"directory_service_or_local_user_names"`
}

// RestrictedSoftwareDataSourceModel is the flat data source model. Surfaces a
// read-only projection of the most-frequently looked-up fields so users can
// resolve IDs by name; manage the record as a resource for full detail.
type RestrictedSoftwareDataSourceModel struct {
	ID                               types.String             `tfsdk:"id"`
	Name                             types.String             `tfsdk:"name"`
	ProcessName                      types.String             `tfsdk:"process_name"`
	RestrictExactProcessName         types.Bool               `tfsdk:"restrict_exact_process_name"`
	SendEmailNotificationOnViolation types.Bool               `tfsdk:"send_email_notification_on_violation"`
	KillProcess                      types.Bool               `tfsdk:"kill_process"`
	DeleteApplication                types.Bool               `tfsdk:"delete_application"`
	DisplayMessage                   types.String             `tfsdk:"display_message"`
	SiteID                           types.String             `tfsdk:"site_id"`
	SiteName                         types.String             `tfsdk:"site_name"`
	Timeouts                         datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// restrictedSoftwareIdentityModel represents the identity object for the
// resource and list results.
type restrictedSoftwareIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// RestrictedSoftwareListResourceModel represents the config model for list
// queries. Classic has no RSQL — the filter shape is the shared client-side
// substring block.
type RestrictedSoftwareListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
