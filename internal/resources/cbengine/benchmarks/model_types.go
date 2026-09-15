// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmarks

import (
	"github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
)

// BenchmarksDataSource implements the Terraform data source for listing CBEngine benchmarks.
type BenchmarksDataSource struct {
	client *compliancebenchmarks.Client
}

// BenchmarksDataSourceModel represents the state for the benchmarks listing data source.
type BenchmarksDataSourceModel struct {
	ID         types.String        `tfsdk:"id"`
	Benchmarks []BenchmarkListItem `tfsdk:"benchmarks"`
	Timeouts   timeouts.Value      `tfsdk:"timeouts"`
}

// BenchmarkListItem captures the summary fields returned by ListBenchmarks.
type BenchmarkListItem struct {
	ID                 types.String `tfsdk:"id"`
	Title              types.String `tfsdk:"title"`
	Description        types.String `tfsdk:"description"`
	UpdateAvailable    types.Bool   `tfsdk:"update_available"`
	Modified           types.Bool   `tfsdk:"modified"`
	SyncState          types.String `tfsdk:"sync_state"`
	TargetDeviceGroups types.List   `tfsdk:"target_device_groups"`
}
