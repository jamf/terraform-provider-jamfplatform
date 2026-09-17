// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// These tests talk to the Jamf Pro /pro/v2 mobile device group endpoints.
//
// Two of them need mobile devices that already exist in the tenant, because a
// static group's membership can only name real devices and the suite cannot
// manufacture one. Those tests skip when the tenant holds too few, which is a
// legitimate environment rather than a failure — see TESTING.md.
package static_mobile_device_group_test

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
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

const resourceAddress = "jamfplatform_pro_static_mobile_device_group.test"

// testAccCheckStaticMobileDeviceGroupDestroy verifies every group the test made
// is gone.
func testAccCheckStaticMobileDeviceGroupDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_static_mobile_device_group" {
				continue
			}
			_, err := c.GetStaticMobileDeviceGroupV2(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro static mobile device group %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro static mobile device group %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// requireMobileDeviceIDs returns the Jamf Pro identifiers of the first want
// mobile devices in the tenant, skipping when there are not enough. Membership
// can only name devices that exist, and inventing one is refused.
// requireMobileDeviceIDs mints the mobile devices a membership test needs and
// returns their Jamf Pro identifiers.
//
// It mints rather than borrowing the tenant's inventory for the same reason the
// computer suite does: the estate the acceptance lanes run against holds no
// devices, so listing existing ones and skipping would leave every membership
// assertion here passing without running.
//
// These records stay unmanaged, unlike the computer fixtures. A static mobile
// device group accepts an unmanaged device, and the write that would change that
// answers 500. testhelpers.MintMobileDeviceFixture registers its own cleanup.
func requireMobileDeviceIDs(t *testing.T, want int) []string {
	t.Helper()
	ids := make([]string, 0, want)
	for range want {
		ids = append(ids, testhelpers.MintMobileDeviceFixture(t, "smdg"))
	}
	return ids
}

// checkLiveMembership asserts the group's membership as Jamf Pro holds it.
//
// It is the only honest proof that a membership delta was computed correctly.
// Terraform state after an update echoes the plan, so a write that sent the
// additions and forgot the removals would look right in state and leave the
// removed devices in the group. Reading the server's copy cannot be fooled that
// way.
func checkLiveMembership(t *testing.T, want ...string) resource.TestCheckFunc {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	return testhelpers.CheckLiveObject(
		resourceAddress,
		func(ctx context.Context, id string) ([]string, error) {
			devices, err := c.ListStaticMobileDeviceGroupMembershipV2(ctx, id, nil, "")
			if err != nil {
				return nil, err
			}
			got := make([]string, 0, len(devices))
			for _, d := range devices {
				got = append(got, d.MobileDeviceID)
			}
			slices.Sort(got)
			return got, nil
		},
		func(got []string) error {
			expected := slices.Clone(want)
			slices.Sort(expected)
			if !slices.Equal(got, expected) {
				return fmt.Errorf("membership: want %v, got %v", expected, got)
			}
			return nil
		},
	)
}

// importVerifyIgnore lists the attributes an import cannot be expected to match.
//
// timeouts is never on the wire. assigned_mobile_device_ids is the opt-out
// membership set: an import hydrates it from the group's real members, so it
// differs from a configuration that declared a smaller set or none at all.
// platform_id joins the list only when the integration cannot read the group
// list an import resolves it from, since a create learns it from Jamf Pro
// directly and would then have a value the import cannot recover.
func importVerifyIgnore(t *testing.T) []string {
	t.Helper()
	ignore := []string{"timeouts", "assigned_mobile_device_ids"}
	if !testhelpers.ProbeProGroupsReadable(t) {
		ignore = append(ignore, "platform_id")
	}
	return ignore
}

func configMinimal(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_static_mobile_device_group" "test" {
			name = %q
		}
	`, name)
}

func configDescribed(name, description string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_static_mobile_device_group" "test" {
			name        = %q
			description = %q
		}
	`, name, description)
}

func configWithMembers(name string, ids []string) string {
	var quoted strings.Builder
	for i, id := range ids {
		if i > 0 {
			quoted.WriteString(", ")
		}
		quoted.WriteString(fmt.Sprintf("%q", id))
	}
	return fmt.Sprintf(`
		resource "jamfplatform_pro_static_mobile_device_group" "test" {
			name                       = %q
			assigned_mobile_device_ids = [%s]
		}
	`, name, quoted.String())
}

func configInSite(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_site" "test" {
			name = "%s-site"
		}

		resource "jamfplatform_pro_static_mobile_device_group" "test" {
			name    = %q
			site_id = jamfplatform_pro_site.test.id
		}
	`, name, name)
}

// TestAccResource_ProStaticMobileDeviceGroup_Basic covers create, an in-place
// rename, an import round-trip on a group whose membership the configuration
// never mentions, and then clearing the description.
//
// The description steps matter more than they look: the scalars full-replace, so
// a write that stopped emitting the description would empty it, and dropping the
// attribute from the configuration must leave the stored value alone because it
// is Optional+Computed.
//
// Clearing it comes last on purpose. An authored empty string stays an empty
// string in state, while the same empty echo read with no prior state reads as
// null, so an import verified against that step would compare a stored "" with
// an absent attribute and fail on every run. An import cannot know whether an
// empty description was authored, which is provider behaviour rather than a
// defect, so the import step runs while the description still has a value.
func TestAccResource_ProStaticMobileDeviceGroup_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := "tf-acc-pro-smdg-" + suffix
	renamed := "tf-acc-pro-smdg-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticMobileDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(original),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceAddress, "id"),
					resource.TestCheckResourceAttrSet(resourceAddress, "platform_id"),
					resource.TestCheckResourceAttr(resourceAddress, "name", original),
					resource.TestCheckResourceAttr(resourceAddress, "site_id", "-1"),
					resource.TestCheckResourceAttr(resourceAddress, "member_count", "0"),
					resource.TestCheckNoResourceAttr(resourceAddress, "assigned_mobile_device_ids.#"),
				),
			},
			{
				Config: configDescribed(renamed, "Spares desk"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "name", renamed),
					resource.TestCheckResourceAttr(resourceAddress, "description", "Spares desk"),
				),
			},
			{
				ResourceName:            resourceAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: importVerifyIgnore(t),
			},
			{
				Config: configDescribed(renamed, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "description", ""),
				),
			},
		},
	})
}

// TestAccResource_ProStaticMobileDeviceGroup_Membership is the test the whole
// package exists for: the endpoint merges the membership it is given rather than
// replacing it, so each step proves the provider computed the right difference.
//
// The sequence is deliberate. Two members, then one — which only shrinks if the
// update sent a removal, since sending the remaining addition alone would leave
// both in the group. Then the empty set, which only clears if the update sent
// removals, because an empty body is a no-op here. Then the attribute dropped
// entirely, which must leave the group exactly as it was.
func TestAccResource_ProStaticMobileDeviceGroup_Membership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ids := requireMobileDeviceIDs(t, 2)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-members-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticMobileDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configWithMembers(name, ids),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "assigned_mobile_device_ids.#", "2"),
					resource.TestCheckTypeSetElemAttr(resourceAddress, "assigned_mobile_device_ids.*", ids[0]),
					resource.TestCheckTypeSetElemAttr(resourceAddress, "assigned_mobile_device_ids.*", ids[1]),
					resource.TestCheckResourceAttr(resourceAddress, "member_count", "2"),
					checkLiveMembership(t, ids[0], ids[1]),
				),
			},
			{
				Config: configWithMembers(name, ids[:1]),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "assigned_mobile_device_ids.#", "1"),
					resource.TestCheckTypeSetElemAttr(resourceAddress, "assigned_mobile_device_ids.*", ids[0]),
					resource.TestCheckResourceAttr(resourceAddress, "member_count", "1"),
					checkLiveMembership(t, ids[0]),
				),
			},
			{
				Config: configWithMembers(name, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "assigned_mobile_device_ids.#", "0"),
					resource.TestCheckResourceAttr(resourceAddress, "member_count", "0"),
					checkLiveMembership(t),
				),
			},
			{
				Config: configWithMembers(name, ids[:1]),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "assigned_mobile_device_ids.#", "1"),
					resource.TestCheckResourceAttr(resourceAddress, "member_count", "1"),
					checkLiveMembership(t, ids[0]),
				),
			},
			{
				Config: configMinimal(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(resourceAddress, "assigned_mobile_device_ids.#"),
					resource.TestCheckResourceAttr(resourceAddress, "member_count", "1"),
					checkLiveMembership(t, ids[0]),
				),
			},
			{
				ResourceName:            resourceAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: importVerifyIgnore(t),
			},
		},
	})
}

// TestAccResource_ProStaticMobileDeviceGroup_InSite covers the reason this
// resource exists at all: a group that belongs to a Jamf Pro site, which
// jamfplatform_device_group cannot express.
//
// Membership is left out on purpose. A group in a site accepts only mobile
// devices assigned to that same site, and the suite has no way to move a device
// into the site it just created.
func TestAccResource_ProStaticMobileDeviceGroup_InSite(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-site-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticMobileDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configInSite(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(resourceAddress, "site_id", "jamfplatform_pro_site.test", "id"),
					resource.TestCheckResourceAttr(resourceAddress, "name", name),
				),
			},
			{
				ResourceName:            resourceAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: importVerifyIgnore(t),
			},
		},
	})
}

// TestAccResource_ProStaticMobileDeviceGroup_RejectsAnEmptyName pins the one
// attribute validator the schema declares.
func TestAccResource_ProStaticMobileDeviceGroup_RejectsAnEmptyName(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      configMinimal(""),
				ExpectError: regexp.MustCompile("Invalid Attribute Value Length"),
			},
		},
	})
}

func TestAccDataSource_ProStaticMobileDeviceGroup_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-ds-id-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticMobileDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_static_mobile_device_group" "test" {
						name        = %q
						description = "Lookup fixture"
					}

					data "jamfplatform_pro_static_mobile_device_group" "lookup" {
						id = jamfplatform_pro_static_mobile_device_group.test.id
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_static_mobile_device_group.lookup", "name", resourceAddress, "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_mobile_device_group.lookup", "description", "Lookup fixture"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_mobile_device_group.lookup", "member_count", "0"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_mobile_device_group.lookup", "assigned_mobile_device_ids.#", "0"),
				),
			},
		},
	})
}

func TestAccDataSource_ProStaticMobileDeviceGroup_ByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-ds-name-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticMobileDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_static_mobile_device_group" "test" {
						name = %q
					}

					data "jamfplatform_pro_static_mobile_device_group" "lookup" {
						name = jamfplatform_pro_static_mobile_device_group.test.name
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_static_mobile_device_group.lookup", "id", resourceAddress, "id"),
				),
			},
		},
	})
}

// TestAccDataSource_ProStaticMobileDeviceGroup_RequiresOneSelector covers the
// ExactlyOneOf validator from both sides. The two cases report different
// summaries, so the regex matches the detail they share.
func TestAccDataSource_ProStaticMobileDeviceGroup_RequiresOneSelector(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      `data "jamfplatform_pro_static_mobile_device_group" "lookup" {}`,
				ExpectError: regexp.MustCompile("Exactly one of these attributes must be configured"),
			},
			{
				Config: `
					data "jamfplatform_pro_static_mobile_device_group" "lookup" {
						id   = "1"
						name = "anything"
					}
				`,
				ExpectError: regexp.MustCompile("Exactly one of these attributes must be configured"),
			},
		},
	})
}

func TestAccDataSource_ProStaticMobileDeviceGroups_FilterByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-plural-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticMobileDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_static_mobile_device_group" "test" {
						name = %q
					}

					data "jamfplatform_pro_static_mobile_device_groups" "lookup" {
						filter = [
							{
								selector = "groupName"
								argument = jamfplatform_pro_static_mobile_device_group.test.name
							}
						]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_mobile_device_groups.lookup", "static_mobile_device_groups.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_mobile_device_groups.lookup", "static_mobile_device_groups.0.name", name),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_mobile_device_groups.lookup", "static_mobile_device_groups.0.member_count", "0"),
				),
			},
		},
	})
}

// TestAccDataSource_ProStaticMobileDeviceGroups_RejectsAnUnknownSelector pins the
// selector allow-list, which is validated at plan time because the search names
// only three filterable fields.
func TestAccDataSource_ProStaticMobileDeviceGroups_RejectsAnUnknownSelector(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_pro_static_mobile_device_groups" "lookup" {
						filter = [
							{
								selector = "groupDescription"
								argument = "anything"
							}
						]
					}
				`,
				ExpectError: regexp.MustCompile("Invalid Attribute Value Match"),
			},
		},
	})
}

// TestAccListResource_ProStaticMobileDeviceGroup_Basic exercises the list
// resource through the query workflow, including the hydrated resource body.
//
// The membership assertion is the point: the list never fetches membership, so a
// generated configuration must leave the attribute undeclared rather than claim
// the group holds nobody.
func TestAccListResource_ProStaticMobileDeviceGroup_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticMobileDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceAddress, "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_static_mobile_device_group" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = [
								{
									selector = "groupName"
									argument = %q
								}
							]
						}
					}
				`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_static_mobile_device_group.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_static_mobile_device_group.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("site_id"), KnownValue: knownvalue.StringExact("-1")},
							{Path: tfjsonpath.New("member_count"), KnownValue: knownvalue.Int64Exact(0)},
							{Path: tfjsonpath.New("assigned_mobile_device_ids"), KnownValue: knownvalue.Null()},
						},
					),
				},
			},
		},
	})
}

// TestAccResource_ProStaticMobileDeviceGroup_UnmanagedMembershipSurvivesAnUpdate
// is the proof of the opt-out contract: leave assigned_mobile_device_ids out and
// Terraform never touches who is in the group, not even while changing something
// else about it.
//
// It has to assign a device OUTSIDE Terraform to mean anything. A test that only
// omitted the attribute would pass against an implementation that cleared
// membership on every update, because there would be nothing in the group to
// lose. So the first step creates the group with no membership declared, puts a
// device in it directly, and the second step renames the group through Terraform
// and asserts the device is still there — and still absent from state, because an
// unmanaged set stays null rather than adopting what it found.
//
// This is the mobile counterpart of the computer group's test of the same name,
// and the write it exercises is the opposite shape: the mobile endpoint
// delta-merges its assignments and treats an empty list as a no-op, where the
// computer endpoint replaces wholesale and treats an empty list as a clear. Both
// have to reach "change nothing", by different routes.
func TestAccResource_ProStaticMobileDeviceGroup_UnmanagedMembershipSurvivesAnUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ids := requireMobileDeviceIDs(t, 1)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-smdg-unmanaged-" + suffix
	renamed := "tf-acc-pro-smdg-unmanaged-renamed-" + suffix

	assignOutOfBand := func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceAddress]
		if !ok {
			return fmt.Errorf("%s not found in state", resourceAddress)
		}
		selected := true
		site := "-1"
		assignments := []pro.Assignment{{MobileDeviceID: &ids[0], Selected: &selected}}
		_, err := pro.New(testhelpers.NewAcceptanceClient(t)).PatchStaticMobileDeviceGroupV2(
			context.Background(), rs.Primary.ID,
			&pro.StaticGroupAssignment{GroupName: name, SiteID: &site, Assignments: &assignments},
		)
		if err != nil {
			return fmt.Errorf("assigning a mobile device to group %s outside Terraform: %w", rs.Primary.ID, err)
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticMobileDeviceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceAddress, "id"),
					assignOutOfBand,
					checkLiveMembership(t, ids[0]),
				),
			},
			{
				Config: configDescribed(renamed, "Renamed with members left alone"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "name", renamed),
					checkLiveMembership(t, ids[0]),
					resource.TestCheckNoResourceAttr(resourceAddress, "assigned_mobile_device_ids.#"),
				),
			},
		},
	})
}
