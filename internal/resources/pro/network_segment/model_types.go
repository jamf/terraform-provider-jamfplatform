// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package network_segment

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// NetworkSegmentResourceModel represents the Terraform resource model for a Jamf Pro network segment.
type NetworkSegmentResourceModel struct {
	ID                  types.String           `tfsdk:"id"`
	Name                types.String           `tfsdk:"name"`
	StartingAddress     types.String           `tfsdk:"starting_address"`
	EndingAddress       types.String           `tfsdk:"ending_address"`
	Building            types.String           `tfsdk:"building"`
	Department          types.String           `tfsdk:"department"`
	OverrideBuildings   types.Bool             `tfsdk:"override_buildings"`
	OverrideDepartments types.Bool             `tfsdk:"override_departments"`
	DistributionPoint   types.String           `tfsdk:"distribution_point"`
	DistributionServer  types.String           `tfsdk:"distribution_server"`
	SwuServer           types.String           `tfsdk:"swu_server"`
	URL                 types.String           `tfsdk:"url"`
	Timeouts            resourceTimeouts.Value `tfsdk:"timeouts"`
}

// NetworkSegmentDataSourceModel represents the Terraform data source model for a Jamf Pro
// network segment. Either id or name must be supplied (enforced by ExactlyOneOf at
// config validation).
type NetworkSegmentDataSourceModel struct {
	ID                  types.String             `tfsdk:"id"`
	Name                types.String             `tfsdk:"name"`
	StartingAddress     types.String             `tfsdk:"starting_address"`
	EndingAddress       types.String             `tfsdk:"ending_address"`
	Building            types.String             `tfsdk:"building"`
	Department          types.String             `tfsdk:"department"`
	OverrideBuildings   types.Bool               `tfsdk:"override_buildings"`
	OverrideDepartments types.Bool               `tfsdk:"override_departments"`
	DistributionPoint   types.String             `tfsdk:"distribution_point"`
	DistributionServer  types.String             `tfsdk:"distribution_server"`
	SwuServer           types.String             `tfsdk:"swu_server"`
	URL                 types.String             `tfsdk:"url"`
	Timeouts            datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// networkSegmentIdentityModel represents the identity object for network segment
// resources and list results.
type networkSegmentIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// NetworkSegmentListResourceModel represents the config model for network segment list queries.
// Classic has no RSQL — the filter shape is the shared client-side substring block.
type NetworkSegmentListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}

// NetworkSegmentsDataSourceModel represents the Terraform data source model for
// network segment searches.
type NetworkSegmentsDataSourceModel struct {
	ID              types.String                           `tfsdk:"id"`
	NetworkSegments []NetworkSegmentsDataSourceResultModel `tfsdk:"network_segments"`
	Filter          *filters.ClassicFilterModel            `tfsdk:"filter"`
	Timeouts        datasourceTimeouts.Value               `tfsdk:"timeouts"`
}

// NetworkSegmentsDataSourceResultModel represents a single network segment in the
// search results. Only the fields the classic list endpoint actually returns are
// exposed — id, name, and the IP range. Per-item fields beyond these require a
// singular data source lookup.
type NetworkSegmentsDataSourceResultModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	StartingAddress types.String `tfsdk:"starting_address"`
	EndingAddress   types.String `tfsdk:"ending_address"`
}
