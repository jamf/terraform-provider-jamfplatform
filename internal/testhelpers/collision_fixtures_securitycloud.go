// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package testhelpers

import (
	"context"
	"os"
	"regexp"
	"testing"

	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/securitycloudtenant"
)

// dedicatedGatewayDatacenter is the datacenter the dedicated gateways these
// fixtures create are placed in. It is the wire value of "Europe - UK".
const dedicatedGatewayDatacenter = "eu-west-2"

// CreateOutOfBandSecurityCloudDNSZone creates a custom DNS zone outside Terraform
// and returns its ID.
func CreateOutOfBandSecurityCloudDNSZone(t *testing.T, name, domain, gatewayID, nameServerIP string) string {
	t.Helper()
	c := securitycloud.New(NewAcceptanceClient(t))
	ctx := context.Background()
	created, err := c.CreateDnsZoneV1(ctx, &securitycloud.ZoneWrite{
		Name:        name,
		Domains:     []string{domain},
		NameServers: []securitycloud.NameServer{{IP: nameServerIP, GatewayID: gatewayID}},
	})
	if err != nil {
		t.Fatalf("creating DNS zone %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteDnsZoneV1(ctx, created.ID) })
	return created.ID
}

// CreateOutOfBandSecurityCloudZtnaApp creates a custom access policy application
// outside Terraform and returns its ID.
func CreateOutOfBandSecurityCloudZtnaApp(t *testing.T, name, hostname, categoryName string) string {
	t.Helper()
	c := securitycloud.New(NewAcceptanceClient(t))
	ctx := context.Background()
	hostnames := []string{hostname}
	none := []string{}
	created, err := c.CreateZtnaAppV1(ctx, &securitycloud.AppCreateRequest{
		Name:         &name,
		Hostnames:    &hostnames,
		BareIps:      &none,
		CategoryName: categoryName,
		Assignments: securitycloud.Assignments{
			Inclusions: securitycloud.AssignmentsInclusions{AllUsers: true, Groups: &none},
		},
		Routing: securitycloud.Routing{Type: securitycloud.RoutingTypeDirect},
	})
	if err != nil {
		t.Fatalf("creating application %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteZtnaAppV1(ctx, created.ID) })
	return created.ID
}

// CreateOutOfBandSecurityCloudDedicatedGateway creates a dedicated internet
// gateway outside Terraform and returns its ID. It skips the test when the
// account has no dedicated IP addresses left, which the first two gateways of a
// tenant consume.
func CreateOutOfBandSecurityCloudDedicatedGateway(t *testing.T, name string) string {
	t.Helper()
	c := securitycloud.New(NewAcceptanceClient(t))
	ctx := context.Background()

	tenantID, err := securitycloudtenant.Resolve(ctx, os.Getenv("JAMFPLATFORM_TENANT_ID"), func(ctx context.Context) (*securitycloud.ConnectorPage, error) {
		return c.ListUemConnectorsV1(ctx)
	})
	if err != nil {
		t.Fatalf("resolving the Security Cloud tenant ID: %v", err)
	}
	enabled := true
	created, err := c.CreateZtnaGatewayV1(ctx, &securitycloud.GatewayCreateRequest{
		Name:         name,
		Datacenter:   dedicatedGatewayDatacenter,
		Contact:      securitycloud.GatewayContact{Name: "Terraform Acceptance", Email: "tf-acc@example.com"},
		Enabled:      &enabled,
		TenantIds:    []string{tenantID},
		DedicatedIps: &securitycloud.DedicatedIps{Enabled: true},
	})
	if err != nil {
		if skipForDedicatedIPs(err) {
			t.Skipf("Skipping: no dedicated IP addresses left for a gateway outside Terraform: %v", err)
		}
		t.Fatalf("creating gateway %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteZtnaGatewayV1(ctx, created.ID) })
	return created.ID
}

// CreateOutOfBandSecurityCloudGroupedGateway creates a grouped gateway over the
// given member gateways outside Terraform and returns its ID.
func CreateOutOfBandSecurityCloudGroupedGateway(t *testing.T, name string, memberIDs []string) string {
	t.Helper()
	c := securitycloud.New(NewAcceptanceClient(t))
	ctx := context.Background()

	tenantID, err := securitycloudtenant.Resolve(ctx, os.Getenv("JAMFPLATFORM_TENANT_ID"), func(ctx context.Context) (*securitycloud.ConnectorPage, error) {
		return c.ListUemConnectorsV1(ctx)
	})
	if err != nil {
		t.Fatalf("resolving the Security Cloud tenant ID: %v", err)
	}
	created, err := c.CreateZtnaGroupedGatewayV1(ctx, &securitycloud.GroupedGatewayCreateRequest{
		Name:               name,
		GatewayIds:         memberIDs,
		RecoveryDelayInSec: 300,
		RoutingStrategy:    securitycloud.RoutingStrategyNearest,
		TenantIds:          []string{tenantID},
	})
	if err != nil {
		t.Fatalf("creating grouped gateway %q outside Terraform: %v", name, err)
	}
	t.Cleanup(func() { _ = c.DeleteZtnaGroupedGatewayV1(ctx, created.ID) })
	return created.ID
}

// dedicatedIPsLimit matches the 409 a tenant answers when its dedicated IP quota is
// used. The Jamf Security Cloud SDK generates no constant for the code.
var dedicatedIPsLimit = regexp.MustCompile(`DEDICATED_IPS_LIMIT`)

func skipForDedicatedIPs(err error) bool {
	if err == nil {
		return false
	}
	return dedicatedIPsLimit.MatchString(err.Error()) || noDedicatedIPsEntitlement.MatchString(err.Error())
}
