// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package scope

import "github.com/hashicorp/terraform-plugin-framework/types"

// This file implements the read-merge-write overlay that gives classic scope
// its per-category granular ownership. The classic wire offers no per-category
// preserve gesture: a scope PUT replaces the ENTIRE subtree (targets +
// limitations + exclusions, across every category) the moment any category
// element is present in the body — even an empty one — while a body whose
// scope carries no category elements at all is ignored. (Wire-probed on 8
// endpoints, all identical; see STYLE_GUIDE.md §Scope helper omission
// semantics.) So "omit = leave the category to the admin UI" must be
// synthesised at apply time: the resource's Update GETs the live object,
// flattens its scope with every category hydrated, overlays the declared plan
// categories via these Merge* helpers, and builds the wire scope from the
// merged model. Every merged field is non-null, which makes the per-resource
// builder emit every category explicitly (empty wrappers for empty
// categories) — guaranteeing the replace fires and the final wire state
// equals the merged model exactly.
//
// The merge is scope-only by design: no other section of the GET response is
// ever echoed back into the PUT. Create never merges — there is nothing to
// preserve on a brand-new object, so undeclared (null) categories simply stay
// off the POST body.
//
// All-flag precedence has two halves, both keyed on the categories the flag's
// validator declares it mutually exclusive with (its "paired" categories —
// all_computers pairs with computers and computer groups, never with
// buildings or departments; wire-probed 2026-10-14 across every scope-bearing
// endpoint, see STYLE_GUIDE.md §Scope helper):
//
//   - A flag that ends up true empties its paired categories. The server wipes
//     them anyway when the flag is set, and emitting members alongside a true
//     flag is undefined; every other category, limitations and exclusions
//     coexist with a true flag and are preserved as normal.
//   - A DECLARED non-empty paired category beats an UNMANAGED flag (see
//     PickAllFlag): the flag is sent false rather than inheriting a true the
//     admin UI set, which would wipe the members the configuration just asked
//     for and fail the post-apply consistency check (issue #467).

// HasMembers reports whether any of the given declared sets carries at least
// one element. A null or unknown set carries none.
func HasMembers(sets ...types.Set) bool {
	for _, s := range sets {
		if !s.IsNull() && !s.IsUnknown() && len(s.Elements()) > 0 {
			return true
		}
	}
	return false
}

// PickAllFlag resolves an all-flag for the read-merge-write overlay. A declared
// flag always wins; otherwise a declared non-empty paired category turns the
// flag off — the configuration owns the audience through those members, and a
// preserved true from the admin UI would wipe them. With neither, the live
// value is kept (false when there is none). paired are the PLAN's sets for the
// categories the flag conflicts with, never the merged ones.
func PickAllFlag(declared, server types.Bool, paired ...types.Set) types.Bool {
	if !declared.IsNull() {
		return declared
	}
	if HasMembers(paired...) {
		return types.BoolValue(false)
	}
	return pickBool(declared, server)
}

// pickSet returns the declared plan set when the category is managed
// (non-null), else the server's current value, else an empty set — merged
// output is always non-null so the builder emits every category explicitly.
func pickSet(declared, server types.Set) types.Set {
	if !declared.IsNull() {
		return declared
	}
	if !server.IsNull() {
		return server
	}
	return EmptyStringSet()
}

// pickBool is the all-flag sibling of pickSet: declared wins, else the
// server's current value, else false.
func pickBool(declared, server types.Bool) types.Bool {
	if !declared.IsNull() {
		return declared
	}
	if !server.IsNull() {
		return server
	}
	return types.BoolValue(false)
}

// mergeComputerTargets overlays declared computer-platform target categories
// onto the server's, applying all-flag precedence.
func mergeComputerTargets(plan, server ComputerScopeTargetsModel) *ComputerScopeTargetsModel {
	merged := &ComputerScopeTargetsModel{
		AllComputers:     PickAllFlag(plan.AllComputers, server.AllComputers, plan.ComputerIDs, plan.ComputerGroupIDs),
		AllJssUsers:      PickAllFlag(plan.AllJssUsers, server.AllJssUsers, plan.UserIDs, plan.UserGroupIDs),
		ComputerIDs:      pickSet(plan.ComputerIDs, server.ComputerIDs),
		ComputerGroupIDs: pickSet(plan.ComputerGroupIDs, server.ComputerGroupIDs),
		BuildingIDs:      pickSet(plan.BuildingIDs, server.BuildingIDs),
		DepartmentIDs:    pickSet(plan.DepartmentIDs, server.DepartmentIDs),
		UserIDs:          pickSet(plan.UserIDs, server.UserIDs),
		UserGroupIDs:     pickSet(plan.UserGroupIDs, server.UserGroupIDs),
	}
	if merged.AllComputers.ValueBool() {
		merged.ComputerIDs = EmptyStringSet()
		merged.ComputerGroupIDs = EmptyStringSet()
	}
	if merged.AllJssUsers.ValueBool() {
		merged.UserIDs = EmptyStringSet()
		merged.UserGroupIDs = EmptyStringSet()
	}
	return merged
}

// MergeComputerScope overlays a declared iBeacon-bearing computer scope onto
// the server's current scope. plan nil (scope unmanaged) returns nil — the
// caller must not send a scope element at all. server nil is treated as an
// empty live scope.
func MergeComputerScope(plan, server *ComputerScopeModel) *ComputerScopeModel {
	if plan == nil {
		return nil
	}
	var srv ComputerScopeModel
	if server != nil {
		srv = *server
	}
	pl, sl := derefOrZero(plan.Limitations), derefOrZero(srv.Limitations)
	pe, se := derefOrZero(plan.Exclusions), derefOrZero(srv.Exclusions)
	return &ComputerScopeModel{
		Targets: mergeComputerTargets(plan.TargetsOrZero(), srv.TargetsOrZero()),
		Limitations: &ComputerScopeLimitationsModel{
			NetworkSegmentIDs:                pickSet(pl.NetworkSegmentIDs, sl.NetworkSegmentIDs),
			IbeaconIDs:                       pickSet(pl.IbeaconIDs, sl.IbeaconIDs),
			DirectoryServiceOrLocalUserNames: pickSet(pl.DirectoryServiceOrLocalUserNames, sl.DirectoryServiceOrLocalUserNames),
			DirectoryServiceUserGroupNames:   pickSet(pl.DirectoryServiceUserGroupNames, sl.DirectoryServiceUserGroupNames),
		},
		Exclusions: &ComputerScopeExclusionsModel{
			ComputerIDs:                      pickSet(pe.ComputerIDs, se.ComputerIDs),
			ComputerGroupIDs:                 pickSet(pe.ComputerGroupIDs, se.ComputerGroupIDs),
			BuildingIDs:                      pickSet(pe.BuildingIDs, se.BuildingIDs),
			DepartmentIDs:                    pickSet(pe.DepartmentIDs, se.DepartmentIDs),
			UserIDs:                          pickSet(pe.UserIDs, se.UserIDs),
			UserGroupIDs:                     pickSet(pe.UserGroupIDs, se.UserGroupIDs),
			NetworkSegmentIDs:                pickSet(pe.NetworkSegmentIDs, se.NetworkSegmentIDs),
			IbeaconIDs:                       pickSet(pe.IbeaconIDs, se.IbeaconIDs),
			DirectoryServiceOrLocalUserNames: pickSet(pe.DirectoryServiceOrLocalUserNames, se.DirectoryServiceOrLocalUserNames),
			DirectoryServiceUserGroupNames:   pickSet(pe.DirectoryServiceUserGroupNames, se.DirectoryServiceUserGroupNames),
		},
	}
}

// MergeComputerScopeNoIbeacons is MergeComputerScope for the no-iBeacon model
// variant (mac_app_store_app).
func MergeComputerScopeNoIbeacons(plan, server *ComputerScopeModelNoIbeacons) *ComputerScopeModelNoIbeacons {
	if plan == nil {
		return nil
	}
	var srv ComputerScopeModelNoIbeacons
	if server != nil {
		srv = *server
	}
	pl, sl := derefOrZero(plan.Limitations), derefOrZero(srv.Limitations)
	pe, se := derefOrZero(plan.Exclusions), derefOrZero(srv.Exclusions)
	return &ComputerScopeModelNoIbeacons{
		Targets: mergeComputerTargets(plan.TargetsOrZero(), srv.TargetsOrZero()),
		Limitations: &ComputerScopeLimitationsModelNoIbeacons{
			NetworkSegmentIDs:                pickSet(pl.NetworkSegmentIDs, sl.NetworkSegmentIDs),
			DirectoryServiceOrLocalUserNames: pickSet(pl.DirectoryServiceOrLocalUserNames, sl.DirectoryServiceOrLocalUserNames),
			DirectoryServiceUserGroupNames:   pickSet(pl.DirectoryServiceUserGroupNames, sl.DirectoryServiceUserGroupNames),
		},
		Exclusions: &ComputerScopeExclusionsModelNoIbeacons{
			ComputerIDs:                      pickSet(pe.ComputerIDs, se.ComputerIDs),
			ComputerGroupIDs:                 pickSet(pe.ComputerGroupIDs, se.ComputerGroupIDs),
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

// mergeMobileTargets overlays declared mobile-platform target categories onto
// the server's, applying all-flag precedence.
func mergeMobileTargets(plan, server MobileScopeTargetsModel) *MobileScopeTargetsModel {
	merged := &MobileScopeTargetsModel{
		AllMobileDevices:     PickAllFlag(plan.AllMobileDevices, server.AllMobileDevices, plan.MobileDeviceIDs, plan.MobileDeviceGroupIDs),
		AllJssUsers:          PickAllFlag(plan.AllJssUsers, server.AllJssUsers, plan.UserIDs, plan.UserGroupIDs),
		MobileDeviceIDs:      pickSet(plan.MobileDeviceIDs, server.MobileDeviceIDs),
		MobileDeviceGroupIDs: pickSet(plan.MobileDeviceGroupIDs, server.MobileDeviceGroupIDs),
		BuildingIDs:          pickSet(plan.BuildingIDs, server.BuildingIDs),
		DepartmentIDs:        pickSet(plan.DepartmentIDs, server.DepartmentIDs),
		UserIDs:              pickSet(plan.UserIDs, server.UserIDs),
		UserGroupIDs:         pickSet(plan.UserGroupIDs, server.UserGroupIDs),
	}
	if merged.AllMobileDevices.ValueBool() {
		merged.MobileDeviceIDs = EmptyStringSet()
		merged.MobileDeviceGroupIDs = EmptyStringSet()
	}
	if merged.AllJssUsers.ValueBool() {
		merged.UserIDs = EmptyStringSet()
		merged.UserGroupIDs = EmptyStringSet()
	}
	return merged
}

// MergeMobileScope overlays a declared iBeacon-bearing mobile scope onto the
// server's current scope. See MergeComputerScope for the contract.
func MergeMobileScope(plan, server *MobileScopeModel) *MobileScopeModel {
	if plan == nil {
		return nil
	}
	var srv MobileScopeModel
	if server != nil {
		srv = *server
	}
	pl, sl := derefOrZero(plan.Limitations), derefOrZero(srv.Limitations)
	pe, se := derefOrZero(plan.Exclusions), derefOrZero(srv.Exclusions)
	return &MobileScopeModel{
		Targets: mergeMobileTargets(plan.TargetsOrZero(), srv.TargetsOrZero()),
		Limitations: &MobileScopeLimitationsModel{
			NetworkSegmentIDs:                pickSet(pl.NetworkSegmentIDs, sl.NetworkSegmentIDs),
			IbeaconIDs:                       pickSet(pl.IbeaconIDs, sl.IbeaconIDs),
			DirectoryServiceOrLocalUserNames: pickSet(pl.DirectoryServiceOrLocalUserNames, sl.DirectoryServiceOrLocalUserNames),
			DirectoryServiceUserGroupNames:   pickSet(pl.DirectoryServiceUserGroupNames, sl.DirectoryServiceUserGroupNames),
		},
		Exclusions: &MobileScopeExclusionsModel{
			MobileDeviceIDs:                  pickSet(pe.MobileDeviceIDs, se.MobileDeviceIDs),
			MobileDeviceGroupIDs:             pickSet(pe.MobileDeviceGroupIDs, se.MobileDeviceGroupIDs),
			BuildingIDs:                      pickSet(pe.BuildingIDs, se.BuildingIDs),
			DepartmentIDs:                    pickSet(pe.DepartmentIDs, se.DepartmentIDs),
			UserIDs:                          pickSet(pe.UserIDs, se.UserIDs),
			UserGroupIDs:                     pickSet(pe.UserGroupIDs, se.UserGroupIDs),
			NetworkSegmentIDs:                pickSet(pe.NetworkSegmentIDs, se.NetworkSegmentIDs),
			IbeaconIDs:                       pickSet(pe.IbeaconIDs, se.IbeaconIDs),
			DirectoryServiceOrLocalUserNames: pickSet(pe.DirectoryServiceOrLocalUserNames, se.DirectoryServiceOrLocalUserNames),
			DirectoryServiceUserGroupNames:   pickSet(pe.DirectoryServiceUserGroupNames, se.DirectoryServiceUserGroupNames),
		},
	}
}

// MergeMobileScopeNoIbeacons is MergeMobileScope for the no-iBeacon model
// variant (mobile_device_app).
func MergeMobileScopeNoIbeacons(plan, server *MobileScopeModelNoIbeacons) *MobileScopeModelNoIbeacons {
	if plan == nil {
		return nil
	}
	var srv MobileScopeModelNoIbeacons
	if server != nil {
		srv = *server
	}
	pl, sl := derefOrZero(plan.Limitations), derefOrZero(srv.Limitations)
	pe, se := derefOrZero(plan.Exclusions), derefOrZero(srv.Exclusions)
	return &MobileScopeModelNoIbeacons{
		Targets: mergeMobileTargets(plan.TargetsOrZero(), srv.TargetsOrZero()),
		Limitations: &MobileScopeLimitationsModelNoIbeacons{
			NetworkSegmentIDs:                pickSet(pl.NetworkSegmentIDs, sl.NetworkSegmentIDs),
			DirectoryServiceOrLocalUserNames: pickSet(pl.DirectoryServiceOrLocalUserNames, sl.DirectoryServiceOrLocalUserNames),
			DirectoryServiceUserGroupNames:   pickSet(pl.DirectoryServiceUserGroupNames, sl.DirectoryServiceUserGroupNames),
		},
		Exclusions: &MobileScopeExclusionsModelNoIbeacons{
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

// MergeUserScope overlays a declared user-based scope (vpp_assignment /
// vpp_invitation) onto the server's current scope. See MergeComputerScope for
// the contract.
func MergeUserScope(plan, server *UserScopeModel) *UserScopeModel {
	if plan == nil {
		return nil
	}
	var srv UserScopeModel
	if server != nil {
		srv = *server
	}
	pt, st := plan.TargetsOrZero(), srv.TargetsOrZero()
	pl, sl := derefOrZero(plan.Limitations), derefOrZero(srv.Limitations)
	pe, se := derefOrZero(plan.Exclusions), derefOrZero(srv.Exclusions)
	targets := &UserScopeTargetsModel{
		AllJssUsers:     PickAllFlag(pt.AllJssUsers, st.AllJssUsers, pt.JssUserIDs, pt.JssUserGroupIDs),
		JssUserIDs:      pickSet(pt.JssUserIDs, st.JssUserIDs),
		JssUserGroupIDs: pickSet(pt.JssUserGroupIDs, st.JssUserGroupIDs),
	}
	if targets.AllJssUsers.ValueBool() {
		targets.JssUserIDs = EmptyStringSet()
		targets.JssUserGroupIDs = EmptyStringSet()
	}
	return &UserScopeModel{
		Targets: targets,
		Limitations: &UserScopeLimitationsModel{
			DirectoryServiceUserGroupNames: pickSet(pl.DirectoryServiceUserGroupNames, sl.DirectoryServiceUserGroupNames),
		},
		Exclusions: &UserScopeExclusionsModel{
			JssUserIDs:                     pickSet(pe.JssUserIDs, se.JssUserIDs),
			JssUserGroupIDs:                pickSet(pe.JssUserGroupIDs, se.JssUserGroupIDs),
			DirectoryServiceUserGroupNames: pickSet(pe.DirectoryServiceUserGroupNames, se.DirectoryServiceUserGroupNames),
		},
	}
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
