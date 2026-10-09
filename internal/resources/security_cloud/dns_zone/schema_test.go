// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package dns_zone

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestDNSZoneResource_Metadata(t *testing.T) {
	r := NewDNSZoneResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*DNSZoneResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_dns_zone" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_security_cloud_dns_zone", resp.TypeName)
	}
}

func TestDNSZoneResource_Schema(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range []string{"id", "name", "domains", "authoritative_name_servers", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	id := s.Attributes["id"]
	if id.IsRequired() || !id.IsComputed() {
		t.Errorf("id must be computed-only, got required=%v computed=%v", id.IsRequired(), id.IsComputed())
	}

	for _, name := range []string{"name", "domains", "authoritative_name_servers"} {
		if !s.Attributes[name].IsRequired() {
			t.Errorf("%s must be required", name)
		}
	}
}

// TestDNSZoneResource_CollectionsAreSets pins the Set choice for both
// collections. Jamf Security Cloud re-sorts `domains` on the wire, so a List
// would fail "produced inconsistent result after apply";
// `authoritative_name_servers` carries no ordering semantics, and the Set is what
// makes the per-IP uniqueness the server enforces expressible at plan time.
func TestDNSZoneResource_CollectionsAreSets(t *testing.T) {
	s := resourceSchema(t)

	if _, ok := s.Attributes["domains"].(rschema.SetAttribute); !ok {
		t.Errorf("domains must be a SetAttribute, got %T", s.Attributes["domains"])
	}
	if _, ok := s.Attributes["authoritative_name_servers"].(rschema.SetNestedAttribute); !ok {
		t.Errorf("authoritative_name_servers must be a SetNestedAttribute, got %T", s.Attributes["authoritative_name_servers"])
	}
}

func TestDNSZoneResource_NameServerAttributes(t *testing.T) {
	s := resourceSchema(t)

	nested, ok := s.Attributes["authoritative_name_servers"].(rschema.SetNestedAttribute)
	if !ok {
		t.Fatalf("authoritative_name_servers must be a SetNestedAttribute, got %T", s.Attributes["authoritative_name_servers"])
	}
	for _, name := range []string{"ip_address", "gateway_id"} {
		attr, present := nested.NestedObject.Attributes[name]
		if !present {
			t.Errorf("authoritative_name_servers missing nested attribute %q", name)
			continue
		}
		if !attr.IsRequired() {
			t.Errorf("authoritative_name_servers.%s must be required", name)
		}
	}
}

func TestDNSZoneResource_IdentitySchema(t *testing.T) {
	r := NewDNSZoneResource()
	var resp resource.IdentitySchemaResponse
	r.(*DNSZoneResource).IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)

	if _, ok := resp.IdentitySchema.Attributes["id"]; !ok {
		t.Errorf("identity schema missing id attribute")
	}
}

func TestDNSZoneDataSource_Metadata(t *testing.T) {
	d := NewDNSZoneDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*DNSZoneDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_dns_zone" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_security_cloud_dns_zone", resp.TypeName)
	}
}

// TestDNSZoneDataSource_Schema also pins the List shape of the read-only
// collections: data source attributes returning API data are always lists, which
// is why the data source model does not reuse the resource's sets.
func TestDNSZoneDataSource_Schema(t *testing.T) {
	d := NewDNSZoneDataSource()
	var resp datasource.SchemaResponse
	d.(*DNSZoneDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, name := range []string{"id", "name", "domains", "authoritative_name_servers", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	for _, name := range []string{"id", "name"} {
		attr := s.Attributes[name]
		if attr.IsRequired() {
			t.Errorf("data source %s must not be required — id and name are alternatives", name)
		}
		if !attr.IsOptional() || !attr.IsComputed() {
			t.Errorf("data source %s must be optional+computed, got optional=%v computed=%v", name, attr.IsOptional(), attr.IsComputed())
		}
	}
	if _, ok := s.Attributes["domains"].(dsschema.ListAttribute); !ok {
		t.Errorf("data source domains must be a ListAttribute, got %T", s.Attributes["domains"])
	}
	if _, ok := s.Attributes["authoritative_name_servers"].(dsschema.ListNestedAttribute); !ok {
		t.Errorf("data source authoritative_name_servers must be a ListNestedAttribute, got %T", s.Attributes["authoritative_name_servers"])
	}
}

func TestDNSZoneDataSource_ConfigValidators(t *testing.T) {
	d := NewDNSZoneDataSource()
	validators := d.(*DNSZoneDataSource).ConfigValidators(context.Background())
	if len(validators) == 0 {
		t.Fatal("data source must declare a config validator enforcing exactly one of id or name")
	}
}

func TestDNSZoneListResource_Metadata(t *testing.T) {
	r := NewDNSZoneListResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*DNSZoneListResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_dns_zone" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_security_cloud_dns_zone", resp.TypeName)
	}
}

// TestDNSZoneListResource_Schema asserts the config schema carries only the `timeouts` attribute. The zone
// list endpoint takes only a sort expression, so there is nothing to configure —
// any other attribute appearing here would mean a filter was added without wiring it.
func TestDNSZoneListResource_Schema(t *testing.T) {
	r := NewDNSZoneListResource()
	var resp list.ListResourceSchemaResponse
	r.(*DNSZoneListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["timeouts"]; !ok || len(resp.Schema.Attributes) != 1 {
		t.Errorf("list schema must take only the timeouts attribute, got %v", resp.Schema.Attributes)
	}
}

// resourceSchema builds the resource schema once per test.
func resourceSchema(t *testing.T) rschema.Schema {
	t.Helper()
	r := NewDNSZoneResource()
	var resp resource.SchemaResponse
	r.(*DNSZoneResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}
