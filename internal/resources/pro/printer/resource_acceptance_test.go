// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /printers endpoint. Classic
// has known concurrency issues when multiple writes hit the same resource
// type — keep these tests serial with any other classic acceptance work in
// this package.

package printer_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckPrinterDestroy verifies printers created during the test
// were destroyed.
func testAccCheckPrinterDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_printer" {
				continue
			}
			_, err := c.GetPrinterByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro printer %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro printer %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

const printerResourceAddr = "jamfplatform_pro_printer.test"

// printerLive fetches the server's copy of the printer under test and hands it
// to assert, so a step that drops an optional string can prove the value was
// cleared on the wire — a null in state cannot tell a cleared value from one
// the classic merge retained and the state builder reconciled away (#384).
func printerLive(t *testing.T, assert func(*proclassic.Printer) error) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return testhelpers.CheckLiveObject(printerResourceAddr, c.GetPrinterByID, assert)
}

// TestAccResource_ProPrinter_Generic covers the simplest configuration —
// use_generic is left unset (defaults to true). The server echoes the bundled
// Generic.ppd path even in generic mode, but the cross-field validator forbids
// the PPD trio there, so the state builder collapses ppd/ppd_path/ppd_contents
// to null — the asserts below verify that round-trip. The update step exercises
// PUT partial-merge: rename + populate optional fields without touching the
// PPD trio; the step after it drops those fields again and proves, on the wire,
// that the always-emitted empty element cleared them (#384).
func TestAccResource_ProPrinter_Generic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := "tf-acc-printer-generic-" + suffix
	renamed := "tf-acc-printer-generic-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPrinterDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_printer" "test" {
						name = %q
						uri  = "ipp://10.1.20.120/"
					}
				`, original),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_printer.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "name", original),
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "use_generic", "true"),
					// Server echoes the bundled Generic.ppd path under
					// use_generic=true, but the validator forbids the PPD trio
					// there — the state builder collapses it to null.
					resource.TestCheckNoResourceAttr("jamfplatform_pro_printer.test", "ppd_path"),
					// shared defaults to false (schema Default).
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "shared", "false"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_printer" "test" {
						name     = %q
						uri      = "ipp://10.1.20.120/"
						location = "Building 5"
						notes    = "Updated via acceptance test."
					}
				`, renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "name", renamed),
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "location", "Building 5"),
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "notes", "Updated via acceptance test."),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_printer" "test" {
						name = %q
						uri  = "ipp://10.1.20.120/"
					}
				`, renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("jamfplatform_pro_printer.test", "location"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_printer.test", "notes"),
					printerLive(t, func(p *proclassic.Printer) error {
						if err := testhelpers.RequireEqual("location", "", testhelpers.Deref(p.Location)); err != nil {
							return err
						}
						return testhelpers.RequireEqual("notes", "", testhelpers.Deref(p.Notes))
					}),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_printer.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProPrinter_CustomPPD exercises use_generic=false with the
// full PPD trio populated. ppd_path is the gate field; without it the
// ConfigValidator would block at plan time and the server would silently
// flip use_generic back to true. The second step drops every plain optional
// string, `ppd` included, while keeping use_generic=false and ppd_path: it
// proves on the wire that each was cleared and that an empty <ppd> beside a
// populated <ppd_path> leaves use_generic alone (#384).
func TestAccResource_ProPrinter_CustomPPD(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-printer-custom-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPrinterDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_printer" "test" {
						name            = %q
						uri             = "ipp://printer.example.com/queue1"
						cups_name       = "tf_acc_custom"
						location        = "Building 6"
						model           = "HP DeskJet 2600 series"
						info            = "info field"
						notes           = "notes field"
						make_default    = true
						shared          = true
						use_generic     = false
						ppd             = "HP DeskJet 2600 series.ppd"
						ppd_path        = "/Library/Printers/PPDs/Contents/Resources/HP DeskJet 2600 series.ppd"
						# Trailing newline deliberate: exercises the trimmedStringType
						# custom-type semantic equality. The server strips it; the
						# provider must round-trip without drift.
						ppd_contents    = "*PPD-Adobe: \"4.3\"\n*FormatVersion: \"4.3\"\n*FileVersion: \"1.0\"\n"
						os_requirements = "13.5, 16.0"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "name", name),
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "use_generic", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "make_default", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "shared", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "ppd", "HP DeskJet 2600 series.ppd"),
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "ppd_path", "/Library/Printers/PPDs/Contents/Resources/HP DeskJet 2600 series.ppd"),
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "os_requirements", "13.5, 16.0"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_printer" "test" {
						name         = %q
						uri          = "ipp://printer.example.com/queue1"
						make_default = true
						shared       = true
						use_generic  = false
						ppd_path     = "/Library/Printers/PPDs/Contents/Resources/HP DeskJet 2600 series.ppd"
						ppd_contents = "*PPD-Adobe: \"4.3\"\n*FormatVersion: \"4.3\"\n*FileVersion: \"1.0\"\n"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "use_generic", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "ppd_path", "/Library/Printers/PPDs/Contents/Resources/HP DeskJet 2600 series.ppd"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_printer.test", "cups_name"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_printer.test", "location"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_printer.test", "model"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_printer.test", "info"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_printer.test", "notes"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_printer.test", "ppd"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_printer.test", "os_requirements"),
					printerLive(t, func(p *proclassic.Printer) error {
						for field, got := range map[string]*string{
							"cups_name":       p.CUPSName,
							"location":        p.Location,
							"model":           p.Model,
							"info":            p.Info,
							"notes":           p.Notes,
							"ppd":             p.Ppd,
							"os_requirements": p.OsRequirements,
						} {
							if err := testhelpers.RequireEqual(field, "", testhelpers.Deref(got)); err != nil {
								return err
							}
						}
						if err := testhelpers.RequireEqual("use_generic", false, testhelpers.Deref(p.UseGeneric)); err != nil {
							return err
						}
						return testhelpers.RequireEqual("ppd_path", "/Library/Printers/PPDs/Contents/Resources/HP DeskJet 2600 series.ppd", testhelpers.Deref(p.PpdPath))
					}),
				),
			},
			{
				ResourceName:      "jamfplatform_pro_printer.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
					// terraform-plugin-testing compares ImportStateVerify
					// attributes byte-wise; it does not consult our
					// trimmedStringType's StringSemanticEquals. The applied
					// state preserves the user's trailing newline (config form);
					// the imported state has the server-chomped form. Real
					// Terraform flows reconcile the two via semantic equality
					// during the next plan — see custom_types.go and
					// TestTrimmedStringValue_SemanticEquals_*. This ignore is
					// a test-harness limitation, not a production drift.
					"ppd_contents",
				},
			},
		},
	})
}

// TestAccResource_ProPrinter_CategoryRoundtrip exercises the
// category-by-name reference. A jamfplatform_pro_category resource creates a
// dependency category in the same plan, then the printer references it by
// display name. Drops the category in the next step (omit attribute) and
// asserts the printer state decodes the server sentinel back to null —
// proves the sentinel never leaks into TF state.
func TestAccResource_ProPrinter_CategoryRoundtrip(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	categoryName := "tf-acc-printer-cat-" + suffix
	printerName := "tf-acc-printer-cat-printer-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPrinterDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_category" "test" {
						name     = %q
						priority = 9
					}

					resource "jamfplatform_pro_printer" "test" {
						name     = %q
						category = jamfplatform_pro_category.test.name
					}
				`, categoryName, printerName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_printer.test", "category", categoryName),
				),
			},
			{
				// Drop the category attribute from the printer. The provider
				// emits empty <category></category>, the server clears to the
				// sentinel, and assignPrinterResourceModel decodes back to null.
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_category" "test" {
						name     = %q
						priority = 9
					}

					resource "jamfplatform_pro_printer" "test" {
						name = %q
					}
				`, categoryName, printerName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("jamfplatform_pro_printer.test", "category"),
				),
			},
		},
	})
}

// TestAccResource_ProPrinter_PPDValidator_GenericForbidsPPD exercises the
// use_generic=true + PPD-set rejection. Should not reach apply — the
// plan-time validator catches the conflict and returns an attribute error.
// Split from the use_generic=false case so each ExpectError stands alone
// (multi-step ExpectError sequences are non-idiomatic in plugin-testing).
func TestAccResource_ProPrinter_PPDValidator_GenericForbidsPPD(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-printer-validate-forbids-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_printer" "test" {
						name        = %q
						use_generic = true
						ppd_path    = "/some/path.ppd"
					}
				`, name),
				ExpectError: regexp.MustCompile(`ppd_path forbidden when use_generic is true`),
			},
		},
	})
}

// TestAccResource_ProPrinter_PPDValidator_CustomRequiresPath exercises the
// use_generic=false + missing ppd_path rejection. Without the validator the
// server silently flips use_generic back to true.
func TestAccResource_ProPrinter_PPDValidator_CustomRequiresPath(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-printer-validate-requires-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_printer" "test" {
						name        = %q
						use_generic = false
					}
				`, name),
				ExpectError: regexp.MustCompile(`ppd_path required when use_generic is false`),
			},
		},
	})
}
