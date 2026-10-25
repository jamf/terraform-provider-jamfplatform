// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package dns_zone_test

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

func TestAccResource_SecurityCloudDNSZone_DomainCollision(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	gateways := testhelpers.RequireSecurityCloudSharedGatewayIDs(t, 1)
	suffix := testhelpers.RunSuffix()
	domain := "tf-acc-collide-" + suffix + ".example.com"
	c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					created, err := c.CreateDnsZoneV1(ctx, &securitycloud.ZoneWrite{
						Name:        "tf-acc-jsc-collide-zone-" + suffix,
						Domains:     []string{domain},
						NameServers: []securitycloud.NameServer{{IP: testNameServerIPOne, GatewayID: gateways[0]}},
					})
					if err != nil {
						t.Fatalf("creating DNS zone for %q outside Terraform: %v", domain, err)
					}
					t.Cleanup(func() { _ = c.DeleteDnsZoneV1(ctx, created.ID) })
				},
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_dns_zone" "test" {
						name    = %q
						domains = [%q]

						authoritative_name_servers = [
							{
								ip_address = %q
								gateway_id = %q
							},
						]
					}
				`, "tf-acc-jsc-collide-other-"+suffix, strings.ToUpper(domain), testNameServerIPTwo, gateways[0]),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)Domain\s+already\s+in\s+use.*already\s+exists\s+in\s+Jamf\s+Security\s+Cloud`),
			},
		},
	})
}
