// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package category

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
)

// assignCategoryResourceModel populates a resource model from a Category response.
// Only overwrites state.ID when c.ID is non-nil. Create's post-create GetCategoryV1
// must not clobber the ID already set from the CreateCategoryV1 response if the GET
// ever returns a nil ID.
func assignCategoryResourceModel(state *CategoryResourceModel, c *pro.Category) {
	if c.ID != nil {
		state.ID = types.StringValue(*c.ID)
	}
	state.Name = types.StringValue(c.Name)
	state.Priority = types.Int64Value(int64(c.Priority))
}

// assignCategoryDataSourceModel populates a data source model from a Category response.
// Only overwrites state.ID when c.ID is non-nil, mirroring assignCategoryResourceModel.
func assignCategoryDataSourceModel(state *CategoryDataSourceModel, c *pro.Category) {
	if c.ID != nil {
		state.ID = types.StringValue(*c.ID)
	}
	state.Name = types.StringValue(c.Name)
	state.Priority = types.Int64Value(int64(c.Priority))
}
