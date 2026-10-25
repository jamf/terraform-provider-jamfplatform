// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_mac_app_store_app"

// macAppNamespace is the uniqueness rule for Mac App Store app names. Jamf Pro
// refuses a duplicate with 409 and compares the name without regard to case,
// surrounding spaces or accents. The bundle ID and the URL are not keys
// (wire-probed 2026-10-25).
func macAppNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.mac-applications",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "Mac App Store app",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				list, err := client.ListMacApplications(ctx)
				if err != nil {
					return nil, err
				}
				if list == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(list.MacApplications))
				for _, a := range list.MacApplications {
					if a.ID != nil && a.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*a.ID), Key: *a.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a name that another Mac App Store app already holds.
func (r *MacAppResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: macAppNamespace(r.client),
			Path:      path.Root("general").AtName("name"),
		}},
	})
}
