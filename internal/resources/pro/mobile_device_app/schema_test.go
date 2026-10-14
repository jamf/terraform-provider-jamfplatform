// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package mobile_device_app

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/iconmirror"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/nullprior"
)

const wantTypeName = "jamfplatform_pro_mobile_device_app"

func TestMobileAppResource_Metadata(t *testing.T) {
	r := NewMobileAppResource()
	var resp resource.MetadataResponse
	r.(*MobileAppResource).Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != wantTypeName {
		t.Errorf("expected type name %q, got %q", wantTypeName, resp.TypeName)
	}
}

func TestMobileAppResource_Schema(t *testing.T) {
	r := NewMobileAppResource()
	var resp resource.SchemaResponse
	r.(*MobileAppResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}

	s := resp.Schema
	for _, name := range []string{"id", "general", "scope", "self_service", "vpp", "app_configuration", "timeouts"} {
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
	for _, name := range []string{"name", "version", "bundle_id"} {
		a, ok := general.Attributes[name]
		if !ok {
			t.Errorf("general missing %q", name)
			continue
		}
		if !a.IsRequired() {
			t.Errorf("general.%s must be required", name)
		}
	}
	// os_type is Optional+Computed: only required for in-house apps, which the
	// server enforces with a 409 — the schema does not force it.
	if a, ok := general.Attributes["os_type"]; !ok {
		t.Error("general missing os_type")
	} else if a.IsRequired() || !a.IsOptional() || !a.IsComputed() {
		t.Error("general.os_type must be Optional+Computed")
	}
	for _, name := range []string{"description", "category_name", "site_name"} {
		a, ok := general.Attributes[name]
		if !ok {
			t.Errorf("general missing %q", name)
			continue
		}
		if a.IsRequired() || a.IsOptional() || !a.IsComputed() {
			t.Errorf("general.%s must be computed-only", name)
		}
	}
	// Mobile general carries no inline icon block (read-only echo not modeled).
	if _, ok := general.Attributes["icon"]; ok {
		t.Errorf("general.icon must not be modeled on mobile apps")
	}

	// Scope is mobile-device-only — no computer attrs.
	scopeAttr, ok := s.Attributes["scope"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("scope must be a SingleNestedAttribute")
	}
	targetsAttr, ok := scopeAttr.Attributes["targets"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("scope.targets must be a SingleNestedAttribute")
	}
	for _, name := range []string{"all_mobile_devices", "all_jss_users", "mobile_device_ids", "mobile_device_group_ids"} {
		if _, ok := targetsAttr.Attributes[name]; !ok {
			t.Errorf("scope.targets missing %q", name)
		}
	}
	for _, absent := range []string{"all_computers", "computer_ids", "computer_group_ids"} {
		if _, ok := targetsAttr.Attributes[absent]; ok {
			t.Errorf("scope.targets.%s must not exist on mobile apps", absent)
		}
	}
	// No iBeacon scope on the mobiledeviceapplications endpoint.
	limitations, ok := scopeAttr.Attributes["limitations"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("scope.limitations must be a SingleNestedAttribute")
	}
	if _, ok := limitations.Attributes["ibeacon_ids"]; ok {
		t.Errorf("scope.limitations.ibeacon_ids must not exist on mobile apps")
	}
}

func TestMobileAppDataSource_Metadata(t *testing.T) {
	d := NewMobileAppDataSource()
	var resp datasource.MetadataResponse
	d.(*MobileAppDataSource).Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "jamfplatform"}, &resp)
	if resp.TypeName != wantTypeName {
		t.Errorf("expected type name %q, got %q", wantTypeName, resp.TypeName)
	}
}

func TestMobileAppDataSource_ConfigValidators(t *testing.T) {
	d := NewMobileAppDataSource().(*MobileAppDataSource)
	if got := d.ConfigValidators(context.Background()); len(got) != 1 {
		t.Fatalf("expected 1 config validator, got %d", len(got))
	}
}

func TestMobileAppListResource_Schema(t *testing.T) {
	r := NewMobileAppListResource()
	var resp list.ListResourceSchemaResponse
	r.(*MobileAppListResource).ListResourceConfigSchema(context.Background(), list.ListResourceSchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if _, ok := resp.Schema.Attributes["filter"]; !ok {
		t.Errorf("list schema missing filter attribute")
	}
}

// TestMobileAppResource_SelfServiceIconURIDerivedFromID pins uri as a
// Computed-only attribute that follows id. Jamf Pro derives it from the icon the
// id names and ignores an authored value; UseStateForUnknown carried the
// previous icon's uri into a plan that repointed id. See issue #468.
func TestMobileAppResource_SelfServiceIconURIDerivedFromID(t *testing.T) {
	var resp resource.SchemaResponse
	NewMobileAppResource().(*MobileAppResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	icon := resp.Schema.Attributes["self_service"].(schema.SingleNestedAttribute).Attributes["self_service_icon"].(schema.SingleNestedAttribute)
	iconmirror.AssertPlan(t, "uri", icon.Attributes["uri"])
	if id, ok := icon.Attributes["id"].(schema.StringAttribute); !ok || !id.Optional || !id.Computed {
		t.Errorf("self_service_icon.id must stay Optional+Computed: it is the one input")
	}
}

// TestMobileAppResource_OptionalBlockLeavesPlanUnknownFromNullPrior guards the failure
// where an optional block goes from omitted to declared. Each Optional+Computed
// leaf inside one holds null in state, and UseStateForUnknown planned that null
// where apply then returned the server's default: "Provider produced
// inconsistent result after apply". Reproduced live on self_service.notification_enabled and feature_on_main_page.
func TestMobileAppResource_OptionalBlockLeavesPlanUnknownFromNullPrior(t *testing.T) {
	t.Parallel()
	var resp resource.SchemaResponse
	NewMobileAppResource().(*MobileAppResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	nullprior.AssertOptionalBlockLeavesPlanUnknown(t, resp.Schema)
}
