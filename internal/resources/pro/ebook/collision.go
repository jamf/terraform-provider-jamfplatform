// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_ebook"

// ebookNamespace is the uniqueness rule for ebook names. Jamf Pro refuses a duplicate with 409 Duplicate name and compares the name without regard to case or
// leading and trailing spaces. Accents keep two names apart (wire-probed 2026-10-25). An ebook delete
// completes in the background, so a name reused straight after a delete is reported until the old
// ebook is gone, which is also when Jamf Pro would refuse the write.
func ebookNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.ebooks",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationAccentSensitive,
		Sources: []collision.Source{{
			Noun:         "ebook",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListEbooks(ctx)
				if err != nil {
					return nil, err
				}
				if listed == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(listed.Ebooks))
				for _, item := range listed.Ebooks {
					if item.ID != nil && item.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*item.ID), Key: *item.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a ebook name that another ebook already holds.
func (r *EbookResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: ebookNamespace(r.client),
			Path:      path.Root("general").AtName("name"),
		}},
	})
}
