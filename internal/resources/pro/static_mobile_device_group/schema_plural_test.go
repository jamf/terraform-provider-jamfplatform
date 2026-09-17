// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func TestStaticMobileDeviceGroupsDataSource_Metadata(t *testing.T) {
	d := NewStaticMobileDeviceGroupsDataSource()
	var resp datasource.MetadataResponse
	d.(*StaticMobileDeviceGroupsDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_pro_static_mobile_device_groups" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_static_mobile_device_groups", resp.TypeName)
	}
}

func TestStaticMobileDeviceGroupsDataSource_Schema(t *testing.T) {
	d := NewStaticMobileDeviceGroupsDataSource()
	var resp datasource.SchemaResponse
	d.(*StaticMobileDeviceGroupsDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	s := resp.Schema

	for _, name := range []string{"id", "timeouts", "filter", "static_mobile_device_groups"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	groups, ok := s.Attributes["static_mobile_device_groups"].(datasourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("static_mobile_device_groups should be a ListNestedAttribute, got %T", s.Attributes["static_mobile_device_groups"])
	}
	for _, name := range []string{"id", "platform_id", "name", "description", "site_id", "member_count"} {
		if _, ok := groups.NestedObject.Attributes[name]; !ok {
			t.Errorf("nested result missing attribute %q", name)
		}
	}

	// Membership would cost one request per group, so the plural shape omits it
	// and the singular data source is where a caller asks for it.
	if _, ok := groups.NestedObject.Attributes["assigned_mobile_device_ids"]; ok {
		t.Error("the plural result must not carry membership")
	}
}

func TestFilterSelectors_MatchTheSearchableFields(t *testing.T) {
	want := map[string]bool{"groupId": true, "groupName": true, "siteId": true}
	if len(FilterSelectors) != len(want) {
		t.Fatalf("expected %d selectors, got %v", len(want), FilterSelectors)
	}
	for _, s := range FilterSelectors {
		if !want[s] {
			t.Errorf("unexpected selector %q", s)
		}
	}
}
