// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_enrollment_profile

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_mobile_device_enrollment_profile"

// enrollmentProfileNamespace is the strict uniqueness rule for mobile device
// enrollment profile names. Jamf Pro stores every variant of the name, so there is
// no server rule to mirror. Classic trims the XML text of every field on save, so
// the comparison trims both ends as well as folding case and accents: names a
// person reads as the same object are reported. The rule applies only when
// include_unenforced is set.
func enrollmentProfileNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.mobile-device-enrollment-profiles",
		Product:   "Jamf Pro",
		Enforced:  false,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "mobile device enrollment profile",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				list, err := client.ListMobileDeviceEnrollmentProfiles(ctx)
				if err != nil {
					return nil, err
				}
				if list == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(list.MobileDeviceEnrollmentProfiles))
				for _, p := range list.MobileDeviceEnrollmentProfiles {
					if p.ID != nil && p.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*p.ID), Key: *p.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a name that another mobile device enrollment profile already holds, when
// include_unenforced is set.
func (r *EnrollmentProfileResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: enrollmentProfileNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
