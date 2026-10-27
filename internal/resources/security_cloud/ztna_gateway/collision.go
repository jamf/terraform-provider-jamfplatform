// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_security_cloud_ztna_gateway"

// gatewayNameNamespace is the strict-only uniqueness rule for gateway names.
// Jamf Security Cloud stores two gateway with one name (wire-probed 2026-10-25, dedicated and IPsec forms alike),
// so the rule is Enforced false and applies only when include_unenforced is set.
// The comparison is collision.CollationFolded, the strictest comparison, because
// a gateway accepted next to one whose name differs only by case or accent still
// reads as a duplicate in the Jamf Security Cloud console.
func gatewayNameNamespace(client *securitycloud.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "securitycloud.ztna-gateway-names",
		Product:   "Jamf Security Cloud",
		KeyLabel:  "name",
		Enforced:  false,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "gateway",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				list, err := client.ListZtnaGatewaysV1(ctx)
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

// reportNameCollision reports a gateway name another one already holds, when
// include_unenforced is set.
func (r *GatewayResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[GatewayResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[GatewayResourceModel]{{
			Namespace: gatewayNameNamespace(r.client),
			Path:      path.Root("name"),
			Keys:      func(m *GatewayResourceModel) []types.String { return []types.String{m.Name} },
		}},
	})
}
