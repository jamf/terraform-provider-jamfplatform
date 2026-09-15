// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package local_admin_password_settings

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignLocalAdminPasswordSettingsResourceModel populates resource state from a
// LAPS settings response.
//
// The toggle maps directly. The two interval durations are mapped back to their
// dropdown labels; automatic rotation being off maps to rotation_interval
// "Never" regardless of the (dormant) stored expiration. The server stores
// arbitrary durations and does not enforce the dropdown presets, so a tenant
// edited outside Terraform can hold a value with no matching label. Rather than
// silently snapping to the nearest preset (which would hide the drift), an
// unmapped duration raises a diagnostic naming the offending value and the
// remedy. (rotation_after_viewing_interval has no "off" state, so an unsupported
// value there always surfaces; rotation_interval only surfaces one while
// automatic rotation is on.)
func assignLocalAdminPasswordSettingsResourceModel(state *LocalAdminPasswordSettingsResourceModel, s *pro.LapsSettingsResponseV2, diags *diag.Diagnostics) {
	if s == nil {
		return
	}

	state.LapsForPrestageAccountsEnabled = types.BoolValue(s.AutoDeployEnabled)

	if label, ok := durationToRotationAfterViewing[s.PasswordRotationTime]; ok {
		state.RotationAfterViewingInterval = types.StringValue(label)
	} else {
		diags.AddError(
			"Unsupported LAPS rotation after viewing interval",
			fmt.Sprintf(
				"The Jamf Pro tenant has a rotation after viewing interval of %d seconds, which is not one of the supported values (%s). It was likely set outside Terraform to a custom value. Set a supported interval in Jamf Pro before managing these settings with Terraform.",
				s.PasswordRotationTime, strings.Join(validRotationAfterViewingInterval, ", "),
			),
		)
	}

	switch {
	case !s.AutoRotateEnabled:
		state.RotationInterval = types.StringValue(rotationIntervalNever)
	default:
		if label, ok := durationToRotationInterval[s.AutoRotateExpirationTime]; ok {
			state.RotationInterval = types.StringValue(label)
		} else {
			diags.AddError(
				"Unsupported LAPS rotation interval",
				fmt.Sprintf(
					"The Jamf Pro tenant has an automatic rotation interval of %d seconds, which is not one of the supported values (%s). It was likely set outside Terraform to a custom value. Set a supported interval in Jamf Pro before managing these settings with Terraform.",
					s.AutoRotateExpirationTime, strings.Join(validRotationIntervalDurations, ", "),
				),
			)
		}
	}
}

// assignLocalAdminPasswordSettingsDataSourceModel populates data source state
// from a LAPS settings response, reusing the resource mapping so the data source
// presents the same UI-aligned values.
func assignLocalAdminPasswordSettingsDataSourceModel(state *LocalAdminPasswordSettingsDataSourceModel, s *pro.LapsSettingsResponseV2, diags *diag.Diagnostics) {
	if s == nil {
		return
	}
	res := &LocalAdminPasswordSettingsResourceModel{}
	assignLocalAdminPasswordSettingsResourceModel(res, s, diags)
	state.LapsForPrestageAccountsEnabled = res.LapsForPrestageAccountsEnabled
	state.RotationInterval = res.RotationInterval
	state.RotationAfterViewingInterval = res.RotationAfterViewingInterval
}
