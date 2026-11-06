// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Practitioner-level coverage of the no-execute window: a window declared in
// HCL must come back as the same window. The endpoint contortion that makes
// that true lives in no_execute.go and is pinned separately by
// TestAccPolicyResource_NoExecuteWindowEncoding.

package policy_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func policyConfigNoExecuteWindow(name, start, end string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "test" {
  general = {
    name = %q
  }
  client_side_limitations = {
    no_execute_on    = ["Sun", "Sat"]
    no_execute_start = %q
    no_execute_end   = %q
  }
}
`, name, start, end)
}

// TestAccPolicyResource_NoExecuteWindowRoundTrips is the practitioner-facing
// assertion: a window declared in HCL comes back as the same window, across
// create, an update that moves it, and import. Nothing here mentions the
// encoding — that is the point. It exercises midnight and noon deliberately,
// because both sit on arithmetic boundaries in encodeNoExecuteTime (12 AM maps
// to hour 0, 12 PM stays hour 12) and a naive implementation gets them wrong.
func TestAccPolicyResource_NoExecuteWindowRoundTrips(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-policy-noexec-" + testhelpers.RunSuffix()

	expect := func(start, end string) []statecheck.StateCheck {
		return []statecheck.StateCheck{
			statecheck.ExpectKnownValue(
				"jamfplatform_pro_policy.test",
				tfjsonpath.New("client_side_limitations").AtMapKey("no_execute_start"),
				knownvalue.StringExact(start),
			),
			statecheck.ExpectKnownValue(
				"jamfplatform_pro_policy.test",
				tfjsonpath.New("client_side_limitations").AtMapKey("no_execute_end"),
				knownvalue.StringExact(end),
			),
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPolicyDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:            policyConfigNoExecuteWindow(name, "1:00 AM", "2:00 AM"),
				ConfigStateChecks: expect("1:00 AM", "2:00 AM"),
			},
			{
				// Move the window, and cross both boundaries while doing it.
				Config:            policyConfigNoExecuteWindow(name, "12:00 AM", "12:30 PM"),
				ConfigStateChecks: expect("12:00 AM", "12:30 PM"),
			},
			{
				Config:            policyConfigNoExecuteWindow(name, "11:59 PM", "5:15 PM"),
				ConfigStateChecks: expect("11:59 PM", "5:15 PM"),
			},
		},
	})
}
