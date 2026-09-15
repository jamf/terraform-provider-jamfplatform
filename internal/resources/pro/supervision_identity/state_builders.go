// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package supervision_identity

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignSupervisionIdentityResourceModel populates a resource model from a
// create / read response. The password and certificate are never echoed by Jamf
// Pro and are WriteOnly, so they are deliberately not touched here. state.ID is
// only overwritten when the response carries a non-zero ID.
func assignSupervisionIdentityResourceModel(state *SupervisionIdentityResourceModel, s *pro.SupervisionIdentity) {
	if s == nil {
		return
	}
	if s.ID != 0 {
		state.ID = types.StringValue(strconv.Itoa(s.ID))
	}
	state.DisplayName = types.StringValue(s.DisplayName)
	state.CommonName = types.StringValue(s.CommonName)
	state.ExpirationDate = types.StringValue(s.ExpirationDate)
}

// assignSupervisionIdentityDataSourceModel populates a data source model from a
// read response. Symmetric with the resource assigner; secrets are never exposed.
func assignSupervisionIdentityDataSourceModel(state *SupervisionIdentityDataSourceModel, s *pro.SupervisionIdentity) {
	if s == nil {
		return
	}
	if s.ID != 0 {
		state.ID = types.StringValue(strconv.Itoa(s.ID))
	}
	state.DisplayName = types.StringValue(s.DisplayName)
	state.CommonName = types.StringValue(s.CommonName)
	state.ExpirationDate = types.StringValue(s.ExpirationDate)
}
