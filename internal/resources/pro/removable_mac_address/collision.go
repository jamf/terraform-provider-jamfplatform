// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package removable_mac_address

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_removable_mac_address"

// removableMacAddressNamespace is the uniqueness rule for the address value, which
// the Classic API stores as the record's name. Jamf Pro refuses a duplicate with
// 409. Classic trims the value on save and compares it without regard to case, but
// an accent keeps two values apart (wire-probed 2026-10-25).
func removableMacAddressNamespace(client *proclassic.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "proclassic.removable-mac-addresses",
		Product:   "Jamf Pro",
		KeyLabel:  "value",
		Enforced:  true,
		Collation: collision.CollationAccentSensitive,
		Sources: []collision.Source{{
			Noun:         "removable MAC address",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				list, err := client.ListRemovableMacAddresses(ctx)
				if err != nil {
					return nil, err
				}
				if list == nil {
					return nil, nil
				}
				entries := make([]collision.Entry, 0, len(list.RemovableMacAddresses))
				for _, a := range list.RemovableMacAddresses {
					if a.ID != nil && a.Name != nil {
						entries = append(entries, collision.Entry{ID: strconv.Itoa(*a.ID), Key: *a.Name})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports an address that another record already holds.
func (r *RemovableMacAddressResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: removableMacAddressNamespace(r.client),
			Path:      path.Root("mac_address"),
		}},
	})
}
