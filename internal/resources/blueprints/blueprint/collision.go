// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_blueprints_blueprint"

// blueprintNamespace is the uniqueness rule for blueprint names. Uniqueness: name,
// not enforced (duplicates stored, wire-probed 2026-10-23 and 2026-10-25), checked
// only with include_unenforced. The service stores every variant, so there is no
// rule to mirror; the strict comparison folds case, trailing spaces and accents, so
// names a person reads as the same are reported.
func blueprintNamespace(client *blueprints.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "blueprints.blueprints",
		Product:   "Jamf Blueprints",
		Enforced:  false,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "blueprint",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListBlueprints(ctx, nil, "")
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(listed))
				for _, b := range listed {
					entries = append(entries, collision.Entry{ID: b.ID, Key: b.Name})
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a blueprint name that another blueprint already holds.
func (r *BlueprintResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: blueprintNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
