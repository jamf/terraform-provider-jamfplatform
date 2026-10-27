// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package mobile_device_provisioning_profile_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func createExistingProvisioningProfile(t *testing.T, name string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateMobileDeviceProvisioningProfileByID(ctx, "0", &proclassic.MobileDeviceProvisioningProfile{
		General: &proclassic.MobileDeviceProvisioningProfileGeneral{Name: &name},
	})
	if err != nil {
		t.Fatalf("creating provisioning profile %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteMobileDeviceProvisioningProfileByID(ctx, fmt.Sprint(*created.ID)) })
}

func collisionProfileConfig(name, blob string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_provisioning_profile" "test" {
  name         = %q
  profile_data = %q
}
`, name, blob)
}

func TestAccResource_ProMobileDeviceProvisioningProfile_NameCollision(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-mdpp-" + testhelpers.RunSuffix()
	blob := fixtureBase64(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:   func() { createExistingProvisioningProfile(t, name) },
				Config:      collisionProfileConfig(" "+strings.ToUpper(name)+" ", blob),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*mobile\s+device\s+provisioning\s+profile.*leading\s+and\s+trailing\s+spaces.*import\s+it`),
			},
		},
	})
}

func TestAccResource_ProMobileDeviceProvisioningProfile_AccentVariantDoesNotCollide(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-mdpp-accent-" + testhelpers.RunSuffix()
	blob := fixtureBase64(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig:          func() { createExistingProvisioningProfile(t, name) },
				Config:             collisionProfileConfig(name+"é", blob),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// twoProfilesConfig declares two provisioning profiles. The second depends on the
// first so the creates are sequential: Jamf's duplicate check is not atomic, and
// two concurrent creates of one name can both succeed, which would leave a test
// asserting on a race instead of on the refusal.
func twoProfilesConfig(nameOne, nameTwo, blob string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_provisioning_profile" "one" {
  name         = %q
  profile_data = %q
}

resource "jamfplatform_pro_mobile_device_provisioning_profile" "two" {
  depends_on = [jamfplatform_pro_mobile_device_provisioning_profile.one]

  name         = %q
  profile_data = %q
}
`, nameOne, blob, nameTwo, blob)
}

func TestAccResource_ProMobileDeviceProvisioningProfile_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix
	blob := fixtureBase64(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProvisioningProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: twoProfilesConfig(one, two, blob),
			},
			{
				Config:      twoProfilesConfig(one, one, blob),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_ProMobileDeviceProvisioningProfile_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()
	blob := fixtureBase64(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProvisioningProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      twoProfilesConfig(name, name, blob),
				ExpectError: regexp.MustCompile(`(?s)Error\s+creating\s+Jamf\s+Pro\s+mobile\s+device\s+provisioning\s+profile.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
