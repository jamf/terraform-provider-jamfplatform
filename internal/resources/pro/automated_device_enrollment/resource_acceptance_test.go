// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package automated_device_enrollment_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// ade token env var. Never write tokens to fixtures — supply via env at run time.
const adeTokenEnvVar = "JAMFPLATFORM_ACC_PRO_DEP_TOKEN"

// testAccCheckAutomatedDeviceEnrollmentDestroy verifies ADE instances created
// during the test were destroyed.
func testAccCheckAutomatedDeviceEnrollmentDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_automated_device_enrollment" {
				continue
			}
			_, err := c.GetDeviceEnrollmentV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro Automated Device Enrollment instance %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro Automated Device Enrollment instance %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_ProAutomatedDeviceEnrollment_Basic exercises Create +
// Update (with token rotation via wo_version bump) against a real Jamf Pro
// tenant. The test is skipped unless the JAMFPLATFORM_ACC_PRO_DEP_TOKEN env var is
// set with a base64-encoded `.p7m` server token downloaded from Apple
// Business Manager / Apple School Manager. Tokens MUST come from env — never
// commit token material to fixtures.
//
// ImportState is intentionally omitted: `server_token` is `WriteOnly`, so an
// ImportStateVerify step would always diff on the missing token value.
func TestAccResource_ProAutomatedDeviceEnrollment_Basic(t *testing.T) {
	token := testhelpers.AccEnv(adeTokenEnvVar)
	if token == "" {
		t.Skipf("%s not set; skipping ADE acceptance test", adeTokenEnvVar)
	}

	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ade-" + suffix
	nameUpdated := "tf-acc-pro-ade-updated-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAutomatedDeviceEnrollmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_automated_device_enrollment" "test" {
						name                    = %q
						server_token            = %q
						server_token_wo_version = 1
					}
				`, name, token),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_automated_device_enrollment.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_automated_device_enrollment.test", "name", name),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_automated_device_enrollment.test", "token_expiration_date"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_automated_device_enrollment" "test" {
						name                    = %q
						server_token            = %q
						server_token_wo_version = 2
					}
				`, nameUpdated, token),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_automated_device_enrollment.test", "name", nameUpdated),
					resource.TestCheckResourceAttr("jamfplatform_pro_automated_device_enrollment.test", "server_token_wo_version", "2"),
				),
			},
		},
	})
}
