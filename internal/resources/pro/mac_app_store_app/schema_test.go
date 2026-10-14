// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mac_app_store_app

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/iconmirror"
)

const wantTypeName = "jamfplatform_pro_mac_app_store_app"

func TestMacAppResource_Metadata(t *testing.T) {
	r := NewMacAppResource()
	var resp resource.MetadataResponse
	r.(*MacAppResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != wantTypeName {
		t.Errorf("expected type name %q, got %q", wantTypeName, resp.TypeName)
	}
}

func TestMacAppResource_Schema(t *testing.T) {
	r := NewMacAppResource()
	var resp resource.SchemaResponse
	r.(*MacAppResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, name := range []string{"id", "general", "scope", "self_service", "vpp", "timeouts"} {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing top-level attribute %q", name)
		}
	}

	if id := s.Attributes["id"]; id.IsRequired() || !id.IsComputed() {
		t.Errorf("id must be computed-only")
	}
	if g := s.Attributes["general"]; !g.IsRequired() {
		t.Errorf("general must be required")
	}

	general, ok := s.Attributes["general"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("general must be a SingleNestedAttribute")
	}
	for _, name := range []string{"name", "version", "bundle_id", "url"} {
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

	vpp, ok := s.Attributes["vpp"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("vpp must be a SingleNestedAttribute")
	}
	for _, name := range []string{"total_vpp_licenses", "remaining_vpp_licenses", "used_vpp_licenses"} {
		a, ok := vpp.Attributes[name]
		if !ok {
			t.Errorf("vpp missing %q", name)
			continue
		}
		if a.IsOptional() || a.IsRequired() || !a.IsComputed() {
			t.Errorf("vpp.%s must be computed-only", name)
		}
	}
}

func TestMacAppDataSource_Metadata(t *testing.T) {
	d := NewMacAppDataSource()
	var resp datasource.MetadataResponse
	d.(*MacAppDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != wantTypeName {
		t.Errorf("expected type name %q, got %q", wantTypeName, resp.TypeName)
	}
}

func TestMacAppDataSource_ConfigValidators(t *testing.T) {
	d := NewMacAppDataSource().(*MacAppDataSource)
	if got := d.ConfigValidators(context.Background()); len(got) != 1 {
		t.Fatalf("expected 1 config validator, got %d", len(got))
	}
}

func TestMacAppListResource_Schema(t *testing.T) {
	r := NewMacAppListResource()
	var resp list.ListResourceSchemaResponse
	r.(*MacAppListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["filter"]; !ok {
		t.Errorf("list schema missing filter attribute")
	}
}

// TestMacAppResource_SelfServiceIconURIDerivedFromID pins uri as a Computed-only
// attribute that follows id. Jamf Pro derives it from the icon the id names and
// ignores an authored value; UseNonNullStateForUnknown carried the previous
// icon's uri into a plan that repointed id. See issue #468.
func TestMacAppResource_SelfServiceIconURIDerivedFromID(t *testing.T) {
	var resp resource.SchemaResponse
	NewMacAppResource().(*MacAppResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	icon := resp.Schema.Attributes["self_service"].(schema.SingleNestedAttribute).Attributes["self_service_icon"].(schema.SingleNestedAttribute)
	iconmirror.AssertPlan(t, "uri", icon.Attributes["uri"])
	if id, ok := icon.Attributes["id"].(schema.StringAttribute); !ok || !id.Optional || !id.Computed {
		t.Errorf("self_service_icon.id must stay Optional+Computed: it is the one input")
	}
}
