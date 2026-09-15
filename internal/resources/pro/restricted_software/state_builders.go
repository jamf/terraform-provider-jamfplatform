// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package restricted_software

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// assignRestrictedSoftwareResourceModel populates a resource model from the SDK
// RestrictedSoftware response. general is always refreshed (required block); the
// optional scope block is only refreshed when the caller (plan or current
// state) already manages it. The classic server echoes <scope> on GET with
// default values, so populating an unmanaged block would violate the
// framework's "produced inconsistent result after apply" check (plan said null,
// we'd return a populated object). See feedback_server_derived_echo_attrs.
//
// includeUnmanaged inverts that gate for the list resource's config-generation
// path (terraform query -generate-config-out): there is no plan to stay
// consistent with, so a wire-present scope is allocated and hydrated, yielding a
// complete exported config rather than a general-only one. CRUD callers pass
// false. The flatteners read wire-authoritatively
// (helpers.ReconcileOptionalStringPointer / helpers.BoolPointerValueOrNull),
// adopting the wire value whatever state holds, so allocating an empty section
// is sufficient for it to fully hydrate. Every field on this resource is echoed
// faithfully by the classic GET — wire-probed against Jamf Pro 11.31.1 on
// 2026-09-06 — so none of them keeps a sticky read. See issue #387.
func assignRestrictedSoftwareResourceModel(ctx context.Context, state *RestrictedSoftwareResourceModel, rs *proclassic.RestrictedSoftware, includeUnmanaged bool) diag.Diagnostics {
	var diags diag.Diagnostics
	if rs == nil {
		return diags
	}

	if id := extractRestrictedSoftwareID(rs); id != "" {
		state.ID = types.StringValue(id)
	}

	if state.General == nil {
		state.General = &RestrictedSoftwareGeneralModel{}
	}
	flattenGeneral(rs.General, state.General)

	if includeUnmanaged && state.Scope == nil && rs.Scope != nil {
		state.Scope = &RestrictedSoftwareScopeModel{}
	}
	if state.Scope != nil && rs.Scope != nil {
		flattenScope(ctx, rs.Scope, state.Scope, includeUnmanaged)
	}

	return diags
}

func flattenGeneral(g *proclassic.RestrictedSoftwareGeneral, state *RestrictedSoftwareGeneralModel) {
	if g == nil {
		return
	}
	state.ID = helpers.StringValueFromIntPtr(g.ID)
	state.Name = helpers.StringPointerValueOrNull(g.Name)
	state.ProcessName = helpers.StringPointerValueOrNull(g.ProcessName)
	state.RestrictExactProcessName = helpers.BoolPointerValueOrNull(g.MatchExactProcessName)
	state.SendEmailNotificationOnViolation = helpers.BoolPointerValueOrNull(g.SendNotification)
	state.KillProcess = helpers.BoolPointerValueOrNull(g.KillProcess)
	state.DeleteApplication = helpers.BoolPointerValueOrNull(g.DeleteExecutable)
	state.DisplayMessage = helpers.ReconcileOptionalStringPointer(g.DisplayMessage, state.DisplayMessage)

	if g.Site != nil {
		state.SiteID = helpers.ReconcileOptionalStringPointer(helpers.StringFromIntPtr(g.Site.ID), state.SiteID)
		state.SiteName = helpers.DerivedRefName(g.Site.ID, g.Site.Name)
	} else {
		state.SiteID = helpers.ReconcileOptionalStringPointer(nil, state.SiteID)
		state.SiteName = types.StringNull()
	}
}

// flattenScope refreshes the scope sub-blocks the caller already manages. When
// includeUnmanaged is set (config generation) every wire-present sub-block is
// first allocated so the from-scratch read hydrates the full scope rather than
// leaving unmanaged targets/exclusions null. Restricted software has no
// limitations tab.
func flattenScope(ctx context.Context, s *proclassic.RestrictedSoftwareScope, state *RestrictedSoftwareScopeModel, includeUnmanaged bool) {
	if includeUnmanaged {
		if state.Targets == nil {
			state.Targets = &RestrictedSoftwareScopeTargetsModel{}
		}
		if state.Exclusions == nil && s.Exclusions != nil {
			state.Exclusions = &RestrictedSoftwareScopeExclusionsModel{}
		}
	}

	// Sub-blocks are gated on caller management (typed-pointer models cannot
	// hold categories without the block struct); within a managed sub-block
	// each category refreshes independently via RefreshManagedSet — a category
	// the caller did not declare (null) stays null, so members maintained in
	// the admin UI never enter state. includeUnmanaged bypasses both gates for
	// import / config-generation hydration and for building the live-side
	// merge base in Update. The FlattenIDNameSet/FlattenNameSet wire flatteners
	// return an empty set (never null) for absent elements, so a managed empty
	// category round-trips as `[]`.
	if state.Targets != nil {
		t := state.Targets
		t.AllComputers = scope.RefreshManagedBool(t.AllComputers, s.AllComputers, includeUnmanaged)
		t.ComputerIDs = scope.RefreshManagedSet(t.ComputerIDs, scope.FlattenIDNameSet(ctx, computerSlice(s.Computers)), includeUnmanaged)
		t.ComputerGroupIDs = scope.RefreshManagedSet(t.ComputerGroupIDs, scope.FlattenIDNameSet(ctx, computerGroupSlice(s.ComputerGroups)), includeUnmanaged)
		t.BuildingIDs = scope.RefreshManagedSet(t.BuildingIDs, scope.FlattenIDNameSet(ctx, buildingSlice(s.Buildings)), includeUnmanaged)
		t.DepartmentIDs = scope.RefreshManagedSet(t.DepartmentIDs, scope.FlattenIDNameSet(ctx, departmentSlice(s.Departments)), includeUnmanaged)
	}

	if state.Exclusions != nil && s.Exclusions != nil {
		x, e := state.Exclusions, s.Exclusions
		x.ComputerIDs = scope.RefreshManagedSet(x.ComputerIDs, scope.FlattenIDNameSet(ctx, exclComputerSlice(e.Computers)), includeUnmanaged)
		x.ComputerGroupIDs = scope.RefreshManagedSet(x.ComputerGroupIDs, scope.FlattenIDNameSet(ctx, exclComputerGroupSlice(e.ComputerGroups)), includeUnmanaged)
		x.BuildingIDs = scope.RefreshManagedSet(x.BuildingIDs, scope.FlattenIDNameSet(ctx, exclBuildingSlice(e.Buildings)), includeUnmanaged)
		x.DepartmentIDs = scope.RefreshManagedSet(x.DepartmentIDs, scope.FlattenIDNameSet(ctx, exclDepartmentSlice(e.Departments)), includeUnmanaged)
		x.DirectoryServiceOrLocalUserNames = scope.RefreshManagedSet(x.DirectoryServiceOrLocalUserNames, scope.FlattenNameSet(ctx, exclUsersSlice(e.Users)), includeUnmanaged)
	}
}

// ---- scope sub-slice accessors -------------------------------------------------

func computerSlice(c *proclassic.RestrictedSoftwareScopeComputers) *[]proclassic.IDName {
	if c == nil {
		return nil
	}
	return c.Computer
}

func computerGroupSlice(g *proclassic.RestrictedSoftwareScopeComputerGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.ComputerGroup
}

func buildingSlice(b *proclassic.RestrictedSoftwareScopeBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func departmentSlice(d *proclassic.RestrictedSoftwareScopeDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func exclComputerSlice(c *proclassic.RestrictedSoftwareScopeExclusionsComputers) *[]proclassic.IDName {
	if c == nil {
		return nil
	}
	return c.Computer
}

func exclComputerGroupSlice(g *proclassic.RestrictedSoftwareScopeExclusionsComputerGroups) *[]proclassic.IDName {
	if g == nil {
		return nil
	}
	return g.ComputerGroup
}

func exclBuildingSlice(b *proclassic.RestrictedSoftwareScopeExclusionsBuildings) *[]proclassic.IDName {
	if b == nil {
		return nil
	}
	return b.Building
}

func exclDepartmentSlice(d *proclassic.RestrictedSoftwareScopeExclusionsDepartments) *[]proclassic.IDName {
	if d == nil {
		return nil
	}
	return d.Department
}

func exclUsersSlice(u *proclassic.RestrictedSoftwareScopeExclusionsUsers) *[]proclassic.IDName {
	if u == nil {
		return nil
	}
	return u.User
}

// ---- set flatteners ------------------------------------------------------------
