// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package component

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

// ComponentDataSource defines the data source implementation.
type ComponentDataSource struct {
	client *blueprints.Client
}

// ComponentDataSourceModel defines the data source data model.
type ComponentDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Identifier  types.String `tfsdk:"identifier"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	SupportedOs types.Map    `tfsdk:"supported_os"`
}
