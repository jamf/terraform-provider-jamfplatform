// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package return_to_service

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignReturnToServiceResourceModel populates a resource model from a Return to
// Service configuration response. The server is authoritative for every field.
func assignReturnToServiceResourceModel(state *ReturnToServiceResourceModel, config *pro.ReturnToServiceConfiguration) {
	if config == nil {
		return
	}
	state.ID = types.StringValue(config.ID)
	state.DisplayName = types.StringValue(config.DisplayName)
	state.WifiProfileID = types.StringValue(config.WifiProfileID)
}

// assignReturnToServiceDataSourceModel populates a data source model from a
// Return to Service configuration response. Symmetric with the resource builder.
func assignReturnToServiceDataSourceModel(state *ReturnToServiceDataSourceModel, config *pro.ReturnToServiceConfiguration) {
	if config == nil {
		return
	}
	state.ID = types.StringValue(config.ID)
	state.DisplayName = types.StringValue(config.DisplayName)
	state.WifiProfileID = types.StringValue(config.WifiProfileID)
}
