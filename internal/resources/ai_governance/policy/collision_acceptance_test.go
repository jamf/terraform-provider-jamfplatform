// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package policy_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const strictCollisionProvider = `
provider "jamfplatform" {
  name_collision_check = { include_unenforced = true }
}
`

func TestAccResource_AIGovernancePolicy_NameCollisionStrict(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	tool := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	name := "tf-acc-ai-policy-collide-" + testhelpers.RunSuffix()
	planned := fmt.Sprintf(`
		resource "jamfplatform_ai_governance_policy" "test" {
			name           = %q
			tool_id        = %q
			schema_version = %q
			settings_json  = jsonencode({ verbose = true })
		}
	`, strings.ToUpper(name), tool.ID, tool.SchemaVersion)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { testhelpers.CreateOutOfBandAIGovernancePolicy(t, name, tool) },
				Config:      strictCollisionProvider + planned,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use\s+\(duplicate\s+check\).*already\s+exists\s+in\s+Jamf\s+AI\s+Governance.*accepts\s+duplicates`),
			},
			{
				Config:             planned,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
