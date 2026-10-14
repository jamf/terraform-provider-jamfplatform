// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_policy

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// This file implements the read-merge-write overlay that gives the patch
// policy scope its per-category granular ownership, following
// internal/common/scope/merge.go (the LIMITED computer-scope shape is
// resource-local, so the merge is too). Across the classic family a scope PUT
// replaces the ENTIRE <scope> subtree the moment any category element is
// present in the body (wire-probed 2026-07-08 on 8 sibling endpoints, and on
// /patchpolicies itself 2026-10-14: a body carrying only the flag cleared every
// category). Update therefore GETs the live object, flattens its scope
// hydrate-all, overlays the declared plan categories here, and builds the wire
// scope from the merged model. Every merged field is non-null so the builder
// emits every category explicitly (empty wrappers for empty categories).
//
// All-flag precedence: a merged all_computers that is true empties exactly the
// target categories its AllFlagConflictsWith validator names — computers and
// computer groups, never buildings or departments, which the server keeps
// alongside the flag (wire-probed 2026-10-14) — and never limitations or
// exclusions, which coexist with a true flag and are preserved. A declared
// computer or computer group beats an unmanaged flag (scope.PickAllFlag).

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

// mergePatchPolicyScope overlays a declared patch-policy scope onto the live
// scope. plan nil (scope unmanaged) returns nil — the caller must not send a
// scope element at all. server nil is treated as an empty live scope.
func mergePatchPolicyScope(plan, server *PatchPolicyScopeModel) *PatchPolicyScopeModel {
	if plan == nil {
		return nil
	}
	var srv PatchPolicyScopeModel
	if server != nil {
		srv = *server
	}
	pt, st := plan.TargetsOrZero(), srv.TargetsOrZero()
	targets := &PatchPolicyScopeTargetsModel{
		AllComputers:     scope.PickAllFlag(pt.AllComputers, st.AllComputers, pt.ComputerIDs, pt.ComputerGroupIDs),
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
	}

	pl, sl := derefOrZero(plan.Limitations), derefOrZero(srv.Limitations)
	pe, se := derefOrZero(plan.Exclusions), derefOrZero(srv.Exclusions)
	return &PatchPolicyScopeModel{
		Targets: targets,
		Limitations: &PatchPolicyScopeLimitationsModel{
			NetworkSegmentIDs: pickSet(pl.NetworkSegmentIDs, sl.NetworkSegmentIDs),
			IbeaconIDs:        pickSet(pl.IbeaconIDs, sl.IbeaconIDs),
		},
		Exclusions: &PatchPolicyScopeExclusionsModel{
			ComputerIDs:       pickSet(pe.ComputerIDs, se.ComputerIDs),
			ComputerGroupIDs:  pickSet(pe.ComputerGroupIDs, se.ComputerGroupIDs),
			BuildingIDs:       pickSet(pe.BuildingIDs, se.BuildingIDs),
			DepartmentIDs:     pickSet(pe.DepartmentIDs, se.DepartmentIDs),
			NetworkSegmentIDs: pickSet(pe.NetworkSegmentIDs, se.NetworkSegmentIDs),
			IbeaconIDs:        pickSet(pe.IbeaconIDs, se.IbeaconIDs),
		},
	}
}
