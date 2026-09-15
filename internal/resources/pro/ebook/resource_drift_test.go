// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Drift-detection coverage for issue #387. Kept in its own file so the
// omit-retains contract tests in resource_acceptance_test.go stay untouched.

package ebook_test

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

// ebookDriftConfig declares the general and self_service attributes the drift
// test mutates server-side. All of them are echoed unconditionally by the
// classic /ebooks GET (Jamf Pro 11.31.1, wire-probed 2026-09-06).
//
// Three groups are deliberately absent. file_type is canonicalised by the
// server and deploy_as_managed does not persist, so both keep a sticky read and
// cannot report drift (see flattenEbookGeneral). The notification_* fields are
// echoed only while the tenant-level Self Service notifications toggle is on,
// and an acceptance test must not depend on a tenant setting it does not
// manage — that half is covered by the unit tests in drift_test.go instead.
func ebookDriftConfig(name, author, buttonText string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_ebook" "test" {
			general = {
				name            = %q
				author          = %q
				url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
				file_type       = "PDF"
				version         = "1.0"
				deployment_type = "Make Available in Self Service"
			}
			self_service = {
				display_name             = %[1]q
				install_button_text      = %[3]q
				self_service_description = "Declared by Terraform."
				feature_on_main_page     = true
			}
		}
	`, name, author, buttonText)
}

// mutateEbookOutOfBand rewrites the managed attributes straight through the
// classic endpoint, standing in for an administrator editing the ebook in the
// Jamf Pro UI.
func mutateEbookOutOfBand(t *testing.T, id string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	mutatedAuthor := "Mutated Author"
	mutatedButton := "Mutated Button"
	mutatedDesc := "Mutated outside Terraform."
	no := false
	if err := c.UpdateEbookByID(context.Background(), id, &proclassic.EbookPost{
		General:     &proclassic.EbookPostGeneral{Author: &mutatedAuthor},
		SelfService: &proclassic.EbookPostSelfService{InstallButtonText: &mutatedButton, SelfServiceDescription: &mutatedDesc, FeatureOnMainPage: &no},
	}); err != nil {
		t.Fatalf("out-of-band update of ebook %s failed: %s", id, err)
	}
}

// captureEbookID records the resource id so a later step's PreConfig can reach
// the object directly.
func captureEbookID(into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[ebookResourceAddr]
		if !ok {
			return fmt.Errorf("%s missing from state", ebookResourceAddr)
		}
		*into = rs.Primary.ID
		return nil
	}
}

// TestAccResource_ProEbook_DriftIsReported pins the wire-authoritative read at
// the acceptance level: a change made outside the workspace must plan as an
// in-place update. Before issue #387 the refresh in step 2 adopted nothing and
// the plan was empty.
func TestAccResource_ProEbook_DriftIsReported(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ebook-drift-" + suffix
	const author = "TF Acc"
	const buttonText = "Get it"

	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEbookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ebookDriftConfig(name, author, buttonText),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "general.author", author),
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.install_button_text", buttonText),
					captureEbookID(&id),
				),
			},
			{
				PreConfig: func() { mutateEbookOutOfBand(t, id) },
				Config:    ebookDriftConfig(name, author, buttonText),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(ebookResourceAddr, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "general.author", author),
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.install_button_text", buttonText),
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.self_service_description", "Declared by Terraform."),
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.feature_on_main_page", "true"),
				),
			},
		},
	})
}
