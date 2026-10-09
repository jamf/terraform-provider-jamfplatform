// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ibeacon

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/filters"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// IbeaconResourceModel represents the Terraform resource model for a Jamf Pro iBeacon.
type IbeaconResourceModel struct {
	ID                   types.String           `tfsdk:"id"`
	Name                 types.String           `tfsdk:"name"`
	UUID                 types.String           `tfsdk:"uuid"`
	Major                types.Int64            `tfsdk:"major"`
	Minor                types.Int64            `tfsdk:"minor"`
	IncludeAnyMajorValue types.Bool             `tfsdk:"include_any_major_value"`
	IncludeAnyMinorValue types.Bool             `tfsdk:"include_any_minor_value"`
	Timeouts             resourceTimeouts.Value `tfsdk:"timeouts"`
}

// IbeaconDataSourceModel represents the Terraform data source model for a Jamf Pro
// iBeacon. Either id or name must be supplied (enforced by ExactlyOneOf at config
// validation).
type IbeaconDataSourceModel struct {
	ID                   types.String             `tfsdk:"id"`
	Name                 types.String             `tfsdk:"name"`
	UUID                 types.String             `tfsdk:"uuid"`
	Major                types.Int64              `tfsdk:"major"`
	Minor                types.Int64              `tfsdk:"minor"`
	IncludeAnyMajorValue types.Bool               `tfsdk:"include_any_major_value"`
	IncludeAnyMinorValue types.Bool               `tfsdk:"include_any_minor_value"`
	Timeouts             datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// ibeaconIdentityModel represents the identity object for iBeacon resources and
// list results.
type ibeaconIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// IbeaconListResourceModel represents the config model for iBeacon list queries.
// Classic has no RSQL — the filter shape is the shared client-side substring block.
type IbeaconListResourceModel struct {
	Filter   *filters.ClassicFilterModel `tfsdk:"filter"`
	Timeouts listtimeouts.Value          `tfsdk:"timeouts"`
}
