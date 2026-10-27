// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/aigovernance"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_ai_governance_policy"

// policyNamespace is the uniqueness rule for AI Governance policy names. Uniqueness:
// name, not enforced (duplicates stored, wire-probed 2026-10-23), checked only with
// include_unenforced. The service stores every variant, so there is no rule to
// mirror; the strict comparison folds case, trailing spaces and accents, so names a
// person reads as the same are reported. An archived policy is gone as far as the
// resource is concerned (archiving is its delete), so it is left out of the listing.
func policyNamespace(client *aigovernance.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "aigovernance.policies",
		Product:   "Jamf AI Governance",
		Enforced:  false,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "AI policy",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				listed, err := client.ListPolicies(ctx, nil, false)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(listed))
				for _, p := range listed {
					if p.Status == aigovernance.PolicySummaryStatusArchived {
						continue
					}
					entries = append(entries, collision.Entry{ID: p.ID, Key: p.Name})
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a policy name that another AI policy already holds.
func (r *PolicyResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: policyNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
