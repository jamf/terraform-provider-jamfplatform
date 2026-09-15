// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Drift-detection coverage for issue #387. Kept in its own file so the
// omit-retains contract tests in resource_acceptance_test.go stay untouched.
//
// Every field on this resource is echoed faithfully by the classic
// /licensedsoftware GET, on both the POST and the PUT path (Jamf Pro 11.31.1,
// wire-probed 2026-09-06), so none of them keeps a sticky read and all of them
// must report drift.

package licensed_software_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// licensedSoftwareDriftConfig declares the header fields, one software
// definition and one licence with a purchasing block — covering all three
// levels the sticky read used to hold.
func licensedSoftwareDriftConfig(name string) string {
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
				{ name = "Acme Editor", version = "1.0", compare_type = "is" },
			]

			licenses = [
				{
					serial_number_1   = "TF-387-SN1"
					organization_name = "Acme Corp"
					registered_to     = "TF Acc"
					license_type      = "Standard"
					notes             = "declared by terraform"
					purchasing = {
						license_term       = "perpetual"
						po_number          = "TF-387-PO"
						vendor             = "Acme Reseller"
						purchasing_account = "TF Acc Account"
						purchasing_contact = "TF Acc Contact"
					}
				},
			]
		}
	`, name)
}

// mutateLicensedSoftwareOutOfBand rewrites the managed attributes straight
// through the classic endpoint, standing in for an administrator editing the
// record in the Jamf Pro UI. The licence list is sent whole because the
// classic merge replaces a present <licenses> element's subtree.
func mutateLicensedSoftwareOutOfBand(t *testing.T, id string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	mutated := "Mutated outside Terraform."
	mutatedPublisher := "Mutated Publisher"
	mutatedSerial := "MUTATED-SN1"
	mutatedVendor := "Mutated Vendor"
	no := false
	if err := c.UpdateLicensedSoftwareByID(context.Background(), id, &proclassic.LicensedSoftware{
		General: &proclassic.LicensedSoftwareGeneral{
			Publisher:                          &mutatedPublisher,
			Notes:                              &mutated,
			SendEmailOnViolation:               &no,
			RemoveTitlesFromInventoryReports:   &no,
			ExcludeTitlesPurchasedFromAppStore: &no,
		},
		Licenses: &proclassic.LicensedSoftwareLicenses{
			License: &[]proclassic.LicensedSoftwareLicensesLicenseItem{{
				SerialNumber1: &mutatedSerial,
				Purchasing: &proclassic.LicensedSoftwareLicensesLicenseItemPurchasing{
					Vendor: &mutatedVendor,
				},
			}},
		},
	}); err != nil {
		t.Fatalf("out-of-band update of licensed software %s failed: %s", id, err)
	}
}

// captureLicensedSoftwareID records the resource id so a later step's PreConfig
// can reach the object directly.
func captureLicensedSoftwareID(into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[licensedSoftwareResourceAddr]
		if !ok {
			return fmt.Errorf("%s missing from state", licensedSoftwareResourceAddr)
		}
		*into = rs.Primary.ID
		return nil
	}
}

// TestAccResource_ProLicensedSoftware_DriftIsReported pins the
// wire-authoritative read at the acceptance level, at all three nesting levels:
// a change made outside the workspace must plan as an in-place update. Before
// issue #387 the refresh in step 2 adopted nothing and the plan was empty.
func TestAccResource_ProLicensedSoftware_DriftIsReported(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-licsw-drift-" + suffix

	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLicensedSoftwareDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: licensedSoftwareDriftConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "publisher", "Acme Corp"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.serial_number_1", "TF-387-SN1"),
					captureLicensedSoftwareID(&id),
				),
			},
			{
				PreConfig: func() { mutateLicensedSoftwareOutOfBand(t, id) },
				Config:    licensedSoftwareDriftConfig(name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(licensedSoftwareResourceAddr, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "publisher", "Acme Corp"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "notes", "managed by terraform"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "send_email_on_violation", "true"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "remove_titles_from_inventory_reports", "true"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "exclude_titles_purchased_from_app_store", "true"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.serial_number_1", "TF-387-SN1"),
					resource.TestCheckResourceAttr(licensedSoftwareResourceAddr, "licenses.0.purchasing.vendor", "Acme Reseller"),
				),
			},
		},
	})
}
