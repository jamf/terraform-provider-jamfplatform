// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package device_group

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_security_cloud_device_group"

// deviceGroupNamespace is the uniqueness rule for device group names. Jamf
// Security Cloud refuses a duplicate with 409 GROUP_ALREADY_EXISTS. The match is
// case-sensitive and accent-sensitive, and it trims leading and trailing spaces
// (wire-probed 2026-10-25). The built-in "Default Group" is listed without an id,
// so it cannot be imported and is left out.
func (r *DeviceGroupResource) deviceGroupNamespace() collision.Namespace {
	return collision.Namespace{
		ID:        "securitycloud.device-groups",
		Product:   "Jamf Security Cloud",
		Enforced:  true,
		Collation: collision.Collation{IgnoreLeadingSpace: true, IgnoreTrailingSpace: true},
		Sources: []collision.Source{{
			Noun:         "device group",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				groups, err := r.client.ListDeviceGroupsV2(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(groups.Groups))
				for _, g := range groups.Groups {
					if g.ID != "" {
						entries = append(entries, collision.Entry{ID: g.ID, Key: g.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// ModifyPlan reports a device group name that another device group already holds.
func (r *DeviceGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[DeviceGroupResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[DeviceGroupResourceModel]{{
			Namespace: r.deviceGroupNamespace(),
			Path:      path.Root("name"),
			Keys:      func(m *DeviceGroupResourceModel) []types.String { return []types.String{m.Name} },
		}},
	})
}
