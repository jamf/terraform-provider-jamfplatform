// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
)

// assignBenchmarkModelFromResponse maps the API response into the Terraform benchmark resource model.
// For device-group targeting, the API always returns a slice; we preserve whichever attribute the
// user originally configured (singular or plural) so post-apply state matches plan without drift.
func assignBenchmarkModelFromResponse(model *BenchmarkResourceModel, bench *compliancebenchmarks.BenchmarkResponseV2) {
	if model == nil || bench == nil {
		return
	}

	model.ID = types.StringValue(bench.BenchmarkID)
	model.Title = types.StringValue(bench.Title)
	model.Description = types.StringValue(bench.Description)
	model.TenantID = types.StringValue(bench.TenantID)
	model.Deleted = types.BoolValue(bench.Deleted)
	model.UpdateAvailable = types.BoolValue(bench.UpdateAvailable)
	model.CanSwitchToEnforce = types.BoolValue(bench.CanSwitchToEnforce)
	model.LastUpdatedAt = types.StringValue(bench.LastUpdatedAt.Format(time.RFC3339))
	model.Sources = buildSourcesList(bench.Sources)
	model.SelectedOsVersions = buildOsVersionsSet(bench.SelectedOsVersions)
	model.AvailableOsVersions = buildOsVersionsList(bench.AvailableOsVersions)
	model.Rules = buildRuleModels(bench.Rules)
	assignTargetDeviceGroups(model, bench)
	model.EnforcementMode = types.StringValue(bench.EnforcementMode)
}

// assignTargetDeviceGroups populates the benchmark's device-group targeting from
// the API response.
func assignTargetDeviceGroups(model *BenchmarkResourceModel, bench *compliancebenchmarks.BenchmarkResponseV2) {
	var apiGroups []string
	if bench.Target != nil {
		apiGroups = bench.Target.DeviceGroups
	}
	model.TargetDeviceGroups = buildTargetDeviceGroupsSet(apiGroups)
}

// assignBenchmarkDataSourceFromResponse maps the API response into the Terraform benchmark data source model.
func assignBenchmarkDataSourceFromResponse(model *BenchmarkDataSourceModel, bench *compliancebenchmarks.BenchmarkResponseV2) {
	if model == nil || bench == nil {
		return
	}

	model.BenchmarkID = types.StringValue(bench.BenchmarkID)
	model.TenantID = types.StringValue(bench.TenantID)
	model.Title = types.StringValue(bench.Title)
	model.Description = types.StringValue(bench.Description)
	model.Sources = buildSourcesList(bench.Sources)
	model.SelectedOsVersions = buildOsVersionsSet(bench.SelectedOsVersions)
	model.AvailableOsVersions = buildOsVersionsList(bench.AvailableOsVersions)
	model.Rules = buildRuleModels(bench.Rules)
	var apiGroups []string
	if bench.Target != nil {
		apiGroups = bench.Target.DeviceGroups
	}
	model.TargetDeviceGroups = buildTargetDeviceGroupsSet(apiGroups)
	model.EnforcementMode = types.StringValue(bench.EnforcementMode)
	model.Deleted = types.BoolValue(bench.Deleted)
	model.UpdateAvailable = types.BoolValue(bench.UpdateAvailable)
	model.CanSwitchToEnforce = types.BoolValue(bench.CanSwitchToEnforce)
	model.LastUpdatedAt = types.StringValue(bench.LastUpdatedAt.Format(time.RFC3339))
}

// buildSourcesList converts API source representations into a computed Terraform
// list of source objects. The server always returns the baseline's full source
// set, so this is read-only state.
func buildSourcesList(sources []compliancebenchmarks.Source) types.List {
	vals := make([]attr.Value, len(sources))
	for i, s := range sources {
		vals[i], _ = types.ObjectValue(
			sourceObjectType.AttrTypes,
			map[string]attr.Value{
				"branch":   types.StringValue(s.Branch),
				"revision": types.StringValue(s.Revision),
			},
		)
	}
	result, _ := types.ListValue(sourceObjectType, vals)
	return result
}

// osVersionObjectValue builds a single {os_type, os_version} object value.
func osVersionObjectValue(v compliancebenchmarks.OsVersion) attr.Value {
	obj, _ := types.ObjectValue(
		osVersionObjectType.AttrTypes,
		map[string]attr.Value{
			"os_type":    types.StringValue(v.OsType),
			"os_version": types.Int64Value(int64(v.OsVersion)),
		},
	)
	return obj
}

// buildOsVersionsSet converts API OS-version pairs into a Terraform set of
// {os_type, os_version} objects, returning a null set when the API responded
// with none. A set (not a list) is used because the server canonicalises the
// ordering on echo, so a set keeps configured and returned values comparable
// regardless of order.
func buildOsVersionsSet(versions []compliancebenchmarks.OsVersion) types.Set {
	if len(versions) == 0 {
		return types.SetNull(osVersionObjectType)
	}
	vals := make([]attr.Value, len(versions))
	for i, v := range versions {
		vals[i] = osVersionObjectValue(v)
	}
	result, _ := types.SetValue(osVersionObjectType, vals)
	return result
}

// buildOsVersionsList converts API OS-version pairs into a computed Terraform
// list of {os_type, os_version} objects. Used for read-only available_os_versions.
func buildOsVersionsList(versions []compliancebenchmarks.OsVersion) types.List {
	vals := make([]attr.Value, len(versions))
	for i, v := range versions {
		vals[i] = osVersionObjectValue(v)
	}
	result, _ := types.ListValue(osVersionObjectType, vals)
	return result
}

// buildRuleModels converts API rule responses into Terraform rule models.
func buildRuleModels(rules []compliancebenchmarks.RuleInfo) []RuleModel {
	result := make([]RuleModel, len(rules))
	for i, r := range rules {
		result[i] = buildRuleModel(r)
	}
	return result
}

// buildRuleModel converts a single API rule response into a Terraform rule model.
func buildRuleModel(r compliancebenchmarks.RuleInfo) RuleModel {
	return RuleModel{
		ID:                      types.StringValue(r.ID),
		SectionName:             types.StringValue(r.SectionName),
		Enabled:                 types.BoolValue(r.Enabled),
		Title:                   types.StringValue(r.Title),
		Description:             types.StringValue(r.Description),
		References:              buildStringList(derefStringSlice(r.References)),
		SupportedOS:             buildSupportedOSList(r.SupportedOs),
		OSSpecificDefaults:      buildOSSpecificDefaultsMap(r.OsSpecificDefaults),
		ODVValue:                odvStringValue(r.ODV, func(o *compliancebenchmarks.OrganizationDefinedValue) string { return o.Value }),
		ODVHint:                 odvStringValue(r.ODV, func(o *compliancebenchmarks.OrganizationDefinedValue) string { return o.Hint }),
		ODVPlaceholder:          odvStringValue(r.ODV, func(o *compliancebenchmarks.OrganizationDefinedValue) string { return o.Placeholder }),
		ODVType:                 odvStringValue(r.ODV, func(o *compliancebenchmarks.OrganizationDefinedValue) string { return o.Type }),
		ODVValidationMin:        buildODVValidationMin(r.ODV),
		ODVValidationMax:        buildODVValidationMax(r.ODV),
		ODVValidationEnumValues: buildODVValidationEnumValues(r.ODV),
		ODVValidationRegex:      buildODVValidationRegex(r.ODV),
		DependsOn:               buildDependsOnList(r.RuleRelation),
		Reportable:              types.BoolValue(r.Reportable),
		SmartCard:               types.BoolValue(r.SmartCard),
	}
}

// buildTargetDeviceGroupsSet converts the API slice into a Terraform set of strings,
// returning a null set when the API responded with no groups.
func buildTargetDeviceGroupsSet(deviceGroups []string) types.Set {
	if len(deviceGroups) == 0 {
		return types.SetNull(types.StringType)
	}
	vals := make([]attr.Value, len(deviceGroups))
	for i, g := range deviceGroups {
		vals[i] = types.StringValue(g)
	}
	result, _ := types.SetValue(types.StringType, vals)
	return result
}

// buildStringList converts a string slice into a Terraform list of strings, returning null for empty.
func derefStringSlice(s *[]string) []string {
	if s == nil {
		return nil
	}
	return *s
}

func buildStringList(values []string) types.List {
	if len(values) == 0 {
		return types.ListNull(types.StringType)
	}
	vals := make([]attr.Value, len(values))
	for i, v := range values {
		vals[i] = types.StringValue(v)
	}
	result, _ := types.ListValue(types.StringType, vals)
	return result
}

// buildSupportedOSList converts API OS info into a Terraform list of objects.
func buildSupportedOSList(supportedOS []compliancebenchmarks.OsInfo) types.List {
	if len(supportedOS) == 0 {
		return types.ListNull(osInfoObjectType)
	}
	osVals := make([]attr.Value, len(supportedOS))
	for i, osInfo := range supportedOS {
		osVals[i], _ = types.ObjectValue(
			osInfoObjectType.AttrTypes,
			map[string]attr.Value{
				"os_type":         types.StringValue(osInfo.OsType),
				"os_version":      types.Int64Value(int64(osInfo.OsVersion)),
				"management_type": types.StringValue(osInfo.ManagementType),
			},
		)
	}
	result, _ := types.ListValue(osInfoObjectType, osVals)
	return result
}

// buildOSSpecificDefaultsMap converts API OS-specific defaults into a Terraform map of objects.
func buildOSSpecificDefaultsMap(defaults map[string]compliancebenchmarks.OsSpecificRuleInfo) types.Map {
	if len(defaults) == 0 {
		return types.MapNull(osSpecificDefaultObjectType)
	}
	vals := make(map[string]attr.Value, len(defaults))
	for key, def := range defaults {
		var odvValue, odvHint types.String
		if def.ODV != nil {
			odvValue = types.StringValue(def.ODV.Value)
			odvHint = types.StringValue(def.ODV.Hint)
		} else {
			odvValue = types.StringNull()
			odvHint = types.StringNull()
		}
		vals[key], _ = types.ObjectValue(
			osSpecificDefaultObjectType.AttrTypes,
			map[string]attr.Value{
				"title":       types.StringValue(def.Title),
				"description": types.StringValue(def.Description),
				"odv_value":   odvValue,
				"odv_hint":    odvHint,
			},
		)
	}
	result, _ := types.MapValue(osSpecificDefaultObjectType, vals)
	return result
}

// odvStringValue extracts a string field from an ODV response, returning null if ODV is nil.
func odvStringValue(odv *compliancebenchmarks.OrganizationDefinedValue, getter func(*compliancebenchmarks.OrganizationDefinedValue) string) types.String {
	if odv == nil {
		return types.StringNull()
	}
	return types.StringValue(getter(odv))
}

// buildODVValidationMin extracts the min validation value from the ODV response.
func buildODVValidationMin(odv *compliancebenchmarks.OrganizationDefinedValue) types.Int64 {
	if odv == nil || odv.Validation == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(odv.Validation.Min))
}

// buildODVValidationMax extracts the max validation value from the ODV response.
func buildODVValidationMax(odv *compliancebenchmarks.OrganizationDefinedValue) types.Int64 {
	if odv == nil || odv.Validation == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(odv.Validation.Max))
}

// buildODVValidationEnumValues extracts the enum validation values from the ODV response.
func buildODVValidationEnumValues(odv *compliancebenchmarks.OrganizationDefinedValue) types.List {
	if odv == nil || odv.Validation == nil || len(odv.Validation.EnumValues) == 0 {
		return types.ListNull(types.StringType)
	}
	return buildStringList(odv.Validation.EnumValues)
}

// buildODVValidationRegex extracts the regex validation pattern from the ODV response.
func buildODVValidationRegex(odv *compliancebenchmarks.OrganizationDefinedValue) types.String {
	if odv == nil || odv.Validation == nil {
		return types.StringNull()
	}
	return types.StringValue(odv.Validation.Regex)
}

// buildDependsOnList extracts the depends_on list from a rule relation.
func buildDependsOnList(relation *compliancebenchmarks.RuleRelation) types.List {
	if relation == nil || len(relation.DependsOn) == 0 {
		return types.ListNull(types.StringType)
	}
	return buildStringList(relation.DependsOn)
}
