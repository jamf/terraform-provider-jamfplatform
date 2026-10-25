// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package cloud_identity_provider_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func TestAccResource_ProCloudIdentityProvider_DisplayNameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-cip-collide-" + testhelpers.RunSuffix()

	first := fmt.Sprintf(`
resource "jamfplatform_pro_cloud_identity_provider" "first" {
  display_name  = %q
  provider_name = "ENTRA_ID"
  entra_id = {
    tenant_id = "d5749c84-5cc5-4691-a187-4545c02ff915"
  }
}
`, name)
	second := first + fmt.Sprintf(`
resource "jamfplatform_pro_cloud_identity_provider" "second" {
  display_name  = %q
  provider_name = "ENTRA_ID"
  entra_id = {
    tenant_id = "d5749c84-5cc5-4691-a187-4545c02ff916"
  }
}
`, strings.ToUpper(name))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCloudIdentityProviderDestroy(t),
		Steps: []resource.TestStep{
			{Config: first},
			{
				Config:      second,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Display\s+name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro`),
			},
		},
	})
}
