// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package removable_mac_address

import (
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignRemovableMacAddressResourceModel populates a resource model from a
// RemovableMacAddress response. state.ID is only overwritten when the API ID is non-nil
// so a transient GET that drops the ID does not clobber the ID already persisted from
// Create. The wire `name` field maps to mac_address.
func assignRemovableMacAddressResourceModel(state *RemovableMacAddressResourceModel, m *proclassic.RemovableMacAddress) {
	if m == nil {
		return
	}
	if m.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(m.ID)
	}
	if m.Name != nil {
		state.MacAddress = helpers.StringPointerValueOrNull(m.Name)
	}
}

// assignRemovableMacAddressDataSourceModel populates a data source model from a
// RemovableMacAddress response. Symmetric with assignRemovableMacAddressResourceModel:
// nil API fields do not overwrite the caller-supplied selector. The DS accepts either
// id or mac_address as input; if the SDK ever responds with a nil for the field the
// caller supplied, preserving the caller value is more useful than silently nulling it.
func assignRemovableMacAddressDataSourceModel(state *RemovableMacAddressDataSourceModel, m *proclassic.RemovableMacAddress) {
	if m == nil {
		return
	}
	if m.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(m.ID)
	}
	if m.Name != nil {
		state.MacAddress = helpers.StringPointerValueOrNull(m.Name)
	}
}
