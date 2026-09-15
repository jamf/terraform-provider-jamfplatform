// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_request_form_field

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignAppRequestFormFieldResourceModel populates a resource model from an
// AppRequestFormInputField response. state.ID is only overwritten when the API ID is
// non-nil so a transient response that drops the ID does not clobber the ID already
// persisted from Create. Description echoes back faithfully (null stays null, "" stays "").
func assignAppRequestFormFieldResourceModel(state *AppRequestFormFieldResourceModel, m *pro.AppRequestFormInputField) {
	if m == nil {
		return
	}
	if m.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(m.ID)
	}
	state.Title = types.StringValue(m.Title)
	// types.StringPointerValue (not helpers.StringPointerValueOrNull) so an explicit
	// empty-string description round-trips faithfully: the server stores and echoes ""
	// verbatim (wire-probed 2026-06-13), and the write path sends "" as a non-nil pointer,
	// so collapsing "" to null on read would cause "inconsistent result after apply".
	state.Description = types.StringPointerValue(m.Description)
	state.Priority = types.Int64Value(int64(m.Priority))
}

// assignAppRequestFormFieldDataSourceModel populates a data source model from an
// AppRequestFormInputField response. Symmetric with the resource assigner.
func assignAppRequestFormFieldDataSourceModel(state *AppRequestFormFieldDataSourceModel, m *pro.AppRequestFormInputField) {
	if m == nil {
		return
	}
	if m.ID != nil {
		state.ID = helpers.StringValueFromIntPtr(m.ID)
	}
	state.Title = types.StringValue(m.Title)
	// types.StringPointerValue (not helpers.StringPointerValueOrNull) so an explicit
	// empty-string description round-trips faithfully: the server stores and echoes ""
	// verbatim (wire-probed 2026-06-13), and the write path sends "" as a non-nil pointer,
	// so collapsing "" to null on read would cause "inconsistent result after apply".
	state.Description = types.StringPointerValue(m.Description)
	state.Priority = types.Int64Value(int64(m.Priority))
}
