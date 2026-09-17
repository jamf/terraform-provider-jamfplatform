// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/criteria"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/impact"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// A smart mobile device group is a scopeable object: policies, profiles and apps
// can all be scoped to it, so editing the criteria changes what those objects
// apply to. That knock-on effect is what the alert is about, not the group.
//
// Nothing here can report a post-change count. Jamf Pro derives the membership
// by evaluating the criteria against inventory after the write lands, so the
// configuration does not decide it and the alert says so rather than guessing.
// The count before the change is not available either, because this resource
// deliberately keeps no membership count in state.
//
// reportMembershipImpact emits the plan-time impact alert for a change to this
// group's criteria. It runs for creates and destroys as well as updates.
//
// A criteria list Terraform cannot resolve yet still raises the alert. The
// planned list cannot be shown equal to the stored one, so the change is taken
// as real rather than dropped — the alternative is the alert silently vanishing
// on exactly the plans where an operator most wants it, and it is advisory
// either way. It reads the attribute through criteriaAtPlanTime instead of
// decoding the resource model for the reason recorded on that helper.
func (r *SmartMobileDeviceGroupResource) reportMembershipImpact(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse, suppressed []criteria.CriterionModel) {
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

	var planned, prior []criteria.CriterionModel
	plannedSettled := true
	if !destroying {
		models, settled, diags := criteriaAtPlanTime(ctx, req.Plan)
		if diags.HasError() {
			return
		}
		planned, plannedSettled = models, settled
	}
	if !creating {
		models, settled, diags := criteriaAtPlanTime(ctx, req.State)
		if diags.HasError() || !settled {
			return
		}
		prior = models
	}

	if len(suppressed) > 0 {
		planned = suppressed
	}

	action := impact.ActionUpdate
	switch {
	case creating:
		action = impact.ActionCreate
	case destroying:
		action = impact.ActionDelete
	}

	resp.Diagnostics.Append(impact.ReportMembership(ctx, impact.MembershipRequest{
		Cache:  cache,
		Path:   path.Root("criteria"),
		Label:  progroups.Label(progroups.DeviceTypeMobile, progroups.GroupKindSmart),
		Action: action,
		Membership: impact.Membership{
			Noun:         progroups.MemberNoun(progroups.DeviceTypeMobile),
			Undetermined: impact.CriteriaUndetermined,
			Changed:      !plannedSettled || criterionListsDiffer(planned, prior),
		},
	})...)
}
