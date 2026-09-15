// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package tenant_id

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignTenantIDDataSourceModel maps the tenant identifier response onto the
// data source model, stamping the fixed singleton ID.
//
// Jamf types the identifier as optional, so an empty response decodes cleanly to
// a nil pointer rather than failing. Every observed read returned a value, but a
// nil would otherwise commit an empty string to state and hand a caller a
// silently wrong tenant to point a connector at — worth an error rather than a
// shrug, since the whole purpose of the data source is to supply that one value.
func assignTenantIDDataSourceModel(state *TenantIDDataSourceModel, info *pro.CsaTenantIDInfo) diag.Diagnostics {
	var diags diag.Diagnostics

	if info == nil || info.TenantID == nil || *info.TenantID == "" {
		diags.AddError(
			"Jamf Pro returned no tenant identifier",
			"The read succeeded but carried no tenant identifier. This is unexpected for a provisioned Jamf Pro "+
				"tenant; check that the provider is scoped to a Jamf Pro tenant rather than to another product, "+
				"and retry.",
		)
		return diags
	}

	state.ID = types.StringValue(helpers.SingletonID)
	state.TenantID = types.StringValue(*info.TenantID)
	return diags
}
