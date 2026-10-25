// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_user_search

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_advanced_user_search"

// advancedUserSearchNamespace is the uniqueness rule for advanced user search
// names. Jamf Pro refuses a duplicate with 409. Classic trims the name on save and
// compares it without regard to case, but an accent keeps two names apart
// (wire-probed 2026-10-25).
func advancedUserSearchNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.advanced-user-searches",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationAccentSensitive,
		Sources: []collision.Source{{
			Noun:         "advanced user search",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				list, err := client.ListAdvancedUserSearches(ctx)
				if err != nil {
					return nil, err
				}
				if list == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(list.AdvancedUserSearches))
				for _, s := range list.AdvancedUserSearches {
					if s.ID != nil && s.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*s.ID), Key: *s.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a name that another advanced user search already holds.
func (r *AdvancedUserSearchResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: advancedUserSearchNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
