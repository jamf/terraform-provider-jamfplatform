// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package inventory_preload_record

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

var _ resource.ResourceWithUpgradeState = &InventoryPreloadRecordResource{}

// UpgradeState migrates prior state versions to the current schema.
//
// v0 → v1: extension_attributes became a list, because Jamf Pro stores a repeated
// entry and a set cannot hold one (issue #568). The element type is unchanged and a
// set and a list both serialise as a JSON array, so the upgrader rewrites nothing:
// the raw prior-state JSON is decoded against the current schema with its stored
// order kept verbatim. That order is the set's canonical order rather than the order
// written, and a v0 provider also sent the entries to Jamf Pro in that canonical
// order, so the first plan after upgrading can show one in-place update that only
// reorders the entries. Applying it rewrites the record with the entries in the order
// written; no value changes, because v0 admitted each name only once, so the order
// had nothing to decide. The next plan is empty. A provider cannot avoid that update:
// neither an upgrader nor Read sees configuration, and planning the prior order when
// the entries match regardless of order would be a rule 1a collapse.
//
// The bump exists so that a provider still on v0 refuses v1 state, which can hold a
// repeated entry its set cannot decode: Terraform rejects state written by a newer
// schema version instead of letting the older provider fail on it.
func (r *InventoryPreloadRecordResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {StateUpgrader: r.upgradeFromV0},
	}
}

// upgradeFromV0 decodes v0 state against the current schema unchanged; see
// UpgradeState.
func (r *InventoryPreloadRecordResource) upgradeFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	if req.RawState == nil {
		return
	}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	schemaType := schemaResp.Schema.Type().TerraformType(ctx)

	value, err := req.RawState.Unmarshal(schemaType)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to upgrade jamfplatform_pro_inventory_preload_record state from v0",
			"Could not decode prior state against the current schema: "+helpers.APIErrorDetail(err),
		)
		return
	}

	dynamicValue, err := tfprotov6.NewDynamicValue(schemaType, value)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to upgrade jamfplatform_pro_inventory_preload_record state from v0",
			"Could not encode upgraded state: "+helpers.APIErrorDetail(err),
		)
		return
	}

	resp.DynamicValue = &dynamicValue
}
