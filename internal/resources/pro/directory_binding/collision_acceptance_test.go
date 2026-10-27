// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package directory_binding_test

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
resource "jamfplatform_pro_directory_binding" %[1]q {
  name                = %[2]q
  priority            = 1
  type                = "Active Directory"
  domain              = "corp.example.com"
  username            = "joiner-svc"
  password            = "change-me"
  password_wo_version = 1
  computer_ou         = "OU=Macs,DC=corp,DC=example,DC=com"

  active_directory = {
    create_mobile_account      = true
    require_confirmation       = true
    force_local_home_directory = true
    use_unc_path               = true
    network_protocol           = "smb"
    default_shell              = "/bin/bash"
    uid_attribute_mapping      = "uidNumber"
    user_gid_attribute_mapping = "gidNumber"
    gid_attribute_mapping      = "primaryGroupID"
    multiple_domains           = false
    preferred_domain           = "dc01.corp.example.com"
    admin_groups               = "Mac Admins,Domain Admins"
  }
}
`, label, name)
}

// TestAccResource_ProDirectoryBinding_NameCollision plans a second directory binding whose name differs from an
// existing one only in case, and expects the plan-time collision error.
func TestAccResource_ProDirectoryBinding_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-dirbind-" + testhelpers.RunSuffix()
	existing := collisionBlock("existing", name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: existing},
			{
				Config:      existing + collisionBlock("duplicate", strings.ToUpper(name)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Pro.*import\s+it`),
			},
		},
	})
}
