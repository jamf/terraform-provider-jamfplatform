// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_computer_group

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

func resourceSchema(t *testing.T) resourceschema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	NewSmartComputerGroupResource().(*SmartComputerGroupResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestSmartComputerGroupResource_Metadata(t *testing.T) {
	var resp resource.MetadataResponse
	NewSmartComputerGroupResource().(*SmartComputerGroupResource).Metadata(
		context.Background(),
		resource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	if resp.TypeName != "jamfplatform_pro_smart_computer_group" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_smart_computer_group", resp.TypeName)
	}
	if resp.TypeName != terraformName {
		t.Errorf("Metadata type name %q disagrees with terraformName %q, which the Configure gates use", resp.TypeName, terraformName)
	}
}

func TestSmartComputerGroupResource_SchemaAttributes(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range []string{"id", "platform_id", "name", "description", "site_id", "criteria", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	// The maintainer ruled these two out by name: `.id` IS the Jamf Pro id, and
	// the site is referenced by id alone.
	for _, name := range []string{"site_name", "jamf_pro_id"} {
		if _, ok := s.Attributes[name]; ok {
			t.Errorf("attribute %q must not exist", name)
		}
	}

	if id := s.Attributes["id"]; id.IsRequired() || !id.IsComputed() {
		t.Errorf("id must be computed-only, got required=%v computed=%v", id.IsRequired(), id.IsComputed())
	}
	if platformID := s.Attributes["platform_id"]; platformID.IsRequired() || platformID.IsOptional() || !platformID.IsComputed() {
		t.Errorf("platform_id must be computed-only, got required=%v optional=%v computed=%v",
			platformID.IsRequired(), platformID.IsOptional(), platformID.IsComputed())
	}
	if name := s.Attributes["name"]; !name.IsRequired() {
		t.Error("name must be required")
	}
	for _, attrName := range []string{"description", "site_id"} {
		attr := s.Attributes[attrName]
		if !attr.IsOptional() || !attr.IsComputed() {
			t.Errorf("%s must be optional and computed, got optional=%v computed=%v", attrName, attr.IsOptional(), attr.IsComputed())
		}
	}
	if c := s.Attributes["criteria"]; !c.IsOptional() || c.IsComputed() {
		t.Errorf("criteria must be optional-only, got optional=%v computed=%v", c.IsOptional(), c.IsComputed())
	}
}

// TestSmartComputerGroupResource_CriteriaIsAList pins STYLE_GUIDE §Sets vs
// Lists for this attribute. Priority, parentheses and the and_or joins are all
// positional, and modelling the same shape as a Set scrambled the order and
// broke plan/state correlation on device_group.
func TestSmartComputerGroupResource_CriteriaIsAList(t *testing.T) {
	s := resourceSchema(t)
	criteria, ok := s.Attributes["criteria"]
	if !ok {
		t.Fatal("missing criteria attribute")
	}
	nested, ok := criteria.(resourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("criteria must be a ListNestedAttribute, got %T", criteria)
	}
	for _, name := range []string{"priority", "name", "search_type", "value", "and_or", "has_opening_parenthesis", "has_closing_parenthesis"} {
		if _, ok := nested.NestedObject.Attributes[name]; !ok {
			t.Errorf("criteria nested object missing attribute %q", name)
		}
	}
}

// TestSmartComputerGroupResource_DescriptionOpensWithTheAlternative pins the
// family rule that every description leads with the jamfplatform_device_group
// advice, and that no wire vocabulary reaches user-facing text.
func TestSmartComputerGroupResource_DescriptionOpensWithTheAlternative(t *testing.T) {
	s := resourceSchema(t)
	opener := progroupsAlternative()
	if !strings.HasPrefix(s.MarkdownDescription, opener) {
		t.Errorf("resource description must open with the device-group alternative paragraph, got:\n%s", s.MarkdownDescription)
	}
	assertNoWireVocabulary(t, "resource", s.MarkdownDescription)
	for name, attr := range s.Attributes {
		assertNoWireVocabulary(t, name, attr.GetMarkdownDescription())
	}
}

// TestSmartComputerGroupResource_DescriptionNamesTheKnownLimitation keeps three
// things in front of an operator, in product terms: which pairings Jamf Pro
// refuses, that the provider saves such a group anyway so no configuration
// change is called for, and the one check that is weaker when it does.
//
// The last of those is the reason this is asserted rather than trusted. A group
// saved that way can silently match nobody, and an operator who is not told
// where to look has no way to tell that from a criterion that is simply narrow.
func TestSmartComputerGroupResource_DescriptionNamesTheKnownLimitation(t *testing.T) {
	s := resourceSchema(t)
	for _, want := range []string{
		"patch reporting criterion",
		"extension attribute",
		"another way",
		"no members",
		"PI-1183",
	} {
		if !strings.Contains(s.MarkdownDescription, want) {
			t.Errorf("resource description does not mention %q:\n%s", want, s.MarkdownDescription)
		}
	}
	if strings.Contains(s.MarkdownDescription, "by hand") {
		t.Errorf("the description still tells an operator to make such a group by hand, which the fallback write path removed the need for:\n%s", s.MarkdownDescription)
	}
}

// TestSmartComputerGroupCriterionJoinDirection pins which neighbour a
// criterion's and_or value joins. Jamf Pro offers the selector from the second
// criterion onwards, so the value belongs to the join with the criterion before
// it and the first criterion's value is never used. Both descriptions said the
// opposite once, while the package's own example relied on the backward join to
// mean what its comment claimed, so the wording is asserted rather than trusted.
func TestSmartComputerGroupCriterionJoinDirection(t *testing.T) {
	const (
		want = "the one before it"
		bad  = "the one after it"
	)

	s := resourceSchema(t)
	if !strings.Contains(s.MarkdownDescription, want) {
		t.Errorf("resource description must say a criterion joins to %q:\n%s", want, s.MarkdownDescription)
	}
	if strings.Contains(s.MarkdownDescription, bad) {
		t.Errorf("resource description says a criterion joins to %q, which is the wrong neighbour:\n%s", bad, s.MarkdownDescription)
	}

	var resp datasource.SchemaResponse
	NewSmartComputerGroupDataSource().(*SmartComputerGroupDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	criteriaAttr, ok := resp.Schema.Attributes["criteria"].(datasourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("data source criteria is %T, expected a list nested attribute", resp.Schema.Attributes["criteria"])
	}
	andOr := criteriaAttr.NestedObject.Attributes["and_or"].GetMarkdownDescription()
	if !strings.Contains(andOr, want) {
		t.Errorf("data source and_or description must say it joins to %q, got %q", want, andOr)
	}
	if strings.Contains(andOr, bad) {
		t.Errorf("data source and_or description says %q, which is the wrong neighbour, got %q", bad, andOr)
	}
}

// TestSmartComputerGroupResource_WriteTimeoutsExceedTheGatewayBudget pins the
// reason the house minute is not used: Tyk allows a write on this surface 180
// seconds, so a 60-second budget would cancel a write still in flight.
func TestSmartComputerGroupResource_WriteTimeoutsExceedTheGatewayBudget(t *testing.T) {
	const gatewayBudget = 180
	if defaultCreateTimeout.Seconds() <= gatewayBudget {
		t.Errorf("defaultCreateTimeout is %s, which does not outlast the gateway's 180s write budget", defaultCreateTimeout)
	}
	if defaultUpdateTimeout.Seconds() <= gatewayBudget {
		t.Errorf("defaultUpdateTimeout is %s, which does not outlast the gateway's 180s write budget", defaultUpdateTimeout)
	}
}

func TestSmartComputerGroupDataSource_Metadata(t *testing.T) {
	var resp datasource.MetadataResponse
	NewSmartComputerGroupDataSource().(*SmartComputerGroupDataSource).Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	if resp.TypeName != "jamfplatform_pro_smart_computer_group" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_smart_computer_group", resp.TypeName)
	}
}

func TestSmartComputerGroupDataSource_Schema(t *testing.T) {
	var resp datasource.SchemaResponse
	NewSmartComputerGroupDataSource().(*SmartComputerGroupDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, name := range []string{"id", "platform_id", "name", "description", "site_id", "criteria", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	for _, name := range []string{"id", "name"} {
		attr := s.Attributes[name]
		if !attr.IsOptional() || !attr.IsComputed() {
			t.Errorf("%s must be optional and computed so either can be the selector, got optional=%v computed=%v", name, attr.IsOptional(), attr.IsComputed())
		}
	}
	assertNoWireVocabulary(t, "data source", s.MarkdownDescription)
}

// TestSmartComputerGroupDataSource_ConfigValidators pins the exactly-one-of
// selector rule.
func TestSmartComputerGroupDataSource_ConfigValidators(t *testing.T) {
	validators := NewSmartComputerGroupDataSource().(*SmartComputerGroupDataSource).ConfigValidators(context.Background())
	if len(validators) != 1 {
		t.Fatalf("expected exactly one config validator, got %d", len(validators))
	}
}

func TestSmartComputerGroupListResource_Metadata(t *testing.T) {
	var resp resource.MetadataResponse
	NewSmartComputerGroupListResource().(*SmartComputerGroupListResource).Metadata(
		context.Background(),
		resource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	// A list resource's type name has to equal the managed resource's, so this is
	// deliberately singular even though the construct lists many groups.
	if resp.TypeName != "jamfplatform_pro_smart_computer_group" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_pro_smart_computer_group", resp.TypeName)
	}
}

func TestSmartComputerGroupListResource_Schema(t *testing.T) {
	var resp list.ListResourceSchemaResponse
	NewSmartComputerGroupListResource().(*SmartComputerGroupListResource).ListResourceConfigSchema(
		context.Background(),
		list.ListResourceSchemaRequest{},
		&resp,
	)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["filter"]; !ok {
		t.Error("list schema missing filter attribute")
	}
}

// progroupsAlternative is the paragraph every construct in this family opens
// its description with, rendered from the shared helper so the assertion cannot
// pass against a stale copy of the sentence.
func progroupsAlternative() string {
	return progroups.DeviceGroupAlternative(deviceType, groupKind)
}

// wireVocabulary is the text STYLE_GUIDE §User-facing descriptions are
// UI-aligned, not wire-aligned keeps out of a schema description. Case matters
// for the request methods: prose legitimately says "Delete", never "DELETE".
var wireVocabulary = []string{
	"/pro/v",
	"/JSSResource",
	"SDK",
	"HTTP",
	"wire",
	"endpoint",
	"classic API",
	"POST",
	"PUT",
	"DELETE",
	"GET",
}

// assertNoWireVocabulary fails when a user-facing description carries API
// plumbing. The rendered permissions section is excluded: it is generated,
// shared by every construct, and legitimately names the Platform API.
func assertNoWireVocabulary(t *testing.T, label, description string) {
	t.Helper()
	prose := description
	if idx := strings.Index(prose, "**Required Jamf permissions**"); idx >= 0 {
		prose = prose[:idx]
	}
	for _, banned := range wireVocabulary {
		if strings.Contains(prose, banned) {
			t.Errorf("%s description contains wire vocabulary %q:\n%s", label, banned, prose)
		}
	}
}
