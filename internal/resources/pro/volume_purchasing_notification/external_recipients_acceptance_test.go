// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package volume_purchasing_notification_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func recipientsConfig(name, recipients string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_volume_purchasing_notification" "test" {
  name                = %q
  external_recipients = %s
}
`, name, recipients)
}

// TestAccResource_ProVolumePurchasingNotification_ExternalRecipientValidation
// pins the plan-time rejection of an email address listed twice. Jamf Pro
// answers that write with a bare HTTP 500, and it compares addresses without
// regard to case, so both a different name and a different case are
// rejected. The last steps apply two recipients sharing a name, which Jamf Pro
// stores as two, and check the next plan is empty.
func TestAccResource_ProVolumePurchasingNotification_ExternalRecipientValidation(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-vpn-recipients-" + testhelpers.RunSuffix()

	sameName := `[
    { email = "first@example.com", name = "Shared Name" },
    { email = "second@example.com", name = "Shared Name" },
  ]`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      recipientsConfig(name, `[{ email = "first@example.com", name = "A" }, { email = "first@example.com", name = "B" }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`External recipient listed more than once`),
			},
			{
				Config:      recipientsConfig(name, `[{ email = "first@example.com", name = "A" }, { email = "First@Example.com", name = "A" }]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`External recipient listed more than once`),
			},
			{
				Config: recipientsConfig(name, sameName),
				Check:  resource.TestCheckResourceAttr(resAddr, "external_recipients.#", "2"),
			},
			{
				Config: recipientsConfig(name, sameName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}
