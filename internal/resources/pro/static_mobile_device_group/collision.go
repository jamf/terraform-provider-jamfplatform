// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/groupcollision"
)

const resourceTypeName = "jamfplatform_pro_static_mobile_device_group"

// reportNameCollision reports a group name that another mobile device group
// already holds. Smart groups, static groups and Platform device groups of type
// MOBILE share one pool of names, so the namespace spans all of them.
func (r *StaticMobileDeviceGroupResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil || r.pd == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.pd.CollisionChecker(),
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: groupcollision.MobileNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}

// withConflictHint appends the create-conflict hint to each error in diags when
// err is a duplicate refusal. Two new groups declared with one name both pass the
// plan-time check, and the second is then refused at create, so the hint names
// that cause where the bare refusal does not.
func (r *StaticMobileDeviceGroupResource) withConflictHint(diags diag.Diagnostics, err error) diag.Diagnostics {
	hint := r.pd.CollisionChecker().CreateConflictHint(err, resourceTypeName)
	if hint == "" {
		return diags
	}
	var out diag.Diagnostics
	for _, d := range diags {
		if d.Severity() != diag.SeverityError {
			out.Append(d)
			continue
		}
		out.AddError(d.Summary(), d.Detail()+hint)
	}
	return out
}
