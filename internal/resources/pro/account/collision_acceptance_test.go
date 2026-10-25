// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package account_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func collisionBlock(label, username, email string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_account" %[1]q {
  username      = %[2]q
  full_name     = "TF Acc Collision"
  email_address = %[3]q
  access_level  = "Full Access"
  privilege_set = "Custom"

  password            = "Pr0bePassw0rd-collision"
  password_wo_version = 1

  privileges = {
    jamf_pro_server_objects = ["Read Categories"]
  }
}
`, label, username, email)
}

// TestAccResource_ProAccount_UsernameCollision plans a second account whose
// username differs from an existing one only in case, and expects the plan-time
// collision error.
func TestAccResource_ProAccount_UsernameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	username := "tf-acc-collide-user-" + suffix
	existing := collisionBlock("existing", username, "tf-acc-collide-user-"+suffix+"@example.invalid")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: existing},
			{
				Config:      existing + collisionBlock("duplicate", strings.ToUpper(username), "tf-acc-collide-user-other-"+suffix+"@example.invalid"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Username\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

// TestAccResource_ProAccount_EmailCollision plans a second account whose email
// address differs from an existing one only in case, and expects the plan-time
// collision error.
func TestAccResource_ProAccount_EmailCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	email := "tf-acc-collide-mail-" + suffix + "@example.invalid"
	existing := collisionBlock("existing", "tf-acc-collide-mail-"+suffix, email)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: existing},
			{
				Config:      existing + collisionBlock("duplicate", "tf-acc-collide-mail-other-"+suffix, strings.ToUpper(email)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Email\s+address\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}

// TestAccResource_ProAccount_LeadingSpaceUsernameDoesNotCollide plans an account
// whose username differs from an existing one by a leading space. Jamf Pro keeps
// the two apart, so the plan must not error.
func TestAccResource_ProAccount_LeadingSpaceUsernameDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	username := "tf-acc-collide-lead-" + suffix
	existing := collisionBlock("existing", username, "tf-acc-collide-lead-"+suffix+"@example.invalid")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: existing},
			{
				Config:             existing + collisionBlock("spaced", " "+username, "tf-acc-collide-lead-other-"+suffix+"@example.invalid"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
