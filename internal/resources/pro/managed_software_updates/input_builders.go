// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package managed_software_updates

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildManagedSoftwareUpdateInput converts the Terraform plan model into an SDK PUT payload.
//
// Only `enabled` is sent (mapped to the SDK `toggle` field). The four sub-enables
// (dss/recipe/forceInstallLocalDate/customVersion) are server-coerced — wire-probed
// 2026-06-13: PUT-ing any of them is ignored, the server returns its own derived value — so
// they are modelled Computed-only and never authored on the write (their *bool fields are
// left nil and omitted).
//
// `enabled` is Optional+Computed with UseStateForUnknown: on update an omitted value is a
// known prior value (carried by USFU). On first create there is no prior state, so the
// `current` merge base — the live value read in Create — supplies the value the user
// omitted, so the feature is adopted rather than flipped. On update current is nil.
func buildManagedSoftwareUpdateInput(plan ManagedSoftwareUpdateResourceModel, current *pro.ManagedSoftwareUpdatePlanToggle) *pro.ManagedSoftwareUpdatePlanToggle {
	enabled := false
	switch {
	case !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown():
		enabled = plan.Enabled.ValueBool()
	case current != nil:
		enabled = current.Toggle
	}
	return &pro.ManagedSoftwareUpdatePlanToggle{Toggle: enabled}
}
