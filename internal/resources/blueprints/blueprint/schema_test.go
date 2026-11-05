// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// --- Resource ---

func TestBlueprintResource_Metadata(t *testing.T) {
	r := NewBlueprintResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*BlueprintResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_blueprints_blueprint" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_blueprints_blueprint", resp.TypeName)
	}
}

func TestBlueprintResource_Schema(t *testing.T) {
	r := NewBlueprintResource()
	req := resource.SchemaRequest{}
	var resp resource.SchemaResponse
	r.(*BlueprintResource).Schema(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	if s.Version != 5 {
		t.Errorf("expected schema version 5, got %d", s.Version)
	}

	requiredAttrs := []string{"name", "deployed", "device_groups"}
	for _, name := range requiredAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("missing required attribute %q", name)
			continue
		}
		if !attr.IsRequired() {
			t.Errorf("attribute %q should be required", name)
		}
	}

	computedAttrs := []string{"id", "created", "updated", "deployment_state"}
	for _, name := range computedAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("missing computed attribute %q", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("attribute %q should be computed", name)
		}
	}

	optionalAttrs := []string{"description", "timeouts", "component_blocks"}
	for _, name := range optionalAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("missing optional attribute %q", name)
			continue
		}
		if !attr.IsOptional() {
			t.Errorf("attribute %q should be optional", name)
		}
	}
}

func TestBlueprintResource_SchemaComponentAttributes(t *testing.T) {
	r := NewBlueprintResource()
	req := resource.SchemaRequest{}
	var resp resource.SchemaResponse
	r.(*BlueprintResource).Schema(context.Background(), req, &resp)

	blocks, ok := resp.Schema.Attributes["component_blocks"].(resourceschema.ListNestedAttribute)
	if !ok {
		t.Fatal("component_blocks is missing or not a ListNestedAttribute")
	}

	componentAttrs := []string{
		"audio_accessory_settings",
		"custom_declarations",
		"disk_management_settings",
		"math_settings",
		"passcode_policy",
		"safari_bookmarks",
		"safari_extensions",
		"safari_settings",
		"service_background_tasks",
		"service_configuration_files",
		"software_update",
		"software_update_settings",
	}
	for _, name := range componentAttrs {
		attr, ok := blocks.NestedObject.Attributes[name]
		if !ok {
			t.Errorf("component_blocks is missing component attribute %q", name)
			continue
		}
		if _, ok := attr.(resourceschema.SingleNestedAttribute); !ok {
			t.Errorf("component attribute %q should be SingleNestedAttribute", name)
		}
	}
}

// --- Data Source ---

func TestBlueprintDataSource_Metadata(t *testing.T) {
	d := NewBlueprintDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp datasource.MetadataResponse
	d.(*BlueprintDataSource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_blueprints_blueprint" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_blueprints_blueprint", resp.TypeName)
	}
}

func TestBlueprintDataSource_Schema(t *testing.T) {
	d := NewBlueprintDataSource()
	req := datasource.SchemaRequest{}
	var resp datasource.SchemaResponse
	d.(*BlueprintDataSource).Schema(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	optionalAttrs := []string{"id", "name"}
	for _, name := range optionalAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("missing optional attribute %q", name)
			continue
		}
		if !attr.IsOptional() {
			t.Errorf("attribute %q should be optional", name)
		}
	}

	computedAttrs := []string{"blueprint_id", "description", "created", "updated", "deployment_state", "device_groups"}
	for _, name := range computedAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("missing computed attribute %q", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("attribute %q should be computed", name)
		}
	}
}

// --- List Resource ---

func TestBlueprintListResource_Metadata(t *testing.T) {
	r := NewBlueprintListResource()
	req := resource.MetadataRequest{ProviderTypeName: "jamfplatform"}
	var resp resource.MetadataResponse
	r.(*BlueprintListResource).Metadata(context.Background(), req, &resp)

	if resp.TypeName != "jamfplatform_blueprints_blueprint" {
		t.Errorf("expected type name %q, got %q", "jamfplatform_blueprints_blueprint", resp.TypeName)
	}
}

// TestBlueprintResource_SchemaDeclarationValidators checks the schema wiring rather than the
// validator's own behaviour. An unexported constructor that nothing references is legal Go, so
// deleting a `Validators` line still compiles and still passes every behavioural test in this
// package, while silently dropping the plan-time schema check from every declaration a blueprint
// carries.
func TestBlueprintResource_SchemaDeclarationValidators(t *testing.T) {
	r := NewBlueprintResource()
	var resp resource.SchemaResponse
	r.(*BlueprintResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	block, ok := resp.Schema.Attributes["component_blocks"].(resourceschema.ListNestedAttribute)
	if !ok {
		t.Fatal("component_blocks is missing or not a ListNestedAttribute")
	}

	// The same validator is attached through a different interface on each: apple_declarations is a
	// list, custom_declarations an object wrapping a set. Each attribute's kind is asserted too,
	// because reshaping one would move where a finding lands without failing anything else.
	t.Run("component_blocks[].custom_declarations", func(t *testing.T) {
		attribute, ok := block.NestedObject.Attributes["custom_declarations"].(resourceschema.SingleNestedAttribute)
		if !ok {
			t.Fatal("component_blocks[].custom_declarations is missing or not a SingleNestedAttribute")
		}
		found := false
		for _, attached := range attribute.Validators {
			if _, ok := attached.(declarationSchemaValidator); ok {
				found = true
			}
		}
		if !found {
			t.Error("no declaration schema validator is attached to component_blocks[].custom_declarations")
		}
	})

	t.Run("component_blocks[].apple_declarations", func(t *testing.T) {
		attribute, ok := block.NestedObject.Attributes["apple_declarations"].(resourceschema.ListNestedAttribute)
		if !ok {
			t.Fatal("component_blocks[].apple_declarations is missing or not a ListNestedAttribute")
		}
		found := false
		for _, attached := range attribute.Validators {
			if _, ok := attached.(declarationSchemaValidator); ok {
				found = true
			}
		}
		if !found {
			t.Error("no declaration schema validator is attached to component_blocks[].apple_declarations")
		}
	})
}
