// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// buildPatchPolicyInput projects a plan model into an SDK *proclassic.PatchPolicy
// suitable for Create / Update.
//
// Only the writable general fields are emitted; the server-derived fields
// (release_date, incremental_update, reboot, minimum_os, kill_apps) are never
// sent — the server populates them from the target_version's patch definition.
// software_title_configuration_id is set in the body in addition to being passed
// on the Create path arg.
//
// Scope categories follow the granular-ownership omission rules in
// STYLE_GUIDE.md §Scope helper: a null (unmanaged) category is omitted from
// the body; a declared category — including `[]`, via BuildIDSlice returning a
// non-nil empty slice — is emitted explicitly, empty wrapper and all, because
// the scope subtree replaces wholesale once any category element is present.
// On Update the caller passes a plan whose Scope is the read-merge-write
// merged model, so every category is non-null and the full explicit skeleton
// is emitted. user_interaction is emitted only when the caller declared the
// block; nested blocks the caller omitted are not sent (the server retains /
// defaults them).
func buildPatchPolicyInput(ctx context.Context, plan PatchPolicyResourceModel) (*proclassic.PatchPolicy, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := &proclassic.PatchPolicy{
		SoftwareTitleConfigurationID: helpers.StringIDPtr(plan.SoftwareTitleConfigurationID),
		General:                      buildGeneral(plan),
	}

	if plan.Scope != nil {
		s, d := buildScope(ctx, plan.Scope)
		diags.Append(d...)
		out.Scope = s
	}

	if plan.UserInteraction != nil {
		out.UserInteraction = buildUserInteraction(plan.UserInteraction)
	}

	return out, diags
}

// buildGeneral maps the writable general fields. target_version + name are
// required; the rest are Optional+Computed and only emitted when configured.
func buildGeneral(plan PatchPolicyResourceModel) *proclassic.PatchPolicyGeneral {
	return &proclassic.PatchPolicyGeneral{
		Name:               helpers.OptionalStringPointer(plan.Name),
		TargetVersion:      helpers.OptionalStringPointer(plan.TargetVersion),
		Enabled:            helpers.OptionalBoolPointer(plan.Enabled),
		DistributionMethod: helpers.OptionalStringPointer(plan.DistributionMethod),
		AllowDowngrade:     helpers.OptionalBoolPointer(plan.AllowDowngrade),
		PatchUnknown:       helpers.OptionalBoolPointer(plan.PatchUnknown),
	}
}

func buildScope(ctx context.Context, m *PatchPolicyScopeModel) (*proclassic.PatchPolicyScope, diag.Diagnostics) {
	var diags diag.Diagnostics
	t := m.TargetsOrZero()
	s := &proclassic.PatchPolicyScope{
		AllComputers: helpers.OptionalBoolPointer(t.AllComputers),
	}

	computers, d := scope.BuildIDSlice(ctx, t.ComputerIDs, func(id int) proclassic.PatchPolicyScopeComputersComputerItem {
		return proclassic.PatchPolicyScopeComputersComputerItem{ID: &id}
	})
	diags.Append(d...)
	if computers != nil {
		s.Computers = &proclassic.PatchPolicyScopeComputers{Computer: computers}
	}

	computerGroups, d := scope.BuildIDSlice(ctx, t.ComputerGroupIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if computerGroups != nil {
		s.ComputerGroups = &proclassic.PatchPolicyScopeComputerGroups{ComputerGroup: computerGroups}
	}

	buildings, d := scope.BuildIDSlice(ctx, t.BuildingIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if buildings != nil {
		s.Buildings = &proclassic.PatchPolicyScopeBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, t.DepartmentIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if departments != nil {
		s.Departments = &proclassic.PatchPolicyScopeDepartments{Department: departments}
	}

	if m.Limitations != nil {
		l, ld := buildScopeLimitations(ctx, m.Limitations)
		diags.Append(ld...)
		s.Limitations = l
	}

	if m.Exclusions != nil {
		e, ed := buildScopeExclusions(ctx, m.Exclusions)
		diags.Append(ed...)
		s.Exclusions = e
	}

	// Collapse to nil when every child pointer is nil so the payload omits
	// <scope> entirely (STYLE_GUIDE.md §Scope helper omission semantics).
	if s.AllComputers == nil && s.Computers == nil && s.ComputerGroups == nil &&
		s.Buildings == nil && s.Departments == nil && s.Limitations == nil && s.Exclusions == nil {
		return nil, diags
	}
	return s, diags
}

func buildScopeLimitations(ctx context.Context, m *PatchPolicyScopeLimitationsModel) (*proclassic.PatchPolicyScopeLimitations, diag.Diagnostics) {
	var diags diag.Diagnostics
	l := &proclassic.PatchPolicyScopeLimitations{}

	networkSegments, d := scope.BuildIDSlice(ctx, m.NetworkSegmentIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if networkSegments != nil {
		l.NetworkSegments = &proclassic.PatchPolicyScopeLimitationsNetworkSegments{NetworkSegment: networkSegments}
	}

	ibeacons, d := scope.BuildIDSlice(ctx, m.IbeaconIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if ibeacons != nil {
		l.Ibeacons = &proclassic.PatchPolicyScopeLimitationsIbeacons{Ibeacon: ibeacons}
	}

	// Always emit the block when the user declared `limitations` (the caller's
	// gate): the scope subtree replaces wholesale once any category element is
	// present (family law; see crud.go for the /patchpolicies probe status), so
	// a declared-but-empty block must land as an explicit
	// <limitations></limitations> — the clear gesture for `[]`. Undeclared
	// (null) categories inside a declared block are handled by the Update
	// merge, which re-emits the live members.
	return l, diags
}

func buildScopeExclusions(ctx context.Context, m *PatchPolicyScopeExclusionsModel) (*proclassic.PatchPolicyScopeExclusions, diag.Diagnostics) {
	var diags diag.Diagnostics
	e := &proclassic.PatchPolicyScopeExclusions{}

	computers, d := scope.BuildIDSlice(ctx, m.ComputerIDs, func(id int) proclassic.PatchPolicyScopeExclusionsComputersComputerItem {
		return proclassic.PatchPolicyScopeExclusionsComputersComputerItem{ID: &id}
	})
	diags.Append(d...)
	if computers != nil {
		e.Computers = &proclassic.PatchPolicyScopeExclusionsComputers{Computer: computers}
	}

	computerGroups, d := scope.BuildIDSlice(ctx, m.ComputerGroupIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if computerGroups != nil {
		e.ComputerGroups = &proclassic.PatchPolicyScopeExclusionsComputerGroups{ComputerGroup: computerGroups}
	}

	buildings, d := scope.BuildIDSlice(ctx, m.BuildingIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if buildings != nil {
		e.Buildings = &proclassic.PatchPolicyScopeExclusionsBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, m.DepartmentIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if departments != nil {
		e.Departments = &proclassic.PatchPolicyScopeExclusionsDepartments{Department: departments}
	}

	networkSegments, d := scope.BuildIDSlice(ctx, m.NetworkSegmentIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if networkSegments != nil {
		e.NetworkSegments = &proclassic.PatchPolicyScopeExclusionsNetworkSegments{NetworkSegment: networkSegments}
	}

	ibeacons, d := scope.BuildIDSlice(ctx, m.IbeaconIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if ibeacons != nil {
		e.Ibeacons = &proclassic.PatchPolicyScopeExclusionsIbeacons{Ibeacon: ibeacons}
	}

	// Always emit the block when declared — see buildScopeLimitations.
	return e, diags
}

// buildUserInteraction maps the declared user_interaction block. Nested blocks
// the caller omitted are left nil so the server retains / defaults them. Leaf
// scalars use Optional* helpers so an unconfigured leaf is omitted (the server
// defaults it).
func buildUserInteraction(m *PatchPolicyUserInteractionModel) *proclassic.PatchPolicyUserInteraction {
	ui := &proclassic.PatchPolicyUserInteraction{
		InstallButtonText:      helpers.OptionalStringPointer(m.InstallButtonText),
		SelfServiceDescription: helpers.OptionalStringPointer(m.SelfServiceDescription),
	}

	if icon := helpers.StringIDPtr(m.SelfServiceIconID); icon != nil {
		ui.SelfServiceIcon = &proclassic.PatchPolicyUserInteractionSelfServiceIcon{ID: icon}
	}

	if m.Notifications != nil {
		n := &proclassic.PatchPolicyUserInteractionNotifications{
			NotificationEnabled: helpers.OptionalBoolPointer(m.Notifications.Enabled),
			NotificationSubject: helpers.OptionalStringPointer(m.Notifications.Subject),
			NotificationMessage: helpers.OptionalStringPointer(m.Notifications.Message),
			NotificationType:    helpers.OptionalStringPointer(m.Notifications.Type),
		}
		if m.Notifications.Reminders != nil {
			n.Reminders = &proclassic.PatchPolicyUserInteractionNotificationsReminders{
				NotificationRemindersEnabled:  helpers.OptionalBoolPointer(m.Notifications.Reminders.Enabled),
				NotificationReminderFrequency: helpers.OptionalInt64Pointer(m.Notifications.Reminders.Frequency),
			}
		}
		ui.Notifications = n
	}

	if m.Deadlines != nil {
		ui.Deadlines = &proclassic.PatchPolicyUserInteractionDeadlines{
			DeadlineEnabled: helpers.OptionalBoolPointer(m.Deadlines.Enabled),
			DeadlinePeriod:  helpers.OptionalInt64Pointer(m.Deadlines.Period),
		}
	}

	if m.GracePeriod != nil {
		ui.GracePeriod = &proclassic.PatchPolicyUserInteractionGracePeriod{
			GracePeriodDuration:       helpers.OptionalInt64Pointer(m.GracePeriod.Duration),
			NotificationCenterSubject: helpers.OptionalStringPointer(m.GracePeriod.NotificationCenterSubject),
			Message:                   helpers.OptionalStringPointer(m.GracePeriod.Message),
		}
	}

	return ui
}
