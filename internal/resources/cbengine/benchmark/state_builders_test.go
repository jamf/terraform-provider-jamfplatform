// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package benchmark

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/compliancebenchmarks"
)

func TestAssignBenchmarkModelFromResponse_Full(t *testing.T) {
	// Pre-populate the singular slot so assignTargetDeviceGroups takes the
	// plural set from the API response.
	model := &BenchmarkResourceModel{
		TargetDeviceGroups: types.SetNull(types.StringType),
	}
	lastUpdated, _ := time.Parse(time.RFC3339, "2025-06-15T10:30:00Z")
	bench := &compliancebenchmarks.BenchmarkResponseV2{
		BenchmarkID:     "bench-1",
		TenantID:        "tenant-1",
		Title:           "Test Benchmark",
		Description:     "A test benchmark",
		EnforcementMode: "audit",
		Deleted:         false,
		UpdateAvailable: true,
		LastUpdatedAt:   lastUpdated,
		Sources: []compliancebenchmarks.Source{
			{Branch: "main", Revision: "abc123"},
		},
		SelectedOsVersions: []compliancebenchmarks.OsVersion{
			{OsType: "MAC_OS", OsVersion: 26},
		},
		AvailableOsVersions: []compliancebenchmarks.OsVersion{
			{OsType: "MAC_OS", OsVersion: 26},
			{OsType: "MAC_OS", OsVersion: 15},
		},
		Target: &compliancebenchmarks.TargetV2{
			DeviceGroups: []string{"group-1"},
		},
		Rules: []compliancebenchmarks.RuleInfo{
			{
				ID:          "rule-1",
				SectionName: "Section A",
				Enabled:     true,
				Title:       "Rule Title",
				Description: "Rule Description",
				References:  &[]string{"ref-1", "ref-2"},
				ODV: &compliancebenchmarks.OrganizationDefinedValue{
					Value:       "30",
					Hint:        "Enter a number",
					Placeholder: "30",
					Type:        "int",
					Validation: &compliancebenchmarks.ValidationConstraints{
						Min:        1,
						Max:        100,
						EnumValues: []string{"10", "20", "30"},
						Regex:      `^\d+$`,
					},
				},
				SupportedOs: []compliancebenchmarks.OsInfo{
					{OsType: "macOS", OsVersion: 15, ManagementType: "SUPERVISED"},
				},
				OsSpecificDefaults: map[string]compliancebenchmarks.OsSpecificRuleInfo{
					"macOS_15": {
						Title:       "macOS 15 Rule",
						Description: "macOS 15 specific",
						ODV: &compliancebenchmarks.ODVRecommendation{
							Value: "30",
							Hint:  "Recommended: 30",
						},
					},
				},
				RuleRelation: &compliancebenchmarks.RuleRelation{
					DependsOn: []string{"rule-0"},
				},
			},
		},
	}

	assignBenchmarkModelFromResponse(model, bench)

	if model.ID.ValueString() != "bench-1" {
		t.Errorf("expected ID 'bench-1', got %q", model.ID.ValueString())
	}
	if model.Title.ValueString() != "Test Benchmark" {
		t.Errorf("expected Title 'Test Benchmark', got %q", model.Title.ValueString())
	}
	if model.Description.ValueString() != "A test benchmark" {
		t.Errorf("expected Description 'A test benchmark', got %q", model.Description.ValueString())
	}
	if model.TenantID.ValueString() != "tenant-1" {
		t.Errorf("expected TenantID 'tenant-1', got %q", model.TenantID.ValueString())
	}
	if model.EnforcementMode.ValueString() != "audit" {
		t.Errorf("expected EnforcementMode 'audit', got %q", model.EnforcementMode.ValueString())
	}
	if model.Deleted.ValueBool() != false {
		t.Errorf("expected Deleted false, got %v", model.Deleted.ValueBool())
	}
	if model.UpdateAvailable.ValueBool() != true {
		t.Errorf("expected UpdateAvailable true, got %v", model.UpdateAvailable.ValueBool())
	}
	if model.LastUpdatedAt.ValueString() != "2025-06-15T10:30:00Z" {
		t.Errorf("expected LastUpdatedAt '2025-06-15T10:30:00Z', got %q", model.LastUpdatedAt.ValueString())
	}
	if got := setStrings(model.TargetDeviceGroups); len(got) != 1 || got[0] != "group-1" {
		t.Errorf("expected TargetDeviceGroups [group-1], got %v", got)
	}
	if l := len(model.Sources.Elements()); l != 1 {
		t.Fatalf("expected 1 source, got %d", l)
	}
	srcObj, ok := model.Sources.Elements()[0].(types.Object)
	if !ok {
		t.Fatalf("expected source element to be types.Object, got %T", model.Sources.Elements()[0])
	}
	if branch := srcObj.Attributes()["branch"].(types.String).ValueString(); branch != "main" {
		t.Errorf("expected source branch 'main', got %q", branch)
	}
	if rev := srcObj.Attributes()["revision"].(types.String).ValueString(); rev != "abc123" {
		t.Errorf("expected source revision 'abc123', got %q", rev)
	}

	if model.SelectedOsVersions.IsNull() || len(model.SelectedOsVersions.Elements()) != 1 {
		t.Fatalf("expected 1 selected OS version, got %v", model.SelectedOsVersions)
	}
	selObj := model.SelectedOsVersions.Elements()[0].(types.Object)
	if selObj.Attributes()["os_type"].(types.String).ValueString() != "MAC_OS" {
		t.Errorf("expected selected os_type MAC_OS, got %v", selObj.Attributes()["os_type"])
	}
	if selObj.Attributes()["os_version"].(types.Int64).ValueInt64() != 26 {
		t.Errorf("expected selected os_version 26, got %v", selObj.Attributes()["os_version"])
	}
	if model.AvailableOsVersions.IsNull() || len(model.AvailableOsVersions.Elements()) != 2 {
		t.Errorf("expected 2 available OS versions, got %v", model.AvailableOsVersions)
	}

	if len(model.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(model.Rules))
	}
	rule := model.Rules[0]
	if rule.ID.ValueString() != "rule-1" {
		t.Errorf("expected rule ID 'rule-1', got %q", rule.ID.ValueString())
	}
	if rule.SectionName.ValueString() != "Section A" {
		t.Errorf("expected SectionName 'Section A', got %q", rule.SectionName.ValueString())
	}
	if rule.Enabled.ValueBool() != true {
		t.Errorf("expected Enabled true, got %v", rule.Enabled.ValueBool())
	}
	if rule.Title.ValueString() != "Rule Title" {
		t.Errorf("expected Title 'Rule Title', got %q", rule.Title.ValueString())
	}
	if rule.Description.ValueString() != "Rule Description" {
		t.Errorf("expected Description 'Rule Description', got %q", rule.Description.ValueString())
	}
	if rule.ODVValue.ValueString() != "30" {
		t.Errorf("expected ODVValue '30', got %q", rule.ODVValue.ValueString())
	}
	if rule.ODVHint.ValueString() != "Enter a number" {
		t.Errorf("expected ODVHint 'Enter a number', got %q", rule.ODVHint.ValueString())
	}
	if rule.ODVPlaceholder.ValueString() != "30" {
		t.Errorf("expected ODVPlaceholder '30', got %q", rule.ODVPlaceholder.ValueString())
	}
	if rule.ODVType.ValueString() != "int" {
		t.Errorf("expected ODVType 'int', got %q", rule.ODVType.ValueString())
	}
	if rule.ODVValidationMin.ValueInt64() != 1 {
		t.Errorf("expected ODVValidationMin 1, got %d", rule.ODVValidationMin.ValueInt64())
	}
	if rule.ODVValidationMax.ValueInt64() != 100 {
		t.Errorf("expected ODVValidationMax 100, got %d", rule.ODVValidationMax.ValueInt64())
	}
	if rule.ODVValidationRegex.ValueString() != `^\d+$` {
		t.Errorf("expected ODVValidationRegex '^\\d+$', got %q", rule.ODVValidationRegex.ValueString())
	}
}

func TestAssignBenchmarkModelFromResponse_NilInputs(t *testing.T) {
	assignBenchmarkModelFromResponse(nil, &compliancebenchmarks.BenchmarkResponseV2{})
	assignBenchmarkModelFromResponse(&BenchmarkResourceModel{}, nil)
}

func TestAssignBenchmarkDataSourceFromResponse(t *testing.T) {
	model := &BenchmarkDataSourceModel{}
	lastUpdatedDS, _ := time.Parse(time.RFC3339, "2025-01-01T00:00:00Z")
	bench := &compliancebenchmarks.BenchmarkResponseV2{
		BenchmarkID:     "bench-ds-1",
		TenantID:        "tenant-2",
		Title:           "DS Benchmark",
		Description:     "Data source test",
		EnforcementMode: "enforce",
		Deleted:         true,
		UpdateAvailable: false,
		LastUpdatedAt:   lastUpdatedDS,
		Sources:         []compliancebenchmarks.Source{{Branch: "dev", Revision: "xyz"}},
		Target:          &compliancebenchmarks.TargetV2{DeviceGroups: []string{"dg-1"}},
		Rules:           nil,
	}

	assignBenchmarkDataSourceFromResponse(model, bench)

	if model.BenchmarkID.ValueString() != "bench-ds-1" {
		t.Errorf("expected BenchmarkID 'bench-ds-1', got %q", model.BenchmarkID.ValueString())
	}
	if model.TenantID.ValueString() != "tenant-2" {
		t.Errorf("expected TenantID 'tenant-2', got %q", model.TenantID.ValueString())
	}
	if model.Deleted.ValueBool() != true {
		t.Errorf("expected Deleted true, got %v", model.Deleted.ValueBool())
	}
	if model.EnforcementMode.ValueString() != "enforce" {
		t.Errorf("expected EnforcementMode 'enforce', got %q", model.EnforcementMode.ValueString())
	}
}

func TestBuildSourcesList(t *testing.T) {
	sources := []compliancebenchmarks.Source{
		{Branch: "main", Revision: "aaa"},
		{Branch: "release", Revision: "bbb"},
	}
	result := buildSourcesList(sources)
	if result.IsNull() {
		t.Fatal("expected non-null list")
	}
	if l := len(result.Elements()); l != 2 {
		t.Fatalf("expected 2 sources, got %d", l)
	}
	first := result.Elements()[0].(types.Object)
	if first.Attributes()["branch"].(types.String).ValueString() != "main" {
		t.Errorf("expected branch 'main', got %v", first.Attributes()["branch"])
	}
	second := result.Elements()[1].(types.Object)
	if second.Attributes()["revision"].(types.String).ValueString() != "bbb" {
		t.Errorf("expected revision 'bbb', got %v", second.Attributes()["revision"])
	}
}

func TestBuildSourcesList_Empty(t *testing.T) {
	result := buildSourcesList(nil)
	if result.IsNull() {
		t.Error("expected an empty (non-null) list for no sources")
	}
	if len(result.Elements()) != 0 {
		t.Errorf("expected 0 sources, got %d", len(result.Elements()))
	}
}

func TestBuildOsVersionsSet(t *testing.T) {
	result := buildOsVersionsSet([]compliancebenchmarks.OsVersion{
		{OsType: "MAC_OS", OsVersion: 26},
		{OsType: "MAC_OS", OsVersion: 15},
	})
	if result.IsNull() {
		t.Fatal("expected non-null set")
	}
	if len(result.Elements()) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result.Elements()))
	}
	obj := result.Elements()[0].(types.Object)
	if obj.Attributes()["os_type"].(types.String).ValueString() != "MAC_OS" {
		t.Errorf("expected os_type MAC_OS, got %v", obj.Attributes()["os_type"])
	}
}

func TestBuildOsVersionsSet_Empty(t *testing.T) {
	if !buildOsVersionsSet(nil).IsNull() {
		t.Error("expected null set for empty OS versions")
	}
}

func TestBuildOsVersionsList(t *testing.T) {
	result := buildOsVersionsList([]compliancebenchmarks.OsVersion{
		{OsType: "MAC_OS", OsVersion: 26},
	})
	if result.IsNull() {
		t.Fatal("expected non-null list")
	}
	if len(result.Elements()) != 1 {
		t.Fatalf("expected 1 element, got %d", len(result.Elements()))
	}
	obj := result.Elements()[0].(types.Object)
	if obj.Attributes()["os_version"].(types.Int64).ValueInt64() != 26 {
		t.Errorf("expected os_version 26, got %v", obj.Attributes()["os_version"])
	}
}

func TestBuildOsVersionsList_Empty(t *testing.T) {
	result := buildOsVersionsList(nil)
	if result.IsNull() {
		t.Error("expected an empty (non-null) list for no OS versions")
	}
	if len(result.Elements()) != 0 {
		t.Errorf("expected 0 elements, got %d", len(result.Elements()))
	}
}

func TestBuildRuleModel_NoODV(t *testing.T) {
	rule := compliancebenchmarks.RuleInfo{
		ID:          "rule-no-odv",
		SectionName: "Section B",
		Enabled:     false,
		Title:       "No ODV Rule",
		Description: "Rule without ODV",
	}
	result := buildRuleModel(rule)

	if result.ID.ValueString() != "rule-no-odv" {
		t.Errorf("expected ID 'rule-no-odv', got %q", result.ID.ValueString())
	}
	if !result.ODVValue.IsNull() {
		t.Error("expected null ODVValue when ODV is nil")
	}
	if !result.ODVHint.IsNull() {
		t.Error("expected null ODVHint when ODV is nil")
	}
	if !result.ODVValidationMin.IsNull() {
		t.Error("expected null ODVValidationMin when ODV is nil")
	}
	if !result.ODVValidationMax.IsNull() {
		t.Error("expected null ODVValidationMax when ODV is nil")
	}
	if !result.ODVValidationRegex.IsNull() {
		t.Error("expected null ODVValidationRegex when ODV is nil")
	}
}

func TestBuildRuleModel_ODVWithoutValidation(t *testing.T) {
	rule := compliancebenchmarks.RuleInfo{
		ID:      "rule-odv-no-val",
		Enabled: true,
		ODV: &compliancebenchmarks.OrganizationDefinedValue{
			Value: "custom",
			Type:  "string",
		},
	}
	result := buildRuleModel(rule)

	if result.ODVValue.ValueString() != "custom" {
		t.Errorf("expected ODVValue 'custom', got %q", result.ODVValue.ValueString())
	}
	if result.ODVType.ValueString() != "string" {
		t.Errorf("expected ODVType 'string', got %q", result.ODVType.ValueString())
	}
	if !result.ODVValidationMin.IsNull() {
		t.Error("expected null ODVValidationMin when validation is nil")
	}
	if !result.ODVValidationMax.IsNull() {
		t.Error("expected null ODVValidationMax when validation is nil")
	}
}

func TestAssignBenchmarkModelFromResponse_PluralPath(t *testing.T) {
	// Model carries TargetDeviceGroups pre-set (the plural-path signal).
	// State assignment must populate the plural set with every group the API
	// returned.
	preset, _ := types.SetValue(types.StringType, []attr.Value{types.StringValue("placeholder")})
	model := &BenchmarkResourceModel{
		TargetDeviceGroups: preset,
	}
	bench := &compliancebenchmarks.BenchmarkResponseV2{
		BenchmarkID: "bench-plural",
		Target:      &compliancebenchmarks.TargetV2{DeviceGroups: []string{"g1", "g2", "g3"}},
	}

	assignBenchmarkModelFromResponse(model, bench)

	if model.TargetDeviceGroups.IsNull() {
		t.Fatal("expected TargetDeviceGroups populated")
	}
	if len(model.TargetDeviceGroups.Elements()) != 3 {
		t.Errorf("expected 3 elements in TargetDeviceGroups, got %d", len(model.TargetDeviceGroups.Elements()))
	}
}

func TestAssignBenchmarkModelFromResponse_ImportDefaultsToPlural(t *testing.T) {
	// Both attribute slots null/unknown (typical import). State builder should
	// default to populating the plural set so imports surface every group.
	model := &BenchmarkResourceModel{
		TargetDeviceGroups: types.SetNull(types.StringType),
	}
	bench := &compliancebenchmarks.BenchmarkResponseV2{
		BenchmarkID: "bench-import",
		Target:      &compliancebenchmarks.TargetV2{DeviceGroups: []string{"g1"}},
	}

	assignBenchmarkModelFromResponse(model, bench)

	if model.TargetDeviceGroups.IsNull() || len(model.TargetDeviceGroups.Elements()) != 1 {
		t.Errorf("expected plural populated on import")
	}
}

func TestBuildTargetDeviceGroupsSet(t *testing.T) {
	result := buildTargetDeviceGroupsSet([]string{"a", "b", "c"})
	if result.IsNull() {
		t.Fatal("expected non-null set")
	}
	if len(result.Elements()) != 3 {
		t.Errorf("expected 3 elements, got %d", len(result.Elements()))
	}
}

func TestBuildTargetDeviceGroupsSet_Empty(t *testing.T) {
	result := buildTargetDeviceGroupsSet(nil)
	if !result.IsNull() {
		t.Error("expected null set for empty input")
	}
}

func TestBuildStringList(t *testing.T) {
	result := buildStringList([]string{"a", "b", "c"})
	if result.IsNull() {
		t.Fatal("expected non-null list")
	}
	elems := result.Elements()
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(elems))
	}
}

func TestBuildStringList_Empty(t *testing.T) {
	result := buildStringList(nil)
	if !result.IsNull() {
		t.Error("expected null for empty string list")
	}
}

func TestBuildSupportedOSList(t *testing.T) {
	osInfo := []compliancebenchmarks.OsInfo{
		{OsType: "macOS", OsVersion: 14, ManagementType: "SUPERVISED"},
		{OsType: "iOS", OsVersion: 17, ManagementType: "UNSUPERVISED"},
	}
	result := buildSupportedOSList(osInfo)
	if result.IsNull() {
		t.Fatal("expected non-null list")
	}
	elems := result.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
}

func TestBuildSupportedOSList_Empty(t *testing.T) {
	result := buildSupportedOSList(nil)
	if !result.IsNull() {
		t.Error("expected null for empty supported OS list")
	}
}

func TestBuildOSSpecificDefaultsMap(t *testing.T) {
	defaults := map[string]compliancebenchmarks.OsSpecificRuleInfo{
		"macOS_15": {
			Title:       "macOS 15",
			Description: "For macOS 15",
			ODV: &compliancebenchmarks.ODVRecommendation{
				Value: "recommended",
				Hint:  "Use recommended",
			},
		},
	}
	result := buildOSSpecificDefaultsMap(defaults)
	if result.IsNull() {
		t.Fatal("expected non-null map")
	}
	elems := result.Elements()
	if len(elems) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elems))
	}
}

func TestBuildOSSpecificDefaultsMap_NilODV(t *testing.T) {
	defaults := map[string]compliancebenchmarks.OsSpecificRuleInfo{
		"macOS_14": {
			Title:       "macOS 14",
			Description: "No ODV",
			ODV:         nil,
		},
	}
	result := buildOSSpecificDefaultsMap(defaults)
	if result.IsNull() {
		t.Fatal("expected non-null map even with nil ODV")
	}
}

func TestBuildOSSpecificDefaultsMap_Empty(t *testing.T) {
	result := buildOSSpecificDefaultsMap(nil)
	if !result.IsNull() {
		t.Error("expected null for empty defaults map")
	}
}

func TestOdvStringValue_NilODV(t *testing.T) {
	result := odvStringValue(nil, func(o *compliancebenchmarks.OrganizationDefinedValue) string { return o.Value })
	if !result.IsNull() {
		t.Error("expected null when ODV is nil")
	}
}

func TestOdvStringValue_WithODV(t *testing.T) {
	odv := &compliancebenchmarks.OrganizationDefinedValue{Value: "test-val", Hint: "test-hint"}
	result := odvStringValue(odv, func(o *compliancebenchmarks.OrganizationDefinedValue) string { return o.Hint })
	if result.ValueString() != "test-hint" {
		t.Errorf("expected 'test-hint', got %q", result.ValueString())
	}
}

func TestBuildODVValidationEnumValues(t *testing.T) {
	odv := &compliancebenchmarks.OrganizationDefinedValue{
		Validation: &compliancebenchmarks.ValidationConstraints{
			EnumValues: []string{"low", "medium", "high"},
		},
	}
	result := buildODVValidationEnumValues(odv)
	if result.IsNull() {
		t.Fatal("expected non-null enum values list")
	}
	elems := result.Elements()
	if len(elems) != 3 {
		t.Fatalf("expected 3 enum values, got %d", len(elems))
	}
}

func TestBuildODVValidationEnumValues_Empty(t *testing.T) {
	odv := &compliancebenchmarks.OrganizationDefinedValue{
		Validation: &compliancebenchmarks.ValidationConstraints{},
	}
	result := buildODVValidationEnumValues(odv)
	if !result.IsNull() {
		t.Error("expected null for empty enum values")
	}
}

func TestBuildODVValidationEnumValues_NilODV(t *testing.T) {
	result := buildODVValidationEnumValues(nil)
	if !result.IsNull() {
		t.Error("expected null for nil ODV")
	}
}

func TestBuildDependsOnList(t *testing.T) {
	relation := &compliancebenchmarks.RuleRelation{
		DependsOn: []string{"dep-1", "dep-2"},
	}
	result := buildDependsOnList(relation)
	if result.IsNull() {
		t.Fatal("expected non-null depends_on list")
	}
	elems := result.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 depends_on entries, got %d", len(elems))
	}
}

func TestBuildDependsOnList_NilRelation(t *testing.T) {
	result := buildDependsOnList(nil)
	if !result.IsNull() {
		t.Error("expected null for nil relation")
	}
}

func TestBuildDependsOnList_EmptyDependsOn(t *testing.T) {
	relation := &compliancebenchmarks.RuleRelation{DependsOn: nil}
	result := buildDependsOnList(relation)
	if !result.IsNull() {
		t.Error("expected null for empty depends_on")
	}
}

func TestBuildODVValidationRegex_NilValidation(t *testing.T) {
	result := buildODVValidationRegex(nil)
	if !result.IsNull() {
		t.Error("expected null for nil ODV")
	}

	odv := &compliancebenchmarks.OrganizationDefinedValue{}
	result = buildODVValidationRegex(odv)
	if !result.IsNull() {
		t.Error("expected null for nil validation")
	}
}

func TestBuildODVValidationRegex_WithRegex(t *testing.T) {
	odv := &compliancebenchmarks.OrganizationDefinedValue{
		Validation: &compliancebenchmarks.ValidationConstraints{
			Regex: `^[a-z]+$`,
		},
	}
	result := buildODVValidationRegex(odv)
	if result.ValueString() != `^[a-z]+$` {
		t.Errorf("expected regex '^[a-z]+$', got %q", result.ValueString())
	}
}

func TestBuildODVValidationMin_NilChain(t *testing.T) {
	if !buildODVValidationMin(nil).IsNull() {
		t.Error("expected null for nil ODV")
	}
	if !buildODVValidationMin(&compliancebenchmarks.OrganizationDefinedValue{}).IsNull() {
		t.Error("expected null for nil validation")
	}
	result := buildODVValidationMin(&compliancebenchmarks.OrganizationDefinedValue{
		Validation: &compliancebenchmarks.ValidationConstraints{},
	})
	if result.IsNull() {
		t.Error("expected non-null Int64 when validation is set")
	}
	if result.ValueInt64() != 0 {
		t.Errorf("expected 0 for default min, got %d", result.ValueInt64())
	}
}

func TestBuildODVValidationMax_NilChain(t *testing.T) {
	if !buildODVValidationMax(nil).IsNull() {
		t.Error("expected null for nil ODV")
	}
	if !buildODVValidationMax(&compliancebenchmarks.OrganizationDefinedValue{}).IsNull() {
		t.Error("expected null for nil validation")
	}
	result := buildODVValidationMax(&compliancebenchmarks.OrganizationDefinedValue{
		Validation: &compliancebenchmarks.ValidationConstraints{Max: 50},
	})
	if result.ValueInt64() != 50 {
		t.Errorf("expected max 50, got %d", result.ValueInt64())
	}
}

// suppress unused import warning for types package
var _ = types.StringNull

// setStrings flattens a types.Set of strings for assertion purposes.
func setStrings(set types.Set) []string {
	out := make([]string, 0, len(set.Elements()))
	for _, el := range set.Elements() {
		if v, ok := el.(types.String); ok {
			out = append(out, v.ValueString())
		}
	}
	return out
}
