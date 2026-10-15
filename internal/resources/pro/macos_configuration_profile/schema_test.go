// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package macos_configuration_profile

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/keyedcategories"
)

func TestResource_Schema(t *testing.T) {
	t.Parallel()
	r := NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema returned diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range []string{"id", "general", "scope", "self_service", "timeouts"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Fatalf("expected attribute %q", name)
		}
	}
}

func TestResource_GeneralAttributes(t *testing.T) {
	t.Parallel()
	r := NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	g, ok := resp.Schema.Attributes["general"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("expected general to be SingleNestedAttribute")
	}
	for _, child := range []string{"id", "name", "description", "level", "distribution_method", "user_removable", "redeploy_on_update", "uuid", "payloads", "category_id", "category_name", "site_id", "site_name"} {
		if _, ok := g.Attributes[child]; !ok {
			t.Fatalf("expected general.%s", child)
		}
	}
}

func TestResource_ScopeAttributes(t *testing.T) {
	t.Parallel()
	r := NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	s, ok := resp.Schema.Attributes["scope"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("expected scope to be SingleNestedAttribute")
	}
	for _, child := range []string{"targets", "limitations", "exclusions"} {
		if _, ok := s.Attributes[child]; !ok {
			t.Fatalf("expected scope.%s", child)
		}
	}
	targets, ok := s.Attributes["targets"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("expected scope.targets to be SingleNestedAttribute")
	}
	for _, child := range []string{"all_computers", "all_jss_users", "computer_ids", "computer_group_ids", "building_ids", "department_ids", "user_ids", "user_group_ids"} {
		if _, ok := targets.Attributes[child]; !ok {
			t.Fatalf("expected scope.targets.%s", child)
		}
	}
}

func TestResource_SelfServiceAttributes(t *testing.T) {
	t.Parallel()
	r := NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	ss, ok := resp.Schema.Attributes["self_service"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("expected self_service to be SingleNestedAttribute")
	}
	for _, child := range []string{"self_service_display_name", "install_button_text", "self_service_description", "ensure_users_view_description", "feature_on_main_page", "display_notifications", "notification_location", "notification_subject", "notification_message", "removal_disallowed", "categories"} {
		if _, ok := ss.Attributes[child]; !ok {
			t.Fatalf("expected self_service.%s", child)
		}
	}
}

func TestResource_PayloadsAttributeRequired(t *testing.T) {
	t.Parallel()
	r := NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	g := resp.Schema.Attributes["general"].(rschema.SingleNestedAttribute)
	p, ok := g.Attributes["payloads"].(rschema.StringAttribute)
	if !ok {
		t.Fatalf("expected general.payloads to be StringAttribute")
	}
	if !p.Required {
		t.Fatal("expected general.payloads to be Required")
	}
	// Payload diff suppression is implemented at the resource level via
	// ModifyPlan (so it can read and write three-way-compare references
	// in private state); the attribute itself carries no plan modifier.
	if _, ok := r.(resource.ResourceWithModifyPlan); !ok {
		t.Fatal("expected Resource to implement ResourceWithModifyPlan for payload diff suppression")
	}
}

// TestMacOSConfigurationProfileResource_SelfServiceCategoriesCarryByID pins the category element
// shape: Jamf Pro resolves a category by id, so name is Computed-only, and it
// and the flags carry prior state by id rather than by position. See
// keyedcategories.AssertCarriedByID.
func TestMacOSConfigurationProfileResource_SelfServiceCategoriesCarryByID(t *testing.T) {
	t.Parallel()
	var resp resource.SchemaResponse
	NewResource().(*Resource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	ss := resp.Schema.Attributes["self_service"].(rschema.SingleNestedAttribute)
	keyedcategories.AssertCarriedByID(t, ss.Attributes["categories"].(rschema.ListNestedAttribute).NestedObject.Attributes, "display_in", "feature_in")
}
