// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package managed_software_updates

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignManagedSoftwareUpdateResourceModel populates a resource model from an SDK response.
// The SDK `toggle` field maps to the `enabled` attribute. The four sub-enables are `*bool`
// on the wire but the API always echoes them; a missing value maps to false so the Computed
// attributes stay deterministic.
func assignManagedSoftwareUpdateResourceModel(state *ManagedSoftwareUpdateResourceModel, s *pro.ManagedSoftwareUpdatePlanToggle) {
	state.Enabled = types.BoolValue(s.Toggle)
	state.DssEnabled = types.BoolValue(boolPtrValue(s.DssEnabled))
	state.RecipeEnabled = types.BoolValue(boolPtrValue(s.RecipeEnabled))
	state.ForceInstallLocalDateEnabled = types.BoolValue(boolPtrValue(s.ForceInstallLocalDateEnabled))
	state.CustomVersionEnabled = types.BoolValue(boolPtrValue(s.CustomVersionEnabled))
}

// boolPtrValue dereferences a *bool, treating nil as false.
func boolPtrValue(p *bool) bool {
	return p != nil && *p
}
