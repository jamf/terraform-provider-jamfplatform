// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/collision"
)

const resourceTypeName = "jamfplatform_pro_inventory_preload_record"

// inventoryPreloadRecordNamespace is the uniqueness rule for serial numbers. Jamf
// Pro refuses a second record with 400 DUPLICATE_FIELD across both device types,
// whatever the device type of either record, and compares the serial number without
// regard to case, trailing spaces or accents. A leading space keeps two serial
// numbers apart (wire-probed 2026-10-25).
func inventoryPreloadRecordNamespace(client *pro.Client) collision.Namespace {
	return collision.Namespace{
		ID:        "pro.inventory-preload-serial-numbers",
		Product:   "Jamf Pro",
		KeyLabel:  "serial number",
		Enforced:  true,
		Collation: collision.CollationFolded,
		Sources: []collision.Source{{
			Noun:         "inventory preload record",
			ResourceType: resourceTypeName,
			List: func(ctx context.Context) ([]collision.Entry, error) {
				records, err := client.ListInventoryPreloadRecordsV2(ctx, nil, "")
				if err != nil {
					return nil, err
				}
				entries := make([]collision.Entry, 0, len(records))
				for _, rec := range records {
					if rec.ID != nil {
						entries = append(entries, collision.Entry{ID: *rec.ID, Key: rec.SerialNumber})
					}
				}
				return entries, nil
			},
		}},
	}
}

// reportNameCollision reports a serial number that another record already holds.
func (r *InventoryPreloadRecordResource) reportNameCollision(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}
	collision.ReportAttributePlan(ctx, req, resp, collision.AttributeReport{
		Checker:      r.collisions,
		ResourceType: resourceTypeName,
		Rules: []collision.AttributeRule{{
			Namespace: inventoryPreloadRecordNamespace(r.client),
			Path:      path.Root("serial_number"),
		}},
	})
}
