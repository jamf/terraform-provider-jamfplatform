// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package allowed_file_extension

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignAllowedFileExtensionResourceModel populates a resource model from an
// AllowedFileExtension response. state.ID is only overwritten when the API ID is non-nil
// so a transient GET that drops the ID does not clobber the ID already persisted from
// Create. The wire `extension` field maps to extension.
func assignAllowedFileExtensionResourceModel(state *AllowedFileExtensionResourceModel, m *proclassic.AllowedFileExtension) {
	if m == nil {
		return
	}
	if m.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(m.ID)
	}
	if m.Extension != nil {
		state.Extension = helpers.StringPointerValueOrNull(m.Extension)
	}
}

// assignAllowedFileExtensionDataSourceModel populates a data source model from an
// AllowedFileExtension response. Symmetric with assignAllowedFileExtensionResourceModel:
// nil API fields do not overwrite the caller-supplied selector. The DS accepts either id
// or extension as input; if the SDK ever responds with a nil for the field the caller
// supplied, preserving the caller value is more useful than silently nulling it.
func assignAllowedFileExtensionDataSourceModel(state *AllowedFileExtensionDataSourceModel, m *proclassic.AllowedFileExtension) {
	if m == nil {
		return
	}
	if m.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(m.ID)
	}
	if m.Extension != nil {
		state.Extension = helpers.StringPointerValueOrNull(m.Extension)
	}
}
