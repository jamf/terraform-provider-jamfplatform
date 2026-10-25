// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package ztna_app_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func TestAccResource_SecurityCloudZtnaApp_HostnameCollision(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	categories := testhelpers.RequireSecurityCloudContentCategories(t)
	suffix := testhelpers.RunSuffix()
	host := "collide-" + suffix + testHostnameSuffix
	c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					name := "tf-acc-jsc-collide-app-" + suffix
					hostnames := []string{host}
					none := []string{}
					created, err := c.CreateZtnaAppV1(ctx, &securitycloud.AppCreateRequest{
						Name:         &name,
						Hostnames:    &hostnames,
						BareIps:      &none,
						CategoryName: categories[0].DisplayName,
						Assignments: securitycloud.Assignments{
							Inclusions: securitycloud.AssignmentsInclusions{AllUsers: true, Groups: &none},
						},
						Routing: securitycloud.Routing{Type: securitycloud.RoutingTypeDirect},
					})
					if err != nil {
						t.Fatalf("creating application for %q outside Terraform: %v", host, err)
					}
					t.Cleanup(func() { _ = c.DeleteZtnaAppV1(ctx, created.ID) })
				},
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_ztna_app" "test" {
						name              = %q
						category          = %q
						hostnames         = [%q]
						all_device_groups = true

						routing = {
							traffic_routing = "Direct device routing"
						}
					}
				`, "tf-acc-jsc-collide-other-"+suffix, categories[0].DisplayName, host),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Hostname\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Security\s+Cloud`),
			},
		},
	})
}

func TestAccResource_SecurityCloudZtnaApp_NameCollisionIsStrictOnly(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	categories := testhelpers.RequireSecurityCloudContentCategories(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-collide-name-" + suffix

	config := func(providerBlock string) string {
		return providerBlock + fmt.Sprintf(`
			resource "jamfplatform_security_cloud_ztna_app" "test" {
				name              = %q
				category          = %q
				hostnames         = [%q]
				all_device_groups = true

				routing = {
					traffic_routing = "Direct device routing"
				}
			}
		`, strings.ToUpper(name), categories[0].DisplayName, "other-"+suffix+testHostnameSuffix)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					testhelpers.CreateOutOfBandSecurityCloudZtnaApp(t, name, "collide-name-"+suffix+testHostnameSuffix, categories[0].DisplayName)
				},
				Config: config(`
					provider "jamfplatform" {
						name_collision_check = { include_unenforced = true }
					}
				`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Name\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Security\s+Cloud.*accepts\s+duplicates`),
			},
			{
				Config:             config(""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
