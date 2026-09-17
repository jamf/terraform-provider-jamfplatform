// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"context"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// buildGroupInput assembles a create or update body from the plan and an
// already-computed membership delta.
//
// Every scalar is emitted on every write. The three of them full-replace, so a
// body that leaves `groupDescription` out empties the description and one that
// leaves `siteId` out is refused outright on this endpoint.
//
// The assignments slice is emitted even when it is empty, and is never sent as
// JSON null: omitting the key answers 500 with an empty error list and applies
// nothing at all, including the scalars. An empty array is a no-op on membership,
// which is exactly what a write that does not manage membership wants.
func buildGroupInput(plan StaticMobileDeviceGroupResourceModel, assignments []pro.Assignment) *pro.StaticGroupAssignment {
	if assignments == nil {
		assignments = []pro.Assignment{}
	}
	return &pro.StaticGroupAssignment{
		GroupName:        plan.Name.ValueString(),
		GroupDescription: descriptionForWrite(plan.Description),
		SiteID:           progroups.SiteIDForWrite(plan.SiteID),
		Assignments:      &assignments,
	}
}

// descriptionForWrite returns the description to send, always non-nil.
//
// An unset description is sent as the empty string rather than omitted, because
// omission is indistinguishable from "clear it" on a full-replace scalar and the
// attribute is Optional+Computed: the value in the plan is the whole truth about
// what the group's description should be.
func descriptionForWrite(planned types.String) *string {
	out := ""
	if helpers.IsConfiguredValue(planned) {
		out = planned.ValueString()
	}
	return &out
}

// buildCreateAssignments turns a planned membership set into the create body's
// delta.
//
// A group being created holds nobody, so there is nothing to remove and the
// delta is one "add" per planned identifier. An unmanaged create sends the empty
// array: there is no prior membership for it to preserve.
func buildCreateAssignments(ctx context.Context, planned types.Set) ([]pro.Assignment, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !helpers.IsConfiguredValue(planned) {
		return []pro.Assignment{}, diags
	}
	ids, idDiags := helpers.SetToStringSlice(ctx, planned)
	diags.Append(idDiags...)
	if diags.HasError() {
		return nil, diags
	}
	return assignmentsFor(sortedUnique(ids), true), diags
}

// assignmentsForUpdate returns the membership delta that moves a group from the
// devices it currently holds to the devices the configuration asks for.
//
// The endpoint merges rather than replaces, so making the configuration
// authoritative is the caller's job: a device the plan names and the group does
// not hold is added, a device the group holds and the plan does not name is
// removed, and a device on both sides is left out of the body entirely. That is
// why an update managing membership has to read the current membership first;
// there is no body that says "these and only these".
//
// The result is ordered — additions first, each half sorted — so a request is
// reproducible and a test can assert on it. The endpoint does not care about
// order.
//
// An empty delta still returns an empty slice rather than nil, because the
// caller must emit the key regardless.
func assignmentsForUpdate(planned, current []string) []pro.Assignment {
	plannedSet := sortedUnique(planned)
	currentSet := sortedUnique(current)

	var additions, removals []string
	for _, id := range plannedSet {
		if !slices.Contains(currentSet, id) {
			additions = append(additions, id)
		}
	}
	for _, id := range currentSet {
		if !slices.Contains(plannedSet, id) {
			removals = append(removals, id)
		}
	}

	out := make([]pro.Assignment, 0, len(additions)+len(removals))
	out = append(out, assignmentsFor(additions, true)...)
	out = append(out, assignmentsFor(removals, false)...)
	return out
}

// assignmentsFor builds one delta entry per identifier with the given selection.
// A true entry adds the device to the group and a false one removes it.
func assignmentsFor(ids []string, selected bool) []pro.Assignment {
	out := make([]pro.Assignment, 0, len(ids))
	for _, id := range ids {
		deviceID := id
		chosen := selected
		out = append(out, pro.Assignment{MobileDeviceID: &deviceID, Selected: &chosen})
	}
	return out
}

// sortedUnique returns the identifiers sorted with duplicates and blanks
// dropped. A Terraform set cannot hold a duplicate, but the current membership
// read is a server collection and the delta must not tell Jamf Pro to add the
// same device twice.
func sortedUnique(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || slices.Contains(out, id) {
			continue
		}
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
