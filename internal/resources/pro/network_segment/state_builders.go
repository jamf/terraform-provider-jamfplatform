// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package network_segment

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignNetworkSegmentResourceModel populates a resource model from a NetworkSegment
// response. state.ID is only overwritten when the API ID is non-nil so a transient
// GET that drops the ID does not clobber the value persisted from Create. Optional
// writable string fields (building, department) and the override bools are reconciled
// through helpers.ReconcileOptional*Pointer so the explicit-null vs API-empty
// distinction the user set in config is preserved across refreshes. Computed-only
// server-derived fields (distribution_point, distribution_server, swu_server, url)
// are populated unconditionally with the API value (or null when absent).
// adopt selects between the two read contexts, the way the licensed-software and
// mobile-device-enrollment-profile builders do. A CRUD read reconciles: an
// Optional+Computed value the practitioner never authored stays null, so a
// refresh does not snap it to the server default. Config generation has no plan
// to reconcile against — the list resource builds a fresh model — so the server
// value is authoritative and a value-carrying flag has to survive into the
// exported configuration. Reconciling there left override_buildings and
// override_departments null on every generated network segment even though the
// endpoint returns them.
func assignNetworkSegmentResourceModel(state *NetworkSegmentResourceModel, s *proclassic.NetworkSegment, adopt bool) {
	if s == nil {
		return
	}
	if s.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(s.ID)
	}
	if s.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(s.Name)
	}
	if s.StartingAddress != nil {
		state.StartingAddress = helpers.StringPointerValueOrNull(s.StartingAddress)
	}
	if s.EndingAddress != nil {
		state.EndingAddress = helpers.StringPointerValueOrNull(s.EndingAddress)
	}
	// The two strings need no adopt switch: ReconcileOptionalStringPointer
	// already returns a wire value the caller never authored, which is why
	// building and department survived into generated config while the two bools
	// beside them did not.
	state.Building = helpers.ReconcileOptionalStringPointer(s.Building, state.Building)
	state.Department = helpers.ReconcileOptionalStringPointer(s.Department, state.Department)
	state.OverrideBuildings = helpers.ReconcileOrAdoptBoolPointer(s.OverrideBuildings, state.OverrideBuildings, adopt)
	state.OverrideDepartments = helpers.ReconcileOrAdoptBoolPointer(s.OverrideDepartments, state.OverrideDepartments, adopt)
	state.DistributionPoint = helpers.StringPointerValueOrNull(s.DistributionPoint)
	state.DistributionServer = helpers.StringPointerValueOrNull(s.DistributionServer)
	state.SwuServer = helpers.StringPointerValueOrNull(s.SwuServer)
	state.URL = helpers.StringPointerValueOrNull(s.URL)
}

// assignNetworkSegmentDataSourceModel populates a data source model from a NetworkSegment
// response. Symmetric with assignNetworkSegmentResourceModel: nil API fields preserve the
// caller-supplied selector (id or name) — DS accepts either as input so silently nulling
// the non-supplied one if the SDK omits it would be hostile.
func assignNetworkSegmentDataSourceModel(state *NetworkSegmentDataSourceModel, s *proclassic.NetworkSegment) {
	if s == nil {
		return
	}
	if s.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(s.ID)
	}
	if s.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(s.Name)
	}
	state.StartingAddress = helpers.StringPointerValueOrNull(s.StartingAddress)
	state.EndingAddress = helpers.StringPointerValueOrNull(s.EndingAddress)
	state.Building = helpers.StringPointerValueOrNull(s.Building)
	state.Department = helpers.StringPointerValueOrNull(s.Department)
	state.OverrideBuildings = helpers.BoolPointerValueOrNull(s.OverrideBuildings)
	state.OverrideDepartments = helpers.BoolPointerValueOrNull(s.OverrideDepartments)
	state.DistributionPoint = helpers.StringPointerValueOrNull(s.DistributionPoint)
	state.DistributionServer = helpers.StringPointerValueOrNull(s.DistributionServer)
	state.SwuServer = helpers.StringPointerValueOrNull(s.SwuServer)
	state.URL = helpers.StringPointerValueOrNull(s.URL)
}
