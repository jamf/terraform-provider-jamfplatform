// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package network_segment

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_network_segment"

// networkSegmentNamespace is the uniqueness rule for network segment names. Jamf Pro refuses a duplicate with 409 Duplicate name and compares the name without regard to case or
// leading and trailing spaces. Accents keep two names apart (wire-probed 2026-10-25).
func networkSegmentNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.network-segments",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationAccentSensitive,
		Sources: []collision.Source{{
			Noun:         "network segment",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListNetworkSegments(ctx)
				if err != nil {
					return nil, err
				}
				if listed == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(listed.NetworkSegments))
				for _, item := range listed.NetworkSegments {
					if item.ID != nil && item.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*item.ID), Key: *item.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

var _ resource.ResourceWithModifyPlan = &NetworkSegmentResource{}

// ModifyPlan reports a network segment name that another network segment already holds.
func (r *NetworkSegmentResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
}

// reportNameCollision reports a network segment name that another network segment already holds.
func (r *NetworkSegmentResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: networkSegmentNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
