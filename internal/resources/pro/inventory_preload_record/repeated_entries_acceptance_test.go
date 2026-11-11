// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package inventory_preload_record_test

import (
	"context"
	"fmt"
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

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// eaEntry is one extension_attributes element; a nil value is an entry with no value.
type eaEntry struct {
	name  string
	value *string
}

func eaVal(name, value string) eaEntry { return eaEntry{name: name, value: &value} }

// eaRender renders entries as name=value, `name` alone for no value, in order.
func eaRender(entries []eaEntry) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.value == nil {
			parts = append(parts, e.name)
			continue
		}
		parts = append(parts, e.name+"="+*e.value)
	}
	return strings.Join(parts, " ")
}

// preloadConfigWithEAs renders a record whose extension_attributes are entries, in
// order.
func preloadConfigWithEAs(serial string, entries []eaEntry) string {
	var b strings.Builder
	for _, e := range entries {
		if e.value == nil {
			fmt.Fprintf(&b, "    { name = %q },\n", e.name)
			continue
		}
		fmt.Fprintf(&b, "    { name = %q, value = %q },\n", e.name, *e.value)
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_inventory_preload_record" "test" {
  serial_number = %q
  device_type   = "Computer"
  extension_attributes = [
%s  ]
}
`, serial, b.String())
}

// eaListCheck asserts state holds exactly entries, in order.
func eaListCheck(addr string, entries []eaEntry) statecheck.StateCheck {
	checks := make([]knownvalue.Check, 0, len(entries))
	for _, e := range entries {
		var value knownvalue.Check = knownvalue.Null()
		if e.value != nil {
			value = knownvalue.StringExact(*e.value)
		}
		checks = append(checks, knownvalue.ObjectExact(map[string]knownvalue.Check{
			"name":  knownvalue.StringExact(e.name),
			"value": value,
		}))
	}
	return statecheck.ExpectKnownValue(addr, tfjsonpath.New("extension_attributes"), knownvalue.ListExact(checks))
}

// storedEAs asserts Jamf Pro stores exactly entries, in order, read back with the
// SDK rather than through the provider.
func storedEAs(t *testing.T, addr string, entries []eaEntry) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		got, err := pro.New(testhelpers.NewAcceptanceClient(t)).GetInventoryPreloadRecordV2(context.Background(), s.RootModule().Resources[addr].Primary.ID)
		if err != nil {
			return err
		}
		var stored []eaEntry
		if got.ExtensionAttributes != nil {
			for _, ea := range *got.ExtensionAttributes {
				stored = append(stored, eaEntry{name: ea.Name, value: ea.Value})
			}
		}
		if eaRender(stored) != eaRender(entries) {
			return fmt.Errorf("Jamf Pro stores %q, want %q", eaRender(stored), eaRender(entries))
		}
		return nil
	}
}

// TestAccResource_ProInventoryPreloadRecord_RepeatedExtensionAttributes covers issue
// #568: identical entries and one name with different values round-trip in the order
// written with an empty plan, before and after refresh; a reorder plans an update,
// Jamf Pro stores the new order, and the plan is empty afterwards.
func TestAccResource_ProInventoryPreloadRecord_RepeatedExtensionAttributes(t *testing.T) {
	testhelpers.AccPreCheck(t)
	serial := "ZTFACC568R" + testhelpers.RunSuffix()
	const addr = "jamfplatform_pro_inventory_preload_record.test"

	first := []eaEntry{
		eaVal("ZTF Acc EA Z", "z"),
		eaVal("ZTF Acc EA A", "same"),
		eaVal("ZTF Acc EA A", "same"),
		eaVal("ZTF Acc EA A", "different"),
		{name: "ZTF Acc EA N"},
		{name: "ZTF Acc EA N"},
		eaVal("ZTF Acc EA E", ""),
	}
	second := []eaEntry{first[6], first[3], first[4], first[0], first[1], first[5], first[2]}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInventoryPreloadRecordDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:            preloadConfigWithEAs(serial, first),
				ConfigStateChecks: []statecheck.StateCheck{eaListCheck(addr, first)},
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh:  []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: storedEAs(t, addr, first),
			},
			{
				Config: preloadConfigWithEAs(serial, second),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
					PostApplyPreRefresh:  []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				ConfigStateChecks: []statecheck.StateCheck{eaListCheck(addr, second)},
				Check:             storedEAs(t, addr, second),
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
				},
			},
		},
	})
}

// createRepeatsRecordOutOfBand writes, with the SDK, a record a set-typed schema
// could not represent, and deletes it at cleanup.
func createRepeatsRecordOutOfBand(t *testing.T, serial string, entries []eaEntry) string {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	eas := make([]pro.InventoryPreloadExtensionAttribute, 0, len(entries))
	for _, e := range entries {
		eas = append(eas, pro.InventoryPreloadExtensionAttribute{Name: e.name, Value: e.value})
	}
	created, err := c.CreateInventoryPreloadRecordV2(context.Background(), &pro.InventoryPreloadRecordV2{
		SerialNumber: serial, DeviceType: "Computer", ExtensionAttributes: &eas,
	})
	if err != nil {
		t.Fatalf("creating inventory preload record %q outside Terraform: %v", serial, err)
	}
	t.Cleanup(func() { _ = c.DeleteInventoryPreloadRecordV2(context.Background(), created.ID) })
	return created.ID
}

// TestAccResource_ProInventoryPreloadRecord_RepeatedExtensionAttributesImportAndQuery
// covers the other half of issue #568 against a record created with repeats outside
// Terraform: an import block with generated configuration plans a no-op import with
// every repeat in Jamf Pro's order, and a config-generating `terraform query` returns
// it alongside a clean sibling. Both failed with `Duplicate Set Element` on v0.
func TestAccResource_ProInventoryPreloadRecord_RepeatedExtensionAttributesImportAndQuery(t *testing.T) {
	testhelpers.AccPreCheck(t)
	prefix := "ZTFACC568Q" + testhelpers.RunSuffix()
	repeatsSerial := prefix + "R"
	cleanSerial := prefix + "C"
	entries := []eaEntry{
		eaVal("ZTF Acc EA One", "v"),
		eaVal("ZTF Acc EA One", "v"),
		{name: "ZTF Acc EA One"},
		eaVal("ZTF Acc EA One", "other"),
	}
	repeatsID := createRepeatsRecordOutOfBand(t, repeatsSerial, entries)
	createRepeatsRecordOutOfBand(t, cleanSerial, nil)

	anchor := `resource "terraform_data" "anchor" {}`
	generated := "jamfplatform_pro_inventory_preload_record.generated"
	listChecks := make([]knownvalue.Check, 0, len(entries))
	for _, e := range entries {
		var value knownvalue.Check = knownvalue.Null()
		if e.value != nil {
			value = knownvalue.StringExact(*e.value)
		}
		listChecks = append(listChecks, knownvalue.ObjectExact(map[string]knownvalue.Check{"name": knownvalue.StringExact(e.name), "value": value}))
	}

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: anchor},
			{
				Config:          anchor,
				ResourceName:    "jamfplatform_pro_inventory_preload_record.repeats",
				ImportState:     true,
				ImportStateKind: resource.ImportBlockWithID,
				ImportStateId:   repeatsID,
				GenerateConfig:  true,
				ImportPlanChecks: resource.ImportPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectKnownValue(generated, tfjsonpath.New("extension_attributes"), knownvalue.ListExact(listChecks)),
					},
				},
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
provider "jamfplatform" {}

list "jamfplatform_pro_inventory_preload_record" "test" {
  provider         = jamfplatform
  include_resource = true

  config {
    filter = [
      {
        selector = "serialNumber"
        argument = %q
      },
      {
        selector  = "serialNumber"
        argument  = %q
        join_with = "or"
      }
    ]
  }
}
`, repeatsSerial, cleanSerial),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_inventory_preload_record.test", 2),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_inventory_preload_record.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(repeatsSerial)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("extension_attributes"), KnownValue: knownvalue.ListExact(listChecks)},
						},
					),
					querycheck.ExpectResourceDisplayName("jamfplatform_pro_inventory_preload_record.test", queryfilter.ByDisplayName(knownvalue.StringExact(cleanSerial)), knownvalue.StringExact(cleanSerial)),
				},
			},
		},
	})
}
