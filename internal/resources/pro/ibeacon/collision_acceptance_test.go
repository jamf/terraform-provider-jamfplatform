// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ibeacon_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func collisionBlock(label, name string, minor int) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_ibeacon" %q {
  name  = %q
  uuid  = "e2c56db5-dffb-48d2-b060-d0f5a71096e0"
  major = 1
  minor = %d
}
`, label, name, minor)
}

// TestAccResource_ProIbeacon_NameCollision plans a second iBeacon whose name differs from an
// existing one only in case, and expects the plan-time collision error.
func TestAccResource_ProIbeacon_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-ibeacon-" + testhelpers.RunSuffix()
	existing := collisionBlock("existing", name, 11)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: existing},
			{
				Config:      existing + collisionBlock("duplicate", strings.ToUpper(name), 12),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}
