// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package policy_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccListResource_AIGovernancePolicy_Basic creates a policy, then queries it back twice: once
// with the default list deadline and once with `timeouts.list` set, so both the config decode and
// the Bound call are exercised against a real tenant. include_resource is set so the per-policy
// read, which has its own deadline, runs too.
func TestAccListResource_AIGovernancePolicy_Basic(t *testing.T) {
	testhelpers.AccPreCheckAIGovernance(t)
	tool := testhelpers.RequireAIGovernanceTool(t, "com.anthropic.claudecode")
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ai-policy-list-" + suffix

	query := func(listBlockExtras string) string {
		return fmt.Sprintf(`
			provider "jamfplatform" {}

			list "jamfplatform_ai_governance_policy" "test" {
				provider         = jamfplatform
				include_resource = true

				config {
					%s
				}
			}
		`, listBlockExtras)
	}

	checks := []querycheck.QueryResultCheck{
		querycheck.ExpectResourceKnownValues(
			"jamfplatform_ai_governance_policy.test",
			queryfilter.ByDisplayName(knownvalue.StringExact(name)),
			[]querycheck.KnownValueCheck{
				{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
				{Path: tfjsonpath.New("tool_id"), KnownValue: knownvalue.StringExact(tool.ID)},
				{Path: tfjsonpath.New("schema_version"), KnownValue: knownvalue.StringExact(tool.SchemaVersion)},
			},
		),
	}

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_ai_governance_policy" "src" {
						name           = %q
						description    = "created by the provider acceptance suite"
						tool_id        = %q
						schema_version = %q
						settings_json  = jsonencode({ verbose = true })
					}
				`, name, tool.ID, tool.SchemaVersion),
				Check: resource.TestCheckResourceAttrSet("jamfplatform_ai_governance_policy.src", "id"),
			},
			{
				Query:             true,
				Config:            query(""),
				QueryResultChecks: checks,
			},
			{
				Query:             true,
				Config:            query(`timeouts = { list = "5m" }`),
				QueryResultChecks: checks,
			},
			{
				Query:       true,
				Config:      query(`timeouts = { list = "soon" }`),
				ExpectError: regexp.MustCompile(`(?is)(invalid|duration)`),
			},
		},
	})
}
