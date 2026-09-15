// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /licensedsoftware endpoint.
// Classic has known concurrency issues when multiple writes hit the same
// resource type — keep these tests serial with any future classic acceptance
// work in this package.
//
// No pre-existing tenant target objects are required: the record header leaves
// site unset (defaults to "None"), and software definitions / licences are
// free-text. No fixture strings contain a literal '&' (the ProClassic content
// 409 bug — see project_proclassic_payload_amp_content_bug).

package licensed_software_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const licensedSoftwareResourceAddr = "jamfplatform_pro_licensed_software.test"

// testAccCheckLicensedSoftwareDestroy verifies records created during the test
// were destroyed.
func testAccCheckLicensedSoftwareDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_licensed_software" {
				continue
			}
			_, err := c.GetLicensedSoftwareByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro licensed software %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro licensed software %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// definitionsOnlyConfig: one software definition, licenses OMITTED (opt-out —
// unmanaged; nothing exists to retain on a fresh record). The header bools are
// left unset so the server defaults resolve (every general bool false, platform
// "Any", site "None").
func definitionsOnlyConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name = %q
			software_definitions = [
				{ name = "Acme Editor", version = "1.0", compare_type = "is" },
			]
		}
	`, name)
}

// fullConfig: every mutable header field set, two software definitions (add),
// and two licences (add) — the first carrying a full purchasing block, the
// second a bare licence. Used for the import round-trip and the grow step.
func fullConfig(name, poDate, expires string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name                                    = %q
			publisher                               = "Acme Corp"
			platform                                = "Mac"
			notes                                   = "managed by terraform"
			send_email_on_violation                 = true
			remove_titles_from_inventory_reports    = true
			exclude_titles_purchased_from_app_store = true

			software_definitions = [
				{ name = "Acme Editor", version = "2.0", compare_type = "is" },
				{ name = "Acme Viewer", compare_type = "like" },
			]

			licenses = [
				{
					serial_number_1   = "SER-0001"
					organization_name = "Acme Corp"
					registered_to     = "IT Department"
					license_type      = "Standard"
					license_count     = 25
					notes             = "primary licence"
					purchasing = {
						license_term       = "perpetual"
						po_number          = "PO-12345"
						po_date            = %q
						vendor             = "Acme Reseller"
						license_expires    = %q
						purchase_price     = "1999.00"
						life_expectancy    = 3
						purchasing_account = "Finance"
						purchasing_contact = "Jane Doe"
					}
				},
				{
					serial_number_1 = "SER-0002"
					license_type    = "Concurrent"
					license_count   = 0
					purchasing = {
						license_term = "annual"
						vendor       = "Acme Reseller"
					}
				},
			]
		}
	`, name, poDate, expires)
}

// shrunkConfig: one software definition (remove), one licence (remove), header
// fields changed back. Drives the present→absent removal path on both lists.
func shrunkConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name      = %q
			publisher = "Acme Corp Updated"
			platform  = "Any"

			software_definitions = [
				{ name = "Acme Viewer", compare_type = "like" },
			]

			licenses = [
				{
					serial_number_1 = "SER-0002"
					license_type    = "Concurrent"
					license_count   = 10
				},
			]
		}
	`, name)
}

// TestAccResource_ProLicensedSoftware exercises the full lifecycle: create with
// definitions only, import round-trip, a grow update (header mutation + add a
// second definition AND two licences with computed epoch/utc re-resolution),
// then a shrink update (remove a definition AND a licence). Covers the §1.8
// update round-trip, nested-list add+remove on both positional lists, and
// computed-sibling re-resolution.
func TestAccResource_ProLicensedSoftware(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-licsw-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLicensedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				// Create: definitions only, no licences.
				Config: definitionsOnlyConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(licensedSoftwareResourceAddr, "id"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "name", name),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "platform", "Any"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "send_email_on_violation", "false"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "site_id", "-1"),
					// No site: site_name is null (absent) on the "-1" sentinel — the
					// server echo of "NONE" is flaky, so DerivedRefName nulls it.
					resource.TestCheckNoResourceAttr(licensedSoftwareResourceAddr, "site_name"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.#", "1"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.0.name", "Acme Editor"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.0.compare_type", "is"),
					// licenses omitted -> null list (no .# entry).
					resource.TestCheckNoResourceAttr(licensedSoftwareResourceAddr, "licenses.#"),
				),
			},
			{
				// Grow: full header + 2 definitions + 2 licences.
				Config: fullConfig(name, "2026-03-15", "2027-03-15"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "publisher", "Acme Corp"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "platform", "Mac"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "send_email_on_violation", "true"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.#", "2"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.1.name", "Acme Viewer"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.#", "2"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.serial_number_1", "SER-0001"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.license_count", "25"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.purchasing.license_term", "perpetual"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.purchasing.life_expectancy", "3"),
					// Computed epoch/utc re-resolution from the date strings.
					resource.TestCheckResourceAttrSet(licensedSoftwareResourceAddr, "licenses.0.purchasing.po_date_epoch"),
					resource.TestCheckResourceAttrSet(licensedSoftwareResourceAddr, "licenses.0.purchasing.po_date_utc"),
					resource.TestCheckResourceAttrSet(licensedSoftwareResourceAddr, "licenses.0.purchasing.license_expires_epoch"),
					// Second licence: count 0 preserved, annual term round-trips.
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.1.license_count", "0"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.1.purchasing.license_term", "annual"),
				),
			},
			{
				ResourceName:      licensedSoftwareResourceAddr,
				ImportState:       true,
				ImportStateVerify: true,
				// software_definitions and licenses are opt-out lists: the importer
				// has no prior model, so it leaves them unmanaged (null) and they
				// are re-declared to take ownership. computers is a read-only echo.
				ImportStateVerifyIgnore: []string{"timeouts", "computers", "software_definitions", "licenses"},
			},
			{
				// Shrink: 1 definition (remove), 1 licence (remove), header change.
				Config: shrunkConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "publisher", "Acme Corp Updated"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "platform", "Any"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.#", "1"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.0.name", "Acme Viewer"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.#", "1"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.serial_number_1", "SER-0002"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.license_count", "10"),
				),
			},
		},
	})
}

// posListsConfig renders a record whose two positional lists carry the supplied
// ordered definition names and licence serials. Definitions all use the default
// compare_type EXCEPT the one named "Bravo" (which omits it) so the schema
// default ("like") is exercised end-to-end. licence_count is derived from the
// 1-based position so a value landing at the wrong index is detectable.
func posListsConfig(name string, defNames, licSerials []string) string {
	var defs strings.Builder
	for _, d := range defNames {
		if d == "Bravo" {
			fmt.Fprintf(&defs, "\t\t\t\t{ name = %q, version = \"2.0\" },\n", d) // compare_type omitted -> default "like"
		} else {
			fmt.Fprintf(&defs, "\t\t\t\t{ name = %q, version = \"1.0\", compare_type = \"is\" },\n", d)
		}
	}
	var lics strings.Builder
	for i, s := range licSerials {
		lics.WriteString(fmt.Sprintf("\t\t\t\t{ serial_number_1 = %q, license_type = \"Standard\", license_count = %d },\n", s, i+1))
	}
	return fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name = %q
			software_definitions = [
%s			]
			licenses = [
%s			]
		}
	`, name, defs.String(), lics.String())
}

// TestAccResource_ProLicensedSoftware_PositionalLists targets the defining risk
// of the two id-less lists: ordering and per-index correlation. It exercises
// 3-element lists, the compare_type default, a full reorder (asserting the
// server preserves send-order — the load-bearing wire assumption), a mid-list
// element removal with an in-place mutation of a survivor, and a non-empty →
// empty clear. Terraform's implicit post-apply plan check makes each step a
// drift/idempotency guard.
func TestAccResource_ProLicensedSoftware_PositionalLists(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-licsw-pos-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLicensedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				// 3 + 3, with "Bravo" relying on the compare_type default.
				Config: posListsConfig(name, []string{"Alpha", "Bravo", "Charlie"}, []string{"SER-1", "SER-2", "SER-3"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.#", "3"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.0.name", "Alpha"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.1.name", "Bravo"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.2.name", "Charlie"),
					// Default compare_type resolved for the entry that omitted it.
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.1.compare_type", "like"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.#", "3"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.serial_number_1", "SER-1"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.license_count", "1"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.2.serial_number_1", "SER-3"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.2.license_count", "3"),
				),
			},
			{
				// Full reorder: the server must return the new send-order, or the
				// implicit post-apply plan is non-empty and this step fails.
				Config: posListsConfig(name, []string{"Charlie", "Bravo", "Alpha"}, []string{"SER-3", "SER-2", "SER-1"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.0.name", "Charlie"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.2.name", "Alpha"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.serial_number_1", "SER-3"),
					// license_count tracks position (SER-3 now first => count 1).
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.license_count", "1"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.2.serial_number_1", "SER-1"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.2.license_count", "3"),
				),
			},
			{
				// Mid-list removal: drop "Bravo" / "SER-2" from the middle. The
				// survivors must re-correlate cleanly at their new indices.
				Config: posListsConfig(name, []string{"Charlie", "Alpha"}, []string{"SER-3", "SER-1"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.#", "2"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.0.name", "Charlie"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.1.name", "Alpha"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.#", "2"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.serial_number_1", "SER-3"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.1.serial_number_1", "SER-1"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.1.license_count", "2"),
				),
			},
			{
				// Non-empty → cleared via explicit []: under the opt-out contract,
				// `[]` is the clear signal (omitting would instead retain). The
				// provider emits an empty wire element and state reconciles to a
				// known empty list (.# == 0).
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_licensed_software" "test" {
						name                 = %q
						software_definitions = []
						licenses             = []
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.#", "0"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.#", "0"),
				),
			},
		},
	})
}

// TestAccResource_ProLicensedSoftware_InvalidPlatform asserts the platform OneOf
// validator rejects an out-of-vocabulary value.
func TestAccResource_ProLicensedSoftware_InvalidPlatform(t *testing.T) {
	testhelpers.AccPreCheck(t)
	config := `
		resource "jamfplatform_pro_licensed_software" "test" {
			name     = "tf-acc-invalid-platform"
			platform = "BadPlatform"
		}
	`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`BadPlatform`),
			},
		},
	})
}

// TestAccResource_ProLicensedSoftware_InvalidCompareType asserts the
// software_definitions.compare_type OneOf validator rejects an unknown operator.
func TestAccResource_ProLicensedSoftware_InvalidCompareType(t *testing.T) {
	testhelpers.AccPreCheck(t)
	config := `
		resource "jamfplatform_pro_licensed_software" "test" {
			name = "tf-acc-invalid-compare"
			software_definitions = [
				{ name = "X", compare_type = "BadCompareOp" },
			]
		}
	`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`BadCompareOp`),
			},
		},
	})
}

// TestAccResource_ProLicensedSoftware_InvalidLicenseTerm asserts the
// purchasing.license_term OneOf validator rejects an unknown term.
func TestAccResource_ProLicensedSoftware_InvalidLicenseTerm(t *testing.T) {
	testhelpers.AccPreCheck(t)
	config := `
		resource "jamfplatform_pro_licensed_software" "test" {
			name = "tf-acc-invalid-term"
			licenses = [
				{
					serial_number_1 = "S1"
					purchasing = { license_term = "BadTermValue" }
				},
			]
		}
	`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`BadTermValue`),
			},
		},
	})
}

// TestAccResource_ProLicensedSoftware_InvalidLifeExpectancy asserts the
// life_expectancy Between(1,5) validator rejects an out-of-range value.
func TestAccResource_ProLicensedSoftware_InvalidLifeExpectancy(t *testing.T) {
	testhelpers.AccPreCheck(t)
	config := `
		resource "jamfplatform_pro_licensed_software" "test" {
			name = "tf-acc-invalid-life"
			licenses = [
				{
					serial_number_1 = "S1"
					purchasing = {
						license_term    = "perpetual"
						life_expectancy = 9
					}
				},
			]
		}
	`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`life_expectancy`),
			},
		},
	})
}

// TestAccDataSource_ProLicensedSoftware_ByID looks up a created record by ID and
// verifies the flat projection resolves the general header.
func TestAccDataSource_ProLicensedSoftware_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-licsw-ds-id-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLicensedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_licensed_software" "src" {
						name      = %q
						publisher = "Acme Corp"
						platform  = "Mac"
					}

					data "jamfplatform_pro_licensed_software" "lookup" {
						id = jamfplatform_pro_licensed_software.src.id
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_licensed_software.lookup", "name", "jamfplatform_pro_licensed_software.src", "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_licensed_software.lookup", "publisher", "Acme Corp"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_licensed_software.lookup", "platform", "Mac"),
				),
			},
		},
	})
}

// TestAccDataSource_ProLicensedSoftware_ByName looks up a created record by exact
// name and verifies the ID resolves.
func TestAccDataSource_ProLicensedSoftware_ByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-licsw-ds-name-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLicensedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_licensed_software" "src" {
						name = %q
					}

					data "jamfplatform_pro_licensed_software" "lookup" {
						name = jamfplatform_pro_licensed_software.src.name
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_licensed_software.lookup", "id", "jamfplatform_pro_licensed_software.src", "id"),
				),
			},
		},
	})
}

// TestAccListResource_ProLicensedSoftware exercises the list resource via the
// `terraform query` workflow, filtered to the record created in the first step.
func TestAccListResource_ProLicensedSoftware(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-licsw-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLicensedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_licensed_software" "src" {
						name      = %q
						publisher = "Acme Corp"

						software_definitions = [
							{
								name         = "Acme Editor"
								version      = "2.0"
								compare_type = "is"
							},
						]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_licensed_software.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_licensed_software" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = {
								name_substring = %q
							}
						}
					}
				`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_licensed_software.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_licensed_software.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							// Neither publisher nor software_definitions is in the
							// /licensedsoftware summary row, which carries id and name
							// only. Asserting name alone is what let a list resource
							// emitting nulls for the rest pass, while
							// `terraform query -generate-config-out` wrote a title with
							// no definitions and no licences — and applying that back
							// stripped both. This pins the per-item GET.
							{Path: tfjsonpath.New("publisher"), KnownValue: knownvalue.StringExact("Acme Corp")},
							{Path: tfjsonpath.New("software_definitions").AtSliceIndex(0).AtMapKey("name"), KnownValue: knownvalue.StringExact("Acme Editor")},
							{Path: tfjsonpath.New("software_definitions").AtSliceIndex(0).AtMapKey("version"), KnownValue: knownvalue.StringExact("2.0")},
						},
					),
				},
			},
		},
	})
}

// checkServerLicenseCount reads the record straight from Jamf Pro (bypassing
// Terraform state) and asserts the stored licence count — the only way to prove
// that omitting the attribute RETAINED the server's licences while state shows
// them unmanaged.
func checkServerLicenseCount(t *testing.T, want int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[licensedSoftwareResourceAddr]
		if !ok {
			return fmt.Errorf("%s not found in state", licensedSoftwareResourceAddr)
		}
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		got, err := c.GetLicensedSoftwareByID(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("server GET %s: %w", rs.Primary.ID, err)
		}
		n := 0
		if got.Licenses != nil && got.Licenses.License != nil {
			n = len(*got.Licenses.License)
		}
		if n != want {
			return fmt.Errorf("server licence count = %d, want %d", n, want)
		}
		return nil
	}
}

// TestAccResource_ProLicensedSoftware_OptOut proves the opt-out contract on the
// licenses list: omitting the attribute leaves the server's licences intact
// (managed nowhere in state), while setting it to [] clears them. The live
// server read is the assertion that omit != clear.
func TestAccResource_ProLicensedSoftware_OptOut(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-licsw-optout-" + suffix

	withLicence := fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name = %q
			licenses = [
				{ serial_number_1 = "OPTOUT-1", license_type = "Standard", license_count = 1 },
			]
		}
	`, name)
	omitLicences := fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name = %q
		}
	`, name)
	clearLicences := fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name     = %q
			licenses = []
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLicensedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: withLicence,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.#", "1"),
					checkServerLicenseCount(t, 1),
				),
			},
			{
				// Omit → retain: state no longer manages licences, but the server
				// keeps the licence it had (the merge PUT omits the wrapper).
				Config: omitLicences,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(licensedSoftwareResourceAddr, "licenses.#"),
					checkServerLicenseCount(t, 1),
				),
			},
			{
				// [] → clear: now the server actually drops the licence.
				Config: clearLicences,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.#", "0"),
					checkServerLicenseCount(t, 0),
				),
			},
		},
	})
}

// licensedSoftwareOmitRetainsConfig is the fully declared shape for the
// omit-retains contract: both opt-out lists, a purchasing block on the first
// licence and every Optional+Computed header scalar, each carrying a
// distinctive value so a server that stopped retaining an omitted element is
// caught on content, not presence.
func licensedSoftwareOmitRetainsConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name                    = %q
			publisher               = "Omit Retains Publisher"
			notes                   = "omit-retains header notes"
			send_email_on_violation = true

			software_definitions = [
				{ name = "Omit Retains Editor", version = "7.3", compare_type = "is" },
				{ name = "Omit Retains Viewer", version = "2.1", compare_type = "like" },
			]

			licenses = [
				{
					serial_number_1   = "OMIT-RETAINS-1"
					organization_name = "Retain Org"
					registered_to     = "Retain Team"
					license_type      = "Standard"
					license_count     = 13
					notes             = "primary omit-retains licence"
					purchasing = {
						license_term       = "perpetual"
						po_number          = "PO-OMIT-RETAINS"
						po_date            = "2026-03-15"
						vendor             = "Retain Reseller"
						license_expires    = "2027-03-15"
						purchase_price     = "4242.00"
						life_expectancy    = 4
						purchasing_account = "Retain Finance"
						purchasing_contact = "Retain Contact"
					}
				},
				{
					serial_number_1 = "OMIT-RETAINS-2"
					license_type    = "Concurrent"
					license_count   = 7
				},
			]
		}
	`, name)
}

// licensedSoftwareOmitRetainsListsOnlyConfig keeps both lists with the same
// elements and drops only the Optional+Computed notes header, so the PUT
// re-sends both wrappers and the header carries notes from prior state.
//
// The first licence deliberately keeps its purchasing block. A sent <licenses>
// wrapper REPLACES the collection, and the classic merge does not descend into
// a re-sent element: wire-verified 2026-09-06, a PUT whose <license> carried no
// <purchasing> came back on GET with the server's default block (is_perpetual
// true, every other field empty or 0), so the nested block is not a retain
// gate and dropping it here would fail on content by design, not by defect.
func licensedSoftwareOmitRetainsListsOnlyConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name                    = %q
			publisher               = "Omit Retains Publisher"
			send_email_on_violation = true

			software_definitions = [
				{ name = "Omit Retains Editor", version = "7.3", compare_type = "is" },
				{ name = "Omit Retains Viewer", version = "2.1", compare_type = "like" },
			]

			licenses = [
				{
					serial_number_1   = "OMIT-RETAINS-1"
					organization_name = "Retain Org"
					registered_to     = "Retain Team"
					license_type      = "Standard"
					license_count     = 13
					notes             = "primary omit-retains licence"
					purchasing = {
						license_term       = "perpetual"
						po_number          = "PO-OMIT-RETAINS"
						po_date            = "2026-03-15"
						vendor             = "Retain Reseller"
						license_expires    = "2027-03-15"
						purchase_price     = "4242.00"
						life_expectancy    = 4
						purchasing_account = "Retain Finance"
						purchasing_contact = "Retain Contact"
					}
				},
				{
					serial_number_1 = "OMIT-RETAINS-2"
					license_type    = "Concurrent"
					license_count   = 7
				},
			]
		}
	`, name)
}

// licensedSoftwareOmitRetainsNameOnlyConfig drops every optional attribute, so
// the PUT carries <general> alone and neither list wrapper.
func licensedSoftwareOmitRetainsNameOnlyConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name = %q
		}
	`, name)
}

// licensedSoftwareOmitRetainsClearDefinitionsConfig declares an explicit [] for
// software_definitions while still omitting licenses, so one PUT carries an
// empty <software_definitions> wrapper (clear) and no <licenses> (retain).
func licensedSoftwareOmitRetainsClearDefinitionsConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_licensed_software" "test" {
			name                 = %q
			software_definitions = []
		}
	`, name)
}

// licensedSoftwareRetainedOnServer asserts the server's copy still carries every
// value the omit-retains config declared in its first step. wantDefinitions
// false flips the software_definitions assertion to "cleared", for the step
// that declares [] — the other half of the opt-out contract.
func licensedSoftwareRetainedOnServer(t *testing.T, wantDefinitions bool) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return testhelpers.CheckLiveObject(licensedSoftwareResourceAddr,
		func(ctx context.Context, id string) (*proclassic.LicensedSoftware, error) {
			return c.GetLicensedSoftwareByID(ctx, id)
		},
		func(ls *proclassic.LicensedSoftware) error {
			if ls.General == nil {
				return fmt.Errorf("general: absent")
			}
			if err := testhelpers.RequireEqual("general.publisher", "Omit Retains Publisher", testhelpers.Deref(ls.General.Publisher)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("general.notes", "omit-retains header notes", testhelpers.Deref(ls.General.Notes)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("general.send_email_on_violation", true, testhelpers.Deref(ls.General.SendEmailOnViolation)); err != nil {
				return err
			}

			var defs []proclassic.LicensedSoftwareDefintion
			if ls.SoftwareDefinitions != nil && ls.SoftwareDefinitions.Definition != nil {
				defs = *ls.SoftwareDefinitions.Definition
			}
			if !wantDefinitions {
				if len(defs) != 0 {
					return fmt.Errorf("software_definitions: want cleared, got %d element(s)", len(defs))
				}
			} else {
				if len(defs) != 2 {
					return fmt.Errorf("software_definitions: want 2 elements, got %d", len(defs))
				}
				for i, want := range []proclassic.LicensedSoftwareDefintion{
					{Name: new("Omit Retains Editor"), Version: new("7.3"), CompareType: new("is")},
					{Name: new("Omit Retains Viewer"), Version: new("2.1"), CompareType: new("like")},
				} {
					prefix := fmt.Sprintf("software_definitions[%d]", i)
					if err := testhelpers.RequireEqual(prefix+".name", *want.Name, testhelpers.Deref(defs[i].Name)); err != nil {
						return err
					}
					if err := testhelpers.RequireEqual(prefix+".version", *want.Version, testhelpers.Deref(defs[i].Version)); err != nil {
						return err
					}
					if err := testhelpers.RequireEqual(prefix+".compare_type", *want.CompareType, testhelpers.Deref(defs[i].CompareType)); err != nil {
						return err
					}
				}
			}

			if ls.Licenses == nil || ls.Licenses.License == nil || len(*ls.Licenses.License) != 2 {
				return fmt.Errorf("licenses: want 2 elements, got %+v", ls.Licenses)
			}
			lics := *ls.Licenses.License
			if err := testhelpers.RequireEqual("licenses[0].serial_number_1", "OMIT-RETAINS-1", testhelpers.Deref(lics[0].SerialNumber1)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[0].organization_name", "Retain Org", testhelpers.Deref(lics[0].OrganizationName)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[0].registered_to", "Retain Team", testhelpers.Deref(lics[0].RegisteredTo)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[0].license_type", "Standard", testhelpers.Deref(lics[0].LicenseType)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[0].license_count", 13, testhelpers.Deref(lics[0].LicenseCount)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[0].notes", "primary omit-retains licence", testhelpers.Deref(lics[0].Notes)); err != nil {
				return err
			}
			p := lics[0].Purchasing
			if p == nil {
				return fmt.Errorf("licenses[0].purchasing: absent")
			}
			if err := testhelpers.RequireEqual("licenses[0].purchasing.is_perpetual", true, testhelpers.Deref(p.IsPerpetual)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[0].purchasing.po_number", "PO-OMIT-RETAINS", testhelpers.Deref(p.PoNumber)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[0].purchasing.vendor", "Retain Reseller", testhelpers.Deref(p.Vendor)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[0].purchasing.life_expectancy", 4, testhelpers.Deref(p.LifeExpectancy)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[0].purchasing.purchasing_account", "Retain Finance", testhelpers.Deref(p.PurchasingAccount)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[0].purchasing.purchasing_contact", "Retain Contact", testhelpers.Deref(p.PurchasingContact)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[1].serial_number_1", "OMIT-RETAINS-2", testhelpers.Deref(lics[1].SerialNumber1)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("licenses[1].license_type", "Concurrent", testhelpers.Deref(lics[1].LicenseType)); err != nil {
				return err
			}
			return testhelpers.RequireEqual("licenses[1].license_count", 7, testhelpers.Deref(lics[1].LicenseCount))
		})
}

// TestAccResource_ProLicensedSoftware_OmittedBlocksRetained pins the opt-out
// contract on both positional lists at once, with content the plan output
// cannot show. Step 2 re-sends both lists and drops only the notes header;
// step 3 drops both lists, so the PUT carries <general> alone and the server
// must retain every element and every purchasing field; step 4 declares
// software_definitions = [] while still
// omitting licenses, so the same PUT must clear one collection and retain the
// other — the half of the contract that separates "not managed" from "empty".
// Each step's implicit post-apply plan must be empty. If a step fails on
// content, the endpoint no longer merges at that granularity and nothing that
// suppresses the removal plan may ship for this resource.
func TestAccResource_ProLicensedSoftware_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-licsw-omit-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLicensedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: licensedSoftwareOmitRetainsConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.#", "2"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.#", "2"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.purchasing.po_number", "PO-OMIT-RETAINS"),
					licensedSoftwareRetainedOnServer(t, true),
				),
			},
			{
				Config: licensedSoftwareOmitRetainsListsOnlyConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.purchasing.po_number", "PO-OMIT-RETAINS"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "notes", "omit-retains header notes"),
					licensedSoftwareRetainedOnServer(t, true),
				),
			},
			{
				Config: licensedSoftwareOmitRetainsNameOnlyConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(licensedSoftwareResourceAddr, "software_definitions.#"),
					resource.TestCheckNoResourceAttr(licensedSoftwareResourceAddr, "licenses.#"),
					licensedSoftwareRetainedOnServer(t, true),
				),
			},
			{
				Config: licensedSoftwareOmitRetainsClearDefinitionsConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "software_definitions.#", "0"),
					resource.TestCheckNoResourceAttr(licensedSoftwareResourceAddr, "licenses.#"),
					licensedSoftwareRetainedOnServer(t, false),
				),
			},
		},
	})
}
