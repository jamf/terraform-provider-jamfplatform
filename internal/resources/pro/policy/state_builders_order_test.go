// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package policy

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

// repeatedEntriesPolicy is a wire policy that lists the same item more than
// once in each of the five collections issue #568 found Jamf Pro keeps
// repeats in: identical entries, and entries sharing an id that differ in
// parameters or settings. Each collection comes back in the order Jamf Pro
// stores it (scripts by id then priority, packages by name).
func repeatedEntriesPolicy() *proclassic.Policy {
	return &proclassic.Policy{
		ID:      new(70),
		General: &proclassic.PolicyGeneral{ID: new(70), Name: new("tf-568")},
		Scripts: &proclassic.PolicyScripts{Script: &[]proclassic.PolicyScriptsScriptItem{
			{ID: new(10), Name: new("Script A"), Priority: new("After"), Parameter4: new("first")},
			{ID: new(10), Name: new("Script A"), Priority: new("After"), Parameter4: new("first")},
			{ID: new(10), Name: new("Script A"), Priority: new("After"), Parameter4: new("second")},
			{ID: new(10), Name: new("Script A"), Priority: new("Before"), Parameter4: new("first")},
		}},
		PackageConfiguration: &proclassic.PolicyPackageConfiguration{Packages: &proclassic.PolicyPackageConfigurationPackages{
			Package: &[]proclassic.PolicyPackageConfigurationPackagesPackageItem{
				{ID: new(20), Name: new("Pkg"), Action: new("Install"), Fut: new(false), Feu: new(false), UpdateAutorun: new(false)},
				{ID: new(20), Name: new("Pkg"), Action: new("Install"), Fut: new(false), Feu: new(false), UpdateAutorun: new(false)},
				{ID: new(20), Name: new("Pkg"), Action: new("Cache"), Fut: new(true), Feu: new(false), UpdateAutorun: new(false)},
			},
		}},
		Printers: &proclassic.PolicyPrinters{Printer: &[]proclassic.PolicyPrintersPrinterItem{
			{ID: new(30), Name: new("Printer"), Action: new("install"), MakeDefault: new(false)},
			{ID: new(30), Name: new("Printer"), Action: new("install"), MakeDefault: new(false)},
			{ID: new(30), Name: new("Printer"), Action: new("uninstall"), MakeDefault: new(false)},
		}},
		DockItems: &proclassic.PolicyDockItems{DockItem: &[]proclassic.PolicyDockItemsDockItemItem{
			{ID: new(40), Name: new("Dock"), Action: new("Add To End")},
			{ID: new(40), Name: new("Dock"), Action: new("Add To End")},
			{ID: new(40), Name: new("Dock"), Action: new("Remove")},
		}},
		AccountMaintenance: &proclassic.PolicyAccountMaintenance{DirectoryBindings: &proclassic.PolicyAccountMaintenanceDirectoryBindings{
			Binding: &[]proclassic.IDName{{ID: new(50), Name: new("Binding")}, {ID: new(50), Name: new("Binding")}},
		}},
	}
}

// setPolicyState writes a model into a tfsdk.State against the real schema,
// the step that failed with "Duplicate Set Element" before issue #568.
func setPolicyState(t *testing.T, m *PolicyResourceModel) {
	t.Helper()
	s := policySchemaForTest(t)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(context.Background()), nil)}
	if diags := state.Set(context.Background(), m); diags.HasError() {
		t.Fatalf("setting state: %v", diags)
	}
}

// TestAssignPolicyResourceModel_KeepsEveryRepeatedEntry is the unit-level
// reproduction of issue #568 for the read and import paths: every repeat in
// all five collections survives, and the model sets cleanly.
func TestAssignPolicyResourceModel_KeepsEveryRepeatedEntry(t *testing.T) {
	t.Parallel()
	state := &PolicyResourceModel{
		General:             &PolicyGeneralModel{},
		ScriptAssignments:   []PolicyScriptItemModel{},
		PrinterAssignments:  []PolicyPrinterItemModel{},
		DockItemAssignments: []PolicyDockItemModel{},
		Packages:            &PolicyPackagesModel{Packages: []PolicyPackageItemModel{}},
		DirectoryBindings:   []PolicyDirectoryBindingItemModel{},
		Timeouts:            helpers.NewResourceTimeoutsNullValue(policyTimeoutAttributeTypes),
	}
	if diags := assignPolicyResourceModel(context.Background(), state, repeatedEntriesPolicy(), false); diags.HasError() {
		t.Fatalf("assign: %v", diags)
	}
	assertEveryRepeatKept(t, state.ScriptAssignments, state.Packages.Packages, state.PrinterAssignments, state.DockItemAssignments, state.DirectoryBindings)
	setPolicyState(t, state)
}

// TestAssignPolicyResourceModel_DeprecatedWrappersKeepEveryRepeatedEntry is
// the same for a configuration still on the deprecated wrapper blocks, which
// share the flatteners.
func TestAssignPolicyResourceModel_DeprecatedWrappersKeepEveryRepeatedEntry(t *testing.T) {
	t.Parallel()
	state := &PolicyResourceModel{
		General:           &PolicyGeneralModel{},
		Scripts:           &PolicyScriptsModel{Scripts: []PolicyScriptItemModel{}},
		Printers:          &PolicyPrintersModel{Printers: []PolicyPrinterItemModel{}},
		DockItems:         &PolicyDockItemsModel{DockItems: []PolicyDockItemModel{}},
		Packages:          &PolicyPackagesModel{Packages: []PolicyPackageItemModel{}},
		DirectoryBindings: []PolicyDirectoryBindingItemModel{},
		Timeouts:          helpers.NewResourceTimeoutsNullValue(policyTimeoutAttributeTypes),
	}
	if diags := assignPolicyResourceModel(context.Background(), state, repeatedEntriesPolicy(), false); diags.HasError() {
		t.Fatalf("assign: %v", diags)
	}
	assertEveryRepeatKept(t, state.Scripts.Scripts, state.Packages.Packages, state.Printers.Printers, state.DockItems.DockItems, state.DirectoryBindings)
	setPolicyState(t, state)
}

func assertEveryRepeatKept(t *testing.T, scripts []PolicyScriptItemModel, pkgs []PolicyPackageItemModel, printers []PolicyPrinterItemModel, dock []PolicyDockItemModel, bindings []PolicyDirectoryBindingItemModel) {
	t.Helper()
	var got []string
	for _, s := range scripts {
		got = append(got, s.Priority.ValueString()+"/"+s.Parameter4.ValueString())
	}
	if want := "After/first After/first After/second Before/first"; strings.Join(got, " ") != want {
		t.Errorf("scripts = %v, want %s", got, want)
	}
	got = nil
	for _, p := range pkgs {
		got = append(got, p.Action.ValueString())
	}
	if want := "Install Install Cache"; strings.Join(got, " ") != want {
		t.Errorf("packages = %v, want %s", got, want)
	}
	got = nil
	for _, p := range printers {
		got = append(got, p.Action.ValueString())
	}
	if want := "Map Map Unmap"; strings.Join(got, " ") != want {
		t.Errorf("printers = %v, want %s", got, want)
	}
	got = nil
	for _, d := range dock {
		got = append(got, d.Action.ValueString())
	}
	if want := "Add To End|Add To End|Remove"; strings.Join(got, "|") != want {
		t.Errorf("dock items = %v, want %s", got, want)
	}
	if len(bindings) != 2 || bindings[0].ID.ValueString() != "50" || bindings[1].ID.ValueString() != "50" {
		t.Errorf("directory bindings = %v, want binding 50 twice", bindings)
	}
}

// TestFlattenPolicyScripts_KeepsPlannedOrderAcrossServerSort pins the
// alignment against the sort wire-probed on 2026-11-11: written C A C A A,
// stored A A A C C. The read must come back in the written order, with each
// entry's server name filled and its parameters paired with the entry it was
// written as.
func TestFlattenPolicyScripts_KeepsPlannedOrderAcrossServerSort(t *testing.T) {
	t.Parallel()
	planned := func(id, prio, p4 string) PolicyScriptItemModel {
		return PolicyScriptItemModel{
			ID: types.StringValue(id), Name: types.StringUnknown(), Priority: types.StringValue(prio), Parameter4: types.StringValue(p4),
			Parameter5: types.StringValue(""), Parameter6: types.StringUnknown(),
		}
	}
	plan := []PolicyScriptItemModel{
		planned("3", "After", "c1"), planned("1", "After", "a1"), planned("3", "After", "c2"),
		planned("1", "Before", "b1"), planned("1", "After", "a2"),
	}
	wire := func(id int, name, prio, p4 string) proclassic.PolicyScriptsScriptItem {
		return proclassic.PolicyScriptsScriptItem{ID: new(id), Name: new(name), Priority: new(prio), Parameter4: new(p4), Parameter5: new(""), Parameter6: new("")}
	}
	sc := &proclassic.PolicyScripts{Script: &[]proclassic.PolicyScriptsScriptItem{
		wire(1, "One", "After", "a1"), wire(1, "One", "After", "a2"), wire(1, "One", "Before", "b1"),
		wire(3, "Three", "After", "c1"), wire(3, "Three", "After", "c2"),
	}}

	got := flattenPolicyScripts(sc, plan)

	var order []string
	for _, s := range got {
		order = append(order, s.ID.ValueString()+"/"+s.Parameter4.ValueString()+"@"+s.Priority.ValueString())
		if s.Name.IsUnknown() || s.Name.IsNull() {
			t.Errorf("script %s: name not filled from the server", s.ID.ValueString())
		}
		if s.Parameter5.IsNull() || s.Parameter5.ValueString() != "" {
			t.Errorf("script %s/%s: authored \"\" parameter_5 lost: %s", s.ID.ValueString(), s.Parameter4.ValueString(), s.Parameter5)
		}
		if !s.Parameter6.IsNull() {
			t.Errorf("script %s/%s: unset parameter_6 should read null, got %s", s.ID.ValueString(), s.Parameter4.ValueString(), s.Parameter6)
		}
	}
	if want := "3/c1@After 1/a1@After 3/c2@After 1/b1@Before 1/a2@After"; strings.Join(order, " ") != want {
		t.Errorf("order = %s, want %s", strings.Join(order, " "), want)
	}
	if got[0].Name.ValueString() != "Three" || got[1].Name.ValueString() != "One" {
		t.Errorf("names misaligned: %s, %s", got[0].Name, got[1].Name)
	}
}

// TestFlattenPolicyScripts_UnknownIDsPairByParameters covers a create whose
// script ids are interpolated from scripts created in the same apply, so the
// plan knows only the parameters. Jamf Pro sorts the new ids, and each must
// still land at the position whose parameters it carries.
func TestFlattenPolicyScripts_UnknownIDsPairByParameters(t *testing.T) {
	t.Parallel()
	plan := []PolicyScriptItemModel{
		{ID: types.StringUnknown(), Name: types.StringUnknown(), Priority: types.StringValue("After"), Parameter4: types.StringValue("for-200")},
		{ID: types.StringUnknown(), Name: types.StringUnknown(), Priority: types.StringValue("After"), Parameter4: types.StringValue("for-100")},
	}
	sc := &proclassic.PolicyScripts{Script: &[]proclassic.PolicyScriptsScriptItem{
		{ID: new(100), Priority: new("After"), Parameter4: new("for-100")},
		{ID: new(200), Priority: new("After"), Parameter4: new("for-200")},
	}}
	got := flattenPolicyScripts(sc, plan)
	if len(got) != 2 || got[0].ID.ValueString() != "200" || got[1].ID.ValueString() != "100" {
		t.Fatalf("got %v, want 200 then 100", got)
	}
}

// TestFlattenPolicyPackageConfiguration_KeepsPlannedOrderAcrossNameSort pins
// the package alignment: Jamf Pro returns packages sorted by name, and the
// written order must survive the read.
func TestFlattenPolicyPackageConfiguration_KeepsPlannedOrderAcrossNameSort(t *testing.T) {
	t.Parallel()
	state := &PolicyPackagesModel{Packages: []PolicyPackageItemModel{
		{ID: types.StringValue("2"), Name: types.StringUnknown(), Action: types.StringValue("Install")},
		{ID: types.StringValue("1"), Name: types.StringUnknown(), Action: types.StringValue("Cache")},
		{ID: types.StringValue("3"), Name: types.StringUnknown(), Action: types.StringValue("Install")},
		{ID: types.StringValue("1"), Name: types.StringUnknown(), Action: types.StringValue("Install")},
	}}
	pc := &proclassic.PolicyPackageConfiguration{Packages: &proclassic.PolicyPackageConfigurationPackages{
		Package: &[]proclassic.PolicyPackageConfigurationPackagesPackageItem{
			{ID: new(3), Name: new("Aaa"), Action: new("Install")},
			{ID: new(1), Name: new("Mmm"), Action: new("Cache")},
			{ID: new(1), Name: new("Mmm"), Action: new("Install")},
			{ID: new(2), Name: new("Zzz"), Action: new("Install")},
		},
	}}
	flattenPolicyPackageConfiguration(pc, state)
	var got []string
	for _, p := range state.Packages {
		got = append(got, p.ID.ValueString()+"/"+p.Name.ValueString()+"/"+p.Action.ValueString())
	}
	if want := "2/Zzz/Install 1/Mmm/Cache 3/Aaa/Install 1/Mmm/Install"; strings.Join(got, " ") != want {
		t.Errorf("packages = %s, want %s", strings.Join(got, " "), want)
	}
}

// TestAssignPolicyResourceModel_OrderedCollectionsReportServerOrder pins the
// other half of the design: Jamf Pro keeps dock items, printers and directory
// bindings in the order written, so their order means something, and a
// reorder made outside Terraform must surface as drift rather than be aligned
// away.
func TestAssignPolicyResourceModel_OrderedCollectionsReportServerOrder(t *testing.T) {
	t.Parallel()
	state := &PolicyResourceModel{
		General: &PolicyGeneralModel{},
		DockItemAssignments: []PolicyDockItemModel{
			{ID: types.StringValue("1"), Action: types.StringValue("Add To End")},
			{ID: types.StringValue("2"), Action: types.StringValue("Add To End")},
		},
		PrinterAssignments: []PolicyPrinterItemModel{
			{ID: types.StringValue("1"), Action: types.StringValue("Map")},
			{ID: types.StringValue("2"), Action: types.StringValue("Map")},
		},
		DirectoryBindings: []PolicyDirectoryBindingItemModel{{ID: types.StringValue("1")}, {ID: types.StringValue("2")}},
	}
	p := &proclassic.Policy{
		DockItems: &proclassic.PolicyDockItems{DockItem: &[]proclassic.PolicyDockItemsDockItemItem{
			{ID: new(2), Action: new("Add To End")}, {ID: new(1), Action: new("Add To End")},
		}},
		Printers: &proclassic.PolicyPrinters{Printer: &[]proclassic.PolicyPrintersPrinterItem{
			{ID: new(2), Action: new("install")}, {ID: new(1), Action: new("install")},
		}},
		AccountMaintenance: &proclassic.PolicyAccountMaintenance{DirectoryBindings: &proclassic.PolicyAccountMaintenanceDirectoryBindings{
			Binding: &[]proclassic.IDName{{ID: new(2)}, {ID: new(1)}},
		}},
	}
	if diags := assignPolicyResourceModel(context.Background(), state, p, false); diags.HasError() {
		t.Fatalf("assign: %v", diags)
	}
	if state.DockItemAssignments[0].ID.ValueString() != "2" || state.PrinterAssignments[0].ID.ValueString() != "2" || state.DirectoryBindings[0].ID.ValueString() != "2" {
		t.Errorf("expected server order 2,1 for dock items, printers and bindings, got %s, %s, %s",
			state.DockItemAssignments[0].ID, state.PrinterAssignments[0].ID, state.DirectoryBindings[0].ID)
	}
}
