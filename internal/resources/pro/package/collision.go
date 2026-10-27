// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pkg

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_package"

// packageNamespace builds one uniqueness rule over the package list. Jamf Pro
// holds two independent keys on a package, the display name and the file name,
// and refuses a duplicate of either with 400 DUPLICATE_FIELD. Both fold case and
// accents, and neither folds a leading space, wire-probed 2026-10-25. A trailing
// space cannot reach the file name rule, because the extension check refuses the
// name first.
func packageNamespace(client *pro.Client, id, keyLabel string, key func(pro.Package) string) collision.Namespace {
	return collision.Namespace{
		ID:        id,
		Product:   "Jamf Pro",
		KeyLabel:  keyLabel,
		Enforced:  true,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "package",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				packages, err := client.ListPackagesV1(ctx, nil, "")
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(packages))
				for _, p := range packages {
					if p.ID != nil {
						entries = append(entries, collision.Entry{ID: *p.ID, Key: key(p)})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a display name or file name that another package
// already holds.
func (r *PackageResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[PackageResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[PackageResourceModel]{
			{
				Namespace: packageNamespace(r.client, "pro.package-names", "display name", func(p pro.Package) string { return p.PackageName }),
				Path:      path.Root("display_name"),
				Keys:      func(m *PackageResourceModel) []types.String { return []types.String{m.DisplayName} },
			},
			{
				Namespace: packageNamespace(r.client, "pro.package-file-names", "file name", func(p pro.Package) string { return p.FileName }),
				Path:      path.Root("file_name"),
				Keys:      func(m *PackageResourceModel) []types.String { return []types.String{m.FileName} },
			},
		},
	})
}
