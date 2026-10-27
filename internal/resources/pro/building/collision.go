// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package building

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_building"

// buildingNamespace is the uniqueness rule for building names. Jamf Pro refuses a duplicate with 400 DUPLICATE_FIELD and compares the name without regard to case,
// trailing spaces or accents. A leading space keeps two names apart (wire-probed 2026-10-25).
func buildingNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.buildings",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "building",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				items, err := client.ListBuildingsV1(ctx, nil, "")
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(items))
				for _, item := range items {
					if item.ID != nil {
						entries = append(entries, collision.Entry{ID: *item.ID, Key: item.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

var _ resource.ResourceWithModifyPlan = &BuildingResource{}

// ModifyPlan reports a building name that another building already holds.
func (r *BuildingResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
}

// reportNameCollision reports a building name that another building already holds.
func (r *BuildingResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: buildingNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
