// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package ebook

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/keyedcategories"
)

const wantTypeName = "jamfplatform_pro_ebook"

func TestEbookResource_Metadata(t *testing.T) {
	r := NewEbookResource()
	var resp resource.MetadataResponse
	r.(*EbookResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != wantTypeName {
		t.Errorf("expected type name %q, got %q", wantTypeName, resp.TypeName)
	}
}

func TestEbookResource_Schema(t *testing.T) {
	r := NewEbookResource()
	var resp resource.SchemaResponse
	r.(*EbookResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, name := range []string{"id", "general", "scope", "self_service", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing top-level attribute %q", name)
		}
	}

	if id := s.Attributes["id"]; id.IsRequired() || !id.IsComputed() {
		t.Errorf("id must be computed-only")
	}

	general, ok := s.Attributes["general"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("general must be a SingleNestedAttribute")
	}
	if !s.Attributes["general"].IsRequired() {
		t.Errorf("general must be required")
	}
	for _, name := range []string{"name", "url"} {
		a, ok := general.Attributes[name]
		if !ok {
			t.Errorf("general missing %q", name)
			continue
		}
		if !a.IsRequired() {
			t.Errorf("general.%s must be required", name)
		}
	}
	for _, name := range []string{"category_name", "site_name"} {
		a, ok := general.Attributes[name]
		if !ok {
			t.Errorf("general missing %q", name)
			continue
		}
		if a.IsRequired() || a.IsOptional() || !a.IsComputed() {
			t.Errorf("general.%s must be computed-only", name)
		}
	}
	for _, name := range []string{"file_type", "version", "deployment_type"} {
		a, ok := general.Attributes[name]
		if !ok {
			t.Errorf("general missing %q", name)
			continue
		}
		if !a.IsOptional() || !a.IsComputed() {
			t.Errorf("general.%s must be optional+computed", name)
		}
	}
}

func TestEbookResource_ScopeIsDualTargetUnion(t *testing.T) {
	r := NewEbookResource()
	var resp resource.SchemaResponse
	r.(*EbookResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	scopeAttr, ok := resp.Schema.Attributes["scope"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("scope must be a SingleNestedAttribute")
	}

	// scope splits into targets / limitations / exclusions, mirroring the UI.
	for _, name := range []string{"targets", "limitations", "exclusions"} {
		if _, ok := scopeAttr.Attributes[name]; !ok {
			t.Errorf("scope missing %q", name)
		}
	}

	targetsAttr, ok := scopeAttr.Attributes["targets"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("scope.targets must be a SingleNestedAttribute")
	}

	// The dual-target union nests under targets: computer + mobile + user
	// targets, plus classes.
	wantTargets := []string{
		"all_computers", "all_mobile_devices", "all_jss_users",
		"computer_ids", "computer_group_ids",
		"mobile_device_ids", "mobile_device_group_ids",
		"building_ids", "department_ids",
		"user_ids", "user_group_ids",
		"class_ids",
	}
	for _, name := range wantTargets {
		if _, ok := targetsAttr.Attributes[name]; !ok {
			t.Errorf("scope.targets missing %q", name)
		}
	}

	// No iBeacon targets anywhere in ebook scope.
	if _, ok := targetsAttr.Attributes["ibeacon_ids"]; ok {
		t.Errorf("scope.targets must NOT expose ibeacon_ids")
	}
	excl, ok := scopeAttr.Attributes["exclusions"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("scope.exclusions must be a SingleNestedAttribute")
	}
	if _, ok := excl.Attributes["ibeacon_ids"]; ok {
		t.Errorf("scope.exclusions must NOT expose ibeacon_ids")
	}
	// Exclusions carry the mobile targets too (full union).
	for _, name := range []string{"mobile_device_ids", "mobile_device_group_ids"} {
		if _, ok := excl.Attributes[name]; !ok {
			t.Errorf("scope.exclusions missing %q", name)
		}
	}

	// All three all-flags must carry a validator (the AllFlagConflictsWith
	// guard) and be Optional-only — the null/false distinction carries the
	// granular per-category ownership contract, so Computed is forbidden.
	for _, name := range []string{"all_computers", "all_mobile_devices", "all_jss_users"} {
		ba, ok := targetsAttr.Attributes[name].(schema.BoolAttribute)
		if !ok {
			t.Errorf("scope.targets.%s must be a BoolAttribute", name)
			continue
		}
		if len(ba.Validators) == 0 {
			t.Errorf("scope.targets.%s must declare an all-flag conflict validator", name)
		}
		if !ba.IsOptional() || ba.IsComputed() {
			t.Errorf("scope.targets.%s must be Optional-only (ownership contract), got optional=%v computed=%v", name, ba.IsOptional(), ba.IsComputed())
		}
	}
}

func TestEbookResource_SelfServiceCategories(t *testing.T) {
	r := NewEbookResource()
	var resp resource.SchemaResponse
	r.(*EbookResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	ss, ok := resp.Schema.Attributes["self_service"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("self_service must be a SingleNestedAttribute")
	}
	cats, ok := ss.Attributes["categories"].(schema.SetNestedAttribute)
	if !ok {
		t.Fatalf("self_service.categories must be a SetNestedAttribute")
	}
	if idAttr, ok := cats.NestedObject.Attributes["id"]; !ok || !idAttr.IsRequired() {
		t.Errorf("categories.id must be required")
	}
	if _, ok := ss.Attributes["icon_id"]; !ok {
		t.Errorf("self_service missing icon_id")
	}
	if uri := ss.Attributes["icon_uri"]; uri.IsOptional() || uri.IsRequired() || !uri.IsComputed() {
		t.Errorf("self_service.icon_uri must be computed-only")
	}
}

func TestEbookDataSource_Metadata(t *testing.T) {
	d := NewEbookDataSource()
	var resp datasource.MetadataResponse
	d.(*EbookDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != wantTypeName {
		t.Errorf("expected type name %q, got %q", wantTypeName, resp.TypeName)
	}
}

func TestEbookDataSource_ConfigValidators(t *testing.T) {
	d := NewEbookDataSource().(*EbookDataSource)
	if got := d.ConfigValidators(context.Background()); len(got) != 1 {
		t.Fatalf("expected 1 config validator, got %d", len(got))
	}
}

func TestEbookListResource_Schema(t *testing.T) {
	r := NewEbookListResource()
	var resp list.ListResourceSchemaResponse
	r.(*EbookListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["filter"]; !ok {
		t.Errorf("list schema missing filter attribute")
	}
}

// TestEbookResource_SelfServiceCategoriesCarryByID pins the category element
// shape: Jamf Pro resolves a category by id, so name is Computed-only, and it
// and the flags carry prior state by id rather than by position. See
// keyedcategories.AssertCarriedByID.
func TestEbookResource_SelfServiceCategoriesCarryByID(t *testing.T) {
	t.Parallel()
	var resp resource.SchemaResponse
	NewEbookResource().(*EbookResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	ss := resp.Schema.Attributes["self_service"].(schema.SingleNestedAttribute)
	keyedcategories.AssertCarriedByID(t, ss.Attributes["categories"].(schema.SetNestedAttribute).NestedObject.Attributes, "display_in", "feature_in")
}
