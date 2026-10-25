// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package department

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_department"

// departmentNamespace is the uniqueness rule for department names. Jamf Pro refuses a duplicate with 400 DUPLICATE_FIELD and compares the name without regard to case,
// leading or trailing spaces or accents (wire-probed 2026-10-25).
func departmentNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.departments",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "department",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				items, err := client.ListDepartmentsV1(ctx, nil, "")
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

var _ resource.ResourceWithModifyPlan = &DepartmentResource{}

// ModifyPlan reports a department name that another department already holds.
func (r *DepartmentResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
}

// reportNameCollision reports a department name that another department already holds.
func (r *DepartmentResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: departmentNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
