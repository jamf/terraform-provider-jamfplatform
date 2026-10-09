// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package sso_domain

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// resourceAttributeNames is every attribute the resource schema declares, so a
// rename shows up as a test failure rather than as a silent state migration.
var resourceAttributeNames = []string{
	"id",
	"domain",
	"verification_status",
	"verification_key",
	"verification_txt_record",
	"parent_domain_id",
	"shared",
	"account_id",
	"created_by",
	"created_at",
	"last_modified_at",
	"last_verified_at",
	"verification_expires_at",
	"timeouts",
}

func TestDomainResource_Metadata(t *testing.T) {
	r := NewDomainResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*DomainResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_account_sso_domain" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_account_sso_domain", resp.TypeName)
	}
}

func TestDomainResource_Schema(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range resourceAttributeNames {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(s.Attributes) != len(resourceAttributeNames) {
		t.Errorf("schema declares %d attributes, the test knows %d — update resourceAttributeNames", len(s.Attributes), len(resourceAttributeNames))
	}

	if !s.Attributes["domain"].IsRequired() {
		t.Error("domain must be required")
	}
}

// TestDomainResource_OnlyDomainIsWritable pins the create-and-destroy shape.
// Jamf Account exposes no update for a claim, so every attribute other than
// `domain` and `timeouts` has to be read-only — an Optional attribute here would
// be one a practitioner could set and the provider could never send.
func TestDomainResource_OnlyDomainIsWritable(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range resourceAttributeNames {
		if name == "domain" || name == "timeouts" {
			continue
		}
		attr := s.Attributes[name]
		if attr.IsRequired() || attr.IsOptional() {
			t.Errorf("%s must be computed-only, got required=%v optional=%v", name, attr.IsRequired(), attr.IsOptional())
		}
		if !attr.IsComputed() {
			t.Errorf("%s must be computed", name)
		}
	}
}

// TestDomainResource_DomainRequiresReplace pins the immutability of the claim. A
// claim cannot be edited — the routes that would do it are unmapped — so a change
// to `domain` has to replace the resource rather than reach Update.
//
// The assertion is behavioural rather than nominal: the modifier is invoked with a
// request representing an update to an existing resource, and must answer that
// replacement is required. A presence check would stay green if RequiresReplace()
// were swapped for any other modifier, and every Jamf Account acceptance test
// skips in CI, so this is CI's only gate on it.
func TestDomainResource_DomainRequiresReplace(t *testing.T) {
	ctx := context.Background()
	s := resourceSchema(t)
	raw := existingResourceRaw(ctx, s)
	state := tfsdk.State{Schema: s, Raw: raw}
	plan := tfsdk.Plan{Schema: s, Raw: raw}
	config := tfsdk.Config{Schema: s, Raw: raw}

	domain, ok := s.Attributes["domain"].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("domain must be a StringAttribute, got %T", s.Attributes["domain"])
	}
	if got := len(domain.PlanModifiers); got != 1 {
		t.Errorf("domain has %d plan modifiers, expected exactly 1 (RequiresReplace)", got)
	}
	for i, modifier := range domain.PlanModifiers {
		resp := &planmodifier.StringResponse{}
		modifier.PlanModifyString(ctx, planmodifier.StringRequest{
			Path:        path.Root("domain"),
			Config:      config,
			State:       state,
			Plan:        plan,
			ConfigValue: types.StringValue("after.example"),
			StateValue:  types.StringValue("before.example"),
			PlanValue:   types.StringValue("after.example"),
		}, resp)
		if !resp.RequiresReplace {
			t.Errorf("domain plan modifier %d does not require replacement when the value changes", i)
		}
	}
}

// existingResourceRaw builds the raw object of an already-created resource: every
// attribute null, but the object itself known. The framework's RequiresReplace
// modifier reads nothing else from a request's State and Plan — it no-ops when
// either raw value is null, which is how it recognises create and destroy — so a
// known object is what turns the request under test into an update.
func existingResourceRaw(ctx context.Context, s rschema.Schema) tftypes.Value {
	objectType, ok := s.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		return tftypes.Value{}
	}
	values := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		values[name] = tftypes.NewValue(attributeType, nil)
	}
	return tftypes.NewValue(objectType, values)
}

// TestDomainResource_DomainValidators pins that the domain name is checked at
// plan time. The lower-case rule is the load-bearing one: Jamf lower-cases the
// value it stores, and a Required attribute cannot be canonicalised by a plan
// modifier, so strict acceptance is the only correct option.
func TestDomainResource_DomainValidators(t *testing.T) {
	s := resourceSchema(t)

	domain, ok := s.Attributes["domain"].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("domain must be a StringAttribute, got %T", s.Attributes["domain"])
	}
	if len(domain.Validators) < 2 {
		t.Errorf("domain must be length-bounded and syntax-checked, got %d validators", len(domain.Validators))
	}
}

// TestDomainResource_VerificationStatusDocumentsTheVocabulary pins that the
// status description names every value Jamf declares, including the two the
// console has no label for.
func TestDomainResource_VerificationStatusDocumentsTheVocabulary(t *testing.T) {
	s := resourceSchema(t)

	status, ok := s.Attributes["verification_status"].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("verification_status must be a StringAttribute, got %T", s.Attributes["verification_status"])
	}
	assertDocumentsEveryStatus(t, status.MarkdownDescription)
}

func TestDomainResource_IdentitySchema(t *testing.T) {
	r := NewDomainResource()
	var resp resource.IdentitySchemaResponse
	r.(*DomainResource).IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)

	if _, ok := resp.IdentitySchema.Attributes["domain"]; !ok {
		t.Errorf("identity schema must be keyed on domain, got %v", resp.IdentitySchema.Attributes)
	}
	if _, ok := resp.IdentitySchema.Attributes["id"]; ok {
		t.Error("identity schema must not be keyed on id — a withdrawn and re-made claim gets a new one")
	}
}

func TestDomainDataSource_Metadata(t *testing.T) {
	d := NewDomainDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*DomainDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_account_sso_domain" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_account_sso_domain", resp.TypeName)
	}
}

// TestDomainDataSource_Schema also pins the List shape of the assignment
// collection: data source attributes returning Jamf's own data are always lists.
func TestDomainDataSource_Schema(t *testing.T) {
	s := dataSourceSchema(t)

	for _, name := range append(resourceAttributeNames, "assigned_connections", "jamf_id_enabled") {
		if name == "timeouts" {
			continue
		}
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if !s.Attributes["domain"].IsRequired() {
		t.Error("data source domain must be required — it is the only key a single claim can be looked up by")
	}
	if _, ok := s.Attributes["id"]; !ok {
		t.Error("data source must expose the Jamf-assigned id")
	}
	if s.Attributes["id"].IsOptional() {
		t.Error("data source id must be computed-only: nothing reads a single claim by it")
	}
	if _, ok := s.Attributes["assigned_connections"].(dsschema.ListNestedAttribute); !ok {
		t.Errorf("assigned_connections must be a ListNestedAttribute, got %T", s.Attributes["assigned_connections"])
	}
}

func TestDomainDataSource_AssignedConnectionAttributes(t *testing.T) {
	s := dataSourceSchema(t)

	nested, ok := s.Attributes["assigned_connections"].(dsschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("assigned_connections must be a ListNestedAttribute, got %T", s.Attributes["assigned_connections"])
	}
	for _, name := range []string{"connection_id", "connection_organization_id", "region"} {
		attr, present := nested.NestedObject.Attributes[name]
		if !present {
			t.Errorf("assigned_connections missing nested attribute %q", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("assigned_connections.%s must be computed", name)
		}
	}
}

func TestDomainListResource_Metadata(t *testing.T) {
	r := NewDomainListResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*DomainListResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_account_sso_domain" {
		t.Errorf("expected list type name %q, got %q", "jamfplatform_account_sso_domain", resp.TypeName)
	}
}

// TestDomainListResource_Schema asserts the config schema holds nothing but the
// shared timeouts attribute. The domain collection accepts neither a filter nor a sort expression, so an attribute
// appearing here would mean a filter was added without wiring it.
func TestDomainListResource_Schema(t *testing.T) {
	r := NewDomainListResource()
	var resp list.ListResourceSchemaResponse
	r.(*DomainListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["timeouts"]; !ok || len(resp.Schema.Attributes) != 1 {
		t.Errorf("list schema must take only timeouts, got %v", resp.Schema.Attributes)
	}
}

// resourceSchema builds the resource schema once per test.
func resourceSchema(t *testing.T) rschema.Schema {
	t.Helper()
	r := NewDomainResource()
	var resp resource.SchemaResponse
	r.(*DomainResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

// dataSourceSchema builds the singular data source schema once per test.
func dataSourceSchema(t *testing.T) dsschema.Schema {
	t.Helper()
	d := NewDomainDataSource()
	var resp datasource.SchemaResponse
	d.(*DomainDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}
