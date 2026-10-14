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

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/planmodifiers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers/iconmirror"
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
		case rschema.ListNestedAttribute:
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
		{[]string{"restart_options", "startup_disk"}, restartStartupDisks},
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

// TestPolicyResource_AssignmentNamesAreComputedOnly pins `name` on every
// id-keyed assignment as Computed only with no plan modifier. Jamf Pro resolves
// these entries by name ahead of id, so a name authored or carried over from
// state for a re-keyed element defeats the change. See serverDerivedName and
// issue #464.
func TestPolicyResource_AssignmentNamesAreComputedOnly(t *testing.T) {
	t.Parallel()
	for _, path := range [][]string{
		{"packages", "packages", "name"},
		{"scripts", "scripts", "name"},
		{"printers", "printers", "name"},
		{"dock_items", "dock_items", "name"},
		{"directory_bindings", "name"},
		{"self_service", "categories", "name"},
	} {
		a, ok := policyResourceAttribute(t, path...).(rschema.StringAttribute)
		if !ok {
			t.Errorf("%v is not a string attribute", path)
			continue
		}
		if !a.Computed || a.Optional || a.Required {
			t.Errorf("%v: Computed=%v Optional=%v Required=%v, want Computed only", path, a.Computed, a.Optional, a.Required)
		}
		if len(a.PlanModifiers) != 0 {
			t.Errorf("%v: has %d plan modifiers, want none: a carried-over name names the previous object", path, len(a.PlanModifiers))
		}
	}
}

// TestPolicyResource_AssignmentActionsAreRequired pins the action / priority of
// every id-keyed assignment as Required. Jamf Pro refuses an entry that carries
// only an id (409 "Problem with <kind>") and has no default, so an Optional
// attribute only moved the failure from plan to apply. See requiredEnum and
// issue #464.
func TestPolicyResource_AssignmentActionsAreRequired(t *testing.T) {
	t.Parallel()
	for _, path := range [][]string{
		{"packages", "packages", "action"},
		{"scripts", "scripts", "priority"},
		{"printers", "printers", "action"},
		{"dock_items", "dock_items", "action"},
		{"local_accounts", "action"},
		{"local_accounts", "username"},
	} {
		a, ok := policyResourceAttribute(t, path...).(rschema.StringAttribute)
		if !ok {
			t.Errorf("%v is not a string attribute", path)
			continue
		}
		if !a.Required || a.Optional || a.Computed {
			t.Errorf("%v: Required=%v Optional=%v Computed=%v, want Required only", path, a.Required, a.Optional, a.Computed)
		}
		if len(a.Validators) == 0 {
			t.Errorf("%v: no enum validator", path)
		}
	}
}

// TestPolicyResource_KeyedAssignmentAttributesCarryByID pins that the
// Optional+Computed attributes of id-keyed set elements carry prior state by id
// rather than by position (the framework pairs set elements by index). See
// planmodifiers.UseKeyedStateForUnknownString and issue #464.
func TestPolicyResource_KeyedAssignmentAttributesCarryByID(t *testing.T) {
	t.Parallel()
	check := func(path ...string) {
		t.Helper()
		switch a := policyResourceAttribute(t, path...).(type) {
		case rschema.StringAttribute:
			if len(a.PlanModifiers) != 1 || a.PlanModifiers[0].Description(context.Background()) != planmodifiers.UseKeyedStateForUnknownString("id").Description(context.Background()) {
				t.Errorf("%v: want exactly the keyed string modifier, got %v", path, a.PlanModifiers)
			}
		case rschema.BoolAttribute:
			if len(a.PlanModifiers) != 1 || a.PlanModifiers[0].Description(context.Background()) != planmodifiers.UseKeyedStateForUnknownBool("id").Description(context.Background()) {
				t.Errorf("%v: want exactly the keyed bool modifier, got %v", path, a.PlanModifiers)
			}
		default:
			t.Errorf("%v: unexpected attribute type %T", path, a)
		}
	}
	for _, f := range []string{"fut", "feu", "update_autorun"} {
		check("packages", "packages", f)
	}
	for _, f := range []string{"parameter_4", "parameter_11"} {
		check("scripts", "scripts", f)
	}
	check("printers", "printers", "make_default")
	check("self_service", "categories", "display_in")
	check("self_service", "categories", "feature_in")
	for _, f := range []string{"realname", "home", "hint", "picture", "archive_home_directory_to"} {
		check("local_accounts", f)
	}
	for _, f := range []string{"admin", "filevault_enabled", "secure_token_allowed", "permanently_delete_home_directory"} {
		check("local_accounts", f)
	}
}

// TestPolicyResource_IntegerRangesMatchTheWire pins the bounds Jamf Pro enforces
// on two integers: restart_options.delay_minutes (an int32; 2147483648 is a 409)
// and user_interaction.deferral_days (1 to 1000; 0 and 1001 are a 409). See
// maxRestartDelayMinutes and maxDeferralDays.
func TestPolicyResource_IntegerRangesMatchTheWire(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		path      []string
		ok, notOK []int64
	}{
		{[]string{"restart_options", "delay_minutes"}, []int64{0, 5, 2147483647}, []int64{-1, 2147483648}},
		{[]string{"user_interaction", "deferral_days"}, []int64{1, 1000}, []int64{0, 1001, -1}},
	} {
		a, ok := policyResourceAttribute(t, tc.path...).(rschema.Int64Attribute)
		if !ok {
			t.Fatalf("%v is not an int64 attribute", tc.path)
		}
		check := func(v int64) bool {
			resp := &validator.Int64Response{}
			for _, val := range a.Validators {
				val.ValidateInt64(context.Background(), validator.Int64Request{Path: path.Root("x"), ConfigValue: types.Int64Value(v)}, resp)
			}
			return !resp.Diagnostics.HasError()
		}
		for _, v := range tc.ok {
			if !check(v) {
				t.Errorf("%v rejected %d", tc.path, v)
			}
		}
		for _, v := range tc.notOK {
			if check(v) {
				t.Errorf("%v accepted %d", tc.path, v)
			}
		}
	}
}

// TestPolicyResource_SelfServiceIconDerivedAttributes pins uri and filename of
// self_service.self_service_icon as Computed-only attributes that follow id.
// Jamf Pro derives both from the icon the id names and ignores an authored
// value, so an Optional one could never match what apply returned, and
// UseNonNullStateForUnknown carried the previous icon's pair into a plan that
// repointed id. See issue #468.
func TestPolicyResource_SelfServiceIconDerivedAttributes(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"uri", "filename"} {
		iconmirror.AssertPlan(t, name, policyResourceAttribute(t, "self_service", "self_service_icon", name))
	}
	id, ok := policyResourceAttribute(t, "self_service", "self_service_icon", "id").(rschema.StringAttribute)
	if !ok || !id.Optional || !id.Computed {
		t.Errorf("self_service_icon.id must stay Optional+Computed: it is the one input")
	}
}
