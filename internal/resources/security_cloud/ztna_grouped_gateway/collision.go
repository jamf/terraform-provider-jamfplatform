// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_security_cloud_ztna_grouped_gateway"

// groupedGatewayNameNamespace is the strict-only uniqueness rule for grouped gateway names.
// Jamf Security Cloud stores two grouped gateway with one name (wire-probed 2026-10-25),
// so the rule is Enforced false and applies only when include_unenforced is set.
// The comparison is collision.CollationFolded, the strictest comparison, because
// a grouped gateway accepted next to one whose name differs only by case or accent still
// reads as a duplicate in the Jamf Security Cloud console.
func groupedGatewayNameNamespace(client *securitycloud.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "securitycloud.ztna-grouped-gateway-names",
		Product:   "Jamf Security Cloud",
		KeyLabel:  "name",
		Enforced:  false,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "grouped gateway",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				list, err := client.ListZtnaGroupedGatewaysV1(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(list.Results))
				for _, g := range list.Results {
					entries = append(entries, collision.Entry{ID: g.ID, Key: g.Name})
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a grouped gateway name another one already holds, when
// include_unenforced is set.
func (r *GroupedGatewayResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[GroupedGatewayResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[GroupedGatewayResourceModel]{{
			Namespace: groupedGatewayNameNamespace(r.client),
			Path:      path.Root("name"),
			Keys:      func(m *GroupedGatewayResourceModel) []types.String { return []types.String{m.Name} },
		}},
	})
}
