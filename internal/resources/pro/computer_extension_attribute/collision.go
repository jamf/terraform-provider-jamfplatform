// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package computer_extension_attribute

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_computer_extension_attribute"

var _ resource.ResourceWithModifyPlan = &ComputerExtensionAttributeResource{}

// computerExtensionAttributeNamespace is the uniqueness rule for computer
// extension attribute names. Jamf Pro trims a name on save, then refuses a
// duplicate with 400 DUPLICATE_FIELD, folding case and accents as well.
func computerExtensionAttributeNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.computer-extension-attributes",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "computer extension attribute",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				items, err := client.ListComputerExtensionAttributesV1(ctx, nil, "")
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(items))
				for _, item := range items {
					if item.ID != nil {
						entries = append(entries, collision.Entry{ID: *item.ID, Key: item.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// ModifyPlan reports a computer extension attribute name that another one
// already holds.
func (r *ComputerExtensionAttributeResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[ComputerExtensionAttributeResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[ComputerExtensionAttributeResourceModel]{{
			Namespace: computerExtensionAttributeNamespace(r.client),
			Path:      path.Root("name"),
			Keys: func(m *ComputerExtensionAttributeResourceModel) []types.String {
				return []types.String{m.Name}
			},
		}},
	})
}
