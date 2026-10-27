// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ibeacon

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_ibeacon"

// ibeaconNamespace is the uniqueness rule for iBeacon names. Jamf Pro refuses a
// duplicate with 409, and the Classic API trims the name and compares it without regard to case, leading and trailing spaces, or accents.
func ibeaconNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.ibeacons",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "iBeacon",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListIBeacons(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(listed.Ibeacons))
				for _, item := range listed.Ibeacons {
					if item.ID != nil && item.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*item.ID), Key: *item.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a iBeacon name that another iBeacon already holds.
func (r *IbeaconResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: ibeaconNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}

var _ resource.ResourceWithModifyPlan = &IbeaconResource{}

// ModifyPlan reports a iBeacon name that another iBeacon already holds.
func (r *IbeaconResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
}
