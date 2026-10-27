// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package category

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_category"

// categoryNamespace is the uniqueness rule for category names. Jamf Pro refuses a
// duplicate with 400 DUPLICATE_FIELD and folds case, trailing spaces and accents.
func categoryNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.categories",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "category",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				cats, err := client.ListCategoriesV1(ctx, nil, "")
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(cats))
				for _, c := range cats {
					if c.ID != nil {
						entries = append(entries, collision.Entry{ID: *c.ID, Key: c.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// ModifyPlan reports a category name that another category already holds.
func (r *CategoryResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[CategoryResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[CategoryResourceModel]{{
			Namespace: categoryNamespace(r.client),
			Path:      path.Root("name"),
			Keys:      func(m *CategoryResourceModel) []types.String { return []types.String{m.Name} },
		}},
	})
}
