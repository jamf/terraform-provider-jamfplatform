// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/groupcollision"
)

// deviceTypeComputer and deviceTypeMobile are the Terraform-facing device_type
// values, which are the lower-case spellings of the wire values.
const (
	deviceTypeComputer = "computer"
	deviceTypeMobile   = "mobile"
)

// reportNameCollision reports a group name that another group of the same device
// type already holds. Platform device groups share one pool of names per device
// type with the Jamf Pro smart and static groups, so a name taken in the Jamf Pro
// interface refuses the same name here. Uniqueness: name per device type,
// enforced (422 UNPROCESSABLE_CONTENT), compared as the pool's Jamf Pro groups are
// (see internal/common/groupcollision).
//
// The device type is read from the plan, so the rule is chosen per plan. An
// unknown device type skips the check until the apply-time re-plan knows it.
func (r *DeviceGroupResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.proClient == nil || r.pd == nil || req.Plan.Raw.IsNull() {
		return
	}
	var deviceType types.String
	if diags := req.Plan.GetAttribute(ctx, path.Root("device_type"), &deviceType); diags.HasError() || deviceType.IsNull() || deviceType.IsUnknown() {
		return
	}

	var ns collision.Namespace
	switch deviceType.ValueString() {
	case deviceTypeComputer:
		ns = groupcollision.ComputerNamespace(r.proClient)
	case deviceTypeMobile:
		ns = groupcollision.MobileNamespace(r.proClient)
	default:
		return
	}

	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.pd.CollisionChecker(),
		ResourceType: groupcollision.DeviceGroupResourceType,
		Rules:        []collision.AttributeRule{{Namespace: ns, Path: path.Root("name")}},
	})
}
