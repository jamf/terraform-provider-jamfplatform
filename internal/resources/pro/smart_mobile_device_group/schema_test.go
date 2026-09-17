// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package smart_mobile_device_group

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// resourceSchema builds the resource schema once for the assertions below.
func resourceSchema(t *testing.T) resourceschema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	NewSmartMobileDeviceGroupResource().(*SmartMobileDeviceGroupResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestResource_Metadata(t *testing.T) {
	var resp resource.MetadataResponse
	NewSmartMobileDeviceGroupResource().(*SmartMobileDeviceGroupResource).Metadata(
		context.Background(),
		resource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	if resp.TypeName != "jamfplatform_pro_smart_mobile_device_group" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_smart_mobile_device_group", resp.TypeName)
	}
}

func TestResource_Schema_AttributeSet(t *testing.T) {
	s := resourceSchema(t)
	for _, name := range []string{"id", "platform_id", "name", "description", "site_id", "criteria", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	// Two attributes were considered and deliberately left out: site_name,
	// because site_id is the whole contract and a derived name would be one more
	// Computed field to keep consistent, and a membership count, because Jamf Pro
	// recomputes it from inventory and it would read as drift on every refresh.
	for _, name := range []string{"site_name", "member_count", "jamf_pro_id"} {
		if _, ok := s.Attributes[name]; ok {
			t.Errorf("attribute %q must not be in the resource schema", name)
		}
	}
}

func TestResource_Schema_IdentityAttributesAreComputedOnly(t *testing.T) {
	s := resourceSchema(t)
	for _, name := range []string{"id", "platform_id"} {
		a := s.Attributes[name]
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%q must be computed-only", name)
		}
		if !a.IsComputed() {
			t.Errorf("%q must be computed", name)
		}
	}
}

func TestResource_Schema_NameIsRequired(t *testing.T) {
	s := resourceSchema(t)
	if !s.Attributes["name"].IsRequired() {
		t.Error("name must be required")
	}
}

func TestResource_Schema_OptionalComputedWithSiteDefault(t *testing.T) {
	s := resourceSchema(t)
	for _, name := range []string{"description", "site_id"} {
		a := s.Attributes[name]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%q must be optional+computed", name)
		}
	}
	site, ok := s.Attributes["site_id"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf("site_id should be a StringAttribute, got %T", s.Attributes["site_id"])
	}
	if site.Default == nil {
		t.Fatal("site_id must carry a default")
	}
	var defaultResp defaults.StringResponse
	site.Default.DefaultString(context.Background(), defaults.StringRequest{}, &defaultResp)
	if defaultResp.PlanValue.ValueString() != progroups.NoSiteID {
		t.Errorf("site_id default must be the no-site sentinel %q, got %q", progroups.NoSiteID, defaultResp.PlanValue.ValueString())
	}
}

func TestResource_Schema_CriteriaIsAnOptionalList(t *testing.T) {
	s := resourceSchema(t)
	criteriaAttr, ok := s.Attributes["criteria"].(resourceschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("criteria should be a ListNestedAttribute, got %T", s.Attributes["criteria"])
	}
	if !criteriaAttr.IsOptional() {
		t.Error("criteria must be optional")
	}
	if criteriaAttr.IsComputed() {
		t.Error("criteria must not be computed: an omitted list is an empty criteria set, not a server-filled one")
	}
	for _, name := range []string{"priority", "name", "search_type", "value", "and_or", "has_opening_parenthesis", "has_closing_parenthesis"} {
		if _, ok := criteriaAttr.NestedObject.Attributes[name]; !ok {
			t.Errorf("criteria nested object missing attribute %q", name)
		}
	}
}

func TestResource_Schema_DescriptionOpensWithTheDeviceGroupAlternative(t *testing.T) {
	s := resourceSchema(t)
	want := progroups.DeviceGroupAlternative(progroups.DeviceTypeMobile, progroups.GroupKindSmart)
	if !strings.HasPrefix(s.MarkdownDescription, want) {
		t.Errorf("resource description must open with the jamfplatform_device_group advice, got:\n%s", s.MarkdownDescription)
	}
}

// TestResource_Schema_DescriptionsAreUIAligned pins STYLE_GUIDE.md §"User-facing
// descriptions are UI-aligned, not wire-aligned" for every description this
// package renders. The wire field names this endpoint uses are recorded in the
// package doc comment instead.
func TestResource_Schema_DescriptionsAreUIAligned(t *testing.T) {
	forbidden := []string{
		"groupName", "groupDescription", "groupId", "siteId",
		"/pro/v2", "SDK", "POST", "PUT", "DELETE", "RSQL",
		"INVALID_PRIVILEGE", "HAS_DEPENDENCIES",
	}
	descriptions := map[string]string{}

	s := resourceSchema(t)
	descriptions["resource"] = s.MarkdownDescription
	for name, attribute := range s.Attributes {
		descriptions["resource."+name] = attribute.GetMarkdownDescription()
	}

	var dsResp datasource.SchemaResponse
	NewSmartMobileDeviceGroupDataSource().(*SmartMobileDeviceGroupDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &dsResp)
	descriptions["data source"] = dsResp.Schema.MarkdownDescription
	for name, attribute := range dsResp.Schema.Attributes {
		descriptions["data source."+name] = attribute.GetMarkdownDescription()
	}

	var pluralResp datasource.SchemaResponse
	NewSmartMobileDeviceGroupsDataSource().(*SmartMobileDeviceGroupsDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &pluralResp)
	descriptions["plural data source"] = pluralResp.Schema.MarkdownDescription
	if nested, ok := pluralResp.Schema.Attributes["smart_mobile_device_groups"].(datasourceschema.ListNestedAttribute); ok {
		for name, attribute := range nested.NestedObject.Attributes {
			descriptions["plural data source.smart_mobile_device_groups."+name] = attribute.GetMarkdownDescription()
		}
	}

	for where, text := range descriptions {
		// The permission table's own scope sentence is rendered by the shared
		// permissions package, so trim it before looking for jargon.
		body := text
		if idx := strings.Index(body, "Required Jamf permissions"); idx >= 0 {
			body = body[:idx]
		}
		for _, token := range forbidden {
			if strings.Contains(body, token) {
				t.Errorf("%s description contains wire-facing token %q", where, token)
			}
		}
		if strings.Count(body, "—") > 1 {
			t.Errorf("%s description uses more than one em dash", where)
		}
	}
}

func TestResource_ConfigValidators(t *testing.T) {
	got := NewSmartMobileDeviceGroupResource().(*SmartMobileDeviceGroupResource).ConfigValidators(context.Background())
	if len(got) != 1 {
		t.Fatalf("expected 1 config validator, got %d", len(got))
	}
}

func TestDataSource_Metadata(t *testing.T) {
	var resp datasource.MetadataResponse
	NewSmartMobileDeviceGroupDataSource().(*SmartMobileDeviceGroupDataSource).Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	if resp.TypeName != "jamfplatform_pro_smart_mobile_device_group" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_smart_mobile_device_group", resp.TypeName)
	}
}

func TestDataSource_Schema(t *testing.T) {
	var resp datasource.SchemaResponse
	NewSmartMobileDeviceGroupDataSource().(*SmartMobileDeviceGroupDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	s := resp.Schema
	for _, name := range []string{"id", "platform_id", "name", "description", "site_id", "criteria", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	for _, selector := range []string{"id", "name"} {
		a := s.Attributes[selector]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%q must be optional+computed on the data source", selector)
		}
	}
	// The read reports a count, and the data source deliberately does not: the
	// plural data source is where a membership figure belongs, so the family
	// reports it in exactly one place.
	if _, ok := s.Attributes["member_count"]; ok {
		t.Error("member_count must not be on the singular data source")
	}
}

func TestDataSource_ConfigValidators_ExactlyOneSelector(t *testing.T) {
	got := NewSmartMobileDeviceGroupDataSource().(*SmartMobileDeviceGroupDataSource).ConfigValidators(context.Background())
	if len(got) != 1 {
		t.Fatalf("expected 1 config validator, got %d", len(got))
	}
}

func TestListResource_Metadata(t *testing.T) {
	var resp resource.MetadataResponse
	NewSmartMobileDeviceGroupListResource().(*SmartMobileDeviceGroupListResource).Metadata(
		context.Background(),
		resource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	// A list resource carries the managed resource's type name, singular, which
	// is what ties a query result back to the resource it can generate.
	if resp.TypeName != "jamfplatform_pro_smart_mobile_device_group" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_pro_smart_mobile_device_group", resp.TypeName)
	}
}

func TestListResource_Schema(t *testing.T) {
	var resp list.ListResourceSchemaResponse
	NewSmartMobileDeviceGroupListResource().(*SmartMobileDeviceGroupListResource).ListResourceConfigSchema(
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
