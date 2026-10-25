// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package webhook_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProWebhook_NameCollision proves the plan-time check fires: a second resource
// whose name differs from the first only in case is refused at plan, before any write.
func TestAccResource_ProWebhook_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-webhook-" + testhelpers.RunSuffix()
	existing := fmt.Sprintf(`
					resource "jamfplatform_pro_webhook" "existing" {
						name  = %q
						url   = "https://example.com/walk"
						event = "ComputerAdded"
					}
				`, name)
	duplicate := fmt.Sprintf(`
					resource "jamfplatform_pro_webhook" "duplicate" {
						name  = %q
						url   = "https://example.com/walk"
						event = "ComputerAdded"
					}
				`, strings.ToUpper(name))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: existing},
			{
				Config:      existing + duplicate,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro`),
			},
		},
	})
}
