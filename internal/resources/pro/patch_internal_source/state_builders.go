// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package patch_internal_source

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignPatchInternalSourceDataSourceModel populates a data source model from a
// PatchInternalSource response. The id and name selectors are preserved when the
// API field is nil so the caller-supplied lookup value is not silently nulled;
// the remaining attributes are surfaced from the response. available_titles is
// fetched separately in Read.
func assignPatchInternalSourceDataSourceModel(state *PatchInternalSourceDataSourceModel, s *proclassic.PatchInternalSource) {
	if s == nil {
		return
	}
	if s.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(s.ID)
	}
	if s.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(s.Name)
	}
	state.Enabled = helpers.BoolPointerValueOrNull(s.Enabled)
	state.Endpoint = helpers.StringPointerValueOrNull(s.Endpoint)
}
