// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_computer_search

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_advanced_computer_search"

// advancedComputerSearchNamespace is the uniqueness rule for advanced computer search
// names. Uniqueness: name, not enforced (duplicates stored, wire-probed 2026-10-23),
// checked only with include_unenforced. Jamf Pro stores every variant, so there is no
// rule to mirror; the strict comparison folds case, accents and leading and trailing
// spaces (the Classic API trims them on save), so names a person reads as the same
// are reported.
func advancedComputerSearchNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.advanced-computer-searches",
		Product:   "Jamf Pro",
		Enforced:  false,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "advanced computer search",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListAdvancedComputerSearches(ctx)
				if err != nil {
					return nil, err
				}
				if listed == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(listed.AdvancedComputerSearches))
				for _, s := range listed.AdvancedComputerSearches {
					if s.ID != nil && s.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*s.ID), Key: *s.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a search name that another advanced computer search already holds.
func (r *AdvancedComputerSearchResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: advancedComputerSearchNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
