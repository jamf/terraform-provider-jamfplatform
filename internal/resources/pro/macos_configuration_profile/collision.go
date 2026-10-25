// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_configuration_profile

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_macos_configuration_profile"

// profileNamespace is the uniqueness rule for macOS configuration profile names.
// Jamf Pro refuses a duplicate with 409 Duplicate name, trims a name on save and
// folds case and accents. macOS and mobile device profiles hold separate pools,
// wire-probed 2026-10-25.
func profileNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.macos-configuration-profiles",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "macOS configuration profile",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				list, err := client.ListOSXConfigurationProfiles(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(list.OsXConfigurationProfiles))
				for _, p := range list.OsXConfigurationProfiles {
					if p.ID != nil && p.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*p.ID), Key: *p.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a profile name that another macOS configuration
// profile already holds.
func (r *Resource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[ResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[ResourceModel]{{
			Namespace: profileNamespace(r.client),
			Path:      path.Root("general").AtName("name"),
			Keys: func(m *ResourceModel) []types.String {
				if m.General == nil {
					return nil
				}
				return []types.String{m.General.Name}
			},
		}},
	})
}
