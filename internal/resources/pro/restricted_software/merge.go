// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package restricted_software

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// This file implements the read-merge-write overlay that gives the restricted
// software scope its per-category granular ownership, following
// internal/common/scope/merge.go (the LIMITED targets+exclusions shape is
// resource-local, so the merge is too). A classic scope PUT replaces the
// ENTIRE <scope> subtree the moment any category element is present in the
// body (wire-probed 2026-07-08 on /restrictedsoftware), so Update GETs the
// live object, flattens its scope hydrate-all, overlays the declared plan
// categories here, and builds the wire scope from the merged model. Every
// merged field is non-null so the builder emits every category explicitly
// (empty wrappers for empty categories). There is NO limitations block
// anywhere in this shape — the endpoint rejects <limitations> outright.
//
// All-flag precedence: a merged all_computers that is true empties exactly the
// target categories its AllFlagConflictsWith validator names — never
// exclusions, which coexist with a true flag and are preserved.
//
// This resource deliberately honours the admin UI rather than the wire. The
// server keeps buildings and departments alongside all_computers on this
// endpoint (wire-probed 2026-10-14), but the UI offers no target at all once
// All Computers is selected, so a building stored that way could be neither
// seen nor removed there. Every target category therefore conflicts with the
// flag, and a declared one beats an unmanaged flag (scope.PickAllFlag).

// pickSet returns the declared plan set when the category is managed
// (non-null), else the live value, else an empty set — merged output is always
// non-null so the builder emits every category explicitly. Local reimplement
// of the unexported shared-scope helper.
func pickSet(declared, server types.Set) types.Set {
	if !declared.IsNull() {
		return declared
	}
	if !server.IsNull() {
		return server
	}
	return scope.EmptyStringSet()
}

// derefOrZero returns the pointed-to sub-block model, or its zero value when
// the block is absent. Zero-value types.Set fields report null, which is
// exactly the "unmanaged" signal pickSet keys on.
func derefOrZero[T any](m *T) T {
	if m != nil {
		return *m
	}
	var zero T
	return zero
}

// mergeRestrictedSoftwareScope overlays a declared restricted-software scope
// onto the live scope. plan nil (scope unmanaged) returns nil — the caller must
// not send a scope element at all. server nil is treated as an empty live scope.
func mergeRestrictedSoftwareScope(plan, server *RestrictedSoftwareScopeModel) *RestrictedSoftwareScopeModel {
	if plan == nil {
		return nil
	}
	var srv RestrictedSoftwareScopeModel
	if server != nil {
		srv = *server
	}
	pt, st := plan.TargetsOrZero(), srv.TargetsOrZero()
	targets := &RestrictedSoftwareScopeTargetsModel{
		AllComputers:     scope.PickAllFlag(pt.AllComputers, st.AllComputers, pt.ComputerIDs, pt.ComputerGroupIDs, pt.BuildingIDs, pt.DepartmentIDs),
		ComputerIDs:      pickSet(pt.ComputerIDs, st.ComputerIDs),
		ComputerGroupIDs: pickSet(pt.ComputerGroupIDs, st.ComputerGroupIDs),
		BuildingIDs:      pickSet(pt.BuildingIDs, st.BuildingIDs),
		DepartmentIDs:    pickSet(pt.DepartmentIDs, st.DepartmentIDs),
	}
	// All-flag precedence — empty exactly the categories the flag's
	// AllFlagConflictsWith validator names (see the resource schema).
	if targets.AllComputers.ValueBool() {
		targets.ComputerIDs = scope.EmptyStringSet()
		targets.ComputerGroupIDs = scope.EmptyStringSet()
		targets.BuildingIDs = scope.EmptyStringSet()
		targets.DepartmentIDs = scope.EmptyStringSet()
	}

	pe, se := derefOrZero(plan.Exclusions), derefOrZero(srv.Exclusions)
	return &RestrictedSoftwareScopeModel{
		Targets: targets,
		Exclusions: &RestrictedSoftwareScopeExclusionsModel{
			ComputerIDs:                      pickSet(pe.ComputerIDs, se.ComputerIDs),
			ComputerGroupIDs:                 pickSet(pe.ComputerGroupIDs, se.ComputerGroupIDs),
			BuildingIDs:                      pickSet(pe.BuildingIDs, se.BuildingIDs),
			DepartmentIDs:                    pickSet(pe.DepartmentIDs, se.DepartmentIDs),
			DirectoryServiceOrLocalUserNames: pickSet(pe.DirectoryServiceOrLocalUserNames, se.DirectoryServiceOrLocalUserNames),
		},
	}
}
