// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package app_request_form_field_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const collisionProviderBlock = `
provider "jamfplatform" {
  name_collision_check = { include_unenforced = true }
}
`

func collisionResource(label, name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_app_request_form_field" "%s" {
	title    = %q
	priority = 10
}
`, label, name)
}

// TestAccResource_ProAppRequestFormField_StrictNameCollision proves the strict check on a type
// that stores duplicates. A second resource at another address plans a name the
// first already holds, in a different case. With include_unenforced set the plan
// fails naming the existing object. With the default configuration the same
// config applies and the tenant ends up with two objects, which is the behaviour
// the strict check exists to flag.
func TestAccResource_ProAppRequestFormField_StrictNameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-arff-" + testhelpers.RunSuffix()
	duplicate := strings.ToUpper(name)

	first := collisionResource("existing", name)
	both := first + collisionResource("duplicate", duplicate)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppRequestFormFieldDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: first,
			},
			{
				Config:      collisionProviderBlock + both,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Title\s+already\s+in\s+use.*duplicate\s+check`),
			},
			{
				Config: both,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_app_request_form_field.existing", "id"),
					resource.TestCheckResourceAttrSet("jamfplatform_pro_app_request_form_field.duplicate", "id"),
				),
			},
		},
	})
}

// twoFieldsConfig declares two form fields. The second depends on the first so
// the creates are sequential.
func twoFieldsConfig(titleOne, titleTwo string) string {
	return collisionProviderBlock + fmt.Sprintf(`
resource "jamfplatform_pro_app_request_form_field" "one" {
	title    = %q
	priority = 10
}

resource "jamfplatform_pro_app_request_form_field" "two" {
	depends_on = [jamfplatform_pro_app_request_form_field.one]

	title    = %q
	priority = 11
}
`, titleOne, titleTwo)
}

// TestAccResource_ProAppRequestFormField_RenameIntoManagedTitleOffersNoImport
// covers the managed-elsewhere wording for a type that stores duplicates, which
// only the strict check reaches. There is no two-new-with-one-title test: Jamf
// Pro accepts duplicate titles, so the create is never refused and the conflict
// hint never fires.
func TestAccResource_ProAppRequestFormField_RenameIntoManagedTitleOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAppRequestFormFieldDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: twoFieldsConfig(one, two),
			},
			{
				Config:      twoFieldsConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Title\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}
