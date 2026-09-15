// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
)

// buildDeviceGroupsRequest extracts the device group IDs the user supplied.
func buildDeviceGroupsRequest(data *BenchmarkResourceModel) []string {
	if data.TargetDeviceGroups.IsNull() || data.TargetDeviceGroups.IsUnknown() {
		return nil
	}
	elements := data.TargetDeviceGroups.Elements()
	out := make([]string, 0, len(elements))
	for _, el := range elements {
		s, ok := el.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		out = append(out, s.ValueString())
	}
	return out
}

// buildBenchmarkRequest constructs the API request body from the Terraform plan model.
// Sources are omitted deliberately:
// the server always derives the full source set from the baseline, so the request
// carries no sources field. Selected OS versions are sent only when configured;
// when omitted the server defaults the benchmark to every available version.
func buildBenchmarkRequest(data *BenchmarkResourceModel) *compliancebenchmarks.BenchmarkRequestV2 {
	reqBody := &compliancebenchmarks.BenchmarkRequestV2{
		Title:              data.Title.ValueString(),
		Description:        data.Description.ValueStringPointer(),
		SourceBaselineID:   data.SourceBaselineID.ValueString(),
		SelectedOsVersions: buildSelectedOsVersionsRequest(data),
		Rules:              make([]compliancebenchmarks.RuleRequest, len(data.Rules)),
		Target: compliancebenchmarks.TargetV2{
			DeviceGroups: buildDeviceGroupsRequest(data),
		},
		EnforcementMode: data.EnforcementMode.ValueString(),
	}
	for i, rule := range data.Rules {
		rr := compliancebenchmarks.RuleRequest{
			ID:      rule.ID.ValueString(),
			Enabled: rule.Enabled.ValueBool(),
		}
		if v := rule.ODVValue.ValueString(); v != "" {
			rr.ODV = &compliancebenchmarks.ODVRequest{Value: v}
		}
		reqBody.Rules[i] = rr
	}
	return reqBody
}

// buildSelectedOsVersionsRequest converts the configured OS-version objects into
// the API's OS-version pairs. Returns nil when the attribute is null or unknown
// (user omitted it) so the request omits the field and the server defaults the
// benchmark to every version available for the baseline.
func buildSelectedOsVersionsRequest(data *BenchmarkResourceModel) *[]compliancebenchmarks.OsVersion {
	if data.SelectedOsVersions.IsNull() || data.SelectedOsVersions.IsUnknown() {
		return nil
	}
	elements := data.SelectedOsVersions.Elements()
	out := make([]compliancebenchmarks.OsVersion, 0, len(elements))
	for _, el := range elements {
		obj, ok := el.(types.Object)
		if !ok || obj.IsNull() || obj.IsUnknown() {
			continue
		}
		attrs := obj.Attributes()
		osType, _ := attrs["os_type"].(types.String)
		osVersion, _ := attrs["os_version"].(types.Int64)
		out = append(out, compliancebenchmarks.OsVersion{
			OsType:    osType.ValueString(),
			OsVersion: int(osVersion.ValueInt64()),
		})
	}
	return &out
}
