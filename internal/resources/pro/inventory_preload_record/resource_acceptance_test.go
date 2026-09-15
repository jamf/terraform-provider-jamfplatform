// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package inventory_preload_record_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckInventoryPreloadRecordDestroy verifies records created during the test
// were destroyed, per-id. DeleteAllInventoryPreloadRecordsV2 is a tenant-wide wipe and
// must never be used for cleanup.
func testAccCheckInventoryPreloadRecordDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_inventory_preload_record" {
				continue
			}
			_, err := c.GetInventoryPreloadRecordV2(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro inventory preload record %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro inventory preload record %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_ProInventoryPreloadRecord_CRUD walks the full lifecycle: minimal
// create (serial + device_type only), a full update populating many optional fields
// (including the free-text dates), an in-place mutation of BOTH serial_number and
// device_type (wire-proven mutable — no RequiresReplace), and an import round-trip.
func TestAccResource_ProInventoryPreloadRecord_CRUD(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	serial := "ZTFACCCRUD" + suffix
	serialMutated := "ZTFACCCRUDM" + suffix
	const addr = "jamfplatform_pro_inventory_preload_record.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInventoryPreloadRecordDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "test" {
						serial_number = %q
						device_type   = "Computer"
					}
				`, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckResourceAttr(addr, "serial_number", serial),
					resource.TestCheckResourceAttr(addr, "device_type", "Computer"),
					resource.TestCheckResourceAttr(addr, "extension_attributes.#", "0"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "test" {
						serial_number       = %q
						device_type         = "Computer"
						username            = "preload.user"
						full_name           = "Preload User"
						email_address       = "preload.user@example.com"
						phone_number        = "555-0100"
						position            = "Technician"
						department          = "IT"
						building            = "HQ"
						room                = "101"
						po_number           = "PO-1234"
						po_date             = "2026-01-15"
						warranty_expiration = "2029-01-15"
						lease_expiration    = "2028-01-15"
						apple_care_id       = "AC-0001"
						life_expectancy     = "4"
						purchase_price      = "1999.00"
						purchasing_contact  = "Purchasing Contact"
						purchasing_account  = "ACCT-1"
						bar_code_1          = "BC-1"
						bar_code_2          = "BC-2"
						asset_tag           = "ASSET-1"
						vendor              = "Vendor Inc"
					}
				`, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "username", "preload.user"),
					resource.TestCheckResourceAttr(addr, "full_name", "Preload User"),
					resource.TestCheckResourceAttr(addr, "email_address", "preload.user@example.com"),
					resource.TestCheckResourceAttr(addr, "department", "IT"),
					resource.TestCheckResourceAttr(addr, "building", "HQ"),
					resource.TestCheckResourceAttr(addr, "po_date", "2026-01-15"),
					resource.TestCheckResourceAttr(addr, "warranty_expiration", "2029-01-15"),
					resource.TestCheckResourceAttr(addr, "lease_expiration", "2028-01-15"),
					resource.TestCheckResourceAttr(addr, "purchase_price", "1999.00"),
					resource.TestCheckResourceAttr(addr, "asset_tag", "ASSET-1"),
					resource.TestCheckResourceAttr(addr, "vendor", "Vendor Inc"),
				),
			},
			{
				// serial_number AND device_type mutate in place (wire-probed: PUT
				// changing either returns 200 with the same record id).
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "test" {
						serial_number = %q
						device_type   = "Mobile Device"
						username      = "preload.user"
					}
				`, serialMutated),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "serial_number", serialMutated),
					resource.TestCheckResourceAttr(addr, "device_type", "Mobile Device"),
					// Omitted Optional+Computed fields are PRESERVED at their prior
					// values (omit = preserve via UseStateForUnknown), not cleared.
					resource.TestCheckResourceAttr(addr, "asset_tag", "ASSET-1"),
					resource.TestCheckResourceAttr(addr, "department", "IT"),
				),
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

// TestAccResource_ProInventoryPreloadRecord_SplitOwnership proves the §768.2
// omit=preserve contract for an Optional+Computed field (`asset_tag`) on the
// full-replace records endpoint: when `asset_tag` is omitted from HCL, an out-of-band
// edit (simulating the Jamf Pro UI / CSV path) survives an unrelated Terraform change
// (a username update) rather than being wiped — and an explicit "" still clears it.
func TestAccResource_ProInventoryPreloadRecord_SplitOwnership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	serial := "ZTFACCSPLIT" + suffix
	const addr = "jamfplatform_pro_inventory_preload_record.test"
	const tfAssetTag = "TF-ASSET"  // initial TF-declared value
	const oobAssetTag = "UI-ASSET" // later set out-of-band

	var recordID string

	// setAssetTagOutOfBand simulates a UI/CSV edit: GET the record, set asset_tag,
	// PUT it back (a full-object write, like the admin console does).
	setAssetTagOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		got, err := c.GetInventoryPreloadRecordV2(ctx, recordID)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}
		v := oobAssetTag
		got.AssetTag = &v
		if _, err := c.UpdateInventoryPreloadRecordV2(ctx, recordID, got); err != nil {
			t.Fatalf("out-of-band PUT: %v", err)
		}
	}

	checkServerAssetTag := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := pro.New(testhelpers.NewAcceptanceClient(t))
			got, err := c.GetInventoryPreloadRecordV2(context.Background(), recordID)
			if err != nil {
				return fmt.Errorf("verify GET: %w", err)
			}
			if helpers.DerefString(got.AssetTag) != want {
				return fmt.Errorf("asset_tag = %q, want %q", helpers.DerefString(got.AssetTag), want)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInventoryPreloadRecordDestroy(t),
		Steps: []resource.TestStep{
			{
				// Create with a TF-declared asset_tag, so the next step proves the
				// out-of-band value is preserved AND not reverted to this prior
				// TF-owned value.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "test" {
						serial_number = %q
						device_type   = "Computer"
						username      = "preload.user"
						asset_tag     = %q
					}
				`, serial, tfAssetTag),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckResourceAttr(addr, "asset_tag", tfAssetTag),
					func(s *terraform.State) error {
						recordID = s.RootModule().Resources[addr].Primary.ID
						return nil
					},
				),
			},
			{
				// Admin overwrites asset_tag out-of-band to a DIFFERENT value; config
				// now REMOVES asset_tag and changes only the username. The out-of-band
				// value must survive — neither wiped by the full-replace PUT nor
				// reverted to the prior TF-owned value.
				PreConfig: setAssetTagOutOfBand,
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "test" {
						serial_number = %q
						device_type   = "Computer"
						username      = "preload.user.updated"
					}
				`, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "username", "preload.user.updated"),
					resource.TestCheckResourceAttr(addr, "asset_tag", oobAssetTag),
					checkServerAssetTag(oobAssetTag),
				),
			},
			{
				// Explicit "" clears it (full-replace), proving TF can still take over.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "test" {
						serial_number = %q
						device_type   = "Computer"
						username      = "preload.user.updated"
						asset_tag     = ""
					}
				`, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "asset_tag", ""),
					checkServerAssetTag(""),
				),
			},
		},
	})
}

// TestAccResource_ProInventoryPreloadRecord_ExtensionAttributes exercises the
// extension_attributes set: declare two entries, replace/modify them, then clear with
// []. No extension-attribute fixture is needed — Jamf Pro stores names unvalidated
// (non-matching names sit inert on the record).
func TestAccResource_ProInventoryPreloadRecord_ExtensionAttributes(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	serial := "ZTFACCEA" + suffix
	const addr = "jamfplatform_pro_inventory_preload_record.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInventoryPreloadRecordDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "test" {
						serial_number = %q
						device_type   = "Computer"
						extension_attributes = [
							{
								name  = "ZTF Acc EA One"
								value = "value-one"
							},
							{
								name = "ZTF Acc EA Two"
							},
						]
					}
				`, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "extension_attributes.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs(addr, "extension_attributes.*", map[string]string{
						"name":  "ZTF Acc EA One",
						"value": "value-one",
					}),
				),
			},
			{
				// Modify one entry's value and replace the other entry entirely.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "test" {
						serial_number = %q
						device_type   = "Computer"
						extension_attributes = [
							{
								name  = "ZTF Acc EA One"
								value = "value-one-updated"
							},
							{
								name  = "ZTF Acc EA Three"
								value = ""
							},
						]
					}
				`, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "extension_attributes.#", "2"),
					resource.TestCheckTypeSetElemNestedAttrs(addr, "extension_attributes.*", map[string]string{
						"name":  "ZTF Acc EA One",
						"value": "value-one-updated",
					}),
					resource.TestCheckTypeSetElemNestedAttrs(addr, "extension_attributes.*", map[string]string{
						"name":  "ZTF Acc EA Three",
						"value": "",
					}),
				),
			},
			{
				// [] clears the collection.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "test" {
						serial_number        = %q
						device_type          = "Computer"
						extension_attributes = []
					}
				`, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "extension_attributes.#", "0"),
				),
			},
		},
	})
}

// TestAccResource_ProInventoryPreloadRecord_DuplicateSerial asserts the server's
// case-insensitive duplicate-serial rejection surfaces cleanly. DUPLICATE_FIELD is a
// no-whitespace token, safe against Terraform's ~80-column error-detail wrapping.
func TestAccResource_ProInventoryPreloadRecord_DuplicateSerial(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	serial := "ZTFACCDUP" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInventoryPreloadRecordDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "first" {
						serial_number = %q
						device_type   = "Computer"
					}

					resource "jamfplatform_pro_inventory_preload_record" "second" {
						depends_on    = [jamfplatform_pro_inventory_preload_record.first]
						serial_number = %q
						device_type   = "Computer"
					}
				`, serial, serial),
				ExpectError: regexp.MustCompile(`DUPLICATE_FIELD`),
			},
			{
				// Recovery step so CheckDestroy verifies the surviving record's
				// deletion cleanly.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "first" {
						serial_number = %q
						device_type   = "Computer"
					}
				`, serial),
			},
		},
	})
}

func TestAccDataSource_ProInventoryPreloadRecord_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	serial := "ZTFACCDSID" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInventoryPreloadRecordDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "src" {
						serial_number = %q
						device_type   = "Computer"
						username      = "preload.user"
						asset_tag     = "ASSET-DS"
					}

					data "jamfplatform_pro_inventory_preload_record" "lookup" {
						id = jamfplatform_pro_inventory_preload_record.src.id
					}
				`, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_inventory_preload_record.lookup", "serial_number", "jamfplatform_pro_inventory_preload_record.src", "serial_number"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_inventory_preload_record.lookup", "device_type", "jamfplatform_pro_inventory_preload_record.src", "device_type"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_inventory_preload_record.lookup", "username", "jamfplatform_pro_inventory_preload_record.src", "username"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_inventory_preload_record.lookup", "asset_tag", "jamfplatform_pro_inventory_preload_record.src", "asset_tag"),
				),
			},
		},
	})
}

func TestAccDataSource_ProInventoryPreloadRecord_BySerialNumber(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	serial := "ZTFACCDSSN" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInventoryPreloadRecordDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "src" {
						serial_number = %q
						device_type   = "Mobile Device"
						username      = "preload.user"
					}

					data "jamfplatform_pro_inventory_preload_record" "lookup" {
						serial_number = jamfplatform_pro_inventory_preload_record.src.serial_number
					}
				`, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_inventory_preload_record.lookup", "id", "jamfplatform_pro_inventory_preload_record.src", "id"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_inventory_preload_record.lookup", "device_type", "jamfplatform_pro_inventory_preload_record.src", "device_type"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_inventory_preload_record.lookup", "username", "jamfplatform_pro_inventory_preload_record.src", "username"),
				),
			},
		},
	})
}

// TestAccListResource_ProInventoryPreloadRecord_Basic exercises the
// jamfplatform_pro_inventory_preload_record list resource via `terraform query`.
func TestAccListResource_ProInventoryPreloadRecord_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	serial := "ZTFACCLIST" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInventoryPreloadRecordDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_inventory_preload_record" "src" {
						serial_number = %q
						device_type   = "Computer"
						username      = "preload.user"
					}
				`, serial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_inventory_preload_record.src", "id"),
				),
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
								}
							]
						}
					}
				`, serial),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_inventory_preload_record.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_inventory_preload_record.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(serial)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("serial_number"), KnownValue: knownvalue.StringExact(serial)},
							{Path: tfjsonpath.New("device_type"), KnownValue: knownvalue.StringExact("Computer")},
							{Path: tfjsonpath.New("username"), KnownValue: knownvalue.StringExact("preload.user")},
						},
					),
				},
			},
		},
	})
}
