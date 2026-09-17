// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// A computer group is a scopeable object: policies, profiles and apps are scoped
// to it, so changing who is in it changes what those objects apply to. That
// knock-on effect is what the alert reports, and it is why the alert fires from
// this side as well as from the deployable side — a plan modifier cannot see a
// sibling resource's plan, so a policy scoped to this group has no way to know
// the group is changing.
//
// The membership after the change is never knowable here. Jamf Pro derives it by
// evaluating the criteria against inventory on its own schedule, so the alert
// says so rather than guessing, and it reports no current figure either: the
// group read carries no membership count and fetching one would cost a request
// per group on every plan.

// reportMembershipImpact emits the plan-time impact alert for a change to this
// group's membership. It runs for creates and destroys as well as updates,
// because a group entering or leaving management changes what everything scoped
// to it applies to.
func (r *SmartComputerGroupResource) reportMembershipImpact(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse, suppressed []criteria.CriterionModel) {
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

	var plan, state SmartComputerGroupResourceModel
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

	if len(suppressed) > 0 {
		plan.Criteria = suppressed
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
		Path:       path.Root("criteria"),
		Label:      groupLabel,
		Action:     action,
		Membership: membershipFor(plan, state),
	})...)
}

// membershipFor describes how this plan changes the group's membership.
//
// Neither figure is ever reported. The membership after the change is Jamf Pro's
// to derive from the criteria, so Undetermined always carries the reason; and the
// group read carries no membership count, so there is no current figure either
// and fetching one would cost a request per group on every plan.
func membershipFor(plan, state SmartComputerGroupResourceModel) impact.Membership {
	return impact.Membership{
		Noun:         progroups.MemberNoun(deviceType),
		Undetermined: impact.CriteriaUndetermined,
		Changed:      membershipChanging(plan, state),
	}
}

// membershipChanging reports whether this plan alters who is in the group.
//
// Two things can. The criteria decide membership outright, and they are compared
// field by field because an edited operator or value changes the result without
// changing how many criteria there are. The site constrains it: a group in a site
// only ever contains computers assigned to that same site, so moving the group
// between sites changes the membership even when the criteria are untouched.
//
// Deliberately not a change: a rename, and a description edit. Jamf Pro's own
// criteria impact alert fires on a membership edit and not on either of those.
func membershipChanging(plan, state SmartComputerGroupResourceModel) bool {
	return criteriaDiffer(plan.Criteria, state.Criteria) || !plan.SiteID.Equal(state.SiteID)
}

// criteriaDiffer reports whether two criteria lists describe different
// membership. priority is excluded: it is the element's own position, so it
// cannot differ without some other field differing too.
func criteriaDiffer(a, b []criteria.CriterionModel) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if !a[i].Name.Equal(b[i].Name) ||
			!a[i].SearchType.Equal(b[i].SearchType) ||
			!a[i].Value.Equal(b[i].Value) ||
			!a[i].AndOr.Equal(b[i].AndOr) ||
			!a[i].HasOpeningParenthesis.Equal(b[i].HasOpeningParenthesis) ||
			!a[i].HasClosingParenthesis.Equal(b[i].HasClosingParenthesis) {
			return true
		}
	}
	return false
}
