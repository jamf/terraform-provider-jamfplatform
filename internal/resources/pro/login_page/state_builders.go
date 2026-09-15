// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package login_page

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignLoginPageSettingsResourceModel populates a resource model from a GET
// response. The GET type (LoginContent) carries the four editable fields the resource
// manages; this is a pure copy. Authoritative state must come from a GET — the PUT echo is
// the LoginContentPut type and is identical for the four editable fields, but Create/Update
// re-GET so a single assigner serves all read paths. (The wire GET also returns read-only
// deployment flags — rampInstance and, on the wire, fedRampInstance/highComplianceInstance —
// which this resource intentionally does not expose; see spike Decision A.)
func assignLoginPageSettingsResourceModel(state *LoginPageSettingsResourceModel, s *pro.LoginContent) {
	state.IncludeCustomDisclaimer = types.BoolValue(s.IncludeCustomDisclaimer)
	state.DisclaimerHeading = types.StringValue(s.DisclaimerHeading)
	state.DisclaimerMainText = types.StringValue(s.DisclaimerMainText)
	state.ActionText = types.StringValue(s.ActionText)
}

// assignLoginPageSettingsDataSourceModel populates a data source model from a GET
// response. Same pure-copy semantics as the resource assigner.
func assignLoginPageSettingsDataSourceModel(state *LoginPageSettingsDataSourceModel, s *pro.LoginContent) {
	state.IncludeCustomDisclaimer = types.BoolValue(s.IncludeCustomDisclaimer)
	state.DisclaimerHeading = types.StringValue(s.DisclaimerHeading)
	state.DisclaimerMainText = types.StringValue(s.DisclaimerMainText)
	state.ActionText = types.StringValue(s.ActionText)
}
