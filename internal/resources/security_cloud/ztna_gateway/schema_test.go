// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_gateway

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestGatewayResource_Metadata(t *testing.T) {
	r := NewGatewayResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*GatewayResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_ztna_gateway" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_security_cloud_ztna_gateway", resp.TypeName)
	}
}

func TestGatewayResource_Schema(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range []string{
		"id", "name", "egress_region", "contact", "enabled", "tenant_ids",
		"ipsec_source_ip_addresses", "dedicated_egress_ip_addresses", "ipsec", "status", "timeouts",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	for _, name := range []string{"name", "egress_region", "contact"} {
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
	if ipsec := s.Attributes["ipsec"]; !ipsec.IsOptional() || ipsec.IsRequired() {
		t.Errorf("ipsec must be optional-only, got optional=%v required=%v", ipsec.IsOptional(), ipsec.IsRequired())
	}
	if status := s.Attributes["status"]; !status.IsComputed() || status.IsOptional() {
		t.Errorf("status must be computed-only, got computed=%v optional=%v", status.IsComputed(), status.IsOptional())
	}
}

// TestGatewayResource_NoDedicatedEgressToggle pins the derived discriminator. The
// gateway's form is chosen by the presence of `ipsec`, and exposing the wire's
// dedicated-egress flag as well would let a user write the one combination the
// API always refuses.
func TestGatewayResource_NoDedicatedEgressToggle(t *testing.T) {
	s := resourceSchema(t)
	if _, present := s.Attributes["dedicated_egress_ips_enabled"]; present {
		t.Error("the resource must not expose a dedicated-egress toggle; the form is derived from the ipsec block")
	}
	if addresses := s.Attributes["dedicated_egress_ip_addresses"]; addresses.IsOptional() || !addresses.IsComputed() {
		t.Errorf("dedicated_egress_ip_addresses must be computed-only, got optional=%v computed=%v", addresses.IsOptional(), addresses.IsComputed())
	}
}

// TestGatewayResource_StatusOmitsUpdatedAt pins the deliberate omission: the wire
// carries a status timestamp that advances on every server-side re-evaluation, and
// surfacing it would report drift on every refresh over a value no configuration
// can act on.
func TestGatewayResource_StatusOmitsUpdatedAt(t *testing.T) {
	s := resourceSchema(t)
	status, ok := s.Attributes["status"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("status must be a SingleNestedAttribute, got %T", s.Attributes["status"])
	}
	if _, present := status.Attributes["updated_at"]; present {
		t.Error("status must not expose updated_at")
	}
	for _, name := range []string{"state", "tunnel_state"} {
		if _, present := status.Attributes[name]; !present {
			t.Errorf("status missing %q", name)
		}
	}
}

// TestGatewayResource_IPSecShape pins the shapes the wire probing settled: the
// cipher algorithms are single values rather than lists, the Jamf-side encryption
// domain is one subnet rather than a collection, and the customer subnets are a
// Set because the server returns them reordered.
func TestGatewayResource_IPSecShape(t *testing.T) {
	s := resourceSchema(t)
	ipsec, ok := s.Attributes["ipsec"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("ipsec must be a SingleNestedAttribute, got %T", s.Attributes["ipsec"])
	}

	for _, name := range []string{"key_exchange_protocol", "phase_1", "phase_2", "jamf_side", "customer_side"} {
		if _, present := ipsec.Attributes[name]; !present {
			t.Errorf("ipsec missing %q", name)
		}
	}

	for _, phase := range []string{"phase_1", "phase_2"} {
		suite, suiteOK := ipsec.Attributes[phase].(rschema.SingleNestedAttribute)
		if !suiteOK {
			t.Errorf("ipsec.%s must be a SingleNestedAttribute, got %T", phase, ipsec.Attributes[phase])
			continue
		}
		for _, name := range []string{"encryption", "integrity", "diffie_hellman_group"} {
			attr, present := suite.Attributes[name]
			if !present {
				t.Errorf("ipsec.%s missing %q", phase, name)
				continue
			}
			if _, isString := attr.(rschema.StringAttribute); !isString {
				t.Errorf("ipsec.%s.%s must be a single string, not a collection: the server accepts exactly one value", phase, name)
			}
		}
	}

	jamfSide, jamfOK := ipsec.Attributes["jamf_side"].(rschema.SingleNestedAttribute)
	if !jamfOK {
		t.Fatalf("ipsec.jamf_side must be a SingleNestedAttribute, got %T", ipsec.Attributes["jamf_side"])
	}
	if _, isString := jamfSide.Attributes["subnet"].(rschema.StringAttribute); !isString {
		t.Error("ipsec.jamf_side.subnet must be a single string: the server accepts exactly one subnet")
	}

	customerSide, customerOK := ipsec.Attributes["customer_side"].(rschema.SingleNestedAttribute)
	if !customerOK {
		t.Fatalf("ipsec.customer_side must be a SingleNestedAttribute, got %T", ipsec.Attributes["customer_side"])
	}
	if _, isSet := customerSide.Attributes["subnets"].(rschema.SetAttribute); !isSet {
		t.Errorf("ipsec.customer_side.subnets must be a SetAttribute — the server returns them reordered; got %T", customerSide.Attributes["subnets"])
	}
}

// TestGatewayResource_AuthenticationSecretIsWriteOnly pins the credential handling: the
// pre-shared key must never reach the state file, and it needs its rotation
// companion because a WriteOnly value is invisible to drift detection.
func TestGatewayResource_AuthenticationSecretIsWriteOnly(t *testing.T) {
	s := resourceSchema(t)
	ipsec := s.Attributes["ipsec"].(rschema.SingleNestedAttribute)
	jamfSide := ipsec.Attributes["jamf_side"].(rschema.SingleNestedAttribute)

	secret, ok := jamfSide.Attributes["authentication_secret"].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("authentication_secret must be a StringAttribute, got %T", jamfSide.Attributes["authentication_secret"])
	}
	if !secret.WriteOnly {
		t.Error("authentication_secret must be WriteOnly so the pre-shared key never lands in state")
	}
	if !secret.Sensitive {
		t.Error("authentication_secret must be Sensitive")
	}
	if secret.Computed {
		t.Error("authentication_secret must not be Computed — Jamf Security Cloud never returns it")
	}
	if _, present := jamfSide.Attributes["authentication_secret_wo_version"]; !present {
		t.Error("a WriteOnly secret must carry its authentication_secret_wo_version rotation companion")
	}
}

func TestGatewayResource_IdentitySchema(t *testing.T) {
	r := NewGatewayResource()
	var resp resource.IdentitySchemaResponse
	r.(*GatewayResource).IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)

	if _, ok := resp.IdentitySchema.Attributes["id"]; !ok {
		t.Error("identity schema missing id attribute")
	}
}

func TestGatewayResource_ConfigValidators(t *testing.T) {
	r := NewGatewayResource()
	if len(r.(*GatewayResource).ConfigValidators(context.Background())) == 0 {
		t.Error("the resource must declare the availability-zones cross-field validator")
	}
}

func TestGatewayDataSource_Metadata(t *testing.T) {
	d := NewGatewayDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*GatewayDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_ztna_gateway" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_security_cloud_ztna_gateway", resp.TypeName)
	}
}

// TestGatewayDataSource_Schema also pins the absence of the pre-shared key: a data
// source that appeared to report it would be reporting nothing, since Jamf
// Security Cloud never returns one.
func TestGatewayDataSource_Schema(t *testing.T) {
	d := NewGatewayDataSource()
	var resp datasource.SchemaResponse
	d.(*GatewayDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, name := range []string{"id", "name", "egress_region", "contact", "enabled", "tenant_ids", "ipsec", "status", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	for _, name := range []string{"id", "name"} {
		attr := s.Attributes[name]
		if attr.IsRequired() {
			t.Errorf("data source %s must not be required — id and name are alternatives", name)
		}
	}
	if _, present := s.Attributes["dedicated_egress_ips_enabled"]; !present {
		t.Error("the data source should report the gateway form, which is not derivable without reading ipsec")
	}
}

func TestGatewayDataSource_ConfigValidators(t *testing.T) {
	d := NewGatewayDataSource()
	if len(d.(*GatewayDataSource).ConfigValidators(context.Background())) == 0 {
		t.Error("data source must declare a config validator enforcing exactly one of id or name")
	}
}

func TestGatewayListResource_Metadata(t *testing.T) {
	r := NewGatewayListResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*GatewayListResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_ztna_gateway" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_security_cloud_ztna_gateway", resp.TypeName)
	}
}

func TestGatewayListResource_Schema(t *testing.T) {
	r := NewGatewayListResource()
	var resp list.ListResourceSchemaResponse
	r.(*GatewayListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

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
	r := NewGatewayResource()
	var resp resource.SchemaResponse
	r.(*GatewayResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}
