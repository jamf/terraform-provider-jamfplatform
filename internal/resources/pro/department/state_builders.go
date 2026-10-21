// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package department

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// assignDepartmentResourceModel populates a resource model from a Department response.
// Only overwrites state.ID when d.ID is non-nil so post-create GETs that omit the ID
// do not clobber the value captured from CreateDepartmentV1's HrefResponse.
//
// The name uses StringValueTrimmed because /v1/departments saves it with its outer
// whitespace removed (probed 2026-10-20, issue #510); other fields are assigned
// from the wire.
func assignDepartmentResourceModel(state *DepartmentResourceModel, d *pro.Department) {
	if d.ID != nil {
		state.ID = types.StringValue(*d.ID)
	}
	state.Name = helpers.StringValueTrimmed(d.Name, state.Name)
}

// assignDepartmentDataSourceModel populates a data source model from a Department response.
func assignDepartmentDataSourceModel(state *DepartmentDataSourceModel, d *pro.Department) {
	if d.ID != nil {
		state.ID = types.StringValue(*d.ID)
	}
	state.Name = types.StringValue(d.Name)
}
