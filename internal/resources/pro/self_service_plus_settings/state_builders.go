// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package self_service_plus_settings

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignSelfServicePlusSettingsResourceModel populates a resource model from an SDK response.
//
// `enabled` is Required in the schema, so state.Enabled must always end up populated
// with a concrete value (Required fields cannot be null/unknown in committed state).
// The SDK type uses `*bool` because the JSON wire format includes `omitempty`; in
// practice the Jamf Pro API echoes the field on every successful GET. The nil branch
// below is a defensive fallback only — `false` is the safest default for a missing
// feature-toggle bool (assumes "off" rather than "on").
//
// If a future singleton field is Optional (not Required), prefer
// helpers.ReconcileOptionalBoolPointer so state preserves user intent on nil rather
// than collapsing to a zero value.
func assignSelfServicePlusSettingsResourceModel(state *SelfServicePlusSettingsResourceModel, s *pro.SelfServicePlusSettings) {
	if s.Enabled != nil {
		state.Enabled = types.BoolValue(*s.Enabled)
	} else {
		state.Enabled = types.BoolValue(false)
	}
}

// assignSelfServicePlusSettingsDataSourceModel populates a data source model from an
// SDK response. Same nil-fallback semantics as the resource assigner; see that
// function's comment for the rationale.
func assignSelfServicePlusSettingsDataSourceModel(state *SelfServicePlusSettingsDataSourceModel, s *pro.SelfServicePlusSettings) {
	if s.Enabled != nil {
		state.Enabled = types.BoolValue(*s.Enabled)
	} else {
		state.Enabled = types.BoolValue(false)
	}
}
