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

// twoZtnaAppsConfig declares two applications, the second depending on the first
// so the creates are sequential: Jamf's duplicate check is not atomic, and two
// concurrent creates of one hostname can both succeed.
func twoZtnaAppsConfig(suffix, category, hostOne, hostTwo string) string {
	return fmt.Sprintf(`
resource "jamfplatform_security_cloud_ztna_app" "one" {
  name              = %q
  category          = %q
  hostnames         = [%q]
  all_device_groups = true

  routing = {
    traffic_routing = "Direct device routing"
  }
}

resource "jamfplatform_security_cloud_ztna_app" "two" {
  depends_on = [jamfplatform_security_cloud_ztna_app.one]

  name              = %q
  category          = %q
  hostnames         = [%q]
  all_device_groups = true

  routing = {
    traffic_routing = "Direct device routing"
  }
}
`, "tf-acc-collide-one-"+suffix, category, hostOne, "tf-acc-collide-two-"+suffix, category, hostTwo)
}

func TestAccResource_SecurityCloudZtnaApp_RenameIntoManagedHostnameOffersNoImport(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	categories := testhelpers.RequireSecurityCloudContentCategories(t)
	suffix := testhelpers.RunSuffix()
	one := "collide-one-" + suffix + testHostnameSuffix
	two := "collide-two-" + suffix + testHostnameSuffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: twoZtnaAppsConfig(suffix, categories[0].DisplayName, one, two)},
			{
				Config:      twoZtnaAppsConfig(suffix, categories[0].DisplayName, one, one),
				ExpectError: regexp.MustCompile(`(?s)Hostname\s+already\s+in\s+use.*already\s+managed\s+by\s+another\s+resource`),
			},
		},
	})
}

func TestAccResource_SecurityCloudZtnaApp_TwoNewWithOneHostnameExplainsTheConflict(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	categories := testhelpers.RequireSecurityCloudContentCategories(t)
	suffix := testhelpers.RunSuffix()
	host := "collide-pair-" + suffix + testHostnameSuffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      twoZtnaAppsConfig(suffix, categories[0].DisplayName, host, host),
				ExpectError: regexp.MustCompile(`(?s)Host\s+name\s+already\s+claimed.*Another\s+resource\s+in\s+this\s+configuration`),
			},
		},
	})
}
