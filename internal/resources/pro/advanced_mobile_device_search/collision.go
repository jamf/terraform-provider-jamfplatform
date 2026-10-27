// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package advanced_mobile_device_search

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_advanced_mobile_device_search"

// advancedMobileDeviceSearchNamespace is the uniqueness rule for advanced mobile
// device search names. Jamf Pro refuses a duplicate with 400 DUPLICATE_FIELD and
// compares the name without regard to case only: a trailing space, a leading space
// and an accent each make a distinct name (wire-probed 2026-10-25).
func advancedMobileDeviceSearchNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.advanced-mobile-device-searches",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationCaseOnly,
		Sources: []collision.Source{{
			Noun:         "advanced mobile device search",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				list, err := client.ListAdvancedMobileDeviceSearchesV1(ctx)
				if err != nil {
					return nil, err
				}
				if list == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(list.Results))
				for _, s := range list.Results {
					if s.ID != nil {
						entries = append(entries, collision.Entry{ID: *s.ID, Key: s.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a name that another advanced mobile device search already holds.
func (r *AdvancedMobileDeviceSearchResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: advancedMobileDeviceSearchNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
