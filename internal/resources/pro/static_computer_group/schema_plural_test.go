// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func TestStaticComputerGroupsDataSource_Metadata(t *testing.T) {
	var resp datasource.MetadataResponse
	NewStaticComputerGroupsDataSource().(*StaticComputerGroupsDataSource).Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	if resp.TypeName != "jamfplatform_pro_static_computer_groups" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_static_computer_groups", resp.TypeName)
	}
}

func TestStaticComputerGroupsDataSource_Schema(t *testing.T) {
	var resp datasource.SchemaResponse
	NewStaticComputerGroupsDataSource().(*StaticComputerGroupsDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	s := resp.Schema

	for _, name := range []string{"id", "timeouts", "filter", "static_computer_groups"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if !strings.HasPrefix(s.MarkdownDescription, deviceGroupAlternative) {
		t.Error("plural data source description must open with the shared jamfplatform_device_group paragraph")
	}

	groups, ok := s.Attributes["static_computer_groups"].(datasourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("static_computer_groups must be a ListNestedAttribute, got %T", s.Attributes["static_computer_groups"])
	}
	for _, name := range []string{"id", "platform_id", "name", "description", "site_id", "member_count"} {
		if _, ok := groups.NestedObject.Attributes[name]; !ok {
			t.Errorf("static_computer_groups nested missing attribute %q", name)
		}
	}
	// Membership identifiers cost one request per group, so they belong to the
	// singular data source. The count comes free with the search result.
	if _, ok := groups.NestedObject.Attributes["assigned_computer_ids"]; ok {
		t.Error("static_computer_groups must not carry assigned_computer_ids — that is one extra request per group")
	}
}

func TestStaticComputerGroupFilterSelectors(t *testing.T) {
	want := map[string]bool{"id": true, "name": true, "siteId": true}
	if len(StaticComputerGroupFilterSelectors) != len(want) {
		t.Fatalf("expected %d selectors, got %v", len(want), StaticComputerGroupFilterSelectors)
	}
	for _, selector := range StaticComputerGroupFilterSelectors {
		if !want[selector] {
			t.Errorf("unexpected filter selector %q", selector)
		}
	}
}
