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
