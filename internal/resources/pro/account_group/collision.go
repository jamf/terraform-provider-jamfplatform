// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package account_group

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_account_group"

// accountGroupNamespace is the uniqueness rule for account group names. Jamf Pro refuses a
// duplicate with 409, and the Classic API trims the name and compares it without regard to case or surrounding spaces but with accents significant.
func accountGroupNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.account-groups",
		Product:   "Jamf Pro",
		KeyLabel:  "display name",
		Enforced:  true,
		Collation: collision.CollationAccentSensitive,
		Sources: []collision.Source{{
			Noun:         "account group",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListAccounts(ctx)
				if err != nil {
					return nil, err
				}
				var entries []collision.Entry
				if listed == nil || listed.Groups == nil || listed.Groups.Group == nil {
					return entries, nil
				}
				for _, g := range *listed.Groups.Group {
					if g.ID != nil && g.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*g.ID), Key: *g.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a account group name that another account group already holds.
func (r *AccountGroupResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: accountGroupNamespace(r.client),
			Path:      path.Root("display_name"),
		}},
	})
}
