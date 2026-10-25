// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package restricted_software

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_restricted_software"

// restrictedSoftwareNamespace is the uniqueness rule for restricted software record names. Jamf Pro refuses a duplicate with 409 Duplicate name and compares the name without regard to case or
// leading and trailing spaces. Accents keep two names apart (wire-probed 2026-10-25).
func restrictedSoftwareNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.restricted-software",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationAccentSensitive,
		Sources: []collision.Source{{
			Noun:         "restricted software record",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListRestrictedSoftware(ctx)
				if err != nil {
					return nil, err
				}
				if listed == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(listed.RestrictedSoftwareTitles))
				for _, item := range listed.RestrictedSoftwareTitles {
					if item.ID != nil && item.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*item.ID), Key: *item.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a restricted software record name that another restricted software record already holds.
func (r *RestrictedSoftwareResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: restrictedSoftwareNamespace(r.client),
			Path:      path.Root("general").AtName("name"),
		}},
	})
}
