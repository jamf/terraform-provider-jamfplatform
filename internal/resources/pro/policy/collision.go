// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_policy"

// policyNamespace is the uniqueness rule for policy names. Jamf Pro refuses a
// duplicate with 409 across every site, and the Classic API trims the name and
// compares it without regard to case but with accents significant.
func policyNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.policies",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationAccentSensitive,
		Sources: []collision.Source{{
			Noun:         "policy",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListPolicies(ctx)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(listed.Policies))
				for _, p := range listed.Policies {
					if p.ID != nil && p.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*p.ID), Key: *p.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a policy name that another policy already holds.
func (r *PolicyResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: policyNamespace(r.client),
			Path:      path.Root("general").AtName("name"),
		}},
	})
}
