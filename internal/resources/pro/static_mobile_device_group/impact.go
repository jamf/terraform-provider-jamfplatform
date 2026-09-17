// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// A static mobile device group is a scopeable object: profiles, apps and books
// can be scoped to it, so a device joining or leaving starts or stops receiving
// everything scoped to the group. That knock-on effect is what the alert is
// about.
//
// Unlike a smart group, a static group's membership after the change is decided
// by the configuration rather than by Jamf Pro, so the alert can report a
// figure instead of explaining why it cannot. The exception is a group whose
// membership this configuration does not manage: nothing about the planned
// change tells us who will be in it, so only the current count is reported, and
// only where there is a reason to report one at all.

// reportMembershipImpact emits the plan-time impact alert for a change to this
// group's membership. It runs for creates and destroys as well as updates,
// because a group entering or leaving management changes what everything scoped
// to it applies to.
func (r *StaticMobileDeviceGroupResource) reportMembershipImpact(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.pd == nil {
		return
	}
	cache := r.pd.ImpactCache()
	if !cache.Enabled() {
		return
	}
	creating := req.State.Raw.IsNull()
	destroying := req.Plan.Raw.IsNull()
	if creating && destroying {
		return
	}

	var plan, state StaticMobileDeviceGroupResourceModel
	if !creating {
		if diags := req.State.Get(ctx, &state); diags.HasError() {
			return
		}
	}
	if !destroying {
		if diags := req.Plan.Get(ctx, &plan); diags.HasError() {
			return
		}
	}

	action := impact.ActionUpdate
	switch {
	case creating:
		action = impact.ActionCreate
	case destroying:
		action = impact.ActionDelete
	}

	m := impact.Membership{Noun: progroups.MemberNoun(deviceType)}
	if !creating && !state.MemberCount.IsNull() && !state.MemberCount.IsUnknown() {
		m.Current = state.MemberCount.ValueInt64()
		m.CurrentKnown = true
	}
	if !destroying && isKnownSet(plan.AssignedMobileDeviceIDs) {
		m.Next = int64(len(plan.AssignedMobileDeviceIDs.Elements()))
		m.NextKnown = true
	}
	m.Changed = membershipChanged(plan.AssignedMobileDeviceIDs, state.AssignedMobileDeviceIDs)

	resp.Diagnostics.Append(impact.ReportMembership(ctx, impact.MembershipRequest{
		Cache:      cache,
		Path:       membersPath,
		Label:      progroups.Label(deviceType, groupKind),
		Action:     action,
		Membership: m,
	})...)
}

// membershipChanged reports whether the plan alters who is in the group. A
// rename or a description edit leaves the membership set untouched on both
// sides and must not raise an alert.
//
// A group whose membership the configuration does not manage compares null
// against null and so reads as unchanged, which is right: the write such a plan
// issues is a no-op on membership.
func membershipChanged(planned, current types.Set) bool {
	return !planned.Equal(current)
}

// isKnownSet reports whether a set carries values to count. A configuration
// deriving the membership from another resource's attributes leaves it unknown
// at plan time, and an unknown count is not a count.
func isKnownSet(value types.Set) bool {
	return !value.IsNull() && !value.IsUnknown()
}
