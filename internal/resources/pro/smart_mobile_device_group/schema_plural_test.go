// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

func TestPluralDataSource_Metadata(t *testing.T) {
	var resp datasource.MetadataResponse
	NewSmartMobileDeviceGroupsDataSource().(*SmartMobileDeviceGroupsDataSource).Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	if resp.TypeName != "jamfplatform_pro_smart_mobile_device_groups" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_smart_mobile_device_groups", resp.TypeName)
	}
}

func TestPluralDataSource_Schema(t *testing.T) {
	var resp datasource.SchemaResponse
	NewSmartMobileDeviceGroupsDataSource().(*SmartMobileDeviceGroupsDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, name := range []string{"id", "timeouts", "filter", "smart_mobile_device_groups"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	nested, ok := s.Attributes["smart_mobile_device_groups"].(datasourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("smart_mobile_device_groups should be a ListNestedAttribute, got %T", s.Attributes["smart_mobile_device_groups"])
	}
	for _, name := range []string{"id", "platform_id", "name", "description", "site_id", "member_count"} {
		if _, ok := nested.NestedObject.Attributes[name]; !ok {
			t.Errorf("result object missing attribute %q", name)
		}
	}
	// The collection read carries no criteria, so a criteria attribute here would
	// always be empty and would read as "this group has none".
	if _, ok := nested.NestedObject.Attributes["criteria"]; ok {
		t.Error("criteria must not be on the plural results: the collection read does not carry it")
	}
}

func TestPluralDataSource_DescriptionOpensWithTheDeviceGroupAlternative(t *testing.T) {
	var resp datasource.SchemaResponse
	NewSmartMobileDeviceGroupsDataSource().(*SmartMobileDeviceGroupsDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	want := progroups.DeviceGroupAlternative(progroups.DeviceTypeMobile, progroups.GroupKindSmart)
	if !strings.HasPrefix(resp.Schema.MarkdownDescription, want) {
		t.Errorf("plural data source description must open with the jamfplatform_device_group advice, got:\n%s", resp.Schema.MarkdownDescription)
	}
}

// TestFilterSelectorsAreTheCollectionsOwn pins the filterable fields to the ones
// the collection read documents. They keep their API-native spelling because
// they travel to Jamf Pro verbatim, which is the one exception to the
// snake_case rule.
func TestFilterSelectorsAreTheCollectionsOwn(t *testing.T) {
	for _, want := range []string{"groupId", "groupName", "siteId"} {
		if !slices.Contains(SmartMobileDeviceGroupFilterSelectors, want) {
			t.Errorf("filter selectors missing %q", want)
		}
	}
	if len(SmartMobileDeviceGroupFilterSelectors) != 3 {
		t.Errorf("expected exactly the three documented selectors, got %v", SmartMobileDeviceGroupFilterSelectors)
	}
}
