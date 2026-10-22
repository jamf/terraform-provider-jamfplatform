// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

// assignResourceModel populates a resource model from a platform environment.
//
// Every field is adopted from Jamf Account as returned. The name arrives already
// trimmed and the type already canonicalised, which the schema's validators pin
// on the configuration side so that plan and state agree.
func assignResourceModel(state *PlatformEnvironmentResourceModel, e *account.Environment) {
	state.ID = types.StringValue(e.ID)
	state.Name = types.StringValue(e.Name)
	state.Type = types.StringPointerValue(e.Label)
	state.Region = types.StringPointerValue(e.MetaRegion)
}

// assignDataSourceModel populates the singular data source model from a
// platform environment.
func assignDataSourceModel(state *PlatformEnvironmentDataSourceModel, e *account.Environment) {
	state.ID = types.StringValue(e.ID)
	state.Name = types.StringValue(e.Name)
	state.Type = types.StringPointerValue(e.Label)
	state.Region = types.StringPointerValue(e.MetaRegion)
}

// buildResultRow maps one platform environment into a plural data source row.
func buildResultRow(e account.Environment) PlatformEnvironmentsDataSourceResultRow {
	return PlatformEnvironmentsDataSourceResultRow{
		ID:     types.StringValue(e.ID),
		Name:   types.StringValue(e.Name),
		Type:   types.StringPointerValue(e.Label),
		Region: types.StringPointerValue(e.MetaRegion),
	}
}
