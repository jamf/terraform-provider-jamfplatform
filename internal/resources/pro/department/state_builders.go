// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package department

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignDepartmentResourceModel populates a resource model from a Department response.
// Only overwrites state.ID when d.ID is non-nil so post-create GETs that omit the ID
// do not clobber the value captured from CreateDepartmentV1's HrefResponse.
func assignDepartmentResourceModel(state *DepartmentResourceModel, d *pro.Department) {
	if d.ID != nil {
		state.ID = types.StringValue(*d.ID)
	}
	state.Name = types.StringValue(d.Name)
}

// assignDepartmentDataSourceModel populates a data source model from a Department response.
func assignDepartmentDataSourceModel(state *DepartmentDataSourceModel, d *pro.Department) {
	if d.ID != nil {
		state.ID = types.StringValue(*d.ID)
	}
	state.Name = types.StringValue(d.Name)
}
