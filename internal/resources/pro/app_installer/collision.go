// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_installer

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_app_installer"

// appInstallerNamespace is the strict uniqueness rule for App Installer deployment
// names. Jamf Pro stores every variant of the name, so there is no server rule to
// mirror. The comparison folds case, trailing spaces and accents, so names a person
// reads as the same object are reported. The rule applies only when
// include_unenforced is set.
func appInstallerNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.app-installer-deployments",
		Product:   "Jamf Pro",
		Enforced:  false,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "App Installer deployment",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				deployments, err := client.ListAppInstallerDeploymentsV1(ctx, nil, "")
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(deployments))
				for _, d := range deployments {
					entries = append(entries, collision.Entry{ID: d.ID, Key: d.Name})
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a name that another App Installer deployment already holds, when
// include_unenforced is set.
func (r *AppInstallerResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: appInstallerNamespace(r.client),
			Path:      path.Root("name"),
		}},
	})
}
