// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file write to the Jamf Pro v3 static computer group endpoints
// and read membership from the classic computer group endpoint. The membership
// tests need real computers on the tenant and skip when there are not enough:
// a static group can only hold computers that already exist, and nothing here
// can manufacture one.

package static_computer_group_test

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const staticComputerGroupAddr = "jamfplatform_pro_static_computer_group.test"

// testAccCheckStaticComputerGroupDestroy asserts every group the test created is
// gone.
func testAccCheckStaticComputerGroupDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_static_computer_group" {
				continue
			}
			_, err := client.GetStaticComputerGroupV3(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("checking Jamf Pro static computer group %s: %w", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro static computer group %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// acceptanceComputerIDs returns the first want computer identifiers the tenant
// reports, and skips when it has fewer. Absence of tenant data is a skip, never
// an inference.
// acceptanceComputerIDs mints the computers a membership test needs and returns
// their Jamf Pro identifiers.
//
// It mints rather than borrowing the tenant's inventory because the estate the
// acceptance lanes run against holds no computers, so a helper that listed
// existing ones and skipped would make every membership assertion here — the
// opt-out guarantee most of all — pass without running. Minting also survives
// the estate being cleaned out, which a seeded fixture would not.
//
// Each record is minted managed, because a static computer group refuses an
// unmanaged computer with the same error it gives for one that does not exist.
// testhelpers.MintComputerFixture registers its own cleanup.
func acceptanceComputerIDs(t *testing.T, want int) []string {
	t.Helper()
	ids := make([]string, 0, want)
	for range want {
		ids = append(ids, testhelpers.MintComputerFixture(t, "scg"))
	}
	return ids
}

// checkMembership reads the group's members through the classic endpoint, which
// is the only place Jamf Pro reports them, and compares them with want.
func checkMembership(t *testing.T, want []string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[staticComputerGroupAddr]
		if !ok {
			return fmt.Errorf("%s not found in state", staticComputerGroupAddr)
		}
		group, err := proclassic.New(testhelpers.NewAcceptanceClient(t)).GetComputerGroupByID(context.Background(), rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("reading the membership of group %s: %w", rs.Primary.ID, err)
		}

		var got []string
		if group != nil && group.Computers != nil && group.Computers.Computer != nil {
			for _, computer := range *group.Computers.Computer {
				if computer.ID == nil {
					continue
				}
				got = append(got, strconv.Itoa(*computer.ID))
			}
		}
		slices.Sort(got)
		expected := slices.Clone(want)
		slices.Sort(expected)
		if !slices.Equal(got, expected) {
			return fmt.Errorf("group %s holds %v, want %v", rs.Primary.ID, got, expected)
		}
		return nil
	}
}

func configMinimal(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_static_computer_group" "test" {
  name = %q
}
`, name)
}

func configDescribed(name, description string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_static_computer_group" "test" {
  name        = %q
  description = %q
}
`, name, description)
}

func configWithMembers(name string, ids []string) string {
	quoted := make([]string, 0, len(ids))
	for _, id := range ids {
		quoted = append(quoted, strconv.Quote(id))
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_static_computer_group" "test" {
  name                  = %q
  assigned_computer_ids = [%s]
}
`, name, joinComma(quoted))
}

func configWithNamedMembers(name, description string, ids []string) string {
	quoted := make([]string, 0, len(ids))
	for _, id := range ids {
		quoted = append(quoted, strconv.Quote(id))
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_static_computer_group" "test" {
  name                  = %q
  description           = %q
  assigned_computer_ids = [%s]
}
`, name, description, joinComma(quoted))
}

func configSiteScoped(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_site" "test" {
  name = "%s-site"
}

resource "jamfplatform_pro_static_computer_group" "test" {
  name    = %q
  site_id = jamfplatform_pro_site.test.id
}
`, name, name)
}

func joinComma(values []string) string {
	var out strings.Builder
	for i, v := range values {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(v)
	}
	return out.String()
}

// TestAccResource_ProStaticComputerGroup_Basic covers the scalars with no
// membership declared: create, a rename, a description edit, clearing the note
// with an empty string, and an import round-trip. An undeclared membership must
// stay out of state, so import lands null and matches.
//
// The empty-string step is the one that earns its place. The schema tells an
// operator to write one to clear the note, every write is a full replace so the
// read echoes the empty string back, and a state builder that collapsed that to
// null would abort the apply with Terraform Core's inconsistent-result error
// against a plan holding "". Nothing short of an apply catches it.
func TestAccResource_ProStaticComputerGroup_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := "tf-acc-pro-scg-" + suffix
	renamed := "tf-acc-pro-scg-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(original),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(staticComputerGroupAddr, "id"),
					resource.TestCheckResourceAttrSet(staticComputerGroupAddr, "platform_id"),
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "name", original),
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "site_id", "-1"),
					resource.TestCheckNoResourceAttr(staticComputerGroupAddr, "assigned_computer_ids.#"),
				),
			},
			{
				Config: configDescribed(renamed, "Second floor"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "name", renamed),
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "description", "Second floor"),
					// The update carries the whole membership whether or not the
					// configuration manages it, and an unmanaged group must come
					// through it empty rather than broken.
					checkMembership(t, nil),
				),
			},
			{
				Config: configDescribed(renamed, "Third floor"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "description", "Third floor"),
				),
			},
			{
				Config: configDescribed(renamed, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "description", ""),
				),
			},
			{
				// The note is cleared, so an import of it lands null while the
				// configuration above holds "". Re-declaring it settles the two
				// before the import step compares them.
				Config: configDescribed(renamed, "Third floor"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "description", "Third floor"),
				),
			},
			{
				ResourceName:      staticComputerGroupAddr,
				ImportState:       true,
				ImportStateVerify: true,
				// timeouts is provider-side configuration with no server
				// representation, so it never survives an import.
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProStaticComputerGroup_Membership covers the membership
// round-trip the whole package exists for: grow, shrink, clear, repopulate, and
// an import that adopts what Jamf Pro reports.
func TestAccResource_ProStaticComputerGroup_Membership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ids := acceptanceComputerIDs(t, 2)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-members-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configWithMembers(name, ids),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "assigned_computer_ids.#", "2"),
					checkMembership(t, ids),
				),
			},
			{
				// A populated list is a full replace, so dropping one member
				// removes it rather than leaving it in place.
				Config: configWithMembers(name, ids[:1]),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "assigned_computer_ids.#", "1"),
					checkMembership(t, ids[:1]),
				),
			},
			{
				// An explicitly empty list empties the group. Omitting the
				// attribute would mean something else entirely, which the
				// unmanaged test covers.
				Config: configWithMembers(name, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "assigned_computer_ids.#", "0"),
					checkMembership(t, nil),
				),
			},
			{
				Config: configWithNamedMembers(name, "Repopulated", ids),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "assigned_computer_ids.#", "2"),
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "description", "Repopulated"),
					checkMembership(t, ids),
				),
			},
			{
				// Import adopts the membership Jamf Pro reports when there is
				// some, so this step compares it rather than ignoring it.
				ResourceName:            staticComputerGroupAddr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProStaticComputerGroup_UnmanagedMembershipSurvivesAnUpdate is
// the test for the read-merge-write in Update. Every update has to carry the
// whole membership, so a configuration that does not manage it must read the
// current members and send them back. Members are assigned outside Terraform
// here, because that is the only way to tell "sent them back" apart from "the
// group was empty anyway".
func TestAccResource_ProStaticComputerGroup_UnmanagedMembershipSurvivesAnUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ids := acceptanceComputerIDs(t, 1)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-unmanaged-" + suffix
	renamed := "tf-acc-pro-scg-unmanaged-renamed-" + suffix

	assignOutOfBand := func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[staticComputerGroupAddr]
		if !ok {
			return fmt.Errorf("%s not found in state", staticComputerGroupAddr)
		}
		assignments := slices.Clone(ids)
		_, err := pro.New(testhelpers.NewAcceptanceClient(t)).UpdateStaticComputerGroupV3(
			context.Background(), rs.Primary.ID,
			&pro.StaticComputerGroupAssignment{Name: name, Assignments: &assignments},
		)
		if err != nil {
			return fmt.Errorf("assigning a computer to group %s outside Terraform: %w", rs.Primary.ID, err)
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(staticComputerGroupAddr, "id"),
					assignOutOfBand,
					checkMembership(t, ids),
				),
			},
			{
				Config: configDescribed(renamed, "Renamed with members left alone"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(staticComputerGroupAddr, "name", renamed),
					// The members assigned outside Terraform are still there,
					// and they are still absent from state.
					checkMembership(t, ids),
					resource.TestCheckNoResourceAttr(staticComputerGroupAddr, "assigned_computer_ids.#"),
				),
			},
		},
	})
}

// TestAccResource_ProStaticComputerGroup_SiteScoped covers the one thing this
// construct adds over jamfplatform_device_group. No membership is declared: a
// group in a site accepts only computers assigned to that same site, and a
// freshly created site has none.
func TestAccResource_ProStaticComputerGroup_SiteScoped(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-site-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configSiteScoped(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(staticComputerGroupAddr, "site_id", "jamfplatform_pro_site.test", "id"),
					resource.TestCheckResourceAttrSet(staticComputerGroupAddr, "platform_id"),
				),
			},
			{
				ResourceName:            staticComputerGroupAddr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProStaticComputerGroup_RejectsAComputerOutsideTheSite covers
// the refusal a site makes possible. Jamf Pro reports a computer that does not
// exist and a computer outside the group's site identically, so the diagnostic
// names both and the test matches what they share.
// TestAccResource_ProStaticComputerGroup_RejectsAComputerOutsideTheSite proves
// the site rule: a group in a site holds only computers assigned to that site.
//
// The refusal is guaranteed for a non-obvious reason worth writing down, because
// the test otherwise looks as though it depends on the estate. The site it names
// is minted by this very configuration, so nothing has been assigned to it yet
// and NO computer can be in it — the minted fixture included. That is what makes
// the rejection certain rather than a happy accident of which computers the
// tenant holds.
func TestAccResource_ProStaticComputerGroup_RejectsAComputerOutsideTheSite(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ids := acceptanceComputerIDs(t, 1)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-sitereject-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jamfplatform_pro_site" "test" {
  name = "%s-site"
}

resource "jamfplatform_pro_static_computer_group" "test" {
  name                  = %q
  site_id               = jamfplatform_pro_site.test.id
  assigned_computer_ids = [%q]
}
`, name, name, ids[0]),
				// Anchored on no-space tokens: Terraform wraps error output at
				// roughly eighty columns.
				ExpectError: regexp.MustCompile(`cannot\s+be\s+a\s+member`),
			},
		},
	})
}

func TestAccDataSource_ProStaticComputerGroup_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-ds-id-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jamfplatform_pro_static_computer_group" "test" {
  name        = %q
  description = "Looked up by identifier"
}

data "jamfplatform_pro_static_computer_group" "lookup" {
  id = jamfplatform_pro_static_computer_group.test.id
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_computer_group.lookup", "name", name),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_computer_group.lookup", "description", "Looked up by identifier"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_computer_group.lookup", "assigned_computer_ids.#", "0"),
					resource.TestCheckResourceAttrPair(
						"data.jamfplatform_pro_static_computer_group.lookup", "platform_id",
						staticComputerGroupAddr, "platform_id",
					),
				),
			},
		},
	})
}

func TestAccDataSource_ProStaticComputerGroup_ByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-ds-name-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jamfplatform_pro_static_computer_group" "test" {
  name = %q
}

data "jamfplatform_pro_static_computer_group" "lookup" {
  name = jamfplatform_pro_static_computer_group.test.name
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"data.jamfplatform_pro_static_computer_group.lookup", "id",
						staticComputerGroupAddr, "id",
					),
				),
			},
		},
	})
}

// TestAccDataSource_ProStaticComputerGroup_SelectorIsExclusive covers the one
// declared cross-field rule in the package, in both directions. The assertion
// matches the shared detail rather than either summary: the framework prints
// "Invalid Attribute Combination" when both are set and "Missing Attribute
// Configuration" when neither is, so matching a summary would pass for the
// wrong reason on the other case.
func TestAccDataSource_ProStaticComputerGroup_SelectorIsExclusive(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-ds-excl-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "jamfplatform_pro_static_computer_group" "both" {
  id   = "1"
  name = %q
}
`, name),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Exactly\s+one\s+of\s+these\s+attributes\s+must\s+be\s+configured`),
			},
			{
				Config: `
data "jamfplatform_pro_static_computer_group" "neither" {
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Exactly\s+one\s+of\s+these\s+attributes\s+must\s+be\s+configured`),
			},
		},
	})
}

func TestAccDataSource_ProStaticComputerGroups_FilterByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-plural-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jamfplatform_pro_static_computer_group" "test" {
  name = %q
}

data "jamfplatform_pro_static_computer_groups" "lookup" {
  filter = [
    {
      selector = "name"
      argument = jamfplatform_pro_static_computer_group.test.name
    }
  ]
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_computer_groups.lookup", "static_computer_groups.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_computer_groups.lookup", "static_computer_groups.0.name", name),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_static_computer_groups.lookup", "static_computer_groups.0.member_count", "0"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_static_computer_groups.lookup", "static_computer_groups.0.platform_id"),
				),
			},
		},
	})
}

// TestAccListResource_ProStaticComputerGroup_Basic covers the list resource,
// which had no acceptance coverage while its mobile counterpart did.
//
// It asserts assigned_computer_ids comes back NULL rather than hydrated, which is
// the list resource's whole contract for an opt-out collection: a generated
// configuration must not adopt membership the operator never declared, or the
// first apply after generating one would take ownership of every member and the
// next change to the group would start removing them.
func TestAccListResource_ProStaticComputerGroup_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckStaticComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(staticComputerGroupAddr, "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_static_computer_group" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = [
								{
									selector = "name"
									argument = %q
								}
							]
						}
					}
				`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_static_computer_group.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_static_computer_group.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("site_id"), KnownValue: knownvalue.StringExact("-1")},
							{Path: tfjsonpath.New("assigned_computer_ids"), KnownValue: knownvalue.Null()},
						},
					),
				},
			},
		},
	})
}
