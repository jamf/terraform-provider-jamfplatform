// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_extension_attribute

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_mobile_device_extension_attribute"

var _ resource.ResourceWithModifyPlan = &MobileDeviceExtensionAttributeResource{}

// mobileDeviceExtensionAttributeNamespace is the uniqueness rule for mobile
// device extension attribute names. Jamf Pro trims a name on save, then refuses
// a duplicate with 400 DUPLICATE_FIELD, folding case and accents as well.
func mobileDeviceExtensionAttributeNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.mobile-device-extension-attributes",
		Product:   "Jamf Pro",
		Enforced:  true,
		Collation: collision.CollationFoldedTrimmed,
		Sources: []collision.Source{{
			Noun:         "mobile device extension attribute",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				items, err := client.ListMobileDeviceExtensionAttributesV1(ctx, nil, "")
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

// ModifyPlan reports a mobile device extension attribute name that another one
// already holds.
func (r *MobileDeviceExtensionAttributeResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportPlan(ctx, req, resp, collision.PlanReport[MobileDeviceExtensionAttributeResourceModel]{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.Rule[MobileDeviceExtensionAttributeResourceModel]{{
			Namespace: mobileDeviceExtensionAttributeNamespace(r.client),
			Path:      path.Root("name"),
			Keys: func(m *MobileDeviceExtensionAttributeResourceModel) []types.String {
				return []types.String{m.Name}
			},
		}},
	})
}
