// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package platform_environment

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"
)

var resourceAttributeNames = []string{"id", "name", "type", "region", "timeouts"}

func resourceSchema(t *testing.T) rschema.Schema {
	t.Helper()
	var resp resource.SchemaResponse
	NewPlatformEnvironmentResource().(*PlatformEnvironmentResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func dataSourceSchema(t *testing.T) dsschema.Schema {
	t.Helper()
	var resp datasource.SchemaResponse
	NewPlatformEnvironmentDataSource().(*PlatformEnvironmentDataSource).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func TestPlatformEnvironmentResource_Metadata(t *testing.T) {
	var resp resource.MetadataResponse
	NewPlatformEnvironmentResource().(*PlatformEnvironmentResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != "jamfplatform_account_platform_environment" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestPlatformEnvironmentResource_Schema(t *testing.T) {
	s := resourceSchema(t)

	for _, name := range resourceAttributeNames {
		if _, ok := s.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(s.Attributes) != len(resourceAttributeNames) {
		t.Errorf("schema declares %d attributes, the test knows %d — update resourceAttributeNames", len(s.Attributes), len(resourceAttributeNames))
	}
	for _, name := range []string{"name", "type"} {
		if !s.Attributes[name].IsRequired() {
			t.Errorf("%s must be required", name)
		}
	}
	region := s.Attributes["region"]
	if !region.IsOptional() || region.IsComputed() {
		t.Errorf("region must be Optional only (null means global), got optional=%v computed=%v", region.IsOptional(), region.IsComputed())
	}
	if id := s.Attributes["id"]; !id.IsComputed() || id.IsOptional() || id.IsRequired() {
		t.Error("id must be computed only")
	}
}

func TestPlatformEnvironmentResource_RegionRequiresReplace(t *testing.T) {
	ctx := context.Background()
	s := resourceSchema(t)
	raw := objectRaw(ctx, s.Type(), nil, nil)

	region, ok := s.Attributes["region"].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("region must be a StringAttribute, got %T", s.Attributes["region"])
	}
	if len(region.PlanModifiers) != 1 {
		t.Fatalf("region has %d plan modifiers, expected exactly 1 (RequiresReplace)", len(region.PlanModifiers))
	}
	resp := &planmodifier.StringResponse{}
	region.PlanModifiers[0].PlanModifyString(ctx, planmodifier.StringRequest{
		Path:        path.Root("region"),
		Config:      tfsdk.Config{Schema: s, Raw: raw},
		State:       tfsdk.State{Schema: s, Raw: raw},
		Plan:        tfsdk.Plan{Schema: s, Raw: raw},
		ConfigValue: types.StringValue("US"),
		StateValue:  types.StringValue("EU"),
		PlanValue:   types.StringValue("US"),
	}, resp)
	if !resp.RequiresReplace {
		t.Error("changing region must require replacement: Jamf Account fixes it when the environment is created")
	}
}

func runStringValidators(t *testing.T, validators []validator.String, value string) (errors int) {
	t.Helper()
	for _, v := range validators {
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("x"), ConfigValue: types.StringValue(value)}, resp)
		errors += resp.Diagnostics.ErrorsCount()
	}
	return errors
}

func TestPlatformEnvironmentResource_TypeAndRegionComeFromTheSDKVocabulary(t *testing.T) {
	s := resourceSchema(t)

	typeAttr := s.Attributes["type"].(rschema.StringAttribute)
	for _, v := range account.EnvironmentLabelValues() {
		if runStringValidators(t, typeAttr.Validators, v) != 0 {
			t.Errorf("type %q was refused, but the SDK lists it", v)
		}
		if !strings.Contains(typeAttr.MarkdownDescription, "`"+v+"`") {
			t.Errorf("type description does not document %q", v)
		}
	}
	if runStringValidators(t, typeAttr.Validators, "sandbox") == 0 {
		t.Error("a lower-case type must be refused: Jamf Account canonicalises it, which would read back changed")
	}

	regionAttr := s.Attributes["region"].(rschema.StringAttribute)
	for _, v := range account.MetaRegionValues() {
		if runStringValidators(t, regionAttr.Validators, v) != 0 {
			t.Errorf("region %q was refused, but the SDK lists it", v)
		}
		if !strings.Contains(regionAttr.MarkdownDescription, "`"+v+"`") {
			t.Errorf("region description does not document %q", v)
		}
	}
	if runStringValidators(t, regionAttr.Validators, "MARS") == 0 {
		t.Error("an unknown region must be refused at plan time: Jamf Account names no field when it refuses one")
	}
}

func TestPlatformEnvironmentResource_NameValidators(t *testing.T) {
	s := resourceSchema(t)
	name := s.Attributes["name"].(rschema.StringAttribute)

	if runStringValidators(t, name.Validators, "Production Elevate") != 0 {
		t.Error("an ordinary name was refused")
	}
	for _, bad := range []string{"", " padded", "padded ", "\tpadded", "   "} {
		if runStringValidators(t, name.Validators, bad) == 0 {
			t.Errorf("name %q was accepted", bad)
		}
	}
}

func TestPlatformEnvironmentResource_IdentitySchema(t *testing.T) {
	var resp resource.IdentitySchemaResponse
	NewPlatformEnvironmentResource().(*PlatformEnvironmentResource).IdentitySchema(context.Background(), resource.IdentitySchemaRequest{}, &resp)
	if _, ok := resp.IdentitySchema.Attributes["id"]; !ok || len(resp.IdentitySchema.Attributes) != 1 {
		t.Errorf("identity schema must be keyed on id alone, got %v", resp.IdentitySchema.Attributes)
	}
}

func TestPlatformEnvironmentResource_DescriptionsCarryNoWireVocabulary(t *testing.T) {
	s := resourceSchema(t)
	for name, attr := range s.Attributes {
		for _, banned := range []string{"label", "metaRegion", "endpoint", "SDK", "HTTP"} {
			if strings.Contains(attr.GetMarkdownDescription(), banned) {
				t.Errorf("%s description mentions %q", name, banned)
			}
		}
	}
}

func TestPlatformEnvironmentDataSource_Metadata(t *testing.T) {
	var resp datasource.MetadataResponse
	NewPlatformEnvironmentDataSource().(*PlatformEnvironmentDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != "jamfplatform_account_platform_environment" {
		t.Errorf("type name = %q", resp.TypeName)
	}
}

func TestPlatformEnvironmentDataSource_Schema(t *testing.T) {
	s := dataSourceSchema(t)

	for _, name := range []string{"id", "name"} {
		attr := s.Attributes[name]
		if !attr.IsOptional() || !attr.IsComputed() {
			t.Errorf("%s must be Optional and Computed so either can be the lookup key", name)
		}
	}
	for _, name := range []string{"type", "region"} {
		attr := s.Attributes[name]
		if !attr.IsComputed() || attr.IsOptional() || attr.IsRequired() {
			t.Errorf("%s must be computed only", name)
		}
	}
}

func TestPlatformEnvironmentDataSource_RequiresExactlyOneKey(t *testing.T) {
	validators := NewPlatformEnvironmentDataSource().(datasource.DataSourceWithConfigValidators).ConfigValidators(context.Background())
	if len(validators) != 1 {
		t.Fatalf("got %d config validators, want ExactlyOneOf(id, name)", len(validators))
	}
}

func TestPlatformEnvironmentsDataSource_Schema(t *testing.T) {
	var resp datasource.SchemaResponse
	d := NewPlatformEnvironmentsDataSource().(*PlatformEnvironmentsDataSource)
	d.Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	nested, ok := resp.Schema.Attributes["platform_environments"].(dsschema.ListNestedAttribute)
	if !ok {
		t.Fatalf("platform_environments must be a ListNestedAttribute, got %T", resp.Schema.Attributes["platform_environments"])
	}
	for _, name := range []string{"id", "name", "type", "region"} {
		attr, present := nested.NestedObject.Attributes[name]
		if !present || !attr.IsComputed() {
			t.Errorf("platform_environments.%s must be present and computed", name)
		}
	}
	if len(nested.NestedObject.Attributes) != 4 {
		t.Errorf("platform_environments declares %d attributes, want 4", len(nested.NestedObject.Attributes))
	}

	var meta datasource.MetadataResponse
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &meta)
	if meta.TypeName != "jamfplatform_account_platform_environments" {
		t.Errorf("type name = %q", meta.TypeName)
	}
}

func TestPlatformEnvironmentListResource_Schema(t *testing.T) {
	r := NewPlatformEnvironmentListResource().(*PlatformEnvironmentListResource)

	var meta resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &meta)
	if meta.TypeName != "jamfplatform_account_platform_environment" {
		t.Errorf("list type name = %q", meta.TypeName)
	}

	var resp list.ListResourceSchemaResponse
	r.ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["timeouts"]; !ok || len(resp.Schema.Attributes) != 1 {
		t.Errorf("list schema must take only timeouts, got %v", resp.Schema.Attributes)
	}
}
