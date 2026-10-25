// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package file_share_distribution_point

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_file_share_distribution_point"

// fileShareDistributionPointNamespace is the uniqueness rule for distribution
// point names. Jamf Pro refuses a duplicate with 400 DUPLICATE_FIELD and trims the
// name on save, so the comparison ignores case, leading and trailing spaces and
// accents (wire-probed 2026-10-25).
func fileShareDistributionPointNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.distribution-points",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "file share distribution point",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				points, err := client.ListDistributionPointsV1(ctx, nil, "")
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(points))
				for _, p := range points {
					if p.ID != nil {
						entries = append(entries, collision.Entry{ID: *p.ID, Key: p.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a name that another distribution point already holds.
func (r *FileShareDistributionPointResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: fileShareDistributionPointNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
