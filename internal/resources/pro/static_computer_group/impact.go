// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
)

// A static computer group is a scopeable object: policies, profiles and apps are
// scoped to it, so changing who is in it changes what they apply to. Jamf Pro
// raises its own alert when an administrator saves such an edit, and the
// provider mirrors it at plan time.
//
// How much is knowable here differs from a smart group, and differs again with
// whether the configuration manages membership.
//
// A managed static group is the easy case: the next membership is the configured
// set, counted from the plan, and the current one is counted from state. Nothing
// is derived from criteria, so there is no undetermined case to explain.
//
// A configuration that does not manage membership knows neither count on an
// update, and that is the right answer rather than a gap: such a plan is not
// changing membership at all, so no alert belongs on it. That holds whether the
// attribute was never declared or has just been dropped, because the assignments
// key is mandatory on every static write and Update answers it by reading the
// current membership and sending it straight back. Handing membership back to
// Jamf Pro is the safest edit there is, and it must not be alerted on. A create
// is different.
// An unmanaged create still writes an empty assignment list, so the group
// reliably starts with no members, and the alert says so.
//
// A rename or a description edit leaves the two sets equal, so Changed stays
// false and no alert is raised.

// ModifyPlan reports the plan-time impact alert. Nothing else in this resource
// modifies the plan.
func (r *StaticComputerGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportMembershipImpact(ctx, req, resp)
}

// reportMembershipImpact emits the alert for a change to this group's
// membership. It runs for creates and destroys as well as updates, because a
// group entering or leaving management changes what everything scoped to it
// applies to.
func (r *StaticComputerGroupResource) reportMembershipImpact(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
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

	var plan, state StaticComputerGroupResourceModel
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

	resp.Diagnostics.Append(impact.ReportMembership(ctx, impact.MembershipRequest{
		Cache:      cache,
		Path:       path.Root("assigned_computer_ids"),
		Label:      groupLabel,
		Action:     action,
		Membership: membershipChange(plan.AssignedComputerIDs, state.AssignedComputerIDs, action),
	})...)
}

// membershipChange derives the alert's arithmetic from the planned and current
// member sets. Split out from reportMembershipImpact so the table it implements
// can be tested without a plan or a client.
//
// planned is meaningless on a delete and current is meaningless on a create, so
// each is consulted only where the action makes it real. A null planned set on
// an update is a third case: the configuration is not managing membership, so
// the write leaves it exactly as it is and neither count is reportable, however
// many members state happens to record from when it was managed. Null is the
// test for that and unknown is not — a set derived from another resource's
// attributes is unknown at plan time and is a real change, so it still alerts,
// with the current count and no next one.
func membershipChange(planned, current types.Set, action impact.Action) impact.Membership {
	m := impact.Membership{Noun: memberNoun}
	unmanagedUpdate := action == impact.ActionUpdate && planned.IsNull()

	if action != impact.ActionCreate && !unmanagedUpdate && !current.IsNull() && !current.IsUnknown() {
		m.Current = int64(len(current.Elements()))
		m.CurrentKnown = true
	}

	if action != impact.ActionDelete && !planned.IsUnknown() {
		switch {
		case !planned.IsNull():
			m.Next = int64(len(planned.Elements()))
			m.NextKnown = true
		case action == impact.ActionCreate:
			// A create that manages no membership still writes an empty
			// assignment list, so the new group reliably starts with none.
			m.NextKnown = true
		}
	}

	if action == impact.ActionUpdate {
		m.Changed = !unmanagedUpdate && !planned.Equal(current)
	}
	return m
}
