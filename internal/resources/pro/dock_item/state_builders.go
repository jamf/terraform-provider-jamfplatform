// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dock_item

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignDockItemResourceModel populates a resource model from a DockItem
// response. state.ID is only overwritten when the API ID is non-nil so a
// transient GET that drops the ID does not clobber the value persisted from
// Create. Contents is server-derived and always copied from the API value.
func assignDockItemResourceModel(state *DockItemResourceModel, di *proclassic.DockItem) diag.Diagnostics {
	var diags diag.Diagnostics
	if di == nil {
		return diags
	}
	if di.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(di.ID)
	}
	if di.Name != nil {
		state.Name = helpers.StringPointerValueOrNull(di.Name)
	}
	if di.Type != nil {
		state.Type = helpers.StringPointerValueOrNull(di.Type)
	}
	if di.Path != nil {
		state.Path = helpers.StringPointerValueOrNull(di.Path)
	}
	state.Contents = helpers.StringPointerValueOrNull(di.Contents)
	return diags
}

// assignDockItemDataSourceModel populates a data source model from a DockItem
// response. Symmetric with the resource builder but always copies the API
// value over the user's selector (the selector is just an input — output is
// Computed).
func assignDockItemDataSourceModel(state *DockItemDataSourceModel, di *proclassic.DockItem) diag.Diagnostics {
	var diags diag.Diagnostics
	if di == nil {
		return diags
	}
	if di.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(di.ID)
	}
	state.Name = helpers.StringPointerValueOrNull(di.Name)
	state.Type = helpers.StringPointerValueOrNull(di.Type)
	state.Path = helpers.StringPointerValueOrNull(di.Path)
	state.Contents = helpers.StringPointerValueOrNull(di.Contents)
	return diags
}
