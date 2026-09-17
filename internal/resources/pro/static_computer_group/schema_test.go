// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_computer_group

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
)

// deviceGroupAlternative is the paragraph every construct in this family opens
// its description with.
var deviceGroupAlternative = progroups.DeviceGroupAlternative(progroups.DeviceTypeComputer, progroups.GroupKindStatic)

func resourceSchema(t *testing.T) resourceschema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	NewStaticComputerGroupResource().(*StaticComputerGroupResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestStaticComputerGroupResource_Metadata(t *testing.T) {
	var resp resource.MetadataResponse
	NewStaticComputerGroupResource().(*StaticComputerGroupResource).Metadata(
		context.Background(),
		resource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	if resp.TypeName != "jamfplatform_pro_static_computer_group" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_static_computer_group", resp.TypeName)
	}
}

func TestStaticComputerGroupResource_Schema(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range []string{"id", "platform_id", "name", "description", "site_id", "assigned_computer_ids", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	// Decided by the maintainer: `id` IS the Jamf Pro identifier and the site is
	// referenced by id alone, so neither of these belongs in the schema.
	for _, name := range []string{"site_name", "jamf_pro_id"} {
		if _, ok := s.Attributes[name]; ok {
			t.Errorf("attribute %q must not exist", name)
		}
	}

	if id := s.Attributes["id"]; id.IsRequired() || id.IsOptional() || !id.IsComputed() {
		t.Errorf("id must be computed-only, got required=%v optional=%v computed=%v", id.IsRequired(), id.IsOptional(), id.IsComputed())
	}
	if pid := s.Attributes["platform_id"]; pid.IsRequired() || pid.IsOptional() || !pid.IsComputed() {
		t.Errorf("platform_id must be computed-only, got required=%v optional=%v computed=%v", pid.IsRequired(), pid.IsOptional(), pid.IsComputed())
	}
	if name := s.Attributes["name"]; !name.IsRequired() {
		t.Error("name must be required — Read uses its nullness as the first-hydration signal")
	}
	if desc := s.Attributes["description"]; !desc.IsOptional() || !desc.IsComputed() {
		t.Error("description must be optional and computed")
	}
	if site := s.Attributes["site_id"]; !site.IsOptional() || !site.IsComputed() {
		t.Error("site_id must be optional and computed")
	}

	// Optional WITHOUT Computed is the opt-out contract: omitting the attribute
	// has to stay distinguishable from declaring an empty set, and a Computed
	// membership would let the server's value become the configuration's.
	members := s.Attributes["assigned_computer_ids"]
	if !members.IsOptional() {
		t.Error("assigned_computer_ids must be optional")
	}
	if members.IsComputed() {
		t.Error("assigned_computer_ids must not be computed — omitted and empty must stay distinguishable")
	}
	if _, ok := members.(resourceschema.SetAttribute); !ok {
		t.Errorf("assigned_computer_ids must be a SetAttribute, got %T", members)
	}
}

func TestStaticComputerGroupResource_SchemaOpensWithTheDeviceGroupAlternative(t *testing.T) {
	s := resourceSchema(t)
	if !strings.HasPrefix(s.MarkdownDescription, deviceGroupAlternative) {
		t.Errorf("resource description must open with the shared jamfplatform_device_group paragraph, got:\n%s", s.MarkdownDescription)
	}
}

// TestDescriptionsAvoidWireVocabulary pins STYLE_GUIDE §"User-facing
// descriptions are UI-aligned, not wire-aligned" for every description this
// package renders.
func TestDescriptionsAvoidWireVocabulary(t *testing.T) {
	banned := []string{
		"/pro/v3", "/JSSResource", "/api/", "proclassic", "SDK",
		"POST", "PUT", "DELETE", "403", "500", "endpoint", "RSQL",
	}

	var resourceResp resource.SchemaResponse
	NewStaticComputerGroupResource().(*StaticComputerGroupResource).Schema(context.Background(), resource.SchemaRequest{}, &resourceResp)

	var dsResp datasource.SchemaResponse
	NewStaticComputerGroupDataSource().(*StaticComputerGroupDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &dsResp)

	var pluralResp datasource.SchemaResponse
	NewStaticComputerGroupsDataSource().(*StaticComputerGroupsDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &pluralResp)

	var listResp list.ListResourceSchemaResponse
	NewStaticComputerGroupListResource().(*StaticComputerGroupListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &listResp)

	descriptions := map[string]string{
		"resource":           resourceResp.Schema.MarkdownDescription,
		"data source":        dsResp.Schema.MarkdownDescription,
		"plural data source": pluralResp.Schema.MarkdownDescription,
		"list resource":      listResp.Schema.Description,
	}
	for _, attribute := range resourceResp.Schema.Attributes {
		descriptions["resource attribute"] += attribute.GetMarkdownDescription() + "\n"
	}
	for _, attribute := range dsResp.Schema.Attributes {
		descriptions["data source attribute"] += attribute.GetMarkdownDescription() + "\n"
	}

	for where, text := range descriptions {
		// The rendered permissions table names the integration scope and the
		// capability identifiers, which are deliberately machine-readable.
		text = strings.Split(text, "**Required Jamf permissions**")[0]
		for _, term := range banned {
			if strings.Contains(text, term) {
				t.Errorf("%s description contains wire vocabulary %q", where, term)
			}
		}
	}
}

func TestStaticComputerGroupDataSource_Metadata(t *testing.T) {
	var resp datasource.MetadataResponse
	NewStaticComputerGroupDataSource().(*StaticComputerGroupDataSource).Metadata(
		context.Background(),
		datasource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	if resp.TypeName != "jamfplatform_pro_static_computer_group" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_static_computer_group", resp.TypeName)
	}
}

func TestStaticComputerGroupDataSource_Schema(t *testing.T) {
	var resp datasource.SchemaResponse
	NewStaticComputerGroupDataSource().(*StaticComputerGroupDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	s := resp.Schema

	for _, name := range []string{"id", "platform_id", "name", "description", "site_id", "assigned_computer_ids", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if id := s.Attributes["id"]; !id.IsOptional() || !id.IsComputed() {
		t.Error("data source id must be optional and computed — it is one half of the selector")
	}
	if name := s.Attributes["name"]; !name.IsOptional() || !name.IsComputed() {
		t.Error("data source name must be optional and computed — it is the other half of the selector")
	}
	if members := s.Attributes["assigned_computer_ids"]; !members.IsComputed() || members.IsOptional() {
		t.Error("data source assigned_computer_ids must be computed-only")
	}
	if !strings.HasPrefix(s.MarkdownDescription, deviceGroupAlternative) {
		t.Error("data source description must open with the shared jamfplatform_device_group paragraph")
	}
}

func TestStaticComputerGroupDataSource_ConfigValidators(t *testing.T) {
	got := NewStaticComputerGroupDataSource().(*StaticComputerGroupDataSource).ConfigValidators(context.Background())
	if len(got) != 1 {
		t.Fatalf("expected exactly one config validator (id/name), got %d", len(got))
	}
}

func TestStaticComputerGroupListResource_Metadata(t *testing.T) {
	var resp resource.MetadataResponse
	NewStaticComputerGroupListResource().(*StaticComputerGroupListResource).Metadata(
		context.Background(),
		resource.MetadataRequest{ProviderTypeName: "jamfplatform"},
		&resp,
	)
	// A list resource's type name is the managed resource type it lists, so it
	// is the singular name even though the construct is a listing.
	if resp.TypeName != "jamfplatform_pro_static_computer_group" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_pro_static_computer_group", resp.TypeName)
	}
}

func TestStaticComputerGroupListResource_Schema(t *testing.T) {
	var resp list.ListResourceSchemaResponse
	NewStaticComputerGroupListResource().(*StaticComputerGroupListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["filter"]; !ok {
		t.Error("list schema missing filter attribute")
	}
}

func TestStaticComputerGroupResource_IdentitySchema(t *testing.T) {
	var resp resource.IdentitySchemaResponse
	NewStaticComputerGroupResource().(*StaticComputerGroupResource).IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)
	attr, ok := resp.IdentitySchema.Attributes["id"]
	if !ok {
		t.Fatal("identity schema missing id")
	}
	if !attr.IsRequiredForImport() {
		t.Error("identity id must be required for import")
	}
}
