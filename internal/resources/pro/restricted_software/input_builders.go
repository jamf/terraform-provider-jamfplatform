// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package restricted_software

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// buildRestrictedSoftwareInput projects a plan model into an SDK
// *proclassic.RestrictedSoftware suitable for Create / Update. Scope
// categories follow the granular-ownership omission rules in STYLE_GUIDE.md
// §Scope helper: a null (unmanaged) category is omitted from the body; a
// declared category — including `[]`, via BuildIDSlice/BuildNameSlice
// returning a non-nil empty slice — is emitted explicitly, empty wrapper and
// all, because the scope subtree replaces wholesale once any category element
// is present. On Update the caller passes a plan whose Scope is the
// read-merge-write merged model, so every category is non-null and the full
// explicit skeleton is emitted.
func buildRestrictedSoftwareInput(ctx context.Context, plan RestrictedSoftwareResourceModel) (*proclassic.RestrictedSoftware, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := &proclassic.RestrictedSoftware{}

	if plan.General != nil {
		out.General = buildGeneral(plan.General)
	}

	if plan.Scope != nil {
		s, d := buildScope(ctx, plan.Scope)
		diags.Append(d...)
		out.Scope = s
	}

	return out, diags
}

// buildGeneral maps the UI-aligned general model into the wire struct. The
// renamed attributes map to their classic wire names here:
// restrict_exact_process_name→MatchExactProcessName,
// send_email_notification_on_violation→SendNotification,
// delete_application→DeleteExecutable.
func buildGeneral(m *RestrictedSoftwareGeneralModel) *proclassic.RestrictedSoftwareGeneral {
	g := &proclassic.RestrictedSoftwareGeneral{
		Name:                  helpers.OptionalStringPointer(m.Name),
		ProcessName:           helpers.OptionalStringPointer(m.ProcessName),
		MatchExactProcessName: helpers.OptionalBoolPointer(m.RestrictExactProcessName),
		SendNotification:      helpers.OptionalBoolPointer(m.SendEmailNotificationOnViolation),
		KillProcess:           helpers.OptionalBoolPointer(m.KillProcess),
		DeleteExecutable:      helpers.OptionalBoolPointer(m.DeleteApplication),
		DisplayMessage:        helpers.OptionalStringPointer(m.DisplayMessage),
	}
	if siteID := helpers.StringIDPtr(m.SiteID); siteID != nil {
		g.Site = &proclassic.SiteObject{ID: siteID}
	}
	return g
}

func buildScope(ctx context.Context, m *RestrictedSoftwareScopeModel) (*proclassic.RestrictedSoftwareScope, diag.Diagnostics) {
	var diags diag.Diagnostics
	t := m.TargetsOrZero()
	s := &proclassic.RestrictedSoftwareScope{
		AllComputers: helpers.OptionalBoolPointer(t.AllComputers),
	}

	computers, d := scope.BuildIDSlice(ctx, t.ComputerIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if computers != nil {
		s.Computers = &proclassic.RestrictedSoftwareScopeComputers{Computer: computers}
	}

	computerGroups, d := scope.BuildIDSlice(ctx, t.ComputerGroupIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if computerGroups != nil {
		s.ComputerGroups = &proclassic.RestrictedSoftwareScopeComputerGroups{ComputerGroup: computerGroups}
	}

	buildings, d := scope.BuildIDSlice(ctx, t.BuildingIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if buildings != nil {
		s.Buildings = &proclassic.RestrictedSoftwareScopeBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, t.DepartmentIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if departments != nil {
		s.Departments = &proclassic.RestrictedSoftwareScopeDepartments{Department: departments}
	}

	if m.Exclusions != nil {
		e, ed := buildScopeExclusions(ctx, m.Exclusions)
		diags.Append(ed...)
		s.Exclusions = e
	}

	// Omission semantics (STYLE_GUIDE.md §Scope helper): collapse to nil when
	// every child pointer is nil so the payload omits <scope> entirely.
	if s.AllComputers == nil && s.Computers == nil && s.ComputerGroups == nil &&
		s.Buildings == nil && s.Departments == nil && s.Exclusions == nil {
		return nil, diags
	}
	return s, diags
}

func buildScopeExclusions(ctx context.Context, m *RestrictedSoftwareScopeExclusionsModel) (*proclassic.RestrictedSoftwareScopeExclusions, diag.Diagnostics) {
	var diags diag.Diagnostics
	e := &proclassic.RestrictedSoftwareScopeExclusions{}

	computers, d := scope.BuildIDSlice(ctx, m.ComputerIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if computers != nil {
		e.Computers = &proclassic.RestrictedSoftwareScopeExclusionsComputers{Computer: computers}
	}

	computerGroups, d := scope.BuildIDSlice(ctx, m.ComputerGroupIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if computerGroups != nil {
		e.ComputerGroups = &proclassic.RestrictedSoftwareScopeExclusionsComputerGroups{ComputerGroup: computerGroups}
	}

	buildings, d := scope.BuildIDSlice(ctx, m.BuildingIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if buildings != nil {
		e.Buildings = &proclassic.RestrictedSoftwareScopeExclusionsBuildings{Building: buildings}
	}

	departments, d := scope.BuildIDSlice(ctx, m.DepartmentIDs, func(id int) proclassic.IDName { return proclassic.IDName{ID: &id} })
	diags.Append(d...)
	if departments != nil {
		e.Departments = &proclassic.RestrictedSoftwareScopeExclusionsDepartments{Department: departments}
	}

	// <users> exclusions are NAME-keyed free-text local usernames.
	users, d := scope.BuildNameSlice(ctx, m.DirectoryServiceOrLocalUserNames, func(name string) proclassic.IDName {
		n := name
		return proclassic.IDName{Name: &n}
	})
	diags.Append(d...)
	if users != nil {
		e.Users = &proclassic.RestrictedSoftwareScopeExclusionsUsers{User: users}
	}

	// Always emit the block when the user declared `exclusions` (the caller's
	// gate): a scope PUT replaces the whole subtree once any category element
	// is present (wire-probed 2026-07-08), so a declared-but-empty block must
	// land as an explicit <exclusions></exclusions> — the clear gesture for
	// `[]`. Undeclared (null) categories inside a declared block are handled by
	// the Update merge, which re-emits the live members.
	return e, diags
}
