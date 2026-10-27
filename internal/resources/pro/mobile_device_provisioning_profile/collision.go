// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_provisioning_profile

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_mobile_device_provisioning_profile"

// provisioningProfileNamespace is the uniqueness rule for provisioning profile
// names. Jamf Pro refuses a duplicate with 409, trims the name on both ends and
// compares without regard to case, with accents significant. Wire-probed
// 2026-10-25 on 11.33.1. The listing is a sweep, so a tenant on 11.32.0, whose
// Classic list left out names collating after "s", can miss a collision. The
// apply still fails there as it does without the check.
func provisioningProfileNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.mobile-device-provisioning-profiles",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationAccentSensitive,
		Sources: []collision.Source{{
			Noun:         "mobile device provisioning profile",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListMobileDeviceProvisioningProfiles(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(listed.MobileDeviceProvisioningProfiles))
				for _, p := range listed.MobileDeviceProvisioningProfiles {
					if p.ID != nil && p.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*p.ID), Key: *p.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// ModifyPlan reports a provisioning profile name that another profile already holds.
func (r *ProvisioningProfileResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: provisioningProfileNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
