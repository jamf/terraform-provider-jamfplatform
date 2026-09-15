// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package pki_json_web_token_configuration

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignJSONWebTokenConfigurationResourceModel populates a resource model from
// the SDK response. `encryption_key_wo` is never written into state
// (WriteOnly; Jamf Pro never returns it), and `encryption_key_wo_version` is
// preserved from the caller's plan/state — the server has no such field. The
// wire `disabled` flag inverts into `enabled`.
func assignJSONWebTokenConfigurationResourceModel(state *JSONWebTokenConfigurationResourceModel, c *proclassic.JsonWebTokenConfiguration) {
	if c == nil {
		return
	}
	if id := extractJSONWebTokenConfigurationID(c); id != "" {
		state.ID = types.StringValue(id)
	}
	state.Name = helpers.StringPointerValueOrNull(c.Name)
	state.TokenExpiry = helpers.Int64FromIntPtr(c.TokenExpiry)
	state.Enabled = enabledFromDisabled(c.Disabled)
}

// assignJSONWebTokenConfigurationDataSourceModel projects an SDK response into
// the flat data source model. The encryption key is never surfaced.
func assignJSONWebTokenConfigurationDataSourceModel(state *JSONWebTokenConfigurationDataSourceModel, c *proclassic.JsonWebTokenConfiguration) {
	if c == nil {
		return
	}
	if id := extractJSONWebTokenConfigurationID(c); id != "" {
		state.ID = types.StringValue(id)
	}
	state.Name = helpers.StringPointerValueOrNull(c.Name)
	state.TokenExpiry = helpers.Int64FromIntPtr(c.TokenExpiry)
	state.Enabled = enabledFromDisabled(c.Disabled)
}

// enabledFromDisabled inverts the wire `disabled` flag into the Terraform
// `enabled` attribute, null for absent.
func enabledFromDisabled(disabled *bool) types.Bool {
	if disabled == nil {
		return types.BoolNull()
	}
	return types.BoolValue(!*disabled)
}
