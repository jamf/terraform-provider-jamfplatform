// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package network_segment

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// buildNetworkSegmentInput converts the Terraform plan model into the SDK
// NetworkSegmentPost payload used for both Create and Update. The four server-
// derived fields (distribution_point, distribution_server, swu_server, url) are
// intentionally omitted — they are populated on read only. ID is omitted on
// write — Create uses path id="0" and Update derives it from state.
//
// The classic /networksegments PUT merges field by field: an omitted element
// keeps the stored value, an empty <building>/<department> clears the name and
// an explicit <override_*>false</override_*> turns the flag off (wire-probed
// 2026-09-06 on Jamf Pro 11.31.1, issue #384). The four optional fields are
// therefore always emitted — helpers.AlwaysEmitStringPointer and
// helpers.AlwaysEmitBoolPointer send "" / false for null — so a value the user
// removes from config is cleared rather than retained, and the state builder's
// Reconcile*Pointer calls fold the echoed "" / false back to null. The server
// accepts an override flag of true beside an empty name (probed), so the
// AlsoRequires validators on the flags are what keep that combination out of
// config.
func buildNetworkSegmentInput(plan NetworkSegmentResourceModel) *proclassic.NetworkSegmentPost {
	return &proclassic.NetworkSegmentPost{
		Name:                helpers.OptionalStringPointer(plan.Name),
		StartingAddress:     helpers.OptionalStringPointer(plan.StartingAddress),
		EndingAddress:       helpers.OptionalStringPointer(plan.EndingAddress),
		Building:            helpers.AlwaysEmitStringPointer(plan.Building),
		Department:          helpers.AlwaysEmitStringPointer(plan.Department),
		OverrideBuildings:   helpers.AlwaysEmitBoolPointer(plan.OverrideBuildings),
		OverrideDepartments: helpers.AlwaysEmitBoolPointer(plan.OverrideDepartments),
	}
}
