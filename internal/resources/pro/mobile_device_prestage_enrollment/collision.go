// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_prestage_enrollment

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_mobile_device_prestage_enrollment"

// mobileDevicePrestageNamespace is the uniqueness rule for mobile device prestage
// display names. Uniqueness: displayName, not enforced (duplicates stored,
// wire-probed 2026-10-25), checked only with include_unenforced. Jamf Pro stores
// every variant, so there is no rule to mirror; the strict comparison folds case,
// trailing spaces and accents, so names a person reads as the same are reported.
func mobileDevicePrestageNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.mobile-device-prestages",
		Product:   "Jamf Pro",
		KeyLabel:  "display name",
		Enforced:  false,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "mobile device prestage",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListMobileDevicePrestagesV3(ctx, nil)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(listed))
				for _, p := range listed {
					entries = append(entries, collision.Entry{ID: p.ID, Key: p.DisplayName})
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a display name that another mobile device prestage already holds.
func (r *MobileDevicePrestageEnrollmentResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: mobileDevicePrestageNamespace(r.client),
			Path:      path.Root("display_name"),
		}},
	})
}
