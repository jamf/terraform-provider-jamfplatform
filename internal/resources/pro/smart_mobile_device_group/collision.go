// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/groupcollision"
)

const resourceTypeName = "jamfplatform_pro_smart_mobile_device_group"

// reportNameCollision reports a group name that another mobile device group
// already holds. Smart groups, static groups and Platform device groups of type
// MOBILE share one pool of names, so the namespace spans all of them.
func (r *SmartMobileDeviceGroupResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
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

// withCreateConflictHint appends the duplicate-create explanation to each
// unanchored error in diags. The create error path is shared with the other
// group resources through progroups, which words a duplicate name as an import
// suggestion; a sibling resource declaring the same name is the likelier cause
// inside one configuration, and the hint says so. Anchored diagnostics are left
// as they are, since rebuilding one would drop its attribute path.
func (r *SmartMobileDeviceGroupResource) withCreateConflictHint(diags diag.Diagnostics, createErr error) diag.Diagnostics {
	hint := r.pd.CollisionChecker().CreateConflictHint(createErr, resourceTypeName)
	if hint == "" {
		return diags
	}
	out := make(diag.Diagnostics, 0, len(diags))
	for _, d := range diags {
		if _, anchored := d.(diag.DiagnosticWithPath); !anchored && d.Severity() == diag.SeverityError {
			d = diag.NewErrorDiagnostic(d.Summary(), d.Detail()+hint)
		}
		out = append(out, d)
	}
	return out
}
