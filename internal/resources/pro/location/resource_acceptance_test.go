// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package location_test

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

// vppTokenEnvVar holds the base64-encoded `.vpptoken` contents. Never write
// tokens to fixtures — supply via env at run time.
const vppTokenEnvVar = "JAMFPLATFORM_ACC_PRO_VPP_TOKEN"

// testAccCheckVolumePurchasingLocationDestroy verifies VPP locations created
// during the test were destroyed.
func testAccCheckVolumePurchasingLocationDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_volume_purchasing_location" {
				continue
			}
			_, err := c.GetVolumePurchasingLocationV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro Volume Purchasing location %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro Volume Purchasing location %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// TestAccResource_ProVolumePurchasingLocation_Basic exercises Create + Update
// (with token rotation via wo_version bump) against a real Jamf Pro tenant.
// The test is skipped unless JAMFPLATFORM_ACC_PRO_VPP_TOKEN is set with the base64
// contents of a `.vpptoken` downloaded from Apple Business Manager / Apple
// School Manager. Tokens MUST come from env — never commit token material to
// fixtures.
//
// ImportState is intentionally omitted: `service_token` is `WriteOnly`, so an
// ImportStateVerify step would always diff on the missing token value.
func TestAccResource_ProVolumePurchasingLocation_Basic(t *testing.T) {
	token := testhelpers.AccEnv(vppTokenEnvVar)
	if token == "" {
		t.Skipf("%s not set; skipping VPP acceptance test", vppTokenEnvVar)
	}

	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-vpp-" + suffix
	nameUpdated := "tf-acc-pro-vpp-updated-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVolumePurchasingLocationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_volume_purchasing_location" "test" {
						name                     = %q
						service_token            = %q
						service_token_wo_version = 1
					}
				`, name, token),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_volume_purchasing_location.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_volume_purchasing_location.test", "name", name),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_volume_purchasing_location.test", "token_expiration"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_volume_purchasing_location.test", "last_sync_time"),
				),
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_volume_purchasing_location" "test" {
						name                     = %q
						service_token            = %q
						service_token_wo_version = 2
					}
				`, nameUpdated, token),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_volume_purchasing_location.test", "name", nameUpdated),
					resource.TestCheckResourceAttr("jamfplatform_pro_volume_purchasing_location.test", "service_token_wo_version", "2"),
				),
			},
		},
	})
}
