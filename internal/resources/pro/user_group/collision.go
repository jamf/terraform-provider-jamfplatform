// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package user_group

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_user_group"

// userGroupNamespace is the uniqueness rule for user group names. Jamf Pro
// refuses a duplicate with 409. The Classic API trims the name and compares it
// case-sensitively with accents significant, so `Lab` and `LAB` can both exist.
func userGroupNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.user-groups",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationCaseSensitive,
		Sources: []collision.Source{{
			Noun:         "user group",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListUserGroups(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(listed.UserGroups))
				for _, g := range listed.UserGroups {
					if g.ID != nil && g.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*g.ID), Key: *g.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a user group name that another user group already holds.
func (r *UserGroupResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil || r.pd == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.pd.CollisionChecker(),
		ResourceType: resourceTypeName,
		Rules:        []collision.AttributeRule{{Namespace: userGroupNamespace(r.client), Path: path.Root("name")}},
	})
}
