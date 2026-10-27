// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/groupcollision"
)

const resourceTypeName = "jamfplatform_pro_static_computer_group"

// reportNameCollision reports a group name that another computer group already
// holds. Smart groups, static groups and Platform device groups of type COMPUTER
// share one pool of names, so the namespace spans all of them.
func (r *StaticComputerGroupResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil || r.pd == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.pd.CollisionChecker(),
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: groupcollision.ComputerNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
