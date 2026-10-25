// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package account_group_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func collisionBlock(label, name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_account_group" %[1]q {
  display_name  = %[2]q
  access_level  = "Full Access"
  privilege_set = "Custom"
  privileges = {
    jamf_pro_server_objects = ["Read Categories"]
  }
}
`, label, name)
}

// TestAccResource_ProAccountGroup_NameCollision plans a second account group whose name differs from an
// existing one only in case, and expects the plan-time collision error.
func TestAccResource_ProAccountGroup_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-acctgrp-" + testhelpers.RunSuffix()
	existing := collisionBlock("existing", name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: existing},
			{
				Config:      existing + collisionBlock("duplicate", strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Display\s+name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

// TestAccResource_ProAccountGroup_NameDifferingByAccentDoesNotCollide plans a second account group whose name differs only by
// an accent. The Classic API compares accents exactly, so the plan must not error.
func TestAccResource_ProAccountGroup_NameDifferingByAccentDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-acctgrp-acc-" + testhelpers.RunSuffix()
	existing := collisionBlock("existing", name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: existing},
			{
				Config:             existing + collisionBlock("accented", strings.Replace(name, "e", "é", 1)),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
