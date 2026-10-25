// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package supervision_identity

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_supervision_identity"

// supervisionIdentityNamespace is the strict uniqueness rule for supervision
// identity display names. Jamf Pro stores every variant of the name, so there is no
// server rule to mirror. The comparison folds case, trailing spaces and accents, so
// names a person reads as the same object are reported. The rule applies only when
// include_unenforced is set.
func supervisionIdentityNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.supervision-identities",
		Product:   "Jamf Pro",
		KeyLabel:  "display name",
		Enforced:  false,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "supervision identity",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				identities, err := client.ListSupervisionIdentitiesV1(ctx, nil)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(identities))
				for _, i := range identities {
					entries = append(entries, collision.Entry{ID: strconv.Itoa(i.ID), Key: i.DisplayName})
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a display name that another supervision identity already holds, when
// include_unenforced is set.
func (r *SupervisionIdentityResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: supervisionIdentityNamespace(r.client),
			Path:      path.Root("display_name"),
		}},
	})
}
