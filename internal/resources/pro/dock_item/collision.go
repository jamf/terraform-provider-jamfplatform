// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dock_item

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_dock_item"

// dockItemNamespace is the uniqueness rule for Dock item names. Jamf Pro refuses a duplicate with 409 Duplicate name and compares the name without regard to case,
// leading or trailing spaces or accents (wire-probed 2026-10-25).
func dockItemNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.dock-items",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "Dock item",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListDockItems(ctx)
				if err != nil {
					return nil, err
				}
				if listed == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(listed.DockItems))
				for _, item := range listed.DockItems {
					if item.ID != nil && item.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*item.ID), Key: *item.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a Dock item name that another Dock item already holds.
func (r *DockItemResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: dockItemNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
