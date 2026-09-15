// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package baselines

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
)

// BaselinesDataSource implements the Terraform data source for mSCP baselines.
type BaselinesDataSource struct {
	client *compliancebenchmarks.Client
}

// BaselinesDataSourceModel represents the Terraform data source model for mSCP baselines.
type BaselinesDataSourceModel struct {
	Baselines []BaselineModel `tfsdk:"baselines"`
	Timeouts  timeouts.Value  `tfsdk:"timeouts"`
}

// BaselineModel represents a single mSCP baseline in the data source.
type BaselineModel struct {
	ID          types.String `tfsdk:"id"`
	BaselineID  types.String `tfsdk:"baseline_id"`
	Title       types.String `tfsdk:"title"`
	Description types.String `tfsdk:"description"`
	RuleCount   types.Int64  `tfsdk:"rule_count"`
}
