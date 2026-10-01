// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"
)

func TestPolicyResource_Schema(t *testing.T) {
	t.Parallel()
	r := NewPolicyResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema returned diagnostics: %v", resp.Diagnostics)
	}
	must := []string{
		"id", "general", "scope", "self_service", "packages",
		"scripts", "printers", "dock_items",
		"local_accounts", "management_account", "directory_bindings", "efi_password",
		"restart_options",
		"maintenance", "files_and_processes", "user_interaction", "disk_encryption",
		"timeouts",
	}
	for _, name := range must {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Fatalf("expected attribute %q in policy schema", name)
		}
	}
}

func TestPolicyResource_ScopeChildAttributes(t *testing.T) {
	t.Parallel()
	r := NewPolicyResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)

	scopeAttr, ok := resp.Schema.Attributes["scope"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("expected scope to be SingleNestedAttribute")
	}
	for _, child := range []string{"targets", "limitations", "exclusions"} {
		if _, ok := scopeAttr.Attributes[child]; !ok {
			t.Fatalf("expected scope.%s", child)
		}
	}
	targetsAttr, ok := scopeAttr.Attributes["targets"].(rschema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("expected scope.targets to be SingleNestedAttribute")
	}
	for _, child := range []string{"all_computers", "all_jss_users", "computer_ids", "computer_group_ids", "building_ids", "department_ids", "user_ids", "user_group_ids"} {
		if _, ok := targetsAttr.Attributes[child]; !ok {
			t.Fatalf("expected scope.targets.%s", child)
		}
	}
}

func TestPolicyDataSource_Schema(t *testing.T) {
	t.Parallel()
	d := NewPolicyDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema returned diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range []string{"id", "name", "enabled", "frequency", "trigger", "category_id", "category_name", "site_id", "site_name"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Fatalf("expected data source attribute %q", name)
		}
	}
	// Sanity-check that id and name are both Optional+Computed (selector pair).
	idAttr, ok := resp.Schema.Attributes["id"].(dsschema.StringAttribute)
	if !ok {
		t.Fatalf("expected id to be a StringAttribute")
	}
	if !idAttr.Optional || !idAttr.Computed {
		t.Fatalf("expected id to be Optional+Computed")
	}
}

// policyResourceAttribute descends the resource schema by attribute name,
// through single-nested objects and the element object of nested sets.
func policyResourceAttribute(t *testing.T, names ...string) rschema.Attribute {
	t.Helper()
	resp := &resource.SchemaResponse{}
	NewPolicyResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	var a rschema.Attribute
	for i, name := range names {
		var ok bool
		if a, ok = attrs[name]; !ok {
			t.Fatalf("no attribute %q at %v", name, names[:i+1])
		}
		switch n := a.(type) {
		case rschema.SingleNestedAttribute:
			attrs = n.Attributes
		case rschema.SetNestedAttribute:
			attrs = n.NestedObject.Attributes
		}
	}
	return a
}

// TestPolicyResource_EnumAttributesRejectUnknownValues pins plan-time
// validation on every policy field Jamf Pro answers a 201 for whatever is
// sent. Each one silently keeps, coerces or stores verbatim an unknown value
// (see optComputedEnum), so the validator is the only place a typo surfaces.
func TestPolicyResource_EnumAttributesRejectUnknownValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path  []string
		valid []string
	}{
		{[]string{"general", "frequency"}, proclassic.PolicyPostGeneralFrequencyValues()},
		{[]string{"packages", "packages", "action"}, packageActions},
		{[]string{"scripts", "scripts", "priority"}, proclassic.PolicyScriptsScriptItemPriorityValues()},
		{[]string{"dock_items", "dock_items", "action"}, proclassic.PolicyPostDockItemsDockItemItemActionValues()},
		{[]string{"management_account", "action"}, proclassic.PolicyAccountMaintenanceManagementAccountActionValues()},
		{[]string{"disk_encryption", "action"}, diskEncryptionActions},
		{[]string{"disk_encryption", "remediate_key_type"}, proclassic.PolicyPostDiskEncryptionRemediateKeyTypeValues()},
		{[]string{"restart_options", "no_user_logged_in"}, noUserLoggedInRestartActions},
		{[]string{"restart_options", "user_logged_in"}, userLoggedInRestartActions},
	}
	validate := func(validators []validator.String, v string) bool {
		resp := &validator.StringResponse{}
		for _, val := range validators {
			val.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("x"), ConfigValue: types.StringValue(v)}, resp)
		}
		return !resp.Diagnostics.HasError()
	}
	for _, tc := range cases {
		a, ok := policyResourceAttribute(t, tc.path...).(rschema.StringAttribute)
		if !ok {
			t.Fatalf("%v is not a string attribute", tc.path)
		}
		for _, v := range tc.valid {
			if !validate(a.Validators, v) {
				t.Errorf("%v rejected valid value %q", tc.path, v)
			}
		}
		if validate(a.Validators, "not-a-value") {
			t.Errorf("%v accepted an unknown value", tc.path)
		}
	}
}

// TestPolicyResource_DerivedValuesAreAccepted pins the two values the classic
// spec omits but a live tenant reads back, so a regeneration that drops the
// local additions cannot make an imported policy unconfigurable.
func TestPolicyResource_DerivedValuesAreAccepted(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		vocabulary []string
		want       string
	}{
		{packageActions, "Uninstall"},
		{diskEncryptionActions, "none"},
	} {
		found := false
		for _, v := range tc.vocabulary {
			found = found || v == tc.want
		}
		if !found {
			t.Errorf("vocabulary %v is missing %q", tc.vocabulary, tc.want)
		}
	}
}

// TestPolicyResource_TriggerIsComputedOnly pins general.trigger as read-only.
// A written <trigger> resets every trigger_* boolean on the server, so the
// attribute must not be settable (see buildPolicyGeneral).
func TestPolicyResource_TriggerIsComputedOnly(t *testing.T) {
	t.Parallel()
	a, ok := policyResourceAttribute(t, "general", "trigger").(rschema.StringAttribute)
	if !ok {
		t.Fatal("general.trigger is not a string attribute")
	}
	if !a.Computed || a.Optional || a.Required {
		t.Errorf("general.trigger: Computed=%v Optional=%v Required=%v, want Computed only", a.Computed, a.Optional, a.Required)
	}
}
