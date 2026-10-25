// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package cloud_identity_provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_cloud_identity_provider"

// cloudIdentityProviderNamespace is the uniqueness rule for display names across
// every cloud identity provider, Google and Entra ID alike. Jamf Pro refuses a
// duplicate with 400 DUPLICATE_FIELD on cloudIdPCommon.displayName. An upper case
// variant, a leading space and a trailing space all collide; an accent variant is
// refused too, with a 500 that carries no body (wire-probed 2026-10-25). The
// comparison therefore folds case, accents and surrounding spaces.
func cloudIdentityProviderNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.cloud-identity-providers",
		Product:   "Jamf Pro",
		KeyLabel:  "display name",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "cloud identity provider",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				idps, err := client.ListCloudIdpV1(ctx, nil)
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(idps))
				for _, idp := range idps {
					entries = append(entries, collision.Entry{ID: idp.ID, Key: idp.DisplayName})
				}
				return entries, nil
			},
		}},
	}
}

func (r *CloudIdentityProviderResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: cloudIdentityProviderNamespace(r.client),
			Path:      path.Root("display_name"),
		}},
	})
}
