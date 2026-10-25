// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package site

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_site"

// siteNamespace is the uniqueness rule for site names. Jamf Pro refuses a duplicate with 409 Duplicate name and compares the name without regard to case,
// leading or trailing spaces or accents (wire-probed 2026-10-25).
func siteNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.sites",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "site",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListSites(ctx)
				if err != nil {
					return nil, err
				}
				if listed == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(listed.Sites))
				for _, item := range listed.Sites {
					if item.ID != nil && item.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*item.ID), Key: *item.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

var _ resource.ResourceWithModifyPlan = &SiteResource{}

// ModifyPlan reports a site name that another site already holds.
func (r *SiteResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	r.reportNameCollision(ctx, req, resp)
}

// reportNameCollision reports a site name that another site already holds.
func (r *SiteResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: siteNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
