// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ztna_app

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestZtnaAppResource_Metadata(t *testing.T) {
	r := NewZtnaAppResource()
	var resp resource.MetadataResponse
	r.(*ZtnaAppResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_ztna_app" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_security_cloud_ztna_app", resp.TypeName)
	}
}

func TestZtnaAppResource_Schema(t *testing.T) {
	s := resourceSchema(t)

	want := []string{
		"id", "name", "predefined_app_id", "app_type", "category", "hostnames",
		"direct_ips_and_subnets", "all_device_groups", "device_group_ids", "routing",
		"routing_overrides", "security", "timeouts",
	}
	for _, name := range want {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	for _, name := range []string{"category", "all_device_groups", "routing"} {
		if !s.Attributes[name].IsRequired() {
			t.Errorf("%s must be required", name)
		}
	}
	for _, name := range []string{"name", "predefined_app_id", "hostnames", "direct_ips_and_subnets", "device_group_ids", "routing_overrides", "security"} {
		if !s.Attributes[name].IsOptional() || s.Attributes[name].IsComputed() {
			t.Errorf("%s must be optional-only, got optional=%v computed=%v",
				name, s.Attributes[name].IsOptional(), s.Attributes[name].IsComputed())
		}
	}
	for _, name := range []string{"id", "app_type"} {
		if s.Attributes[name].IsRequired() || s.Attributes[name].IsOptional() || !s.Attributes[name].IsComputed() {
			t.Errorf("%s must be computed-only", name)
		}
	}
}

// TestZtnaAppResource_CollectionsAreSets pins the Set choice. All three flat
// collections come back from Jamf Security Cloud re-ordered and de-duplicated
// (wire-verified 2026-08-30), so a List would fail "produced inconsistent result
// after apply".
func TestZtnaAppResource_CollectionsAreSets(t *testing.T) {
	s := resourceSchema(t)
	for _, name := range []string{"hostnames", "direct_ips_and_subnets", "device_group_ids"} {
		if _, ok := s.Attributes[name].(rschema.SetAttribute); !ok {
			t.Errorf("%s must be a SetAttribute, got %T", name, s.Attributes[name])
		}
	}
}

// TestZtnaAppResource_RoutingOverridesIsList pins the List choice for the
// overrides, which is the one collection the wire treats as ordered: it echoes the
// array in send order and addresses errors by index
// (`groupOverrides.routingOverrides[0].routing`).
func TestZtnaAppResource_RoutingOverridesIsList(t *testing.T) {
	s := resourceSchema(t)
	overrides, ok := s.Attributes["routing_overrides"].(rschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("routing_overrides must be a ListNestedAttribute, got %T", s.Attributes["routing_overrides"])
	}
	for _, name := range []string{"device_group_ids", "routing"} {
		if _, present := overrides.NestedObject.Attributes[name]; !present {
			t.Errorf("routing_overrides missing nested attribute %q", name)
		}
	}
	if _, ok := overrides.NestedObject.Attributes["device_group_ids"].(rschema.SetAttribute); !ok {
		t.Errorf("routing_overrides[].device_group_ids must be a SetAttribute")
	}
}

// TestZtnaAppResource_PredefinedAppIDRequiresReplace pins the immutability of the
// application's form. The patch body carries no predefinedAppId field at all, so
// there is no in-place edit to attempt in either direction.
func TestZtnaAppResource_PredefinedAppIDRequiresReplace(t *testing.T) {
	s := resourceSchema(t)
	attr, ok := s.Attributes["predefined_app_id"].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("predefined_app_id must be a StringAttribute, got %T", s.Attributes["predefined_app_id"])
	}
	if len(attr.PlanModifiers) == 0 {
		t.Fatal("predefined_app_id must carry a RequiresReplace plan modifier")
	}
}

// TestZtnaAppResource_RoutingBlocksShareShape pins that the application's routing
// and each override's routing expose the same three attributes, which is what lets
// one builder serve both and mirrors the wire sending the same object in both
// positions.
func TestZtnaAppResource_RoutingBlocksShareShape(t *testing.T) {
	s := resourceSchema(t)

	top, ok := s.Attributes["routing"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("routing must be a SingleNestedAttribute, got %T", s.Attributes["routing"])
	}
	overrides := s.Attributes["routing_overrides"].(rschema.ListNestedAttribute)
	nested, ok := overrides.NestedObject.Attributes["routing"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("routing_overrides[].routing must be a SingleNestedAttribute")
	}

	for _, name := range []string{"traffic_routing", "gateway_id", "routing_mode"} {
		if _, present := top.Attributes[name]; !present {
			t.Errorf("routing missing %q", name)
		}
		if _, present := nested.Attributes[name]; !present {
			t.Errorf("routing_overrides[].routing missing %q", name)
		}
	}
	if !top.Attributes["traffic_routing"].IsRequired() {
		t.Error("routing.traffic_routing must be required")
	}
	for _, name := range []string{"gateway_id", "routing_mode"} {
		if !top.Attributes[name].IsOptional() || top.Attributes[name].IsComputed() {
			t.Errorf("routing.%s must be optional-only: it has to round-trip exactly, including absence", name)
		}
	}
}

// TestZtnaAppResource_SecurityCardsAreOptionalWithDefaultedLeaves pins the shape the
// security block needs to be safe.
//
// The cards are Optional-only so an omitted card leaves Jamf's setting alone, which
// the read path implements by filling only what state already declares. The leaves
// are Optional+Computed with defaults because a card maps to a whole wire object of
// non-pointer members — a declared card must send every value, so no leaf may be
// unknown at plan time.
func TestZtnaAppResource_SecurityCardsAreOptionalWithDefaultedLeaves(t *testing.T) {
	s := resourceSchema(t)
	security, ok := s.Attributes["security"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("security must be a SingleNestedAttribute, got %T", s.Attributes["security"])
	}

	cards := map[string][]string{
		"managed_device": {"enabled", "device_push_notifications"},
		"device_risk":    {"enabled", "deny_at_risk_level", "device_push_notifications"},
		"jamf_trust":     {"enabled", "device_push_notifications"},
	}
	for card, leaves := range cards {
		attr, present := security.Attributes[card]
		if !present {
			t.Errorf("security missing card %q", card)
			continue
		}
		if !attr.IsOptional() || attr.IsComputed() {
			t.Errorf("security.%s must be optional-only, got optional=%v computed=%v",
				card, attr.IsOptional(), attr.IsComputed())
		}
		nested, ok := attr.(rschema.SingleNestedAttribute)
		if !ok {
			t.Errorf("security.%s must be a SingleNestedAttribute, got %T", card, attr)
			continue
		}
		for _, leaf := range leaves {
			leafAttr, present := nested.Attributes[leaf]
			if !present {
				t.Errorf("security.%s missing %q", card, leaf)
				continue
			}
			if !leafAttr.IsOptional() || !leafAttr.IsComputed() {
				t.Errorf("security.%s.%s must be optional+computed", card, leaf)
			}
		}
	}
}

// TestZtnaAppResource_DescriptionsAreUIAligned pins STYLE_GUIDE §"User-facing
// descriptions are UI-aligned, not wire-aligned" mechanically for this package,
// because a prose regression is invisible to every other test here. Product framing
// ("Jamf Security Cloud rejects...") is fine; protocol framing is not.
func TestZtnaAppResource_DescriptionsAreUIAligned(t *testing.T) {
	banned := []string{
		"bareIps", "categoryName", "dohIntegration", "dnsIpResolutionType", "riskControls",
		"groupOverrides", "predefinedAppId", "allUsers", "notificationsEnabled",
		"PATCH", "POST", "DELETE", "409", "400", "merge patch", "endpoint",
	}
	for name, description := range collectDescriptions(t) {
		for _, word := range banned {
			if strings.Contains(description, word) {
				t.Errorf("%s description carries wire vocabulary %q: %s", name, word, description)
			}
		}
	}
}

// collectDescriptions returns every MarkdownDescription in the resource schema,
// keyed by a readable path, so the vocabulary check covers nested attributes too.
func collectDescriptions(t *testing.T) map[string]string {
	t.Helper()
	s := resourceSchema(t)
	out := map[string]string{"(resource)": s.MarkdownDescription}

	var walk func(prefix string, attrs map[string]rschema.Attribute)
	walk = func(prefix string, attrs map[string]rschema.Attribute) {
		for name, attr := range attrs {
			path := prefix + name
			out[path] = attr.GetMarkdownDescription()
			switch typed := attr.(type) {
			case rschema.SingleNestedAttribute:
				walk(path+".", typed.Attributes)
			case rschema.ListNestedAttribute:
				walk(path+"[].", typed.NestedObject.Attributes)
			case rschema.SetNestedAttribute:
				walk(path+"[].", typed.NestedObject.Attributes)
			}
		}
	}
	walk("", s.Attributes)
	return out
}

func TestZtnaAppDataSource_Metadata(t *testing.T) {
	d := NewZtnaAppDataSource()
	var resp datasource.MetadataResponse
	d.(*ZtnaAppDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_ztna_app" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_security_cloud_ztna_app", resp.TypeName)
	}
}

// TestZtnaAppDataSource_Schema pins the three lookup keys and that every collection
// is a list. A data source reports API data, which is always read-only, so nothing
// here needs a set.
func TestZtnaAppDataSource_Schema(t *testing.T) {
	d := NewZtnaAppDataSource()
	var resp datasource.SchemaResponse
	d.(*ZtnaAppDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	s := resp.Schema

	for _, name := range []string{"id", "name", "predefined_app_id"} {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("missing lookup key %q", name)
			continue
		}
		if !attr.IsOptional() || !attr.IsComputed() {
			t.Errorf("%s must be optional+computed so it can be either supplied or resolved", name)
		}
	}
	for _, name := range []string{"hostnames", "direct_ips_and_subnets", "device_group_ids"} {
		if _, ok := s.Attributes[name].(dsschema.ListAttribute); !ok {
			t.Errorf("%s must be a ListAttribute on the data source, got %T", name, s.Attributes[name])
		}
	}
}

// TestZtnaAppDataSource_ConfigValidators pins that exactly one lookup key is
// enforced. Without it a config setting none of the three would reach the lookup
// with nothing to look up.
func TestZtnaAppDataSource_ConfigValidators(t *testing.T) {
	d := NewZtnaAppDataSource().(*ZtnaAppDataSource)
	if got := len(d.ConfigValidators(context.Background())); got != 1 {
		t.Fatalf("expected 1 config validator, got %d", got)
	}
}

// TestZtnaAppResource_ConfigValidators pins that all four cross-field rules are
// wired in. Each exists because the server's own refusal names too little to act on
// — or, for the predefined-name case, because it does not refuse at all.
func TestZtnaAppResource_ConfigValidators(t *testing.T) {
	r := NewZtnaAppResource().(*ZtnaAppResource)
	if got := len(r.ConfigValidators(context.Background())); got != 4 {
		t.Fatalf("expected 4 config validators, got %d", got)
	}
}

func TestZtnaAppListResource_Metadata(t *testing.T) {
	lr := NewZtnaAppListResource()
	var resp resource.MetadataResponse
	lr.(*ZtnaAppListResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)

	if resp.TypeName != "jamfplatform_security_cloud_ztna_app" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_security_cloud_ztna_app", resp.TypeName)
	}
}

func TestZtnaAppListResource_ConfigSchema(t *testing.T) {
	lr := NewZtnaAppListResource().(*ZtnaAppListResource)
	var resp list.ListResourceSchemaResponse
	lr.ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if got := len(resp.Schema.Attributes); got != 1 {
		t.Errorf("expected only the timeouts attribute, got %d", got)
	}
	if _, ok := resp.Schema.Attributes["timeouts"]; !ok {
		t.Errorf("expected the timeouts attribute, got %v", resp.Schema.Attributes)
	}
}

// TestZtnaAppResource_IdentitySchema pins the import identity.
func TestZtnaAppResource_IdentitySchema(t *testing.T) {
	r := NewZtnaAppResource().(*ZtnaAppResource)
	var resp resource.IdentitySchemaResponse
	r.IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)

	if _, ok := resp.IdentitySchema.Attributes["id"]; !ok {
		t.Fatal("identity schema must carry an id attribute")
	}
}

// TestZtnaAppResource_CollectionsCarryMinimumSizeValidators pins the invariant the
// read path depends on: every collection attribute anywhere in this resource's
// schema must be constrained to a minimum size, so an explicit empty collection is
// refused at plan time.
//
// state_builders.go collapses an empty response collection to null — stringSetOrNull
// for the flat sets, routingOverrideListValue for the overrides — and justifies that
// by the schema refusing an explicit empty collection. routing_overrides shipped
// without a size validator, which made the justification false for it: with
// `routing_overrides = []` every apply failed with "Provider produced inconsistent
// result after apply: .routing_overrides: was cty.ListValEmpty(...), but now null",
// and because the configuration never stops saying `[]` there was no recovery from
// inside Terraform. The walk covers the whole schema, and the named list is checked
// separately, so both a collection added later without a size validator and one of
// these four ceasing to be a collection fail here rather than at apply.
func TestZtnaAppResource_CollectionsCarryMinimumSizeValidators(t *testing.T) {
	collections := collectCollectionValidators(t)

	for _, name := range []string{"hostnames", "direct_ips_and_subnets", "device_group_ids", "routing_overrides"} {
		if _, ok := collections[name]; !ok {
			t.Errorf("%s is no longer a collection attribute, but the empty-to-null collapse in state_builders.go was written for it", name)
		}
	}

	for path, descriptions := range collections {
		if len(descriptions) == 0 {
			t.Errorf("%s carries no validators: without a minimum-size validator, `%s = []` can never converge", path, path)
			continue
		}
		if !slices.ContainsFunc(descriptions, func(description string) bool {
			return strings.Contains(description, "at least")
		}) {
			t.Errorf("%s carries validators but none constrains its minimum size, so `%s = []` can never converge: %v", path, path, descriptions)
		}
	}
}

// collectCollectionValidators returns the validator descriptions of every collection
// attribute in the resource schema, keyed by a readable path, so the size-validator
// invariant is checked against nested collections too.
func collectCollectionValidators(t *testing.T) map[string][]string {
	t.Helper()
	ctx := context.Background()
	s := resourceSchema(t)
	out := map[string][]string{}

	var walk func(prefix string, attrs map[string]rschema.Attribute)
	walk = func(prefix string, attrs map[string]rschema.Attribute) {
		for name, attr := range attrs {
			path := prefix + name
			switch typed := attr.(type) {
			case rschema.SetAttribute:
				out[path] = validatorDescriptions(ctx, typed.Validators)
			case rschema.ListAttribute:
				out[path] = validatorDescriptions(ctx, typed.Validators)
			case rschema.MapAttribute:
				out[path] = validatorDescriptions(ctx, typed.Validators)
			case rschema.SetNestedAttribute:
				out[path] = validatorDescriptions(ctx, typed.Validators)
				walk(path+"[].", typed.NestedObject.Attributes)
			case rschema.ListNestedAttribute:
				out[path] = validatorDescriptions(ctx, typed.Validators)
				walk(path+"[].", typed.NestedObject.Attributes)
			case rschema.MapNestedAttribute:
				out[path] = validatorDescriptions(ctx, typed.Validators)
				walk(path+"[].", typed.NestedObject.Attributes)
			case rschema.SingleNestedAttribute:
				walk(path+".", typed.Attributes)
			}
		}
	}
	walk("", s.Attributes)
	return out
}

// validatorDescriptions reads the descriptions off a typed validator slice, which is
// the only vocabulary every validator kind shares.
func validatorDescriptions[T interface{ Description(context.Context) string }](ctx context.Context, validators []T) []string {
	out := make([]string, 0, len(validators))
	for _, v := range validators {
		out = append(out, v.Description(ctx))
	}
	return out
}

// resourceSchema builds the resource schema once per test, failing on diagnostics.
func resourceSchema(t *testing.T) rschema.Schema {
	t.Helper()
	r := NewZtnaAppResource()
	var resp resource.SchemaResponse
	r.(*ZtnaAppResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}
