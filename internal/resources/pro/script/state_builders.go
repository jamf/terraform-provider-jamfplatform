// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package script

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignScriptResourceModel populates a resource model from a Script response.
// Only overwrites state.ID when s.ID is non-nil so post-create GETs that omit the ID
// do not clobber the value captured from CreateScriptV1's HrefResponse.
func assignScriptResourceModel(state *ScriptResourceModel, s *pro.Script) {
	if s.ID != nil {
		state.ID = types.StringValue(*s.ID)
	}
	state.Name = types.StringValue(s.Name)
	state.CategoryID = helpers.ReconcileOptionalStringPointer(s.CategoryID, state.CategoryID)
	state.CategoryName = helpers.StringPointerValueOrNull(s.CategoryName)
	state.Info = helpers.ReconcileOptionalStringPointer(s.Info, state.Info)
	state.Notes = helpers.ReconcileOptionalStringPointer(s.Notes, state.Notes)
	state.OsRequirements = helpers.ReconcileOptionalStringPointer(s.OsRequirements, state.OsRequirements)
	state.Priority = helpers.ReconcileOptionalStringPointer(s.Priority, state.Priority)
	state.Parameter4 = helpers.ReconcileOptionalStringPointer(s.Parameter4, state.Parameter4)
	state.Parameter5 = helpers.ReconcileOptionalStringPointer(s.Parameter5, state.Parameter5)
	state.Parameter6 = helpers.ReconcileOptionalStringPointer(s.Parameter6, state.Parameter6)
	state.Parameter7 = helpers.ReconcileOptionalStringPointer(s.Parameter7, state.Parameter7)
	state.Parameter8 = helpers.ReconcileOptionalStringPointer(s.Parameter8, state.Parameter8)
	state.Parameter9 = helpers.ReconcileOptionalStringPointer(s.Parameter9, state.Parameter9)
	state.Parameter10 = helpers.ReconcileOptionalStringPointer(s.Parameter10, state.Parameter10)
	state.Parameter11 = helpers.ReconcileOptionalStringPointer(s.Parameter11, state.Parameter11)
	state.ScriptContents = helpers.ReconcileOptionalStringPointer(s.ScriptContents, state.ScriptContents)
}

// assignScriptDataSourceModel populates a data source model from a Script response.
func assignScriptDataSourceModel(state *ScriptDataSourceModel, s *pro.Script) {
	if s.ID != nil {
		state.ID = types.StringValue(*s.ID)
	}
	state.Name = types.StringValue(s.Name)
	state.CategoryID = helpers.StringPointerValueOrNull(s.CategoryID)
	state.CategoryName = helpers.StringPointerValueOrNull(s.CategoryName)
	state.Info = helpers.StringPointerValueOrNull(s.Info)
	state.Notes = helpers.StringPointerValueOrNull(s.Notes)
	state.OsRequirements = helpers.StringPointerValueOrNull(s.OsRequirements)
	state.Priority = helpers.StringPointerValueOrNull(s.Priority)
	state.Parameter4 = helpers.StringPointerValueOrNull(s.Parameter4)
	state.Parameter5 = helpers.StringPointerValueOrNull(s.Parameter5)
	state.Parameter6 = helpers.StringPointerValueOrNull(s.Parameter6)
	state.Parameter7 = helpers.StringPointerValueOrNull(s.Parameter7)
	state.Parameter8 = helpers.StringPointerValueOrNull(s.Parameter8)
	state.Parameter9 = helpers.StringPointerValueOrNull(s.Parameter9)
	state.Parameter10 = helpers.StringPointerValueOrNull(s.Parameter10)
	state.Parameter11 = helpers.StringPointerValueOrNull(s.Parameter11)
	state.ScriptContents = helpers.StringPointerValueOrNull(s.ScriptContents)
}
