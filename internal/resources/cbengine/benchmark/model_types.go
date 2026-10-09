// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	datasourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/datasource/timeouts"
	resourceTimeouts "github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/listtimeouts"
)

// SourceModel represents a source branch and revision for a benchmark.
type SourceModel struct {
	Branch   types.String `tfsdk:"branch"`
	Revision types.String `tfsdk:"revision"`
}

// BenchmarkResourceModel represents the Terraform resource model for a Jamf Compliance Benchmark.
type BenchmarkResourceModel struct {
	ID                  types.String           `tfsdk:"id"`
	Title               types.String           `tfsdk:"title"`
	Description         types.String           `tfsdk:"description"`
	SourceBaselineID    types.String           `tfsdk:"source_baseline_id"`
	Sources             types.List             `tfsdk:"sources"`
	SelectedOsVersions  types.Set              `tfsdk:"selected_os_versions"`
	AvailableOsVersions types.List             `tfsdk:"available_os_versions"`
	Rules               []RuleModel            `tfsdk:"rules"`
	TargetDeviceGroups  types.Set              `tfsdk:"target_device_groups"`
	EnforcementMode     types.String           `tfsdk:"enforcement_mode"`
	TenantID            types.String           `tfsdk:"tenant_id"`
	Deleted             types.Bool             `tfsdk:"deleted"`
	UpdateAvailable     types.Bool             `tfsdk:"update_available"`
	CanSwitchToEnforce  types.Bool             `tfsdk:"can_switch_to_enforce"`
	LastUpdatedAt       types.String           `tfsdk:"last_updated_at"`
	Timeouts            resourceTimeouts.Value `tfsdk:"timeouts"`
}

// BenchmarkDataSourceModel represents the Terraform data source model for a Jamf Compliance Benchmark.
type BenchmarkDataSourceModel struct {
	ID                  types.String             `tfsdk:"id"`
	Title               types.String             `tfsdk:"title"`
	BenchmarkID         types.String             `tfsdk:"benchmark_id"`
	TenantID            types.String             `tfsdk:"tenant_id"`
	Description         types.String             `tfsdk:"description"`
	Sources             types.List               `tfsdk:"sources"`
	SelectedOsVersions  types.Set                `tfsdk:"selected_os_versions"`
	AvailableOsVersions types.List               `tfsdk:"available_os_versions"`
	Rules               []RuleModel              `tfsdk:"rules"`
	TargetDeviceGroups  types.Set                `tfsdk:"target_device_groups"`
	EnforcementMode     types.String             `tfsdk:"enforcement_mode"`
	Deleted             types.Bool               `tfsdk:"deleted"`
	UpdateAvailable     types.Bool               `tfsdk:"update_available"`
	CanSwitchToEnforce  types.Bool               `tfsdk:"can_switch_to_enforce"`
	LastUpdatedAt       types.String             `tfsdk:"last_updated_at"`
	Timeouts            datasourceTimeouts.Value `tfsdk:"timeouts"`
}

// RuleModel represents a rule in the benchmark, including ODV and computed fields.
type RuleModel struct {
	ID                      types.String `tfsdk:"id"`
	SectionName             types.String `tfsdk:"section_name"`
	Enabled                 types.Bool   `tfsdk:"enabled"`
	Title                   types.String `tfsdk:"title"`
	Description             types.String `tfsdk:"description"`
	References              types.List   `tfsdk:"references"`
	ODVValue                types.String `tfsdk:"odv_value"`
	ODVHint                 types.String `tfsdk:"odv_hint"`
	ODVPlaceholder          types.String `tfsdk:"odv_placeholder"`
	ODVType                 types.String `tfsdk:"odv_type"`
	ODVValidationMin        types.Int64  `tfsdk:"odv_validation_min"`
	ODVValidationMax        types.Int64  `tfsdk:"odv_validation_max"`
	ODVValidationEnumValues types.List   `tfsdk:"odv_validation_enum_values"`
	ODVValidationRegex      types.String `tfsdk:"odv_validation_regex"`
	SupportedOS             types.List   `tfsdk:"supported_os"`
	OSSpecificDefaults      types.Map    `tfsdk:"os_specific_defaults"`
	DependsOn               types.List   `tfsdk:"depends_on"`
	Reportable              types.Bool   `tfsdk:"reportable"`
	SmartCard               types.Bool   `tfsdk:"smart_card"`
}

// benchmarkIdentityModel represents the identity for benchmark resources and list results.
type benchmarkIdentityModel struct {
	ID types.String `tfsdk:"id"`
}

// BenchmarkListResourceModel represents the config model for benchmark list queries.
type BenchmarkListResourceModel struct {
	Search   types.String       `tfsdk:"search"`
	Timeouts listtimeouts.Value `tfsdk:"timeouts"`
}
