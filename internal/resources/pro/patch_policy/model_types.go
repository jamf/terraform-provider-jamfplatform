// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// PatchPolicyResourceModel is the Terraform resource model for a Jamf Pro patch
// policy (the classic /patchpolicies endpoint). A policy is created against a
// patch software title configuration (software_title_configuration_id) and
// targets a single version of that title that has a package assigned.
//
// scope and user_interaction are pointer-typed optional blocks so an undeclared
// block stays null in state. The classic server echoes both <scope> and
// <user_interaction> (with full defaults) on every GET, so Read only refreshes a
// block the caller manages — populating an unmanaged block would violate the
// framework's "produced inconsistent result after apply" check.
type PatchPolicyResourceModel struct {
	ID                           types.String                     `tfsdk:"id"`
	SoftwareTitleConfigurationID types.String                     `tfsdk:"software_title_configuration_id"`
	Name                         types.String                     `tfsdk:"name"`
	Enabled                      types.Bool                       `tfsdk:"enabled"`
	TargetVersion                types.String                     `tfsdk:"target_version"`
	DistributionMethod           types.String                     `tfsdk:"distribution_method"`
	AllowDowngrade               types.Bool                       `tfsdk:"allow_downgrade"`
	PatchUnknown                 types.Bool                       `tfsdk:"patch_unknown"`
	ReleaseDate                  types.Int64                      `tfsdk:"release_date"`
	IncrementalUpdate            types.Bool                       `tfsdk:"incremental_update"`
	Reboot                       types.Bool                       `tfsdk:"reboot"`
	MinimumOS                    types.String                     `tfsdk:"minimum_os"`
	KillApps                     types.List                       `tfsdk:"kill_apps"`
	Scope                        *PatchPolicyScopeModel           `tfsdk:"scope"`
	UserInteraction              *PatchPolicyUserInteractionModel `tfsdk:"user_interaction"`
	Timeouts                     resourceTimeouts.Value           `tfsdk:"timeouts"`
}

// PatchPolicyScopeModel models <patch_policy><scope>. A LIMITED computer-scope
// subset (wire-probed): targets are computers / computer_groups / buildings /
// departments plus the all_computers flag, a limitations block (network segments
// + iBeacons), and exclusions. NO target users or user groups — the classic GET
// never returns user-based patch-policy scope even when set, so they are not
// modelled. Hand-composed from the shared scope primitives.
//
// The all-flag and per-category target ID sets live inside the `targets`
// sub-block, mirroring the admin UI's Targets / Limitations / Exclusions tabs.
type PatchPolicyScopeModel struct {
	Targets     *PatchPolicyScopeTargetsModel     `tfsdk:"targets"`
	Limitations *PatchPolicyScopeLimitationsModel `tfsdk:"limitations"`
	Exclusions  *PatchPolicyScopeExclusionsModel  `tfsdk:"exclusions"`
}

// PatchPolicyScopeTargetsModel models <scope> targets — the all_computers flag
// plus the per-category target ID sets, mirroring the admin UI's Targets tab.
type PatchPolicyScopeTargetsModel struct {
	AllComputers     types.Bool `tfsdk:"all_computers"`
	ComputerIDs      types.Set  `tfsdk:"computer_ids"`
	ComputerGroupIDs types.Set  `tfsdk:"computer_group_ids"`
	BuildingIDs      types.Set  `tfsdk:"building_ids"`
	DepartmentIDs    types.Set  `tfsdk:"department_ids"`
}

// TargetsOrZero returns the targets sub-model, or a zero value with null
// flag/sets when the block was omitted, so input-builders can read target
// fields without a nil-guard.
func (m PatchPolicyScopeModel) TargetsOrZero() PatchPolicyScopeTargetsModel {
	if m.Targets != nil {
		return *m.Targets
	}
	return PatchPolicyScopeTargetsModel{
		AllComputers:     types.BoolNull(),
		ComputerIDs:      types.SetNull(types.StringType),
		ComputerGroupIDs: types.SetNull(types.StringType),
		BuildingIDs:      types.SetNull(types.StringType),
		DepartmentIDs:    types.SetNull(types.StringType),
	}
}

// PatchPolicyScopeLimitationsModel models <scope><limitations>: network segments
// and iBeacons only.
type PatchPolicyScopeLimitationsModel struct {
	NetworkSegmentIDs types.Set `tfsdk:"network_segment_ids"`
	IbeaconIDs        types.Set `tfsdk:"ibeacon_ids"`
}

// PatchPolicyScopeExclusionsModel models <scope><exclusions>: computers,
// computer groups, buildings, departments, network segments, iBeacons.
type PatchPolicyScopeExclusionsModel struct {
	ComputerIDs       types.Set `tfsdk:"computer_ids"`
	ComputerGroupIDs  types.Set `tfsdk:"computer_group_ids"`
	BuildingIDs       types.Set `tfsdk:"building_ids"`
	DepartmentIDs     types.Set `tfsdk:"department_ids"`
	NetworkSegmentIDs types.Set `tfsdk:"network_segment_ids"`
	IbeaconIDs        types.Set `tfsdk:"ibeacon_ids"`
}

// PatchPolicyUserInteractionModel models <patch_policy><user_interaction> — the
// admin UI "User Interaction" tab. The wrapper and its nested blocks are
// Optional-only (NOT Optional+Computed) pointer structs: the server echoes a
// full default superset on GET, and an Optional+Computed SingleNestedAttribute
// backed by a *struct trips the framework's Unknown-decode at apply (see
// feedback_optional_computed_nested_object). The leaf scalars inside a declared
// block ARE Optional+Computed so the server-default null→value transition is
// legal on create. Read is state-gated: a block is only refreshed when already
// managed.
type PatchPolicyUserInteractionModel struct {
	InstallButtonText      types.String                                  `tfsdk:"install_button_text"`
	SelfServiceDescription types.String                                  `tfsdk:"self_service_description"`
	SelfServiceIconID      types.String                                  `tfsdk:"self_service_icon_id"`
	Notifications          *PatchPolicyUserInteractionNotificationsModel `tfsdk:"notifications"`
	Deadlines              *PatchPolicyUserInteractionDeadlinesModel     `tfsdk:"deadlines"`
	GracePeriod            *PatchPolicyUserInteractionGracePeriodModel   `tfsdk:"grace_period"`
}

// PatchPolicyUserInteractionNotificationsModel models <user_interaction><notifications>.
type PatchPolicyUserInteractionNotificationsModel struct {
	Enabled   types.Bool                                             `tfsdk:"enabled"`
	Subject   types.String                                           `tfsdk:"subject"`
	Message   types.String                                           `tfsdk:"message"`
	Type      types.String                                           `tfsdk:"type"`
	Reminders *PatchPolicyUserInteractionNotificationsRemindersModel `tfsdk:"reminders"`
}

// PatchPolicyUserInteractionNotificationsRemindersModel models
// <user_interaction><notifications><reminders>.
type PatchPolicyUserInteractionNotificationsRemindersModel struct {
	Enabled   types.Bool  `tfsdk:"enabled"`
	Frequency types.Int64 `tfsdk:"frequency"`
}

// PatchPolicyUserInteractionDeadlinesModel models <user_interaction><deadlines>.
type PatchPolicyUserInteractionDeadlinesModel struct {
	Enabled types.Bool  `tfsdk:"enabled"`
	Period  types.Int64 `tfsdk:"period"`
}

// PatchPolicyUserInteractionGracePeriodModel models <user_interaction><grace_period>.
type PatchPolicyUserInteractionGracePeriodModel struct {
	Duration                  types.Int64  `tfsdk:"duration"`
	NotificationCenterSubject types.String `tfsdk:"notification_center_subject"`
	Message                   types.String `tfsdk:"message"`
}

// PatchPolicyDataSourceModel is the flat data source model. Surfaces a read-only
// projection of the most-frequently looked-up general fields so users can
// resolve a policy by ID; manage the policy as a resource for full scope /
// user_interaction detail.
type PatchPolicyDataSourceModel struct {
	ID                           types.String             `tfsdk:"id"`
	SoftwareTitleConfigurationID types.String             `tfsdk:"software_title_configuration_id"`
	Name                         types.String             `tfsdk:"name"`
	Enabled                      types.Bool               `tfsdk:"enabled"`
	TargetVersion                types.String             `tfsdk:"target_version"`
	DistributionMethod           types.String             `tfsdk:"distribution_method"`
	AllowDowngrade               types.Bool               `tfsdk:"allow_downgrade"`
	PatchUnknown                 types.Bool               `tfsdk:"patch_unknown"`
	ReleaseDate                  types.Int64              `tfsdk:"release_date"`
	IncrementalUpdate            types.Bool               `tfsdk:"incremental_update"`
	Reboot                       types.Bool               `tfsdk:"reboot"`
	MinimumOS                    types.String             `tfsdk:"minimum_os"`
	KillApps                     types.List               `tfsdk:"kill_apps"`
	Timeouts                     datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// patchPolicyIdentityModel represents the identity object for the resource and
// list results.
type patchPolicyIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// PatchPolicyListResourceModel represents the config model for list queries.
// Classic has no RSQL — the filter shape is the shared client-side substring
// block. Unlike patch software titles, the patch policies list response carries
// a display name, so the filter matches the policy name.
type PatchPolicyListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
