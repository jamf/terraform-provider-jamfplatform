// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package components

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

// ComponentsDataSource defines the data source for blueprint components.
type ComponentsDataSource struct {
	client *blueprints.Client
}

// ComponentsDataSourceModel defines the data structure for the components data source.
type ComponentsDataSourceModel struct {
	Components []ComponentListModel `tfsdk:"components"`
}

// ComponentListModel defines the data structure for a component in the list.
type ComponentListModel struct {
	Identifier  types.String `tfsdk:"identifier"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	SupportedOs types.Map    `tfsdk:"supported_os"`
}
