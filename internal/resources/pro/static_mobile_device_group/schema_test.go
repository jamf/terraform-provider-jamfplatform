// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package static_mobile_device_group

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestStaticMobileDeviceGroupResource_Metadata(t *testing.T) {
	r := NewStaticMobileDeviceGroupResource()
	var resp resource.MetadataResponse
	r.(*StaticMobileDeviceGroupResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_pro_static_mobile_device_group" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_static_mobile_device_group", resp.TypeName)
	}
}

func TestStaticMobileDeviceGroupResource_Schema(t *testing.T) {
	r := NewStaticMobileDeviceGroupResource()
	var resp resource.SchemaResponse
	r.(*StaticMobileDeviceGroupResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	s := resp.Schema

	for _, name := range []string{
		"id", "platform_id", "name", "description", "site_id",
		"assigned_mobile_device_ids", "member_count", "timeouts",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	// The maintainer ruled both of these out: `.id` IS the Jamf Pro identifier,
	// and a site name is one more mutable derived field nobody asked for.
	for _, banned := range []string{"site_name", "jamf_pro_id"} {
		if _, ok := s.Attributes[banned]; ok {
			t.Errorf("attribute %q must not exist", banned)
		}
	}

	for _, computedOnly := range []string{"id", "platform_id", "member_count"} {
		a := s.Attributes[computedOnly]
		if a.IsRequired() || a.IsOptional() {
			t.Errorf("%q must be computed-only", computedOnly)
		}
		if !a.IsComputed() {
			t.Errorf("%q must be computed", computedOnly)
		}
	}

	if !s.Attributes["name"].IsRequired() {
		t.Error("name must be required")
	}

	for _, optComputed := range []string{"description", "site_id"} {
		a := s.Attributes[optComputed]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%q must be optional+computed", optComputed)
		}
	}

	// Optional-only, never Computed: a null set is how "Terraform does not
	// manage membership" is expressed, and a Computed set would make that
	// indistinguishable from an empty one.
	members := s.Attributes["assigned_mobile_device_ids"]
	if !members.IsOptional() {
		t.Error("assigned_mobile_device_ids must be optional")
	}
	if members.IsComputed() {
		t.Error("assigned_mobile_device_ids must not be computed")
	}
}

func TestStaticMobileDeviceGroupResource_DescriptionOpensWithTheAlternative(t *testing.T) {
	r := NewStaticMobileDeviceGroupResource()
	var resp resource.SchemaResponse
	r.(*StaticMobileDeviceGroupResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	if !strings.HasPrefix(resp.Schema.MarkdownDescription, "Use this only when the group must belong to a Jamf Pro site.") {
		t.Errorf("description must open with the jamfplatform_device_group advice, got: %.120s", resp.Schema.MarkdownDescription)
	}
	if !strings.Contains(resp.Schema.MarkdownDescription, "Required Jamf permissions") {
		t.Error("description must carry the rendered permissions section")
	}
}

func TestStaticMobileDeviceGroupResource_IdentitySchema(t *testing.T) {
	r := NewStaticMobileDeviceGroupResource().(*StaticMobileDeviceGroupResource)
	var resp resource.IdentitySchemaResponse
	r.IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)

	if _, ok := resp.IdentitySchema.Attributes["id"]; !ok {
		t.Fatal("identity schema missing id")
	}
}

func TestStaticMobileDeviceGroupDataSource_Metadata(t *testing.T) {
	d := NewStaticMobileDeviceGroupDataSource()
	var resp datasource.MetadataResponse
	d.(*StaticMobileDeviceGroupDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_pro_static_mobile_device_group" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_pro_static_mobile_device_group", resp.TypeName)
	}
}

func TestStaticMobileDeviceGroupDataSource_Schema(t *testing.T) {
	d := NewStaticMobileDeviceGroupDataSource()
	var resp datasource.SchemaResponse
	d.(*StaticMobileDeviceGroupDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	s := resp.Schema

	for _, name := range []string{
		"id", "platform_id", "name", "description", "site_id",
		"assigned_mobile_device_ids", "member_count", "timeouts",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	for _, selector := range []string{"id", "name"} {
		a := s.Attributes[selector]
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%q must be optional+computed", selector)
		}
	}

	for _, computedOnly := range []string{"platform_id", "description", "site_id", "assigned_mobile_device_ids", "member_count"} {
		a := s.Attributes[computedOnly]
		if a.IsOptional() || a.IsRequired() {
			t.Errorf("%q must be computed-only on the data source", computedOnly)
		}
	}
}

func TestStaticMobileDeviceGroupDataSource_ConfigValidators(t *testing.T) {
	d := NewStaticMobileDeviceGroupDataSource().(*StaticMobileDeviceGroupDataSource)
	if got := d.ConfigValidators(context.Background()); len(got) != 1 {
		t.Fatalf("expected 1 config validator, got %d", len(got))
	}
}

func TestStaticMobileDeviceGroupListResource_Metadata(t *testing.T) {
	r := NewStaticMobileDeviceGroupListResource()
	var resp resource.MetadataResponse
	r.(*StaticMobileDeviceGroupListResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	// A list resource lists instances of a managed resource type, so its type
	// name is the singular one.
	if resp.TypeName != "jamfplatform_pro_static_mobile_device_group" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_pro_static_mobile_device_group", resp.TypeName)
	}
}

func TestStaticMobileDeviceGroupListResource_Schema(t *testing.T) {
	r := NewStaticMobileDeviceGroupListResource()
	var resp list.ListResourceSchemaResponse
	r.(*StaticMobileDeviceGroupListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["filter"]; !ok {
		t.Error("list schema missing filter attribute")
	}
}
