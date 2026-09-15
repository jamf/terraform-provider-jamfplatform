// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package return_to_service

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// buildReturnToServiceInput converts a plan model into the SDK request used for
// Create and Update. Both fields are required by the server on every write
// (create and update), so both are always emitted — the write is a full
// replace, not a merge.
func buildReturnToServiceInput(plan ReturnToServiceResourceModel) *pro.ReturnToServiceConfigurationRequest {
	displayName := plan.DisplayName.ValueString()
	wifiProfileID := plan.WifiProfileID.ValueString()
	return &pro.ReturnToServiceConfigurationRequest{
		DisplayName:   &displayName,
		WifiProfileID: &wifiProfileID,
	}
}
