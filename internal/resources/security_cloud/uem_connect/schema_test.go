// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package uem_connect

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func resourceSchema(t *testing.T) schema.Schema {
	t.Helper()
	r := NewUEMConnectResource()
	var resp resource.SchemaResponse
	r.(*UEMConnectResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func dataSourceSchema(t *testing.T) dsschema.Schema {
	t.Helper()
	d := NewUEMConnectDataSource()
	var resp datasource.SchemaResponse
	d.(*UEMConnectDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestUEMConnectResource_Metadata(t *testing.T) {
	r := NewUEMConnectResource()
	var resp resource.MetadataResponse
	r.(*UEMConnectResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_uem_connect" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestUEMConnectDataSource_Metadata(t *testing.T) {
	d := NewUEMConnectDataSource()
	var resp datasource.MetadataResponse
	d.(*UEMConnectDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_uem_connect" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestUEMConnectResource_Schema(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range []string{
		"id", "uem_vendor", "uem_server_url", "platform_tenant", "oauth", "enabled",
		"scheduled_sync_enabled", "sync_refresh_interval_minutes", "uem_auto_delete_behavior",
		"unmanaged_sync_threshold", "device_risk_uem_signaling_enabled", "disable_sync_on_auth_error",
		"concurrent_device_sync_enabled", "user_data_field_mapping", "group_membership_mapping", "timeouts",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	if !s.Attributes["uem_vendor"].IsRequired() {
		t.Error("uem_vendor must be required")
	}
	if s.Attributes["id"].IsRequired() || !s.Attributes["id"].IsComputed() {
		t.Error("id must be computed-only")
	}
}

// TestUEMConnectResource_ObservedStateIsNotInTheResource pins the split between the
// resource and the data source. These four change without any configuration
// change, so a Computed attribute over them would report drift on every refresh
// about something no configuration can act on.
func TestUEMConnectResource_ObservedStateIsNotInTheResource(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range []string{"connected", "jamf_pro_version", "latest_sync", "next_scheduled_sync"} {
		if _, ok := s.Attributes[name]; ok {
			t.Errorf("%q is server-observed and belongs to the data source only", name)
		}
	}
}

// TestUEMConnectResource_ConnectionAttributesRequireReplace pins that everything
// describing the connection forces replacement. There is no update operation for
// it, so an attribute that did not force replacement would produce a plan
// Terraform cannot carry out.
func TestUEMConnectResource_ConnectionAttributesRequireReplace(t *testing.T) {
	s := resourceSchema(t)

	// The framework does not expose plan modifiers through the Attribute
	// interface, so this asserts on the rendered description instead: it is what a
	// user reads, and it must not promise an in-place change the API cannot make.
	// The modifiers themselves are covered by the acceptance suite's replace step.
	desc := s.MarkdownDescription
	if !strings.Contains(desc, "replaces the integration") {
		t.Errorf("the resource description does not warn that connection changes replace the integration:\n%s", desc)
	}
}

// TestUEMConnectResource_SecretIsWriteOnly pins that the client secret never lands
// in state. It cannot be read back, so holding it would mean state carrying a value
// nothing can verify.
func TestUEMConnectResource_SecretIsWriteOnly(t *testing.T) {
	s := resourceSchema(t)

	oauth, ok := s.Attributes["oauth"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("oauth is %T, want SingleNestedAttribute", s.Attributes["oauth"])
	}

	secret, ok := oauth.Attributes["client_secret"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("client_secret is %T", oauth.Attributes["client_secret"])
	}
	if !secret.WriteOnly {
		t.Error("client_secret must be WriteOnly")
	}
	if _, ok := oauth.Attributes["client_secret_wo_version"]; !ok {
		t.Error("a WriteOnly secret needs its rotation companion; client_secret_wo_version is missing")
	}
}

// TestUEMConnectResource_AuthBlocksAreOptionalOnly pins STYLE_GUIDE
// §SingleNestedAttribute blocks: Optional-only when the model uses typed pointers.
// Optional+Computed on a typed-pointer block makes "absent" and "unknown"
// indistinguishable, and absence is exactly what selects the authentication form.
func TestUEMConnectResource_AuthBlocksAreOptionalOnly(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range []string{"platform_tenant", "oauth", "user_data_field_mapping", "group_membership_mapping"} {
		attr := s.Attributes[name]
		if !attr.IsOptional() {
			t.Errorf("%q must be optional", name)
		}
		if attr.IsComputed() {
			t.Errorf("%q must not be Optional+Computed: the model is a typed pointer, so absence has to stay distinguishable", name)
		}
	}
}

// TestUEMConnectResource_GroupMappingsAreOrdered pins a list rather than a set.
// Membership is evaluated top to bottom, so the order is configuration and a set
// would let Terraform reorder it silently.
func TestUEMConnectResource_GroupMappingsAreOrdered(t *testing.T) {
	s := resourceSchema(t)

	group, ok := s.Attributes["group_membership_mapping"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("group_membership_mapping is %T", s.Attributes["group_membership_mapping"])
	}
	if _, ok := group.Attributes["mappings"].(schema.ListNestedAttribute); !ok {
		t.Errorf("mappings is %T, want ListNestedAttribute — membership is evaluated in order", group.Attributes["mappings"])
	}
}

func TestUEMConnectDataSource_Schema(t *testing.T) {
	s := dataSourceSchema(t)

	for _, name := range []string{
		"id", "uem_vendor", "uem_server_url", "platform_tenant_id", "client_id", "enabled",
		"connected", "jamf_pro_version", "latest_sync", "user_data_field_mapping", "group_membership_mapping", "timeouts",
	} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	// No arguments: a tenant holds at most one integration, so there is nothing to
	// select between and every attribute is read-only.
	for name, attr := range s.Attributes {
		if name == "timeouts" {
			continue
		}
		if attr.IsRequired() || attr.IsOptional() {
			t.Errorf("attribute %q must be computed-only, got required=%v optional=%v",
				name, attr.IsRequired(), attr.IsOptional())
		}
	}
}

// TestUEMConnectDataSource_HasNoSecret pins that the data source does not expose a
// credential. The secret is unreadable anyway, but a client_secret attribute would
// invite the assumption that it is available.
func TestUEMConnectDataSource_HasNoSecret(t *testing.T) {
	s := dataSourceSchema(t)

	for _, name := range []string{"client_secret", "oauth", "platform_tenant"} {
		if _, ok := s.Attributes[name]; ok {
			t.Errorf("%q should not be on the data source", name)
		}
	}
}

// TestDescriptionsAreUIAligned pins STYLE_GUIDE §User-facing descriptions are
// UI-aligned, not wire-aligned, across every description in both schemas. This
// resource is the sort where protocol vocabulary creeps in: the wire names differ
// from the UI's throughout, and several diagnostics discuss server behaviour.
func TestDescriptionsAreUIAligned(t *testing.T) {
	banned := []string{
		"endpoint", "/v1/", "HTTP ", " SDK", "payload", "JSON",
		"emmGroupId", "wanderaGroupId", "authStrategy", "deviceSyncAuth", "syncConfig",
		"refreshRateMinutes", "concurrentSyncEnabled", "deviceFieldMappings", "groupSettings",
		"M2M", "JAMF_PRO_OAUTH",
	}

	check := func(t *testing.T, where, desc string) {
		t.Helper()
		lower := strings.ToLower(desc)
		for _, b := range banned {
			if strings.Contains(lower, strings.ToLower(b)) {
				t.Errorf("%s contains wire vocabulary %q:\n%s", where, b, desc)
			}
		}
	}

	rs := resourceSchema(t)
	check(t, "resource description", strings.TrimSuffix(rs.MarkdownDescription, resourcePrivileges))
	for name, attr := range rs.Attributes {
		check(t, "resource."+name, attr.GetMarkdownDescription())
	}

	ds := dataSourceSchema(t)
	check(t, "data source description", strings.TrimSuffix(ds.MarkdownDescription, dataSourcePrivileges))
	for name, attr := range ds.Attributes {
		check(t, "data_source."+name, attr.GetMarkdownDescription())
	}
}

func TestUEMConnectResource_IdentitySchema(t *testing.T) {
	r := NewUEMConnectResource()
	var resp resource.IdentitySchemaResponse
	r.(*UEMConnectResource).IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("identity schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.IdentitySchema.Attributes["id"]; !ok {
		t.Error("identity schema is missing 'id'")
	}
}

// TestUEMConnectResource_ConfigValidators pins that all three cross-field rules are
// declared: exactly one authentication form, the OAuth form together with the
// address it needs, and the address refused on the platform_tenant form, where Jamf
// Security Cloud resolves it from the tenant instead.
func TestUEMConnectResource_ConfigValidators(t *testing.T) {
	r := NewUEMConnectResource().(*UEMConnectResource)
	validators := r.ConfigValidators(context.Background())

	if len(validators) != 3 {
		t.Fatalf("got %d config validators, want 3", len(validators))
	}
}

// TestUEMConnectListResource_Metadata pins that the list resource shares the
// resource's type name, which is what lets `terraform query` generate an import
// block for it.
func TestUEMConnectListResource_Metadata(t *testing.T) {
	r := NewUEMConnectListResource()
	var resp resource.MetadataResponse
	r.(*UEMConnectListResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_uem_connect" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

// TestUEMConnectListResource_TakesNoConfiguration pins that the list resource has no
// filter block. A tenant holds at most one integration, so there is nothing to
// filter on — the resource exists for discovery, not selection.
func TestUEMConnectListResource_TakesNoConfiguration(t *testing.T) {
	r := NewUEMConnectListResource()
	var resp list.ListResourceSchemaResponse
	r.(*UEMConnectListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if len(resp.Schema.Attributes) != 1 {
		t.Errorf("expected only the timeouts attribute, got %d", len(resp.Schema.Attributes))
	}
	if _, ok := resp.Schema.Attributes["timeouts"]; !ok {
		t.Errorf("expected the timeouts attribute, got %v", resp.Schema.Attributes)
	}
	if !strings.Contains(resp.Schema.Description, "import") {
		t.Errorf("the description should say what the list resource is for:\n%s", resp.Schema.Description)
	}
}
