// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_security_cloud_ztna_app"

// ztnaAppNamespace is the uniqueness rule for application hostnames. A hostname
// belongs to one application across the tenant and Jamf Security Cloud refuses a
// second with 409 HOSTNAME_CONFLICT; the application name is not a key. The match
// ignores case and nothing else, so an accented hostname is a different one
// (wire-probed 2026-10-25). A hostname with a trailing space is refused as invalid
// before it can collide.
func ztnaAppNamespace(client *securitycloud.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "securitycloud.ztna-app-hostnames",
		Product:   "Jamf Security Cloud",
		KeyLabel:  "hostname",
		Enforced:  true,
		Collation: collision.Collation{IgnoreCase: true},
		Sources: []collision.Source{{
			Noun:         "access policy application",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				apps, err := client.ListZtnaAppsV1(ctx)
				if err != nil {
					return nil, err
				}
				var entries []collision.Entry
				for _, a := range apps {
					for _, h := range a.Hostnames {
						entries = append(entries, collision.Entry{ID: a.ID, Key: h})
					}
				}
				return entries, nil
			},
		}},
	}
}

// ModifyPlan reports a hostname that another application already holds.
func (r *ZtnaAppResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[ZtnaAppResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[ZtnaAppResourceModel]{{
			Namespace: ztnaAppNamespace(r.client),
			Path:      path.Root("hostnames"),
			Keys:      func(m *ZtnaAppResourceModel) []types.String { return collision.SetKeys(m.Hostnames) },
		}},
	})
}
