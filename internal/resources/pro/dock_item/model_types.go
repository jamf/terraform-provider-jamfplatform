// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dock_item

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// DockItemResourceModel represents the Terraform resource model for a Jamf Pro dock item.
type DockItemResourceModel struct {
	ID       types.String           `tfsdk:"id"`
	Name     types.String           `tfsdk:"name"`
	Type     types.String           `tfsdk:"type"`
	Path     types.String           `tfsdk:"path"`
	Contents types.String           `tfsdk:"contents"`
	Timeouts resourceTimeouts.Value `tfsdk:"timeouts"`
}

// DockItemDataSourceModel represents the Terraform data source model for a Jamf
// Pro dock item. Either id or name must be supplied (enforced by ExactlyOneOf
// at config validation).
type DockItemDataSourceModel struct {
	ID       types.String             `tfsdk:"id"`
	Name     types.String             `tfsdk:"name"`
	Type     types.String             `tfsdk:"type"`
	Path     types.String             `tfsdk:"path"`
	Contents types.String             `tfsdk:"contents"`
	Timeouts datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// dockItemIdentityModel represents the identity object for dock item resources
// and list results.
type dockItemIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// DockItemListResourceModel represents the config model for dock item list
// queries. Classic has no RSQL — the filter shape is the shared client-side
// substring block.
type DockItemListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
