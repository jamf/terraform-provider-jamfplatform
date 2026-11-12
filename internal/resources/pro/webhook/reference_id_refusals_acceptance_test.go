// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package webhook_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccResource_ProWebhook_ReferenceIDRefusals proves the reference IDs that
// Jamf Pro cannot clear to an empty value or 0 are refused at plan time
// (#573), before an apply could fail with an inconsistent result.
func TestAccResource_ProWebhook_ReferenceIDRefusals(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// MinusOneSmartGroup
				Config: `
				resource "jamfplatform_pro_webhook" "test" {
				  name                = "tf-acc-webhook-refuse"
				  url                 = "https://example.invalid/hook"
				  event               = "SmartGroupComputerMembershipChange"
				  content_type        = "application/json"
				  authentication_type = "NONE"
				  smart_group_id      = -1
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid smart group ID`),
			},
			{
				// ZeroSmartGroup
				Config: `
				resource "jamfplatform_pro_webhook" "test" {
				  name                = "tf-acc-webhook-refuse"
				  url                 = "https://example.invalid/hook"
				  event               = "SmartGroupComputerMembershipChange"
				  content_type        = "application/json"
				  authentication_type = "NONE"
				  smart_group_id      = 0
				}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid smart group ID`),
			},
		},
	})
}
