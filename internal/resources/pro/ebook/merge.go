// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/scope"
)

// This file implements the read-merge-write overlay that gives the ebook scope
// its per-category granular ownership, following internal/common/scope/merge.go
// (the ebook union+classes shape is resource-local, so the merge is too). A
// classic scope PUT replaces the ENTIRE <scope> subtree the moment any category
// element is present in the body — and ebook's computer+mobile union is ONE
// subtree: a body carrying only computer categories also wipes the mobile
// categories (wire-probed 2026-07-08 on /ebooks). Update therefore GETs the
// live object, flattens its scope hydrate-all, overlays the declared plan
// categories here, and builds the wire scope from the merged model. Every
// merged field is non-null so the builder emits every category explicitly
// (empty wrappers for empty categories).
//
// All-flag precedence: a merged all-flag that is true empties exactly the
// target categories its AllFlagConflictsWith validator names — never
// limitations/exclusions, which coexist with a true flag and are preserved. A
// declared paired category beats an unmanaged flag (scope.PickAllFlag).
//
// This resource honours the admin UI where it is stricter than the wire: with
// all three flags on, the UI offers no building, department or class, while
// the server keeps them (wire-probed 2026-10-14). That combination therefore
// empties those three categories too, and AllFlagsConflictWith rejects them at
// plan time.

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

// allThreeFlags reports whether all computers, all mobile devices and all users
// are all on — the one combination under which the admin UI offers no building,
// department or class target either.
func allThreeFlags(t *EbookScopeTargetsModel) bool {
	return t.AllComputers.ValueBool() && t.AllMobileDevices.ValueBool() && t.AllJssUsers.ValueBool()
}

// derefOrZero returns the pointed-to sub-block model, or its zero value when
// the block is absent. Zero-value types.Set / types.Bool fields report null,
// which is exactly the "unmanaged" signal pickSet/pickBool key on.
func derefOrZero[T any](m *T) T {
	if m != nil {
		return *m
	}
	var zero T
	return zero
}

// mergeEbookScope overlays a declared ebook dual-target-union scope onto the
// live scope. plan nil (scope unmanaged) returns nil — the caller must not send
// a scope element at all. server nil is treated as an empty live scope.
func mergeEbookScope(plan, server *EbookScopeModel) *EbookScopeModel {
	if plan == nil {
		return nil
	}
	var srv EbookScopeModel
	if server != nil {
		srv = *server
	}
	pt, st := plan.TargetsOrZero(), srv.TargetsOrZero()
	targets := &EbookScopeTargetsModel{
		AllComputers:         scope.PickAllFlag(pt.AllComputers, st.AllComputers, pt.ComputerIDs, pt.ComputerGroupIDs),
		AllMobileDevices:     scope.PickAllFlag(pt.AllMobileDevices, st.AllMobileDevices, pt.MobileDeviceIDs, pt.MobileDeviceGroupIDs),
		AllJssUsers:          scope.PickAllFlag(pt.AllJssUsers, st.AllJssUsers, pt.UserIDs, pt.UserGroupIDs),
		ComputerIDs:          pickSet(pt.ComputerIDs, st.ComputerIDs),
		ComputerGroupIDs:     pickSet(pt.ComputerGroupIDs, st.ComputerGroupIDs),
		MobileDeviceIDs:      pickSet(pt.MobileDeviceIDs, st.MobileDeviceIDs),
		MobileDeviceGroupIDs: pickSet(pt.MobileDeviceGroupIDs, st.MobileDeviceGroupIDs),
		BuildingIDs:          pickSet(pt.BuildingIDs, st.BuildingIDs),
		DepartmentIDs:        pickSet(pt.DepartmentIDs, st.DepartmentIDs),
		UserIDs:              pickSet(pt.UserIDs, st.UserIDs),
		UserGroupIDs:         pickSet(pt.UserGroupIDs, st.UserGroupIDs),
		ClassIDs:             pickSet(pt.ClassIDs, st.ClassIDs),
	}
	// The admin UI offers no building, department or class once all three
	// flags are on, so a declared one of those beats any flag the plan leaves
	// unmanaged: each such flag is turned off rather than inheriting a true the
	// UI set, which the empty-on-all-three rule below would then use to wipe
	// the members the configuration just declared.
	if allThreeFlags(targets) && scope.HasMembers(pt.BuildingIDs, pt.DepartmentIDs, pt.ClassIDs) {
		if pt.AllComputers.IsNull() {
			targets.AllComputers = types.BoolValue(false)
		}
		if pt.AllMobileDevices.IsNull() {
			targets.AllMobileDevices = types.BoolValue(false)
		}
		if pt.AllJssUsers.IsNull() {
			targets.AllJssUsers = types.BoolValue(false)
		}
	}
	// All-flag precedence — empty exactly the categories each flag's
	// AllFlagConflictsWith validator names (see ebookScopeAttributes).
	if targets.AllComputers.ValueBool() {
		targets.ComputerIDs = scope.EmptyStringSet()
		targets.ComputerGroupIDs = scope.EmptyStringSet()
	}
	if targets.AllMobileDevices.ValueBool() {
		targets.MobileDeviceIDs = scope.EmptyStringSet()
		targets.MobileDeviceGroupIDs = scope.EmptyStringSet()
	}
	if targets.AllJssUsers.ValueBool() {
		targets.UserIDs = scope.EmptyStringSet()
		targets.UserGroupIDs = scope.EmptyStringSet()
	}
	if allThreeFlags(targets) {
		targets.BuildingIDs = scope.EmptyStringSet()
		targets.DepartmentIDs = scope.EmptyStringSet()
		targets.ClassIDs = scope.EmptyStringSet()
	}

	pl, sl := derefOrZero(plan.Limitations), derefOrZero(srv.Limitations)
	pe, se := derefOrZero(plan.Exclusions), derefOrZero(srv.Exclusions)
	return &EbookScopeModel{
		Targets: targets,
		Limitations: &EbookScopeLimitationsModel{
			NetworkSegmentIDs:                pickSet(pl.NetworkSegmentIDs, sl.NetworkSegmentIDs),
			DirectoryServiceOrLocalUserNames: pickSet(pl.DirectoryServiceOrLocalUserNames, sl.DirectoryServiceOrLocalUserNames),
			DirectoryServiceUserGroupNames:   pickSet(pl.DirectoryServiceUserGroupNames, sl.DirectoryServiceUserGroupNames),
		},
		Exclusions: &EbookScopeExclusionsModel{
			ComputerIDs:                      pickSet(pe.ComputerIDs, se.ComputerIDs),
			ComputerGroupIDs:                 pickSet(pe.ComputerGroupIDs, se.ComputerGroupIDs),
			MobileDeviceIDs:                  pickSet(pe.MobileDeviceIDs, se.MobileDeviceIDs),
			MobileDeviceGroupIDs:             pickSet(pe.MobileDeviceGroupIDs, se.MobileDeviceGroupIDs),
			BuildingIDs:                      pickSet(pe.BuildingIDs, se.BuildingIDs),
			DepartmentIDs:                    pickSet(pe.DepartmentIDs, se.DepartmentIDs),
			UserIDs:                          pickSet(pe.UserIDs, se.UserIDs),
			UserGroupIDs:                     pickSet(pe.UserGroupIDs, se.UserGroupIDs),
			NetworkSegmentIDs:                pickSet(pe.NetworkSegmentIDs, se.NetworkSegmentIDs),
			DirectoryServiceOrLocalUserNames: pickSet(pe.DirectoryServiceOrLocalUserNames, se.DirectoryServiceOrLocalUserNames),
			DirectoryServiceUserGroupNames:   pickSet(pe.DirectoryServiceUserGroupNames, se.DirectoryServiceUserGroupNames),
		},
	}
}
