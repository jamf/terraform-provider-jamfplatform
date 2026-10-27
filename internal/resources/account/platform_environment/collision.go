// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_account_platform_environment"

// platformEnvironmentNamespace is the strict-only uniqueness rule for platform
// environment names. Jamf Account stores every duplicate: the same name, an upper
// case variant, a name differing only by an accent and a name with surrounding
// spaces were all created side by side (wire-probed 2026-10-25). It trims
// surrounding spaces on save, so the comparison trims them too, and it uses
// CollationFoldedTrimmed because no server rule exists to mirror and a person
// reads "Prod" and "prod" as one name.
func platformEnvironmentNamespace(client *account.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "account.platform-environments",
		Product:   "Jamf Account",
		Enforced:  false,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "platform environment",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				environments, err := client.ListEnvironments(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(environments))
				for _, e := range environments {
					entries = append(entries, collision.Entry{ID: e.ID, Key: e.Name})
				}
				return entries, nil
			},
		}},
	}
}

// ModifyPlan reports an environment name that another environment already holds,
// when include_unenforced is set.
func (r *PlatformEnvironmentResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[PlatformEnvironmentResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[PlatformEnvironmentResourceModel]{{
			Namespace: platformEnvironmentNamespace(r.client),
			Path:      path.Root("name"),
			Keys:      func(m *PlatformEnvironmentResourceModel) []types.String { return []types.String{m.Name} },
		}},
	})
}
