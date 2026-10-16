// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package app_store_app

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func dataSourceSchema(t *testing.T) dsschema.Schema {
	t.Helper()
	var resp datasource.SchemaResponse
	NewAppStoreAppDataSource().Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestAppStoreAppDataSource_Metadata(t *testing.T) {
	var resp datasource.MetadataResponse
	NewAppStoreAppDataSource().Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != "jamfplatform_apple_app_store_app" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestAppStoreAppDataSource_SchemaModes(t *testing.T) {
	s := dataSourceSchema(t)

	optionalComputed := []string{"id", "bundle_id", "country_region"}
	optionalOnly := []string{"store_url", "language"}
	computedOnly := []string{"name", "version", "url", "description", "icon_url", "is_free", "genres", "vpp_device_based_licensing"}

	for _, name := range optionalComputed {
		a := s.Attributes[name]
		if a == nil || !a.IsOptional() || !a.IsComputed() {
			t.Errorf("%s must be Optional+Computed", name)
		}
	}
	for _, name := range optionalOnly {
		a := s.Attributes[name]
		if a == nil || !a.IsOptional() || a.IsComputed() {
			t.Errorf("%s must be Optional only", name)
		}
	}
	for _, name := range computedOnly {
		a := s.Attributes[name]
		if a == nil || a.IsOptional() || a.IsRequired() || !a.IsComputed() {
			t.Errorf("%s must be Computed only", name)
		}
	}
	if !s.Attributes["target"].IsRequired() {
		t.Error("target must be Required")
	}
	if _, ok := s.Attributes["timeouts"]; !ok {
		t.Error("timeouts attribute missing")
	}
	if _, ok := s.Attributes["genres"].(dsschema.ListAttribute); !ok {
		t.Errorf("genres must be a ListAttribute, got %T", s.Attributes["genres"])
	}
	want := len(optionalComputed) + len(optionalOnly) + len(computedOnly) + 2
	if len(s.Attributes) != want {
		t.Errorf("schema has %d attributes, want %d; a new one needs a mode assertion here", len(s.Attributes), want)
	}
}

func TestAppStoreAppDataSource_ExactlyOneSelector(t *testing.T) {
	validators := NewAppStoreAppDataSource().(*AppStoreAppDataSource).ConfigValidators(context.Background())
	if len(validators) != 1 {
		t.Fatalf("ConfigValidators = %d, want 1", len(validators))
	}
	desc := validators[0].Description(context.Background())
	for _, p := range []string{"id", "bundle_id", "store_url"} {
		if !strings.Contains(desc, p) {
			t.Errorf("ExactlyOneOf description %q does not name %s", desc, p)
		}
	}
}

// stringPasses reports whether every validator on a string attribute accepts v.
func stringPasses(t *testing.T, s dsschema.Schema, name, v string) bool {
	t.Helper()
	attr, ok := s.Attributes[name].(dsschema.StringAttribute)
	if !ok {
		t.Fatalf("%s is not a StringAttribute", name)
	}
	for _, val := range attr.Validators {
		var resp validator.StringResponse
		val.ValidateString(context.Background(), validator.StringRequest{Path: path.Root(name), ConfigValue: types.StringValue(v)}, &resp)
		if resp.Diagnostics.HasError() {
			return false
		}
	}
	return true
}

func TestAppStoreAppDataSource_Validators(t *testing.T) {
	s := dataSourceSchema(t)
	cases := []struct {
		attr, value string
		ok          bool
	}{
		{"id", "497799835", true},
		{"id", "id497799835", false},
		{"id", "1,2", false},
		{"id", "", false},
		{"bundle_id", "com.apple.Pages", true},
		{"bundle_id", "a,b", false},
		{"bundle_id", "", false},
		{"store_url", "https://apps.apple.com/gb/app/pages-create-documents/id361309726", true},
		{"store_url", "https://example.com/app/id1", false},
		{"target", "mac", true},
		{"target", "mobile_device", true},
		{"target", "mobile", false},
		{"target", "ios", false},
		{"country_region", "GB", true},
		{"country_region", "gb", false},
		{"country_region", "GBR", false},
		{"language", "fr", true},
		{"language", "", false},
	}
	for _, tc := range cases {
		if got := stringPasses(t, s, tc.attr, tc.value); got != tc.ok {
			t.Errorf("%s = %q: accepted %v, want %v", tc.attr, tc.value, got, tc.ok)
		}
	}
}

func TestStoreURLValidator_DefersOnUnknownAndNull(t *testing.T) {
	for _, v := range []types.String{types.StringUnknown(), types.StringNull()} {
		var resp validator.StringResponse
		storeURLValidator{}.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("store_url"), ConfigValue: v}, &resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%v: %v", v, resp.Diagnostics)
		}
	}
}

func TestAppStoreAppDataSource_DescriptionNamesTargetValues(t *testing.T) {
	desc := dataSourceSchema(t).Attributes["target"].GetMarkdownDescription()
	for _, v := range targetValues() {
		if !strings.Contains(desc, "`"+v+"`") {
			t.Errorf("target description does not list %q", v)
		}
	}
}
