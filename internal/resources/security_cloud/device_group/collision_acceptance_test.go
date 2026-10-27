// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package device_group_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func TestAccResource_SecurityCloudDeviceGroup_NameCollision(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	name := "tf-acc-jsc-collide-" + testhelpers.RunSuffix()
	c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					created, err := c.CreateDeviceGroupV1(ctx, &securitycloud.CreateGroupRequest{Name: name})
					if err != nil {
						t.Fatalf("creating device group %q outside Terraform: %v", name, err)
					}
					t.Cleanup(func() { _ = c.DeleteDeviceGroupV1(ctx, created.ID) })
				},
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_device_group" "test" {
						name = %q
					}
				`, name),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Security\s+Cloud`),
			},
		},
	})
}

// twoDeviceGroupsConfig declares two device groups, the second depending on the
// first so the creates are sequential: Jamf's duplicate check is not atomic, and
// two concurrent creates of one name can both succeed.
func twoDeviceGroupsConfig(nameOne, nameTwo string) string {
	return fmt.Sprintf(`
resource "jamfplatform_security_cloud_device_group" "one" {
  name = %q
}

resource "jamfplatform_security_cloud_device_group" "two" {
  depends_on = [jamfplatform_security_cloud_device_group.one]

  name = %q
}
`, nameOne, nameTwo)
}

func TestAccResource_SecurityCloudDeviceGroup_RenameIntoManagedNameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	one := "tf-acc-collide-managed-one-" + suffix
	two := "tf-acc-collide-managed-two-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: twoDeviceGroupsConfig(one, two)},
			{
				Config:      twoDeviceGroupsConfig(one, one),
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_SecurityCloudDeviceGroup_TwoNewWithOneNameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	name := "tf-acc-collide-pair-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoDeviceGroupsConfig(name, name),
				ExpectError: regexp.MustCompile(`(?s)Device\s+group\s+name\s+already\s+in\s+use.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
