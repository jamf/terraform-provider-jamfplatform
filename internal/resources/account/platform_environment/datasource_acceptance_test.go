// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package platform_environment_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// TestAccDataSource_AccountPlatformEnvironment_ByIDAndByName covers the singular
// data source against an environment made in the same apply, looked up both ways.
// The run suffix keeps the name unique, which is what lets the name lookup
// succeed in an organization where names are not otherwise unique.
func TestAccDataSource_AccountPlatformEnvironment_ByIDAndByName(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	name := acceptanceName("ds")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnvironmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_platform_environment" "test" {
						name   = %q
						type   = %q
						region = %q
					}

					data "jamfplatform_account_platform_environment" "by_id" {
						id = jamfplatform_account_platform_environment.test.id
					}

					data "jamfplatform_account_platform_environment" "by_name" {
						name       = %q
						depends_on = [jamfplatform_account_platform_environment.test]
					}
				`, name, account.EnvironmentLabelSandbox, account.MetaRegionEu, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_account_platform_environment.by_id", "name", name),
					resource.TestCheckResourceAttr("data.jamfplatform_account_platform_environment.by_id", "type", account.EnvironmentLabelSandbox),
					resource.TestCheckResourceAttr("data.jamfplatform_account_platform_environment.by_id", "region", account.MetaRegionEu),
					resource.TestCheckResourceAttrPair(
						"data.jamfplatform_account_platform_environment.by_name", "id",
						environmentAddress, "id",
					),
				),
			},
		},
	})
}

// TestAccDataSource_AccountPlatformEnvironment_UnknownNameIsWritten pins that a
// name matching nothing produces a written diagnostic that points at the plural
// data source.
func TestAccDataSource_AccountPlatformEnvironment_UnknownNameIsWritten(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_account_platform_environment" "missing" {
						name = "tf-acc-no-such-environment-zzz"
					}
				`,
				ExpectError: regexp.MustCompile(`Unable to find Jamf Account platform`),
			},
		},
	})
}

// TestAccDataSource_AccountPlatformEnvironment_NeedsExactlyOneKey pins the
// config validator.
func TestAccDataSource_AccountPlatformEnvironment_NeedsExactlyOneKey(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_account_platform_environment" "neither" {}
				`,
				ExpectError: regexp.MustCompile(`Missing Attribute Configuration`),
			},
		},
	})
}

// TestAccDataSource_AccountPlatformEnvironments_ListsAnEnvironment checks the
// plural data source surfaces an environment made in the same apply. It asserts
// the environment is present rather than asserting a total, because the
// organization holds environments this test did not make.
func TestAccDataSource_AccountPlatformEnvironments_ListsAnEnvironment(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	name := acceptanceName("plural")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnvironmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_platform_environment" "test" {
						name = %q
						type = %q
					}

					data "jamfplatform_account_platform_environments" "all" {
						depends_on = [jamfplatform_account_platform_environment.test]
					}
				`, name, account.EnvironmentLabelSandbox),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_account_platform_environments.all", "id", "platform_environments"),
					resource.TestCheckTypeSetElemNestedAttrs("data.jamfplatform_account_platform_environments.all", "platform_environments.*", map[string]string{
						"name": name,
						"type": account.EnvironmentLabelSandbox,
					}),
				),
			},
		},
	})
}
