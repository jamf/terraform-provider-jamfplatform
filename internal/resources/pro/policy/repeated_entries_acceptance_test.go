// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package policy_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// repeatFixtures are the objects a policy assigns, created outside Terraform
// so each test controls their ids and names: scripts are created in id order
// a < b < c, and packages are created Mmm, Zzz, Aaa, so their id order is not
// their name order and Jamf Pro's name sort is visible.
type repeatFixtures struct {
	scriptA, scriptB, scriptC string
	pkgAaa, pkgMmm, pkgZzz    string
	printer0, printer1        string
	dock0, dock1              string
	binding0, binding1        string
}

// createRepeatFixtures creates the fixtures and registers their deletion.
// t.Cleanup runs after resource.Test has destroyed the policies that reference
// them.
func createRepeatFixtures(t *testing.T, tag string) repeatFixtures {
	t.Helper()
	ctx := context.Background()
	acc := testhelpers.NewAcceptanceClient(t)
	pc := pro.New(acc)
	cc := proclassic.New(acc)
	var f repeatFixtures

	for _, s := range []struct {
		id   *string
		name string
	}{{&f.scriptA, "a"}, {&f.scriptB, "b"}, {&f.scriptC, "c"}} {
		created, err := pc.CreateScriptV1(ctx, &pro.Script{
			Name:           fmt.Sprintf("tf-acc-568-%s-script-%s", tag, s.name),
			ScriptContents: new("#!/bin/sh\nexit 0\n"),
			Priority:       new(pro.ScriptPriorityAfter),
		})
		if err != nil {
			t.Fatalf("creating script fixture: %v", err)
		}
		*s.id = created.ID
		id := created.ID
		t.Cleanup(func() { _ = pc.DeleteScriptV1(context.Background(), id) })
	}
	for _, p := range []struct {
		id   *string
		name string
	}{{&f.pkgMmm, "Mmm"}, {&f.pkgZzz, "Zzz"}, {&f.pkgAaa, "Aaa"}} {
		name := fmt.Sprintf("%s-tf-acc-568-%s", p.name, tag)
		created, err := pc.CreatePackageV1(ctx, &pro.Package{
			PackageName: name, FileName: name + ".pkg", CategoryID: "-1", Priority: 10,
		})
		if err != nil {
			t.Fatalf("creating package fixture: %v", err)
		}
		*p.id = created.ID
		id := created.ID
		t.Cleanup(func() { _ = pc.DeletePackageV1(context.Background(), id) })
	}
	for i, id := range []*string{&f.printer0, &f.printer1} {
		name := fmt.Sprintf("tf-acc-568-%s-printer-%d", tag, i)
		created, err := cc.CreatePrinterByID(ctx, "0", &proclassic.Printer{
			Name: &name, CUPSName: new(strings.ReplaceAll(name, "-", "_")), URI: new(fmt.Sprintf("lpd://10.0.0.%d/", i+1)), Model: new("Generic"),
		})
		if err != nil || created.ID == nil {
			t.Fatalf("creating printer fixture: %v", err)
		}
		*id = strconv.Itoa(*created.ID)
		pid := *id
		t.Cleanup(func() { _ = cc.DeletePrinterByID(context.Background(), pid) })
	}
	for i, id := range []*string{&f.dock0, &f.dock1} {
		name := fmt.Sprintf("tf-acc-568-%s-dock-%d", tag, i)
		created, err := cc.CreateDockItemByID(ctx, "0", &proclassic.DockItem{
			Name: &name, Type: new(proclassic.DockItemTypeApp), Path: new(fmt.Sprintf("file:///Applications/App%d.app/", i)),
		})
		if err != nil || created.ID == nil {
			t.Fatalf("creating dock item fixture: %v", err)
		}
		*id = strconv.Itoa(*created.ID)
		did := *id
		t.Cleanup(func() { _ = cc.DeleteDockItemByID(context.Background(), did) })
	}
	for i, id := range []*string{&f.binding0, &f.binding1} {
		name := fmt.Sprintf("tf-acc-568-%s-binding-%d", tag, i)
		created, err := cc.CreateDirectoryBindingByID(ctx, "0", &proclassic.DirectoryBinding{
			Name: &name, Priority: new(1), Domain: new(fmt.Sprintf("example%d.com", i)), Username: new("u"), Password: new("p"),
			ComputerOu: new("CN=Computers,DC=example,DC=com"), Type: new("Active Directory"),
		})
		if err != nil || created.ID == nil {
			t.Fatalf("creating directory binding fixture: %v", err)
		}
		*id = strconv.Itoa(*created.ID)
		bid := *id
		t.Cleanup(func() { _ = cc.DeleteDirectoryBindingByID(context.Background(), bid) })
	}
	return f
}

// policyCollectionIDs reads a policy through the Classic API and renders each
// collection as the space-separated ids Jamf Pro stores, in stored order.
func policyCollectionIDs(t *testing.T, id string) map[string]string {
	t.Helper()
	p, err := proclassic.New(testhelpers.NewAcceptanceClient(t)).GetPolicyByID(context.Background(), id)
	if err != nil {
		t.Fatalf("reading policy %s: %v", id, err)
	}
	out := map[string]string{}
	join := func(ids []string) string { return strings.Join(ids, " ") }
	var ids []string
	if p.Printers != nil && p.Printers.Printer != nil {
		for _, x := range *p.Printers.Printer {
			ids = append(ids, helpers.StringValueFromIntPtr(x.ID).ValueString())
		}
	}
	out["printers"] = join(ids)
	ids = nil
	if p.DockItems != nil && p.DockItems.DockItem != nil {
		for _, x := range *p.DockItems.DockItem {
			ids = append(ids, helpers.StringValueFromIntPtr(x.ID).ValueString())
		}
	}
	out["dock_items"] = join(ids)
	ids = nil
	if p.AccountMaintenance != nil && p.AccountMaintenance.DirectoryBindings != nil && p.AccountMaintenance.DirectoryBindings.Binding != nil {
		for _, x := range *p.AccountMaintenance.DirectoryBindings.Binding {
			ids = append(ids, helpers.StringValueFromIntPtr(x.ID).ValueString())
		}
	}
	out["directory_bindings"] = join(ids)
	ids = nil
	if p.Scripts != nil && p.Scripts.Script != nil {
		for _, x := range *p.Scripts.Script {
			ids = append(ids, helpers.StringValueFromIntPtr(x.ID).ValueString()+"/"+helpers.DerefString(x.Parameter4)+"@"+helpers.DerefString(x.Priority))
		}
	}
	out["scripts"] = join(ids)
	ids = nil
	if p.PackageConfiguration != nil && p.PackageConfiguration.Packages != nil && p.PackageConfiguration.Packages.Package != nil {
		for _, x := range *p.PackageConfiguration.Packages.Package {
			ids = append(ids, helpers.StringValueFromIntPtr(x.ID).ValueString()+"/"+helpers.DerefString(x.Action))
		}
	}
	out["packages"] = join(ids)
	return out
}

type scriptEntry struct{ id, priority, p4, p5 string }

type packageEntry struct{ id, action string }

func policyConfigOrdered(name string, scripts []scriptEntry, pkgs []packageEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "resource \"jamfplatform_pro_policy\" \"test\" {\n  general = {\n    name = %q\n  }\n  script_assignments = [\n", name)
	for _, s := range scripts {
		fmt.Fprintf(&b, "    { id = %q, priority = %q, parameter_4 = %q, parameter_5 = %q },\n", s.id, s.priority, s.p4, s.p5)
	}
	b.WriteString("  ]\n  packages = {\n    packages = [\n")
	for _, p := range pkgs {
		fmt.Fprintf(&b, "      { id = %q, action = %q },\n", p.id, p.action)
	}
	b.WriteString("    ]\n  }\n}\n")
	return b.String()
}

// scriptOrderChecks asserts the state holds the scripts in the written order.
func scriptOrderChecks(scripts []scriptEntry) []statecheck.StateCheck {
	elems := make([]knownvalue.Check, 0, len(scripts))
	for _, s := range scripts {
		elems = append(elems, knownvalue.ObjectPartial(map[string]knownvalue.Check{
			"id":          knownvalue.StringExact(s.id),
			"priority":    knownvalue.StringExact(s.priority),
			"parameter_4": knownvalue.StringExact(s.p4),
			"parameter_5": knownvalue.StringExact(s.p5),
			"name":        knownvalue.NotNull(),
		}))
	}
	return []statecheck.StateCheck{
		statecheck.ExpectKnownValue("jamfplatform_pro_policy.test", tfjsonpath.New("script_assignments"), knownvalue.ListExact(elems)),
	}
}

func packageOrderCheck(pkgs []packageEntry) statecheck.StateCheck {
	elems := make([]knownvalue.Check, 0, len(pkgs))
	for _, p := range pkgs {
		elems = append(elems, knownvalue.ObjectPartial(map[string]knownvalue.Check{
			"id":     knownvalue.StringExact(p.id),
			"action": knownvalue.StringExact(p.action),
			"name":   knownvalue.NotNull(),
		}))
	}
	return statecheck.ExpectKnownValue("jamfplatform_pro_policy.test", tfjsonpath.New("packages").AtMapKey("packages"), knownvalue.ListExact(elems))
}

// TestAccPolicyResource_ScriptsAndPackagesKeepConfiguredOrder covers issue
// #568 items 2 and 5. Jamf Pro re-sorts a policy's scripts (by id, then
// priority) and packages (by name). A configuration written in another order,
// with the same script listed several times under different parameters and
// one package listed twice, must create and plan clean; a reorder across the
// server's sort groups must apply as one update and then plan clean, keeping
// the new order in state while Jamf Pro's stored order does not move.
func TestAccPolicyResource_ScriptsAndPackagesKeepConfiguredOrder(t *testing.T) {
	testhelpers.AccPreCheck(t)
	tag := "order-" + testhelpers.RunSuffix()
	f := createRepeatFixtures(t, tag)
	name := "tf-acc-policy-568-" + tag

	first := []scriptEntry{
		{f.scriptC, "After", "c-first", "x"},
		{f.scriptA, "Before", "a-before", ""},
		{f.scriptA, "After", "a-first", "y"},
		{f.scriptC, "After", "c-second", "x"},
		{f.scriptB, "After", "b", ""},
		{f.scriptA, "After", "a-second", "y"},
	}
	firstPkgs := []packageEntry{{f.pkgZzz, "Install"}, {f.pkgAaa, "Cache"}, {f.pkgMmm, "Install"}, {f.pkgMmm, "Cache"}}
	second := []scriptEntry{first[4], first[2], first[0], first[5], first[1], first[3]}
	secondPkgs := []packageEntry{firstPkgs[2], firstPkgs[0], firstPkgs[3], firstPkgs[1]}

	stored := func(scripts, pkgs string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			got := policyCollectionIDs(t, s.RootModule().Resources["jamfplatform_pro_policy.test"].Primary.ID)
			if got["scripts"] != scripts {
				return fmt.Errorf("Jamf Pro stores scripts %q, want %q", got["scripts"], scripts)
			}
			if got["packages"] != pkgs {
				return fmt.Errorf("Jamf Pro stores packages %q, want %q", got["packages"], pkgs)
			}
			return nil
		}
	}
	wantScripts := fmt.Sprintf("%[1]s/a-first@After %[1]s/a-second@After %[1]s/a-before@Before %[2]s/b@After %[3]s/c-first@After %[3]s/c-second@After", f.scriptA, f.scriptB, f.scriptC)
	wantPkgs := fmt.Sprintf("%s/Cache %s/Install %s/Cache %s/Install", f.pkgAaa, f.pkgMmm, f.pkgMmm, f.pkgZzz)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:            policyConfigOrdered(name, first, firstPkgs),
				ConfigStateChecks: append(scriptOrderChecks(first), packageOrderCheck(firstPkgs)),
				Check:             stored(wantScripts, wantPkgs),
			},
			{
				Config: policyConfigOrdered(name, second, secondPkgs),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("jamfplatform_pro_policy.test", plancheck.ResourceActionUpdate),
					},
					PostApplyPreRefresh:  []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: append(scriptOrderChecks(second), packageOrderCheck(secondPkgs)),
				Check:             stored(wantScripts, wantPkgs),
			},
		},
	})
}

func policyConfigOrderedCollections(name string, printers, dock, bindings []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "resource \"jamfplatform_pro_policy\" \"test\" {\n  general = {\n    name = %q\n  }\n", name)
	b.WriteString("  printer_assignments = [\n")
	for _, id := range printers {
		fmt.Fprintf(&b, "    { id = %q, action = \"Map\", make_default = false },\n", id)
	}
	b.WriteString("  ]\n  dock_item_assignments = [\n")
	for _, id := range dock {
		fmt.Fprintf(&b, "    { id = %q, action = \"Add To End\" },\n", id)
	}
	b.WriteString("  ]\n  directory_bindings = [\n")
	for _, id := range bindings {
		fmt.Fprintf(&b, "    { id = %q },\n", id)
	}
	b.WriteString("  ]\n}\n")
	return b.String()
}

func idListCheck(attr string, ids []string) statecheck.StateCheck {
	elems := make([]knownvalue.Check, 0, len(ids))
	for _, id := range ids {
		elems = append(elems, knownvalue.ObjectPartial(map[string]knownvalue.Check{"id": knownvalue.StringExact(id)}))
	}
	return statecheck.ExpectKnownValue("jamfplatform_pro_policy.test", tfjsonpath.New(attr), knownvalue.ListExact(elems))
}

// TestAccPolicyResource_OrderedCollectionsReorderIsAnUpdate covers the other
// half of issue #568 item 2: Jamf Pro keeps printers, dock items and directory
// bindings in the order written, so a reorder is a real change. It must plan an
// update, apply, and leave both state and Jamf Pro in the new order, repeats
// included.
func TestAccPolicyResource_OrderedCollectionsReorderIsAnUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	tag := "ordered-" + testhelpers.RunSuffix()
	f := createRepeatFixtures(t, tag)
	name := "tf-acc-policy-568-" + tag

	firstP, firstD, firstB := []string{f.printer0, f.printer1}, []string{f.dock0, f.dock1}, []string{f.binding0, f.binding1}
	secondP, secondD, secondB := []string{f.printer1, f.printer0, f.printer1}, []string{f.dock1, f.dock0, f.dock1}, []string{f.binding1, f.binding0, f.binding1}

	stored := func(p, d, b []string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			got := policyCollectionIDs(t, s.RootModule().Resources["jamfplatform_pro_policy.test"].Primary.ID)
			for attr, want := range map[string][]string{"printers": p, "dock_items": d, "directory_bindings": b} {
				if got[attr] != strings.Join(want, " ") {
					return fmt.Errorf("Jamf Pro stores %s %q, want %q", attr, got[attr], strings.Join(want, " "))
				}
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: policyConfigOrderedCollections(name, firstP, firstD, firstB),
				ConfigStateChecks: []statecheck.StateCheck{
					idListCheck("printer_assignments", firstP), idListCheck("dock_item_assignments", firstD), idListCheck("directory_bindings", firstB),
				},
				Check: stored(firstP, firstD, firstB),
			},
			{
				Config: policyConfigOrderedCollections(name, secondP, secondD, secondB),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("jamfplatform_pro_policy.test", plancheck.ResourceActionUpdate),
					},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					idListCheck("printer_assignments", secondP), idListCheck("dock_item_assignments", secondD), idListCheck("directory_bindings", secondB),
				},
				Check: stored(secondP, secondD, secondB),
			},
		},
	})
}

// createRepeatsPolicyOutOfBand writes, through the Classic API, a policy no
// set-typed schema could represent: every one of the five collections lists an
// entry twice, identically, and the same script also appears with different
// parameters. Entries are written in the order Jamf Pro stores them, since an
// import has no configuration to align to.
func createRepeatsPolicyOutOfBand(t *testing.T, name string, f repeatFixtures) string {
	t.Helper()
	c := testhelpers.NewProClassicClient(t)
	id := func(s string) *int { n, _ := strconv.Atoi(s); return &n }
	script := func(sid, p4, p11 string) proclassic.PolicyScriptsScriptItem {
		return proclassic.PolicyScriptsScriptItem{ID: id(sid), Priority: new(proclassic.PolicyScriptsScriptItemPriorityAfter), Parameter4: new(p4), Parameter11: new(p11)}
	}
	pkg := func(pid string) proclassic.PolicyPackageConfigurationPackagesPackageItem {
		return proclassic.PolicyPackageConfigurationPackagesPackageItem{ID: id(pid), Action: new("Install"), Fut: new(false), Feu: new(false), UpdateAutorun: new(false)}
	}
	created, err := c.CreatePolicyByID(context.Background(), "0", &proclassic.PolicyPost{
		General: &proclassic.PolicyPostGeneral{Name: &name, Enabled: new(false)},
		Scripts: &proclassic.PolicyPostScripts{Script: &[]proclassic.PolicyScriptsScriptItem{
			script(f.scriptA, "same", "same"), script(f.scriptA, "same", "same"), script(f.scriptA, "other-4", "other-11"),
		}},
		PackageConfiguration: &proclassic.PolicyPostPackageConfiguration{Packages: &proclassic.PolicyPackageConfigurationPackages{
			Package: &[]proclassic.PolicyPackageConfigurationPackagesPackageItem{pkg(f.pkgAaa), pkg(f.pkgAaa), pkg(f.pkgMmm)},
		}},
		Printers: &proclassic.PolicyPostPrinters{Printer: &[]proclassic.PolicyPrintersPrinterItem{
			{ID: id(f.printer0), Action: new(proclassic.PolicyPrintersPrinterItemActionInstall), MakeDefault: new(false)},
			{ID: id(f.printer0), Action: new(proclassic.PolicyPrintersPrinterItemActionInstall), MakeDefault: new(false)},
		}},
		DockItems: &proclassic.PolicyPostDockItems{DockItem: &[]proclassic.PolicyPostDockItemsDockItemItem{
			{ID: id(f.dock0), Action: new(proclassic.PolicyPostDockItemsDockItemItemActionAddToEnd)},
			{ID: id(f.dock0), Action: new(proclassic.PolicyPostDockItemsDockItemItemActionAddToEnd)},
		}},
		AccountMaintenance: &proclassic.PolicyPostAccountMaintenance{DirectoryBindings: &proclassic.PolicyAccountMaintenanceDirectoryBindings{
			Binding: &[]proclassic.IDName{{ID: id(f.binding0)}, {ID: id(f.binding0)}},
		}},
	})
	if err != nil || created.ID == nil {
		t.Fatalf("creating policy %q outside Terraform: %v", name, err)
	}
	pid := strconv.Itoa(*created.ID)
	t.Cleanup(func() { _ = c.DeletePolicyByID(context.Background(), pid) })
	return pid
}

// TestAccPolicyResource_RepeatedEntriesImportAndQuery covers issue #568 items
// 4, 5 and 6 against a policy created with repeated entries outside
// Terraform. Importing it must populate every repeat and generate a
// configuration that plans as a no-op import, and a config-generating `terraform
// query` over it and a clean sibling policy must return both.
func TestAccPolicyResource_RepeatedEntriesImportAndQuery(t *testing.T) {
	testhelpers.AccPreCheck(t)
	tag := "import-" + testhelpers.RunSuffix()
	f := createRepeatFixtures(t, tag)
	prefix := "tf-acc-policy-568-" + tag
	repeatsName := prefix + "-repeats"
	cleanName := prefix + "-clean"
	repeatsID := createRepeatsPolicyOutOfBand(t, repeatsName, f)
	testhelpers.CreateOutOfBandClassicPolicy(t, cleanName)

	anchor := `resource "terraform_data" "anchor" {}`
	generated := "jamfplatform_pro_policy.generated"

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: anchor},
			{
				Config:          anchor,
				ResourceName:    "jamfplatform_pro_policy.repeats",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				ImportStateId:   repeatsID,
				GenerateConfig:  true,
				ImportPlanChecks: resource.ImportPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectKnownValue(generated, tfjsonpath.New("script_assignments"), knownvalue.ListExact([]knownvalue.Check{
							knownvalue.ObjectPartial(map[string]knownvalue.Check{"id": knownvalue.StringExact(f.scriptA), "parameter_4": knownvalue.StringExact("same"), "parameter_11": knownvalue.StringExact("same")}),
							knownvalue.ObjectPartial(map[string]knownvalue.Check{"id": knownvalue.StringExact(f.scriptA), "parameter_4": knownvalue.StringExact("same"), "parameter_11": knownvalue.StringExact("same")}),
							knownvalue.ObjectPartial(map[string]knownvalue.Check{"id": knownvalue.StringExact(f.scriptA), "parameter_4": knownvalue.StringExact("other-4"), "parameter_11": knownvalue.StringExact("other-11")}),
						})),
						plancheck.ExpectKnownValue(generated, tfjsonpath.New("packages").AtMapKey("packages"), knownvalue.ListSizeExact(3)),
						plancheck.ExpectKnownValue(generated, tfjsonpath.New("printer_assignments"), knownvalue.ListSizeExact(2)),
						plancheck.ExpectKnownValue(generated, tfjsonpath.New("dock_item_assignments"), knownvalue.ListSizeExact(2)),
						plancheck.ExpectKnownValue(generated, tfjsonpath.New("directory_bindings"), knownvalue.ListSizeExact(2)),
					},
				},
			},
			{
				Query: true,
				Config: `
provider "jamfplatform" {}

list "jamfplatform_pro_policy" "test" {
  provider         = jamfplatform
  include_resource = true

  config {
    filter = {
      name_substring = "` + prefix + `"
    }
  }
}
`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_policy.test", 2),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_policy.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(repeatsName)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("script_assignments"), KnownValue: knownvalue.ListSizeExact(3)},
							{Path: tfjsonpath.New("packages").AtMapKey("packages"), KnownValue: knownvalue.ListSizeExact(3)},
							{Path: tfjsonpath.New("printer_assignments"), KnownValue: knownvalue.ListSizeExact(2)},
							{Path: tfjsonpath.New("dock_item_assignments"), KnownValue: knownvalue.ListSizeExact(2)},
							{Path: tfjsonpath.New("directory_bindings"), KnownValue: knownvalue.ListSizeExact(2)},
						},
					),
					querycheck.ExpectResourceDisplayName("jamfplatform_pro_policy.test", queryfilter.ByDisplayName(knownvalue.StringExact(cleanName)), knownvalue.StringExact(cleanName)),
				},
			},
		},
	})
}
