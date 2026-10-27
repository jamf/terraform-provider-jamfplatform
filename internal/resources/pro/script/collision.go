// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package script

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_script"

// scriptNamespace is the uniqueness rule for script names. Jamf Pro refuses a
// duplicate with 400 DUPLICATE_FIELD and folds case, trailing spaces and accents.
// A leading space keeps two names apart.
func scriptNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.scripts",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "script",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				scripts, err := client.ListScriptsV1(ctx, nil, "")
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(scripts))
				for _, s := range scripts {
					if s.ID != nil {
						entries = append(entries, collision.Entry{ID: *s.ID, Key: s.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a script name that another script already holds.
func (r *ScriptResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[ScriptResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[ScriptResourceModel]{{
			Namespace: scriptNamespace(r.client),
			Path:      path.Root("name"),
			Keys:      func(m *ScriptResourceModel) []types.String { return []types.String{m.Name} },
		}},
	})
}
