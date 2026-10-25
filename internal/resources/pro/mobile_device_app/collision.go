// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_mobile_device_app"

// mobileAppNamespace is the strict uniqueness rule for mobile device app names.
// Jamf Pro stores every variant of the name, so there is no server rule to mirror.
// Classic trims the XML text of every field on save, so the comparison trims both
// ends as well as folding case and accents: names a person reads as the same object
// are reported. The rule applies only when include_unenforced is set.
func mobileAppNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.mobile-device-apps",
		Product:   "Jamf Pro",
		Enforced:  false,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "mobile device app",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				list, err := client.ListMobileDeviceApplications(ctx)
				if err != nil {
					return nil, err
				}
				if list == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(list.MobileDeviceApplications))
				for _, a := range list.MobileDeviceApplications {
					if a.ID != nil && a.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*a.ID), Key: *a.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a name that another mobile device app already holds, when
// include_unenforced is set.
func (r *MobileAppResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: mobileAppNamespace(r.client),
			Path:      path.Root("general").AtName("name"),
		}},
	})
}
