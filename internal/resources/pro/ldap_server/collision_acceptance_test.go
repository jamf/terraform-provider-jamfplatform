// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ldap_server_test

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
resource "jamfplatform_pro_ldap_server" %[1]q {
  connection_settings = {
    display_name        = %[2]q
    directory_service   = "Open Directory"
    hostname            = "ldap.collision.example.com"
    port                = 389
    use_ssl             = false
    authentication_type = "none"
  }
}
`, label, name)
}

// TestAccResource_ProLdapServer_NameCollision plans a second LDAP server whose name differs from an
// existing one only in case, and expects the plan-time collision error.
func TestAccResource_ProLdapServer_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-ldap-" + testhelpers.RunSuffix()
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

// TestAccResource_ProLdapServer_NameDifferingByAccentDoesNotCollide plans a second LDAP server whose name differs only by
// an accent. The Classic API compares accents exactly, so the plan must not error.
func TestAccResource_ProLdapServer_NameDifferingByAccentDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-ldap-acc-" + testhelpers.RunSuffix()
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
