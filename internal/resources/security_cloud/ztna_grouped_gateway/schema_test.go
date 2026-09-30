// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_grouped_gateway

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestGroupedGatewayResource_Metadata(t *testing.T) {
	r := NewGroupedGatewayResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*GroupedGatewayResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_ztna_grouped_gateway" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_security_cloud_ztna_grouped_gateway", resp.TypeName)
	}
}

func TestGroupedGatewayResource_Schema(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range []string{
		"id", "name", "gateway_ids", "routing_strategy", "required_gateway_stability",
		"tenant_ids", "created_at", "timeouts",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	for _, name := range []string{"name", "gateway_ids", "routing_strategy", "required_gateway_stability"} {
		if !s.Attributes[name].IsRequired() {
			t.Errorf("%s must be required", name)
		}
	}

	tenantIDs := s.Attributes["tenant_ids"]
	if !tenantIDs.IsOptional() || !tenantIDs.IsComputed() {
		t.Error("tenant_ids must be Optional and Computed: unset, it defaults to the provider's own Security Cloud tenant")
	}

	if id := s.Attributes["id"]; id.IsRequired() || !id.IsComputed() {
		t.Errorf("id must be computed-only, got required=%v computed=%v", id.IsRequired(), id.IsComputed())
	}
	if createdAt := s.Attributes["created_at"]; createdAt.IsOptional() || !createdAt.IsComputed() {
		t.Errorf("created_at must be computed-only, got optional=%v computed=%v", createdAt.IsOptional(), createdAt.IsComputed())
	}
}

// TestGroupedGatewayResource_GatewayIDsAreOrdered pins the List choice. Membership
// order is the priority order the first-available strategy walks, Jamf Security
// Cloud stores it verbatim, and the admin UI presents it as a drag-to-reorder
// list — a Set would silently discard the operator's intent.
func TestGroupedGatewayResource_GatewayIDsAreOrdered(t *testing.T) {
	s := resourceSchema(t)
	if _, ok := s.Attributes["gateway_ids"].(rschema.ListAttribute); !ok {
		t.Errorf("gateway_ids must be a ListAttribute — order is significant; got %T", s.Attributes["gateway_ids"])
	}
	if _, ok := s.Attributes["tenant_ids"].(rschema.SetAttribute); !ok {
		t.Errorf("tenant_ids must be a SetAttribute — order carries no meaning; got %T", s.Attributes["tenant_ids"])
	}
}

// TestGroupedGatewayResource_OmitsUpdatedAt pins the deliberate omission: the wire
// carries an update timestamp that advances on every write, and surfacing it would
// report drift on refreshes with nothing to act on. `created_at` is immutable and
// stays.
func TestGroupedGatewayResource_OmitsUpdatedAt(t *testing.T) {
	s := resourceSchema(t)
	if _, present := s.Attributes["updated_at"]; present {
		t.Error("the resource must not expose updated_at")
	}
}

func TestGroupedGatewayResource_IdentitySchema(t *testing.T) {
	r := NewGroupedGatewayResource()
	var resp resource.IdentitySchemaResponse
	r.(*GroupedGatewayResource).IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)

	if _, ok := resp.IdentitySchema.Attributes["id"]; !ok {
		t.Error("identity schema missing id attribute")
	}
}

func TestGroupedGatewayDataSource_Metadata(t *testing.T) {
	d := NewGroupedGatewayDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*GroupedGatewayDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_ztna_grouped_gateway" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_security_cloud_ztna_grouped_gateway", resp.TypeName)
	}
}

func TestGroupedGatewayDataSource_Schema(t *testing.T) {
	d := NewGroupedGatewayDataSource()
	var resp datasource.SchemaResponse
	d.(*GroupedGatewayDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, name := range []string{"id", "name", "gateway_ids", "routing_strategy", "required_gateway_stability", "tenant_ids", "created_at", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	for _, name := range []string{"id", "name"} {
		if s.Attributes[name].IsRequired() {
			t.Errorf("data source %s must not be required — id and name are alternatives", name)
		}
	}
	if _, ok := s.Attributes["tenant_ids"].(dsschema.ListAttribute); !ok {
		t.Errorf("data source tenant_ids must be a ListAttribute, got %T", s.Attributes["tenant_ids"])
	}
}

func TestGroupedGatewayDataSource_ConfigValidators(t *testing.T) {
	d := NewGroupedGatewayDataSource()
	if len(d.(*GroupedGatewayDataSource).ConfigValidators(context.Background())) == 0 {
		t.Error("data source must declare a config validator enforcing exactly one of id or name")
	}
}

func TestGroupedGatewayListResource_Metadata(t *testing.T) {
	r := NewGroupedGatewayListResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*GroupedGatewayListResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_ztna_grouped_gateway" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_security_cloud_ztna_grouped_gateway", resp.TypeName)
	}
}

func TestGroupedGatewayListResource_Schema(t *testing.T) {
	r := NewGroupedGatewayListResource()
	var resp list.ListResourceSchemaResponse
	r.(*GroupedGatewayListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if len(resp.Schema.Attributes) != 0 {
		t.Errorf("list schema must take no configuration, got %v", resp.Schema.Attributes)
	}
}

// resourceSchema builds the resource schema once per test.
func resourceSchema(t *testing.T) rschema.Schema {
	t.Helper()
	r := NewGroupedGatewayResource()
	var resp resource.SchemaResponse
	r.(*GroupedGatewayResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}
