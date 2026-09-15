// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package site

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignSiteResourceModel populates a resource model from a Site response.
// state.ID is only overwritten when the API ID is non-nil so a transient GET that
// drops the ID does not clobber the ID already persisted from Create.
func assignSiteResourceModel(state *SiteResourceModel, s *proclassic.Site) {
	if s == nil {
		return
	}
	if s.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(s.ID)
	}
	if s.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(s.Name)
	}
}

// assignSiteDataSourceModel populates a data source model from a Site response.
// Symmetric with assignSiteResourceModel: nil API fields do not overwrite the
// caller-supplied selector. The DS accepts either id or name as input; if the
// SDK ever responds with a nil for the field the caller supplied, preserving
// the caller value is more useful than silently nulling it out.
func assignSiteDataSourceModel(state *SiteDataSourceModel, s *proclassic.Site) {
	if s == nil {
		return
	}
	if s.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(s.ID)
	}
	if s.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(s.Name)
	}
}
