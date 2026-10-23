// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package platform_environment_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/account"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// environmentAddress is the address every test in this file uses for the subject
// resource.
const environmentAddress = "jamfplatform_account_platform_environment.test"

// acceptanceName builds a platform environment name unique to this run. Names are
// not unique in Jamf Account, so the suffix is what lets a lookup by name and a
// leftover from an interrupted run be told apart.
func acceptanceName(part string) string {
	return "tf-acc-" + part + "-" + testhelpers.RunSuffix()
}

// testAccCheckEnvironmentDestroy verifies the platform environments made during
// the test were deleted. Jamf Account exposes no read of a single environment,
// so the check scans the collection for the recorded identifiers.
func testAccCheckEnvironmentDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := account.New(testhelpers.NewAcceptanceClient(t))

		wanted := map[string]bool{}
		for _, rs := range s.RootModule().Resources {
			if rs.Type == "jamfplatform_account_platform_environment" && rs.Primary.ID != "" {
				wanted[rs.Primary.ID] = true
			}
		}
		if len(wanted) == 0 {
			return nil
		}

		environments, err := c.ListEnvironments(context.Background())
		if err != nil {
			return fmt.Errorf("error listing Jamf Account platform environments: %w", err)
		}
		for _, e := range environments {
			if wanted[e.ID] {
				return fmt.Errorf("Jamf Account platform environment %s (%s) still exists", e.Name, e.ID)
			}
		}
		return nil
	}
}

// TestAccResource_AccountPlatformEnvironment_Basic covers create, an in-place
// update of the name and type, and the import round trip.
//
// The environment is created empty and nothing here assigns a tenant, because an
// assignment cannot be undone and would leave an environment that can never be
// deleted. Import is by identifier, which is stable, so the harness default
// ImportStateId (the `id` attribute) is the right value.
func TestAccResource_AccountPlatformEnvironment_Basic(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	name := acceptanceName("basic")
	renamed := name + "-renamed"

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
				`, name, account.EnvironmentLabelSandbox, account.MetaRegionEu),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(environmentAddress, "id"),
					resource.TestCheckResourceAttr(environmentAddress, "name", name),
					resource.TestCheckResourceAttr(environmentAddress, "type", account.EnvironmentLabelSandbox),
					resource.TestCheckResourceAttr(environmentAddress, "region", account.MetaRegionEu),
				),
			},
			{
				ResourceName:            environmentAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_platform_environment" "test" {
						name   = %q
						type   = %q
						region = %q
					}
				`, renamed, account.EnvironmentLabelBeta, account.MetaRegionEu),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(environmentAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(environmentAddress, "name", renamed),
					resource.TestCheckResourceAttr(environmentAddress, "type", account.EnvironmentLabelBeta),
					resource.TestCheckResourceAttr(environmentAddress, "region", account.MetaRegionEu),
				),
			},
		},
	})
}

// TestAccResource_AccountPlatformEnvironment_Global covers the environment made
// without a region, which Jamf Account reads as accepting tenants from any
// region. The region is asserted absent rather than empty: an empty string would
// be a state-builder defect no other assertion would catch.
func TestAccResource_AccountPlatformEnvironment_Global(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	name := acceptanceName("global")

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
				`, name, account.EnvironmentLabelTrial),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(environmentAddress, "type", account.EnvironmentLabelTrial),
					resource.TestCheckNoResourceAttr(environmentAddress, "region"),
				),
			},
			{
				ResourceName:            environmentAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_AccountPlatformEnvironment_RegionChangeReplaces pins that a
// change of region plans a replacement. Jamf Account fixes the region at create
// and ignores it on update, so an in-place plan would apply and change nothing.
func TestAccResource_AccountPlatformEnvironment_RegionChangeReplaces(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	name := acceptanceName("region")

	config := func(region string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_account_platform_environment" "test" {
				name   = %q
				type   = %q
				region = %q
			}
		`, name, account.EnvironmentLabelSandbox, region)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnvironmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(account.MetaRegionEu),
				Check:  resource.TestCheckResourceAttr(environmentAddress, "region", account.MetaRegionEu),
			},
			{
				Config: config(account.MetaRegionUs),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(environmentAddress, plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.TestCheckResourceAttr(environmentAddress, "region", account.MetaRegionUs),
			},
		},
	})
}

// TestAccResource_AccountPlatformEnvironment_Disappears covers the drift path: an
// environment deleted out from under Terraform has to be dropped from state and
// planned afresh. Absence from the collection is the only signal, because there
// is no read of a single environment to return a not-found status.
func TestAccResource_AccountPlatformEnvironment_Disappears(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	name := acceptanceName("gone")

	var environmentID string

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
				`, name, account.EnvironmentLabelSandbox),
				Check: captureAttribute(environmentAddress, "id", &environmentID),
			},
			{
				PreConfig: func() {
					c := account.New(testhelpers.NewAcceptanceClient(t))
					if err := c.DeleteEnvironment(context.Background(), environmentID); err != nil {
						t.Fatalf("drift preconfig: deleting platform environment %s: %v", environmentID, err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccResource_AccountPlatformEnvironment_NameWithPaddingRefusedAtPlan pins the
// normalisation guard. Jamf Account trims the name, so a padded configuration
// would apply and read back changed.
func TestAccResource_AccountPlatformEnvironment_NameWithPaddingRefusedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_account_platform_environment" "test" {
						name = "  padded  "
						type = "SANDBOX"
					}
				`,
				ExpectError: regexp.MustCompile(`Name has leading or trailing whitespace`),
			},
		},
	})
}

// TestAccResource_AccountPlatformEnvironment_LowerCaseTypeRefusedAtPlan pins the
// enum guard. Jamf Account canonicalises the case of a type, so a lower-case
// value would read back changed.
func TestAccResource_AccountPlatformEnvironment_LowerCaseTypeRefusedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_account_platform_environment" "test" {
						name = "tf-acc-lower"
						type = "sandbox"
					}
				`,
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
		},
	})
}

// TestAccListResource_AccountPlatformEnvironment_Basic exercises the list
// resource via the `terraform query` workflow. The collection takes no filter, so
// step 2 asserts the environment made in step 1 appears among the results rather
// than pinning a total.
//
// Requires Terraform 1.14+ (list resources).
func TestAccListResource_AccountPlatformEnvironment_Basic(t *testing.T) {
	testhelpers.AccPreCheckAccount(t)
	name := acceptanceName("list")

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnvironmentDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_account_platform_environment" "test" {
						name = %q
						type = %q
					}
				`, name, account.EnvironmentLabelSandbox),
				Check: resource.TestCheckResourceAttrSet(environmentAddress, "id"),
			},
			{
				Query: true,
				Config: `
					provider "jamfplatform" {}

					list "jamfplatform_account_platform_environment" "test" {
						provider         = jamfplatform
						include_resource = true

						config {}
					}
				`,
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_account_platform_environment.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("type"), KnownValue: knownvalue.StringExact(account.EnvironmentLabelSandbox)},
						},
					),
				},
			},
		},
	})
}

// captureAttribute records an applied attribute so a later step can act on it.
// PreConfig runs without access to state, so a value has to be carried out of the
// apply step this way.
func captureAttribute(address, name string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("resource %s not found in state", address)
		}
		value, ok := rs.Primary.Attributes[name]
		if !ok {
			return fmt.Errorf("resource %s has no %s attribute", address, name)
		}
		*into = value
		return nil
	}
}
