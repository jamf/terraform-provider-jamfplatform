// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package activation_code

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

// assignActivationCodeResourceModel populates a resource model from a ProClassic GET
// response. Both fields are Required, so committed state must always hold a concrete
// value. The classic /activationcode GET returns the code in clear (not masked), so
// state reflects the real value and normal drift detection applies. The nil branches
// are defensive only — the API echoes both fields on every successful read.
func assignActivationCodeResourceModel(state *ActivationCodeResourceModel, s *proclassic.ActivationCode) {
	if s.OrganizationName != nil {
		state.OrganizationName = types.StringValue(*s.OrganizationName)
	} else {
		state.OrganizationName = types.StringValue("")
	}
	if s.Code != nil {
		state.Code = types.StringValue(*s.Code)
	} else {
		state.Code = types.StringValue("")
	}
}

// assignActivationCodeDataSourceModel populates a data source model from a ProClassic
// GET response. Same nil-fallback semantics as the resource assigner.
func assignActivationCodeDataSourceModel(state *ActivationCodeDataSourceModel, s *proclassic.ActivationCode) {
	if s.OrganizationName != nil {
		state.OrganizationName = types.StringValue(*s.OrganizationName)
	} else {
		state.OrganizationName = types.StringValue("")
	}
	if s.Code != nil {
		state.Code = types.StringValue(*s.Code)
	} else {
		state.Code = types.StringValue("")
	}
}
