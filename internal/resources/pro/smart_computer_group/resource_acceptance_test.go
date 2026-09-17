// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file write smart computer groups on the shared acceptance
// tenant. Every write is bounded by the provider's five-minute create and
// update budgets rather than the house minute, because Jamf Pro evaluates the
// criteria as part of the write.
//
// Two coverage gaps are deliberate. There is no test for the identifier bridge
// failing on create, because the only way to produce it is to withhold a
// permission mid-apply. And there is no assertion on membership: Jamf Pro
// decides it from the criteria on its own schedule, so a group created seconds
// ago legitimately reports nobody.

package smart_computer_group_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
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
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/progroups"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const resourceAddress = "jamfplatform_pro_smart_computer_group.test"

// testAccCheckSmartComputerGroupDestroy verifies every group the test created
// is gone. The read is by the Jamf Pro identifier, which is the resource id.
func testAccCheckSmartComputerGroupDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_smart_computer_group" {
				continue
			}
			_, err := client.GetSmartComputerGroupV3(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro smart computer group %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro smart computer group %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func configTwoCriteria(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_smart_computer_group" "test" {
			name        = %q
			description = "tf-acc placeholder note"

			criteria = [
				{
					name        = "Operating System Version"
					search_type = "like"
					value       = "26."
				},
				{
					name        = "Model"
					search_type = "like"
					value       = "MacBook"
					and_or      = "or"
				},
			]
		}
	`, name)
}

func configThreeCriteriaRenamed(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_smart_computer_group" "test" {
			name        = %q
			description = "tf-acc placeholder note, edited"

			criteria = [
				{
					name                    = "Operating System Version"
					search_type             = "like"
					value                   = "26."
					has_opening_parenthesis = true
				},
				{
					name                    = "Model"
					search_type             = "like"
					value                   = "MacBook Pro"
					and_or                  = "or"
					has_closing_parenthesis = true
				},
				{
					name        = "Computer Name"
					search_type = "does not have"
					value       = "tf-acc-excluded"
					and_or      = "and"
				},
			]
		}
	`, name)
}

func configOneCriterion(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_smart_computer_group" "test" {
			name        = %q
			description = "tf-acc placeholder note, edited"

			criteria = [
				{
					name        = "Operating System Version"
					search_type = "like"
					value       = "26."
				},
			]
		}
	`, name)
}

// configNoCriteria drops the attribute entirely, which is how the resource
// expresses a group with no criteria. An empty list is refused at plan, because
// Jamf Pro reports such a group with no criteria at all and a configuration
// asking for an empty one would never settle.
func configNoCriteria(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_smart_computer_group" "test" {
			name        = %q
			description = "tf-acc placeholder note, edited"
		}
	`, name)
}

// TestAccResource_ProSmartComputerGroup_Lifecycle walks the full round trip:
// create with two criteria, grow to three while renaming, editing the note and
// adding the parenthesis flags, shrink back to one, then drop the criteria
// altogether. Every step re-reads the group, so the whole sequence exercises the
// read-after-write path the lossy update response makes necessary.
func TestAccResource_ProSmartComputerGroup_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := "tf-acc-pro-scg-" + suffix
	renamed := "tf-acc-pro-scg-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSmartComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configTwoCriteria(original),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceAddress, "id"),
					resource.TestCheckResourceAttrSet(resourceAddress, "platform_id"),
					resource.TestCheckResourceAttr(resourceAddress, "name", original),
					resource.TestCheckResourceAttr(resourceAddress, "description", "tf-acc placeholder note"),
					resource.TestCheckResourceAttr(resourceAddress, "site_id", "-1"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.#", "2"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.priority", "0"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.name", "Operating System Version"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.1.priority", "1"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.1.and_or", "or"),
				),
			},
			{
				Config: configThreeCriteriaRenamed(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "name", renamed),
					resource.TestCheckResourceAttr(resourceAddress, "description", "tf-acc placeholder note, edited"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.#", "3"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.has_opening_parenthesis", "true"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.1.has_closing_parenthesis", "true"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.1.value", "MacBook Pro"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.2.priority", "2"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.2.search_type", "does not have"),
				),
			},
			{
				Config: configOneCriterion(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "criteria.#", "1"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.name", "Operating System Version"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.has_opening_parenthesis", "false"),
				),
			},
			{
				Config: configNoCriteria(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "criteria.#", "0"),
					resource.TestCheckResourceAttr(resourceAddress, "name", renamed),
				),
			},
			{
				// timeouts never exists on the wire. Nothing else is ignored: a
				// smart group has no membership collection for the importer to
				// leave unmanaged, so every attribute in state is one Jamf Pro
				// reports, including both identifiers.
				ResourceName:            resourceAddress,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProSmartComputerGroup_ImportWithCriteria imports a group that
// still has its criteria, which is the case the lifecycle test's import step
// cannot cover: it runs after the criteria were dropped. It also pins that an
// import resolves platform_id, which no group response carries.
func TestAccResource_ProSmartComputerGroup_ImportWithCriteria(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-import-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSmartComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configTwoCriteria(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "criteria.#", "2"),
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

// TestAccResource_ProSmartComputerGroup_Site covers the one thing this resource
// adds over jamfplatform_device_group. The site is stood up as an in-config
// fixture so the apply orders it first, and the group is then moved off the site
// to prove the write survives both directions.
func TestAccResource_ProSmartComputerGroup_Site(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	groupName := "tf-acc-pro-scg-site-" + suffix
	siteName := "tf-acc-pro-scg-site-fixture-" + suffix

	sited := fmt.Sprintf(`
		resource "jamfplatform_pro_site" "fixture" {
			name = %q
		}

		resource "jamfplatform_pro_smart_computer_group" "test" {
			name    = %q
			site_id = jamfplatform_pro_site.fixture.id

			criteria = [
				{
					name        = "Operating System Version"
					search_type = "like"
					value       = "26."
				},
			]
		}
	`, siteName, groupName)

	unsited := fmt.Sprintf(`
		resource "jamfplatform_pro_site" "fixture" {
			name = %q
		}

		resource "jamfplatform_pro_smart_computer_group" "test" {
			name = %q

			criteria = [
				{
					name        = "Operating System Version"
					search_type = "like"
					value       = "26."
				},
			]
		}
	`, siteName, groupName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSmartComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: sited,
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
			{
				// site_id defaults to the no-site sentinel, so dropping the
				// attribute is a real write rather than a no-op: the endpoint
				// full-replaces and would clear the site either way.
				Config: unsited,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "site_id", "-1"),
				),
			},
		},
	})
}

// TestAccResource_ProSmartComputerGroup_PriorityMismatchRejected covers the
// declared cross-field validator. Jamf Pro requires the priorities to run from
// zero upwards in list order, so an authored value that disagrees is refused at
// plan rather than silently replaced, which would abort the apply as an
// inconsistent result.
func TestAccResource_ProSmartComputerGroup_PriorityMismatchRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-priority-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_smart_computer_group" "test" {
						name = %q

						criteria = [
							{
								priority    = 5
								name        = "Operating System Version"
								search_type = "like"
								value       = "26."
							},
						]
					}
				`, name),
				ExpectError: regexp.MustCompile(`priority`),
			},
		},
	})
}

// TestAccResource_ProSmartComputerGroup_EmptyCriteriaRejected pins the size
// validator. An empty list can never settle, because Jamf Pro reports a group
// with no criteria as having none at all rather than as having an empty set.
func TestAccResource_ProSmartComputerGroup_EmptyCriteriaRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-empty-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_smart_computer_group" "test" {
						name     = %q
						criteria = []
					}
				`, name),
				ExpectError: regexp.MustCompile(`at\s+least\s+1`),
			},
		},
	})
}

func TestAccDataSource_ProSmartComputerGroup_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-ds-id-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSmartComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configTwoCriteria(name) + `
					data "jamfplatform_pro_smart_computer_group" "lookup" {
						id = jamfplatform_pro_smart_computer_group.test.id
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_smart_computer_group.lookup", "name", resourceAddress, "name"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_smart_computer_group.lookup", "platform_id", resourceAddress, "platform_id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_smart_computer_group.lookup", "criteria.#", "2"),
				),
			},
		},
	})
}

func TestAccDataSource_ProSmartComputerGroup_ByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-ds-name-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSmartComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configOneCriterion(name) + `
					data "jamfplatform_pro_smart_computer_group" "lookup" {
						name = jamfplatform_pro_smart_computer_group.test.name
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_smart_computer_group.lookup", "id", resourceAddress, "id"),
				),
			},
		},
	})
}

// TestAccDataSource_ProSmartComputerGroup_SelectorRequired covers the
// exactly-one-of rule on both sides. The two cases report different summaries, so
// the assertion anchors on the shared detail, and on no-space tokens because
// Terraform wraps its error output.
func TestAccDataSource_ProSmartComputerGroup_SelectorRequired(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_pro_smart_computer_group" "lookup" {}
				`,
				ExpectError: regexp.MustCompile(`Exactly\s+one\s+of\s+these\s+attributes\s+must\s+be\s+configured`),
			},
			{
				Config: `
					data "jamfplatform_pro_smart_computer_group" "lookup" {
						id   = "1"
						name = "tf-acc-does-not-exist"
					}
				`,
				ExpectError: regexp.MustCompile(`Exactly\s+one\s+of\s+these\s+attributes\s+must\s+be\s+configured`),
			},
		},
	})
}

func TestAccDataSource_ProSmartComputerGroups_FilterByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-filter-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSmartComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configOneCriterion(name) + fmt.Sprintf(`
					data "jamfplatform_pro_smart_computer_groups" "lookup" {
						filter = [
							{
								selector = "name"
								argument = %q
							}
						]

						depends_on = [jamfplatform_pro_smart_computer_group.test]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_smart_computer_groups.lookup", "smart_computer_groups.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_smart_computer_groups.lookup", "smart_computer_groups.0.name", name),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_smart_computer_groups.lookup", "smart_computer_groups.0.platform_id"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_smart_computer_groups.lookup", "smart_computer_groups.0.member_count"),
				),
			},
		},
	})
}

// TestAccListResource_ProSmartComputerGroup_Basic drives the list resource
// through the terraform query workflow, with include_resource set so the
// per-item read that fills the criteria is exercised.
func TestAccListResource_ProSmartComputerGroup_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-scg-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSmartComputerGroupDestroy(t),
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

					list "jamfplatform_pro_smart_computer_group" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_smart_computer_group.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_smart_computer_group.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("criteria"), KnownValue: knownvalue.ListSizeExact(2)},
						},
					),
				},
			},
		},
	})
}

// patchTitleSourceID is Jamf's own patch title source, which every tenant has.
const patchTitleSourceID = 1

// configureAnyPatchSoftwareTitle configures the first catalogue title the tenant
// will accept and returns its name and its new identifier.
//
// Configuring a title is what makes Jamf Pro generate the per-title
// "Patch Reporting: <name>" criterion, which is the whole point: that criterion
// is one of the combinations Jamf Pro's group endpoints refuse and its older
// group interface stores, so a group built on one is the only way to drive the
// fallback write from end to end. The criterion vocabulary is derived from the
// tenant's own titles, so there is nothing to build it out of until a title
// exists.
//
// It walks the catalogue rather than taking the first entry, because a title can
// be server-side corrupt in a way intrinsic to the title rather than the tenant:
// catalogue title 518 ("010 Editor") is a standing example and it sorts first.
// It SKIPS when the tenant's catalogue publishes nothing usable, which is a
// legitimate estate rather than a failure.
//
// This mirrors the discovery the fallback canary in internal/common/progroups
// does. What it deliberately does not mirror is the canary's assertion that the
// refusal is still there — that job belongs in one place, and duplicating it
// here would mean this test failed on the day Jamf fixed the defect, when all
// that should happen is that the apply below stops needing the fallback.
func configureAnyPatchSoftwareTitle(t *testing.T, ctx context.Context, classic *proclassic.Client) (string, string) {
	t.Helper()

	titles, err := classic.ListPatchAvailableTitlesBySourceID(ctx, strconv.Itoa(patchTitleSourceID))
	if err != nil {
		t.Skipf("listing available patch titles on source %d: %v", patchTitleSourceID, err)
	}
	if titles == nil || titles.AvailableTitles == nil || titles.AvailableTitles.AvailableTitle == nil {
		t.Skipf("the tenant's patch title source %d publishes nothing, so there is no patch reporting criterion to build a group on", patchTitleSourceID)
	}

	attempted := 0
	for _, at := range *titles.AvailableTitles.AvailableTitle {
		if at.AppName == nil || *at.AppName == "" || at.NameID == nil || *at.NameID == "" {
			continue
		}
		appName, nameID, sourceID := *at.AppName, *at.NameID, patchTitleSourceID

		attempted++
		created, err := classic.CreatePatchSoftwareTitleByID(ctx, "0", &proclassic.PatchSoftwareTitle{
			Name:     &appName,
			NameID:   &nameID,
			SourceID: &sourceID,
		})
		if err != nil {
			t.Logf("catalogue title %q (%s) could not be configured, trying the next: %v", appName, nameID, err)
			continue
		}
		if created == nil || created.ID == nil {
			t.Logf("catalogue title %q (%s) reported no id, trying the next", appName, nameID)
			continue
		}
		return appName, strconv.Itoa(*created.ID)
	}

	if attempted == 0 {
		t.Skipf("the tenant's patch title source %d publishes no usable title", patchTitleSourceID)
	}
	t.Skipf("none of the %d catalogue titles on source %d could be configured", attempted, patchTitleSourceID)
	return "", ""
}

// TestAccResource_ProSmartComputerGroup_FallbackWrite drives the fallback write
// path from end to end on a criterion Jamf Pro's group endpoints refuse.
//
// Every other test in this package passes whether the fallback exists or not,
// because none of them reaches a refused combination. This one cannot be written
// any other way than against a real refusal, which is why it mints a patch
// software title first: the "Patch Reporting: <title>" criterion exists only
// once a title is configured, and it is refused on the usual path and stored on
// the older one.
//
// Three steps, each covering a branch the unit tests can only approach from the
// outside. The create proves a fallback create resolves both identifiers, one of
// which the older interface cannot be asked for, and still saves the
// description, which it has no field for. The rename proves the criteria-only
// update settles — a later apply of the group's own unmodified criteria is
// refused just as the first write was, so this step goes through the fallback
// too, and it must do so without a second description write. The last step
// changes only the description, which on this path is a request of its own.
func TestAccResource_ProSmartComputerGroup_FallbackWrite(t *testing.T) {
	testhelpers.AccPreCheck(t)
	ctx := context.Background()
	classic := proclassic.New(testhelpers.NewAcceptanceClient(t))

	titleName, titleID := configureAnyPatchSoftwareTitle(t, ctx, classic)
	t.Cleanup(func() {
		if err := classic.DeletePatchSoftwareTitleByID(ctx, titleID); err != nil {
			t.Logf("cleaning up patch software title %s: %v", titleID, err)
		}
	})

	suffix := testhelpers.RunSuffix()
	original := "tf-acc-pro-scg-fallback-" + suffix
	renamed := "tf-acc-pro-scg-fallback-renamed-" + suffix
	criterionName := progroups.PatchReportingCriterionPrefix + titleName

	config := func(groupName, note string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_pro_smart_computer_group" "test" {
				name        = %q
				description = %q

				criteria = [
					{
						name        = %q
						search_type = "is"
						value       = "Latest Version"
					},
				]
			}
		`, groupName, note, criterionName)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSmartComputerGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(original, "tf-acc placeholder note"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resourceAddress, "id"),
					resource.TestCheckResourceAttrSet(resourceAddress, "platform_id"),
					resource.TestCheckResourceAttr(resourceAddress, "name", original),
					resource.TestCheckResourceAttr(resourceAddress, "description", "tf-acc placeholder note"),
					resource.TestCheckResourceAttr(resourceAddress, "site_id", "-1"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.#", "1"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.name", criterionName),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.priority", "0"),
				),
			},
			{
				Config: config(renamed, "tf-acc placeholder note"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "name", renamed),
					resource.TestCheckResourceAttr(resourceAddress, "description", "tf-acc placeholder note"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.name", criterionName),
				),
			},
			{
				Config: config(renamed, "tf-acc placeholder note, edited"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "description", "tf-acc placeholder note, edited"),
					resource.TestCheckResourceAttr(resourceAddress, "criteria.0.name", criterionName),
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
