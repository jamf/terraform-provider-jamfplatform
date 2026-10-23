// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package tenants_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccDataSource_AccountTenants_ListsTheOrganizationsTenants reads the tenant
// collection, which is read-only: nothing here creates, assigns or changes a
// tenant.
//
// The organization's tenants are not under this test's control, so it asserts the
// shape rather than a content: at least one tenant comes back, every one carries
// an identifier, and a tenant's platform environment, when it has one, is an
// identifier rather than an empty string.
func TestAccDataSource_AccountTenants_ListsTheOrganizationsTenants(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	const address = "data.jamfplatform_account_tenants.all"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "jamfplatform_account_tenants" "all" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", "tenants"),
					resource.TestCheckResourceAttrWith(address, "tenants.#", func(value string) error {
						count, err := strconv.Atoi(value)
						if err != nil || count < 1 {
							return fmt.Errorf("tenants.# = %q, want at least one tenant", value)
						}
						return nil
					}),
					resource.TestCheckResourceAttrSet(address, "tenants.0.id"),
					checkNoEmptyEnvironmentIDs(address),
				),
			},
		},
	})
}

// checkNoEmptyEnvironmentIDs fails if any tenant reports an empty platform
// environment identifier. An unassigned tenant must carry null, which the flat
// state map represents by omitting the key.
func checkNoEmptyEnvironmentIDs(address string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("data source %s not found in state", address)
		}
		count, _ := strconv.Atoi(rs.Primary.Attributes["tenants.#"])
		for i := range count {
			if value, present := rs.Primary.Attributes[fmt.Sprintf("tenants.%d.platform_environment_id", i)]; present && value == "" {
				return fmt.Errorf("tenants.%d.platform_environment_id is an empty string, want null or an identifier", i)
			}
		}
		return nil
	}
}
