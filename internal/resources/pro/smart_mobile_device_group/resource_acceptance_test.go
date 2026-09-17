// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package smart_mobile_device_group_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const resourceAddress = "jamfplatform_pro_smart_mobile_device_group.test"

// testAccCheckDestroy confirms every group the run created is gone. The delete
// is synchronous here, so a group that still reads back is a real leak rather
// than a propagation delay.
func testAccCheckDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_smart_mobile_device_group" {
				continue
			}
			_, err := client.GetSmartMobileDeviceGroupV2(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("checking smart mobile device group %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("smart mobile device group %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// configTwoCriteria is the starting shape: two criteria, no site, no
// description.
func configTwoCriteria(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_smart_mobile_device_group" "test" {
			name = %q

			criteria = [
				{
					name        = "Model"
					search_type = "like"
					value       = "iPad"
				},
				{
					name        = "Display Name"
					search_type = "is not"
					value       = "tf-acc-placeholder"
					and_or      = "and"
				},
			]
		}
	`, name)
}

// configThreeCriteriaRenamed grows the list, renames the group and adds a
// description, so one step covers a scalar update and a nested-element add.
func configThreeCriteriaRenamed(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_smart_mobile_device_group" "test" {
			name        = %q
			description = "Managed by the acceptance suite"

			criteria = [
				{
					name        = "Model"
					search_type = "like"
					value       = "iPad"
				},
				{
					name        = "Display Name"
					search_type = "is not"
					value       = "tf-acc-placeholder"
					and_or      = "and"
				},
				{
					name        = "Supervised"
					search_type = "is"
					value       = "true"
					and_or      = "or"
				},
			]
		}
	`, name)
}

// configOneCriterion shrinks the list back to one, which is the nested-element
// remove half of the round-trip.
func configOneCriterion(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_smart_mobile_device_group" "test" {
			name        = %q
			description = "Managed by the acceptance suite"

			criteria = [
				{
					name        = "Model"
					search_type = "like"
					value       = "iPhone"
				},
			]
		}
	`, name)
}

// configNoCriteria clears the list entirely. The write full-replaces the
// criteria array, so an empty list is what removes every criterion.
func configNoCriteria(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_smart_mobile_device_group" "test" {
			name        = %q
			description = "Managed by the acceptance suite"
			criteria    = []
		}
	`, name)
}

// TestAccResource_ProSmartMobileDeviceGroup_Basic is the update round-trip:
// create, grow the criteria and edit both scalars, shrink the criteria, clear
// them, then import.
func TestAccResource_ProSmartMobileDeviceGroup_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := "tf-acc-pro-smdg-" + suffix
	renamed := "tf-acc-pro-smdg-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configTwoCriteria(original),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceAddress, "id"),
					// The platform identifier comes back from the create itself,
					// so it is known without a bridging read.
					resource.TestCheckResourceAttrSet(resourceAddress, "platform_id"),
					resource.TestCheckResourceAttr(resourceAddress, "name", original),
					resource.TestCheckResourceAttr(resourceAddress, "site_id", "-1"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.#", "2"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.name", "Model"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.priority", "0"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.1.priority", "1"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.1.search_type", "is not"),
				),
			},
			{
				Config: configThreeCriteriaRenamed(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "name", renamed),
					resource.TestCheckResourceAttr(resourceAddress, "description", "Managed by the acceptance suite"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.#", "3"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.2.name", "Supervised"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.2.priority", "2"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.2.and_or", "or"),
				),
			},
			{
				Config: configOneCriterion(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "criteria.#", "1"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.value", "iPhone"),
				),
			},
			{
				Config: configNoCriteria(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "criteria.#", "0"),
				),
			},
			{
				ResourceName:      resourceAddress,
				ImportState:       true,
				ImportStateVerify: true,
				// timeouts is configuration-only and is never returned by a read.
				// platform_id is deliberately NOT ignored: resolving it on import
				// is the behaviour the attribute exists for, so a mismatch here is
				// a real defect rather than harness noise.
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProSmartMobileDeviceGroup_Site covers the one thing this
// construct adds over jamfplatform_device_group. The site is created in the same
// configuration so the run needs no pre-existing tenant object, and the group
// depends on it implicitly through the identifier reference.
func TestAccResource_ProSmartMobileDeviceGroup_Site(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	groupName := "tf-acc-pro-smdg-site-" + suffix
	siteName := "tf-acc-pro-smdg-site-fixture-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_site" "fixture" {
						name = %q
					}

					resource "jamfplatform_pro_smart_mobile_device_group" "test" {
						name    = %q
						site_id = jamfplatform_pro_site.fixture.id

						criteria = [
							{
								name        = "Model"
								search_type = "like"
								value       = "iPad"
							},
						]
					}
				`, siteName, groupName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(resourceAddress, "site_id", "jamfplatform_pro_site.fixture", "id"),
				),
			},
			{
				ResourceName:            resourceAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProSmartMobileDeviceGroup_RejectsANonPositionalPriority covers
// the declared criterion-priority validator. Jamf Pro accepts nothing but
// 0..n-1 in order, so the provider refuses a disagreeing value at plan rather
// than overwriting it during apply.
func TestAccResource_ProSmartMobileDeviceGroup_RejectsANonPositionalPriority(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-priority-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_smart_mobile_device_group" "test" {
						name = %q

						criteria = [
							{
								name        = "Model"
								search_type = "like"
								value       = "iPad"
								priority    = 5
							},
						]
					}
				`, name),
				// Terraform wraps diagnostic output at roughly 80 columns, so the
				// expectation anchors on tokens carrying no internal whitespace.
				ExpectError: regexp.MustCompile(`priority`),
			},
		},
	})
}

// TestAccResource_ProSmartMobileDeviceGroup_RejectsAComputerOnlyDirectoryCriterion
// covers the per-object-class directory-service group allowlist. Mobile accepts
// four of the five directory criteria; this endpoint refuses the computer-only
// one, and the provider refuses it at plan naming the ones that work.
func TestAccResource_ProSmartMobileDeviceGroup_RejectsAComputerOnlyDirectoryCriterion(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-dsgroup-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_smart_mobile_device_group" "test" {
						name = %q

						criteria = [
							{
								name        = "User last logged in - Computer directory service group"
								search_type = "member of"
								value       = "tf-acc-placeholder-group"
							},
						]
					}
				`, name),
				ExpectError: regexp.MustCompile(`directory`),
			},
		},
	})
}

// TestAccResource_ProSmartMobileDeviceGroup_Disappears is the drift-recovery
// case: a group deleted outside Terraform must be dropped from state and
// planned for recreation rather than failing the refresh.
func TestAccResource_ProSmartMobileDeviceGroup_Disappears(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-gone-" + suffix

	var groupID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configTwoCriteria(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources[resourceAddress]
						if !ok {
							return fmt.Errorf("%s is not in state", resourceAddress)
						}
						groupID = rs.Primary.ID
						return nil
					},
				),
			},
			{
				PreConfig: func() {
					client := pro.New(testhelpers.NewAcceptanceClient(t))
					if err := client.DeleteSmartMobileDeviceGroupV2(context.Background(), groupID); err != nil {
						t.Fatalf("deleting group %s out of band: %v", groupID, err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccDataSource_ProSmartMobileDeviceGroup_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-ds-id-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configTwoCriteria(name) + `
					data "jamfplatform_pro_smart_mobile_device_group" "lookup" {
						id = jamfplatform_pro_smart_mobile_device_group.test.id
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_smart_mobile_device_group.lookup", "name", resourceAddress, "name"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_smart_mobile_device_group.lookup", "platform_id", resourceAddress, "platform_id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_smart_mobile_device_group.lookup", "criteria.#", "2"),
				),
			},
		},
	})
}

func TestAccDataSource_ProSmartMobileDeviceGroup_ByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-ds-name-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configTwoCriteria(name) + `
					data "jamfplatform_pro_smart_mobile_device_group" "lookup" {
						name = jamfplatform_pro_smart_mobile_device_group.test.name
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_smart_mobile_device_group.lookup", "id", resourceAddress, "id"),
				),
			},
		},
	})
}

// TestAccDataSource_ProSmartMobileDeviceGroup_RejectsBothSelectors covers the
// ExactlyOneOf validator. Both summaries the framework emits for it share this
// detail sentence, so the expectation matches that rather than either summary.
func TestAccDataSource_ProSmartMobileDeviceGroup_RejectsBothSelectors(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_pro_smart_mobile_device_group" "lookup" {
						id   = "1"
						name = "tf-acc-placeholder"
					}
				`,
				ExpectError: regexp.MustCompile(`Exactly one of these attributes must be configured`),
			},
		},
	})
}

func TestAccDataSource_ProSmartMobileDeviceGroups_FilterByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-plural-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configTwoCriteria(name) + fmt.Sprintf(`
					data "jamfplatform_pro_smart_mobile_device_groups" "lookup" {
						filter = [
							{
								selector = "groupName"
								argument = %q
							},
						]

						depends_on = [jamfplatform_pro_smart_mobile_device_group.test]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_smart_mobile_device_groups.lookup", "smart_mobile_device_groups.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_smart_mobile_device_groups.lookup", "smart_mobile_device_groups.0.name", name),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_smart_mobile_device_groups.lookup", "smart_mobile_device_groups.0.site_id", "-1"),
					// The collection read reports a membership count, which the
					// resource deliberately does not carry.
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_smart_mobile_device_groups.lookup", "smart_mobile_device_groups.0.member_count"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_smart_mobile_device_groups.lookup", "smart_mobile_device_groups.0.platform_id"),
				),
			},
		},
	})
}

// TestAccListResource_ProSmartMobileDeviceGroup_Basic exercises the list
// resource through the terraform query workflow, with the resource body
// included so the per-item read that hydrates the criteria is covered too.
func TestAccListResource_ProSmartMobileDeviceGroup_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configTwoCriteria(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceAddress, "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_smart_mobile_device_group" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = [
								{
									selector = "groupName"
									argument = %q
								},
							]
						}
					}
				`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_smart_mobile_device_group.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_smart_mobile_device_group.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("site_id"), KnownValue: knownvalue.StringExact("-1")},
							{Path: tfjsonpath.New("criteria").AtSliceIndex(0).AtMapKey("name"), KnownValue: knownvalue.StringExact("Model")},
						},
					),
				},
			},
		},
	})
}

// mintRefusedCriterionFixture creates a mobile device extension attribute and
// returns its name, which is the criterion half of a pairing Jamf Pro's group
// endpoint refuses.
//
// The fixture is minted rather than discovered. Reusing whatever extension
// attribute a tenant happens to hold makes the test's subject vary per estate,
// and an estate with none at all would leave the group carrying a criterion that
// names nothing — which the endpoint refuses for a different reason, so the test
// would pass while proving nothing. Minting also means the criterion's name is
// known to this test, so the assertions can name it.
//
// A tenant that will not mint one skips. There is nothing to infer from the
// refusal in that case: a missing privilege on the extension attribute endpoints
// and a fixed Jamf Pro defect are indistinguishable from here.
//
// It deliberately does not assert that the pairing is still refused. That is the
// canary's job, in internal/common/progroups, which fails when the defect is
// fixed. This test drives the write end to end and passes either way, which is
// the point: an apply that succeeds carrying this criterion is the proof the
// fallback works, and the day the endpoint accepts it the same apply succeeds on
// the ordinary path.
func mintRefusedCriterionFixture(t *testing.T, suffix string) string {
	t.Helper()

	client := pro.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	name := "tf-acc-smdg-fallback-" + suffix

	created, err := client.CreateMobileDeviceExtensionAttributeV1(ctx, &pro.MobileDeviceExtensionAttributes{
		Name:                 name,
		DataType:             pro.MobileDeviceExtensionAttributesDataTypeString,
		InputType:            pro.MobileDeviceExtensionAttributesInputTypeText,
		InventoryDisplayType: pro.MobileDeviceExtensionAttributesInventoryDisplayTypeExtensionAttributes,
	})
	if err != nil {
		t.Skipf("the tenant would not mint a mobile device extension attribute to build the criterion from: %v", err)
	}
	if created == nil || created.ID == "" {
		t.Skip("the tenant minted a mobile device extension attribute without reporting an identifier, so it cannot be cleaned up again")
	}

	t.Cleanup(func() {
		if err := client.DeleteMobileDeviceExtensionAttributeV1(ctx, created.ID); err != nil {
			t.Logf("cleaning up mobile device extension attribute %s: %v", created.ID, err)
		}
	})
	return name
}

// configRefusedOperator pairs the minted extension attribute with the operator
// the group endpoint refuses for it.
func configRefusedOperator(groupName, attributeName, description, value string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_smart_mobile_device_group" "test" {
			name        = %q
			description = %q

			criteria = [
				{
					name        = %q
					search_type = "has"
					value       = %q
				},
			]
		}
	`, groupName, description, attributeName, value)
}

// TestAccResource_ProSmartMobileDeviceGroup_FallsBackForARefusedOperator drives
// the second write path end to end.
//
// Every other test in this package takes the ordinary path and would pass with
// the fallback deleted, so this is the only coverage of it. Four steps cover the
// four things that can go wrong, and the third is the one worth reading twice:
//
//	create      the group and set a description the older interface cannot carry
//	describe    change only the description, which is the separate write
//	recriterion change only a criterion, which must leave the description alone
//	import      read the group back through the ordinary path
//
// The third step is where a description that was silently dropped would show.
// The older interface has no description field and preserves what it cannot
// express, so a criteria-only edit must leave the stored description standing —
// and the state check for it fails if a future edit starts sending an empty
// description alongside the criteria.
func TestAccResource_ProSmartMobileDeviceGroup_FallsBackForARefusedOperator(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	attributeName := mintRefusedCriterionFixture(t, suffix)
	groupName := "tf-acc-pro-smdg-fallback-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configRefusedOperator(groupName, attributeName, "Saved the older way", "urgent"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceAddress, "id"),
					resource.TestCheckResourceAttrSet(resourceAddress, "platform_id"),
					resource.TestCheckResourceAttr(resourceAddress, "name", groupName),
					resource.TestCheckResourceAttr(resourceAddress, "description", "Saved the older way"),
					resource.TestCheckResourceAttr(resourceAddress, "site_id", "-1"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.#", "1"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.name", attributeName),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.search_type", "has"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.value", "urgent"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.priority", "0"),
				),
			},
			{
				Config: configRefusedOperator(groupName, attributeName, "Renamed the note", "urgent"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "description", "Renamed the note"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.value", "urgent"),
				),
			},
			{
				Config: configRefusedOperator(groupName, attributeName, "Renamed the note", "retired"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.value", "retired"),
					resource.TestCheckResourceAttr(resourceAddress, "description", "Renamed the note"),
				),
			},
			{
				ResourceName:            resourceAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}
