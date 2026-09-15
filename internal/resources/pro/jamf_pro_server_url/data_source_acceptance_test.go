// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package jamf_pro_server_url_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// checkURLMatchesTenant asserts the data source's url attribute matches the value the
// Jamf Pro API returns for the tenant. The URL is tenant-specific, so it is verified
// against a live SDK fetch rather than a hardcoded literal.
func checkURLMatchesTenant(t *testing.T, dataSourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		got, err := c.GetJamfProServerURLV1(context.Background())
		if err != nil {
			return fmt.Errorf("fetching Jamf Pro server URL from tenant: %w", err)
		}
		if got == nil {
			return fmt.Errorf("expected non-nil Jamf Pro server URL record")
		}
		rs, ok := s.RootModule().Resources[dataSourceName]
		if !ok {
			return fmt.Errorf("data source %q not found in state", dataSourceName)
		}
		if attr := rs.Primary.Attributes["url"]; attr != got.URL {
			return fmt.Errorf("url mismatch: data source = %q, tenant = %q", attr, got.URL)
		}
		return nil
	}
}

func TestAccDataSource_ProJamfProServerURL_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const dsName = "data.jamfplatform_pro_jamf_pro_server_url.lookup"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "jamfplatform_pro_jamf_pro_server_url" "lookup" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dsName, "id", "singleton"),
					resource.TestCheckResourceAttrWith(dsName, "url", func(value string) error {
						if value == "" {
							return fmt.Errorf("expected url to be non-empty")
						}
						return nil
					}),
					checkURLMatchesTenant(t, dsName),
				),
			},
		},
	})
}
