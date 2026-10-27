// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package disk_encryption_configuration_test

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
resource "jamfplatform_pro_disk_encryption_configuration" %[1]q {
  name                     = %[2]q
  key_type                 = "Individual"
  file_vault_enabled_users = "Current or Next User"
}
`, label, name)
}

// TestAccResource_ProDiskEncryptionConfiguration_NameCollision plans a second disk encryption configuration whose name differs from an
// existing one only in case, and expects the plan-time collision error.
func TestAccResource_ProDiskEncryptionConfiguration_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-dec-" + testhelpers.RunSuffix()
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

// twoConfigurationsConfig declares two disk encryption configurations. The
// second depends on the first so the creates are sequential: Jamf's duplicate
// check is not atomic, and two concurrent creates of one name can both succeed,
// which would leave a test asserting on a race instead of on the refusal.
func twoConfigurationsConfig(nameOne, nameTwo string) string {
	second := strings.Replace(
		collisionBlock("two", nameTwo),
		`resource "jamfplatform_pro_disk_encryption_configuration" "two" {`,
		"resource \"jamfplatform_pro_disk_encryption_configuration\" \"two\" {\n  depends_on = [jamfplatform_pro_disk_encryption_configuration.one]\n",
		1,
	)
	return collisionBlock("one", nameOne) + second
}

func TestAccResource_ProDiskEncryptionConfiguration_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: twoConfigurationsConfig(one, two),
			},
			{
				Config:      twoConfigurationsConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProDiskEncryptionConfiguration_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoConfigurationsConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)Error\s+creating\s+Jamf\s+Pro\s+disk\s+encryption\s+configuration.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
