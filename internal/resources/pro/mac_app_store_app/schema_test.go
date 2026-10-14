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
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/nullprior"
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

// TestMacAppResource_OptionalBlockLeavesPlanUnknownFromNullPrior guards the
// failure where an optional block goes from omitted to declared. Each
// Optional+Computed leaf inside one holds null in state, and
// UseStateForUnknown would plan that null where apply then returns the server's
// default: "Provider produced inconsistent result after apply". The mac app
// already uses the non-null modifier; this keeps it that way.
func TestMacAppResource_OptionalBlockLeavesPlanUnknownFromNullPrior(t *testing.T) {
	t.Parallel()
	var resp resource.SchemaResponse
	NewMacAppResource().(*MacAppResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	nullprior.AssertOptionalBlockLeavesPlanUnknown(t, resp.Schema)
}

// TestMacAppResource_NothingCollapsesAPlan is what lets the mac app's
// singleton-block leaves keep UseNonNullStateForUnknown (STYLE_GUIDE rule 1a).
// self_service.install_button_text, for one, stays null once applied when Jamf
// Pro answers it empty, so it plans Unknown whenever the plan differs from
// state. That only shows as a permanent diff if something restores the
// configured change that caused the difference, and this fails the moment a
// plan modifier or a ModifyPlan plan write that could do so is added.
func TestMacAppResource_NothingCollapsesAPlan(t *testing.T) {
	t.Parallel()
	var resp resource.SchemaResponse
	NewMacAppResource().(*MacAppResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	nullprior.AssertNoPlanCollapse(t, resp.Schema, ".")
}

// TestMacAppResource_VPPLicenseCountsCarryNothingForward pins the three VPP
// license counts as plain Computed with no plan modifier. They derive from
// vpp_admin_account_id, which Jamf Pro answers with no counts while it is -1, so
// UseStateForUnknown failed the apply when vpp went from omitted to declared and
// again when the account moved either way. Wire-probed against a VPP-backed
// app; see computedInt64.
func TestMacAppResource_VPPLicenseCountsCarryNothingForward(t *testing.T) {
	var resp resource.SchemaResponse
	NewMacAppResource().(*MacAppResource).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	vpp := resp.Schema.Attributes["vpp"].(schema.SingleNestedAttribute)
	for _, name := range []string{"total_vpp_licenses", "remaining_vpp_licenses", "used_vpp_licenses"} {
		a, ok := vpp.Attributes[name].(schema.Int64Attribute)
		if !ok {
			t.Fatalf("vpp.%s is not an int64 attribute", name)
		}
		if !a.Computed || a.Optional || a.Required {
			t.Errorf("vpp.%s: Computed=%v Optional=%v Required=%v, want Computed only", name, a.Computed, a.Optional, a.Required)
		}
		if len(a.PlanModifiers) != 0 {
			t.Errorf("vpp.%s has %d plan modifiers, want none: the value follows vpp_admin_account_id", name, len(a.PlanModifiers))
		}
	}
}
