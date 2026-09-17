// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

func TestSmartComputerGroupsDataSource_Metadata(t *testing.T) {
	var resp datasource.MetadataResponse
	NewSmartComputerGroupsDataSource().(*SmartComputerGroupsDataSource).Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	if resp.TypeName != "jamfplatform_pro_smart_computer_groups" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_smart_computer_groups", resp.TypeName)
	}
}

func TestSmartComputerGroupsDataSource_Schema(t *testing.T) {
	var resp datasource.SchemaResponse
	NewSmartComputerGroupsDataSource().(*SmartComputerGroupsDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, name := range []string{"id", "timeouts", "filter", "smart_computer_groups"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	groups, ok := s.Attributes["smart_computer_groups"]
	if !ok {
		t.Fatal("missing smart_computer_groups attribute")
	}
	nested, ok := groups.(datasourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("smart_computer_groups should be a ListNestedAttribute, got %T", groups)
	}
	for _, name := range []string{"id", "platform_id", "name", "description", "site_id", "membership_count"} {
		if _, ok := nested.NestedObject.Attributes[name]; !ok {
			t.Errorf("smart_computer_groups nested object missing attribute %q", name)
		}
	}
	// The search Jamf Pro answers with carries no criteria, so the plural result
	// deliberately has none rather than implying an empty list.
	if _, ok := nested.NestedObject.Attributes["criteria"]; ok {
		t.Error("smart_computer_groups nested object must not carry criteria: the search does not report them")
	}

	if !strings.HasPrefix(s.MarkdownDescription, progroupsAlternative()) {
		t.Errorf("plural data source description must open with the device-group alternative paragraph, got:\n%s", s.MarkdownDescription)
	}
	assertNoWireVocabulary(t, "plural data source", s.MarkdownDescription)
}

// TestSmartComputerGroupFilterSelectors pins the filterable fields to what Jamf
// Pro accepts on this search. They keep their API-native spelling, which is the
// one exemption from the snake_case attribute rule.
func TestSmartComputerGroupFilterSelectors(t *testing.T) {
	want := []string{"id", "name", "siteId"}
	if len(SmartComputerGroupFilterSelectors) != len(want) {
		t.Fatalf("expected %d selectors, got %v", len(want), SmartComputerGroupFilterSelectors)
	}
	for i, selector := range want {
		if SmartComputerGroupFilterSelectors[i] != selector {
			t.Errorf("selector %d: expected %q, got %q", i, selector, SmartComputerGroupFilterSelectors[i])
		}
	}
}
