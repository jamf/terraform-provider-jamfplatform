// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /ebooks endpoint. Classic has
// known concurrency issues when multiple writes hit the same resource type —
// keep these tests serial with any future classic acceptance work in this
// package.
//
// Scope happy-paths use the all_* flags so they need no pre-existing tenant
// target objects. Per-target scope (computer_ids, mobile_device_ids, class_ids,
// …) is intentionally NOT acceptance-tested: those need real tenant object IDs,
// and there is no jamfplatform_pro_class resource yet to mint a class id (the
// attributes remain schema-present — see EBOOK_SPIKE.md decision 6).
//
// Delete note: the classic /ebooks DELETE returns a misleading HTTP 400 and
// completes asynchronously with highly variable latency — escalated to the Jamf
// API team. CheckDestroy is a deliberate NO-OP for now (see
// testAccCheckEbookDestroy); restore the GET-until-404 verification once the
// delete latency is bounded.

package ebook_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const ebookResourceAddr = "jamfplatform_pro_ebook.test"

// testAccCheckEbookDestroy is intentionally a NO-OP pending a Jamf API-team fix.
//
// The classic /ebooks DELETE is asynchronous behind a misleading HTTP 400 and
// completes server-side with highly variable latency (wire-probed: ~16s for a
// single delete, but observed >5min on a busy tenant; re-issuing DELETE or
// polling GET tightly appears to delay it further). Until the API team bounds /
// fixes the delete latency, a CheckDestroy that verifies actual removal would
// make the suite flaky for reasons outside the provider's control, so it is
// disabled here. Tracked via the API-team escalation (see EBOOK_SPIKE.md /
// memory project_pro_ebook_spike). Restore the GET-until-404 poll once the
// delete latency is bounded.
//
// It stays wired into the TestCases (rather than removed) so re-enabling is a
// single-function change.
// testAccCheckEbookDestroy is intentionally a NO-OP. The classic /ebooks delete
// is asynchronous AND GET-sensitive: wire-probed 2026-06-04, polling GET-by-id
// keeps the ebook present (one polled every 3s stayed alive past 3.5min and only
// cleared once polling stopped), so there is no GET cadence that can confirm
// removal without itself delaying it. The resource's Delete is fire-and-trust
// (issues the DELETE once, never GETs); destroy-time verification is therefore
// not possible for this endpoint — an API limitation, not a provider gap.
func testAccCheckEbookDestroy(t *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		t.Log("ebook CheckDestroy is a no-op: the classic /ebooks delete is async and GET-sensitive, so removal cannot be confirmed without delaying it (wire-probed)")
		return nil
	}
}

// ebookGeneralOnlyConfig is the import-stable shape: only the required general
// fields plus the in-house file metadata. The importer populates general
// post-Read but leaves the optional blocks (scope / self_service) null, so
// ImportStateVerify must run against a general-only config (see
// feedback_acc_import_optional_sections).
func ebookGeneralOnlyConfig(name, version, deploymentType string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_ebook" "test" {
			general = {
				name            = %q
				author          = "TF Acc"
				url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
				file_type       = "PDF"
				version         = %q
				deployment_type = %q
			}
		}
	`, name, version, deploymentType)
}

// ebookFullConfig adds the dual-target scope (all computers + all mobile
// devices) and a self_service block on top of general.
func ebookFullConfig(name, buttonText string, featureOnMain bool) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_ebook" "test" {
			general = {
				name            = %q
				url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
				file_type       = "PDF"
				version         = "1.0"
				deployment_type = "Make Available in Self Service"
			}
			scope = {
				targets = {
					all_computers      = true
					all_mobile_devices = true
				}
			}
			self_service = {
				display_name                    = %[1]q
				install_button_text             = %q
				self_service_description        = "Managed by Terraform acceptance test."
				force_users_to_view_description = false
				feature_on_main_page            = %t
			}
		}
	`, name, buttonText, featureOnMain)
}

// TestAccResource_ProEbook_InHouseLifecycle exercises create, import, and an
// in-place update for the general-only in-house shape. The version /
// deployment_type / author change verifies the GET-after-Update path (classic
// UpdateEbookByID returns 201 empty).
func TestAccResource_ProEbook_InHouseLifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ebook-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEbookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ebookGeneralOnlyConfig(name, "1.0", "Make Available in Self Service"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ebookResourceAddr, "id"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "general.name", name),
					resource.TestCheckResourceAttr(ebookResourceAddr, "general.version", "1.0"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "general.file_type", "PDF"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "general.deployment_type", "Make Available in Self Service"),
				),
			},
			{
				ResourceName:      ebookResourceAddr,
				ImportState:       true,
				ImportStateVerify: true,
				// scope: import hydrates every category; apply keeps
				// declared-only (this config declares none).
				// self_service: Optional state-gated block this general-only
				// config never declares — import hydrates it from the echoed
				// defaults, which legitimately differs from this config's
				// null. Not verified here.
				ImportStateVerifyIgnore: []string{"timeouts", "scope", "self_service"},
			},
			{
				// In-place update: bump version + flip the distribution method.
				Config: ebookGeneralOnlyConfig(name, "2.0", "Install Automatically/Prompt Users to Install"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "general.version", "2.0"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "general.deployment_type", "Install Automatically/Prompt Users to Install"),
				),
			},
		},
	})
}

// TestAccResource_ProEbook_ScopeAndSelfService exercises the dual-target scope
// and self_service blocks plus an in-place mutation (not a block deletion — the
// classic PUT is partial-merge and will not clear an omitted block).
func TestAccResource_ProEbook_ScopeAndSelfService(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ebook-ss-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEbookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ebookFullConfig(name, "Install", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ebookResourceAddr, "id"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "scope.targets.all_computers", "true"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "scope.targets.all_mobile_devices", "true"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.install_button_text", "Install"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.feature_on_main_page", "true"),
				),
			},
			{
				Config: ebookFullConfig(name, "Get", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.install_button_text", "Get"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.feature_on_main_page", "false"),
				),
			},
		},
	})
}

// TestAccResource_ProEbook_AppStoreServerDerived creates an Apple Books ebook
// without file_type/version and asserts the resource converges (the server
// derives those fields from the books.apple.com URL). file_type/version values
// are not asserted exactly because the server canonicalises them.
func TestAccResource_ProEbook_AppStoreServerDerived(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ebook-store-" + suffix
	config := fmt.Sprintf(`
		resource "jamfplatform_pro_ebook" "test" {
			general = {
				name = %q
				url  = "https://books.apple.com/us/book/intro-to-app-development-with-swift/id1118575552"
			}
		}
	`, name)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEbookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(ebookResourceAddr, "id"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "general.name", name),
					// file_type is server-derived (Computed) — must be known after apply.
					resource.TestCheckResourceAttrSet(ebookResourceAddr, "general.file_type"),
				),
			},
		},
	})
}

// TestAccResource_ProEbook_InvalidDeploymentType asserts the deployment_type
// OneOf validator rejects an out-of-set value at plan time.
func TestAccResource_ProEbook_InvalidDeploymentType(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ebook-bad-dt-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      ebookGeneralOnlyConfig(name, "1.0", "Beam It Straight To Devices"),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile("value must be one of"),
			},
		},
	})
}

// allFlagConflictConfig builds a general-only ebook plus a scope block that sets
// one all_* flag alongside a conflicting per-target set.
func allFlagConflictConfig(name, scopeBody string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_ebook" "test" {
			general = {
				name = %q
				url  = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
			}
			scope = {
				targets = {
					%s
				}
			}
		}
	`, name, scopeBody)
}

// TestAccResource_ProEbook_AllFlagConflicts asserts each of the three
// value-discriminated all-flag validators (computers / mobile devices / users)
// rejects its conflicting per-target set at plan time.
func TestAccResource_ProEbook_AllFlagConflicts(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	cases := []struct {
		name      string
		scopeBody string
	}{
		{"all_computers", `all_computers = true
				computer_ids  = ["1"]`},
		{"all_mobile_devices", `all_mobile_devices = true
				mobile_device_ids  = ["1"]`},
		{"all_jss_users", `all_jss_users = true
				user_ids      = ["1"]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      allFlagConflictConfig("tf-acc-pro-ebook-"+tc.name+"-"+suffix, tc.scopeBody),
						PlanOnly:    true,
						ExpectError: regexp.MustCompile("Conflicts with all-flag"),
					},
				},
			})
		})
	}
}

// ebookCategoriesConfig builds an in-house ebook whose self_service.categories
// reference real jamfplatform_pro_category fixtures. count selects how many of
// the two categories are attached (1 or 2) so the test can grow then shrink the
// nested set.
func ebookCategoriesConfig(name, suffix string, count int) string {
	cat2Block := ""
	if count >= 2 {
		cat2Block = `,
					{ id = jamfplatform_pro_category.two.id, display_in = true, feature_in = true }`
	}
	return fmt.Sprintf(`
		resource "jamfplatform_pro_category" "one" {
			name     = "tf-acc-ebook-cat1-%[2]s"
			priority = 9
		}

		resource "jamfplatform_pro_category" "two" {
			name     = "tf-acc-ebook-cat2-%[2]s"
			priority = 9
		}

		resource "jamfplatform_pro_ebook" "test" {
			general = {
				name            = %[1]q
				url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
				file_type       = "PDF"
				version         = "1.0"
				deployment_type = "Make Available in Self Service"
			}
			self_service = {
				install_button_text = "Get"
				categories = [
					{ id = jamfplatform_pro_category.one.id, display_in = true, feature_in = false }%[3]s
				]
			}
		}
	`, name, suffix, cat2Block)
}

// TestAccResource_ProEbook_SelfServiceCategories grows the self_service category
// set from one to two and back to one, asserting the by-ID reconcile keeps the
// authored display_in / feature_in values. (Sending a populated
// self_service_categories collection replaces it, distinct from the block-level
// partial-merge that retains omitted blocks.)
func TestAccResource_ProEbook_SelfServiceCategories(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ebook-cat-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEbookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ebookCategoriesConfig(name, suffix, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.categories.#", "1"),
				),
			},
			{
				Config: ebookCategoriesConfig(name, suffix, 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.categories.#", "2"),
				),
			},
			{
				Config: ebookCategoriesConfig(name, suffix, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.categories.#", "1"),
				),
			},
		},
	})
}

// TestAccResource_ProEbook_ScopeLimitationsClearWithEmptySet verifies the
// declared-`[]` clear gesture under granular scope ownership: an explicitly
// empty network_segment_ids must be emitted as an empty element so the scope
// subtree replace clears it (omitting the category instead would leave it
// unmanaged and preserved by the read-merge-write update). Uses a
// network-segment fixture (no LDAP needed).
func TestAccResource_ProEbook_ScopeLimitationsClearWithEmptySet(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ebook-limclear-" + suffix
	seg := "tf-acc-netseg-ebook-" + suffix
	cfg := func(segs string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_network_segment" "fixture" {
  name             = %q
  starting_address = "10.97.0.0"
  ending_address   = "10.97.0.255"
}

resource "jamfplatform_pro_ebook" "test" {
  general = {
    name            = %q
    url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
    file_type       = "PDF"
    version         = "1.0"
    deployment_type = "Make Available in Self Service"
  }
  scope = {
    targets = {
      all_computers = true
    }
    limitations = {
      network_segment_ids = [%s]
    }
  }
}
`, seg, name, segs)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEbookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg(`jamfplatform_pro_network_segment.fixture.id`),
				Check:  resource.TestCheckResourceAttr(ebookResourceAddr, "scope.limitations.network_segment_ids.#", "1"),
			},
			{
				// Clear to [] — the declared empty category must be emitted as an
				// explicit empty element (omission would preserve it under granular
				// ownership). Implicit post-step empty-plan enforces the round-trip.
				Config: cfg(``),
				Check:  resource.TestCheckResourceAttr(ebookResourceAddr, "scope.limitations.network_segment_ids.#", "0"),
			},
		},
	})
}

// ebookOmitRetainsFixtures declares the tenant objects the omit-retains configs
// reference: a department, building, static computer group and static mobile
// device group for the targets tab, a network segment and a second department
// and building for the exclusions tab, a network segment for the limitations
// tab, and a category shared by general.category_id and the Self Service
// category set. Every ID-keyed category the test covers therefore carries a
// real, distinct member. The Platform device groups bridge to the classic
// scope through jamf_pro_id. class_ids is covered separately, by
// TestAccResource_ProEbook_ScopeClassesSurviveUpdate — it needs its own steps
// because Jamf Pro will not overwrite a stored class list in a single write.
func ebookOmitRetainsFixtures(suffix string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_category" "omit" {
			name     = "tf-acc-ebook-omit-cat-%[1]s"
			priority = 9
		}

		resource "jamfplatform_pro_department" "target" {
			name = "tf-acc-ebook-omit-dept-target-%[1]s"
		}

		resource "jamfplatform_pro_department" "excluded" {
			name = "tf-acc-ebook-omit-dept-excluded-%[1]s"
		}

		resource "jamfplatform_pro_building" "target" {
			name = "tf-acc-ebook-omit-bldg-target-%[1]s"
		}

		resource "jamfplatform_pro_building" "excluded" {
			name = "tf-acc-ebook-omit-bldg-excluded-%[1]s"
		}

		resource "jamfplatform_pro_network_segment" "limited" {
			name             = "tf-acc-ebook-omit-seg-limited-%[1]s"
			starting_address = "10.98.0.0"
			ending_address   = "10.98.0.255"
		}

		resource "jamfplatform_pro_network_segment" "excluded" {
			name             = "tf-acc-ebook-omit-seg-excluded-%[1]s"
			starting_address = "10.98.1.0"
			ending_address   = "10.98.1.255"
		}

		resource "jamfplatform_device_group" "computers" {
			name        = "tf-acc-ebook-omit-computers-%[1]s"
			description = "tf-acc omit-retains computer scope fixture"
			group_type  = "static"
			device_type = "computer"
		}

		resource "jamfplatform_device_group" "mobile" {
			name        = "tf-acc-ebook-omit-mobile-%[1]s"
			description = "tf-acc omit-retains mobile scope fixture"
			group_type  = "static"
			device_type = "mobile"
		}
	`, suffix)
}

// ebookOmitRetainsGeneral is the general block every omit-retains step shares.
// It carries non-default Optional+Computed values (author, category_id) so the
// wire check can tell a retained general from a defaulted one.
func ebookOmitRetainsGeneral(name string) string {
	return fmt.Sprintf(`
			general = {
				name            = %q
				author          = "Omit Retains"
				url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
				file_type       = "PDF"
				version         = "1.0"
				deployment_type = "Make Available in Self Service"
				category_id     = jamfplatform_pro_category.omit.id
			}
	`, name)
}

// ebookOmitRetainsTargets is the scope.targets sub-block every omit-retains
// step that declares a scope shares: the computer and mobile halves of the
// dual-target union each carry a real group, plus a department and a building.
const ebookOmitRetainsTargets = `
				targets = {
					department_ids          = [jamfplatform_pro_department.target.id]
					building_ids            = [jamfplatform_pro_building.target.id]
					computer_group_ids      = [jamfplatform_device_group.computers.jamf_pro_id]
					mobile_device_group_ids = [jamfplatform_device_group.mobile.jamf_pro_id]
				}
`

// ebookOmitRetainsConfig is the fully declared shape for the omit-retains
// contract: all three scope tabs, a self_service block with every wire-echoed
// leaf set to a distinctive value, and a Self Service category set, so a server that stopped
// retaining an omitted element is caught on content, not on presence.
func ebookOmitRetainsConfig(name, suffix string) string {
	return ebookOmitRetainsFixtures(suffix) + `
		resource "jamfplatform_pro_ebook" "test" {
` + ebookOmitRetainsGeneral(name) + `
			scope = {
` + ebookOmitRetainsTargets + `
				limitations = {
					network_segment_ids                   = [jamfplatform_pro_network_segment.limited.id]
					directory_service_or_local_user_names = ["tf-acc-omit-limited-user"]
				}
				exclusions = {
					department_ids                        = [jamfplatform_pro_department.excluded.id]
					building_ids                          = [jamfplatform_pro_building.excluded.id]
					network_segment_ids                   = [jamfplatform_pro_network_segment.excluded.id]
					directory_service_or_local_user_names = ["tf-acc-omit-excluded-user"]
				}
			}
			self_service = {
				display_name                    = "Omit-retains display name"
				install_button_text             = "Retain me"
				self_service_description        = "Omit-retains contract description."
				force_users_to_view_description = true
				feature_on_main_page            = true
				categories = [
					{ id = jamfplatform_pro_category.omit.id, display_in = true, feature_in = true }
				]
			}
		}
	`
}

// ebookOmitRetainsParentsOnlyConfig keeps the parents that have gated children
// but drops the children: scope keeps targets and loses limitations and
// exclusions, self_service loses its categories and the Optional+Computed
// display_name. The PUT re-emits the scope from the granular merge with
// the two dropped tabs folded in from the live object, and carries a
// self_service element with no self_service_categories at all.
func ebookOmitRetainsParentsOnlyConfig(name, suffix string) string {
	return ebookOmitRetainsFixtures(suffix) + `
		resource "jamfplatform_pro_ebook" "test" {
` + ebookOmitRetainsGeneral(name) + `
			scope = {
` + ebookOmitRetainsTargets + `
			}
			self_service = {
				install_button_text             = "Retain me"
				self_service_description        = "Omit-retains contract description."
				force_users_to_view_description = true
				feature_on_main_page            = true
			}
		}
	`
}

// ebookOmitRetainsGeneralOnlyConfig drops every optional block, so the PUT
// carries <general> alone. The fixtures stay declared so the server's retained
// scope and categories keep pointing at live objects, and depends_on keeps the
// destroy order the dropped references no longer imply: the Platform
// device-groups API refuses to delete a group the retained scope still names
// (422 HAS_DEPENDENCIES), so the ebook must go before its groups.
func ebookOmitRetainsGeneralOnlyConfig(name, suffix string) string {
	return ebookOmitRetainsFixtures(suffix) + `
		resource "jamfplatform_pro_ebook" "test" {
			depends_on = [jamfplatform_device_group.computers, jamfplatform_device_group.mobile]
` + ebookOmitRetainsGeneral(name) + `
		}
	`
}

// ebookStateAttr returns one attribute of a resource in the Terraform state, so
// a wire assertion can compare against the id a fixture was actually allocated.
func ebookStateAttr(s *terraform.State, addr, key string) (string, error) {
	rs, ok := s.RootModule().Resources[addr]
	if !ok {
		return "", fmt.Errorf("fixture %s not found in state", addr)
	}
	v, ok := rs.Primary.Attributes[key]
	if !ok || v == "" {
		return "", fmt.Errorf("fixture %s has no %s in state", addr, key)
	}
	return v, nil
}

// ebookRequireSingleIDName asserts a classic id/name collection holds exactly
// one member carrying the wanted id.
func ebookRequireSingleIDName(field, want string, items *[]proclassic.IDName) error {
	if items == nil || len(*items) != 1 {
		return fmt.Errorf("%s: want exactly one member, got %+v", field, items)
	}
	return testhelpers.RequireEqual(field+"[0].id", want, strconv.Itoa(testhelpers.Deref((*items)[0].ID)))
}

// ebookRequireSingleID is the generic sibling of ebookRequireSingleIDName for
// the ebook scope collections whose item type is not IDName.
func ebookRequireSingleID[T any](field, want string, items *[]T, id func(T) *int) error {
	if items == nil || len(*items) != 1 {
		return fmt.Errorf("%s: want exactly one member, got %+v", field, items)
	}
	return testhelpers.RequireEqual(field+"[0].id", want, strconv.Itoa(testhelpers.Deref(id((*items)[0]))))
}

// ebookRetainedOnServer asserts the server's copy still carries every value the
// omit-retains config declared in its first step. The fixture ids are read from
// the Terraform state rather than assumed, so the check compares against what
// the tenant allocated.
func ebookRetainedOnServer(t *testing.T) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return func(s *terraform.State) error {
		ids := map[string]string{}
		for _, f := range []struct{ key, addr, attr string }{
			{"category", "jamfplatform_pro_category.omit", "id"},
			{"department.target", "jamfplatform_pro_department.target", "id"},
			{"department.excluded", "jamfplatform_pro_department.excluded", "id"},
			{"building.target", "jamfplatform_pro_building.target", "id"},
			{"building.excluded", "jamfplatform_pro_building.excluded", "id"},
			{"segment.limited", "jamfplatform_pro_network_segment.limited", "id"},
			{"segment.excluded", "jamfplatform_pro_network_segment.excluded", "id"},
			{"group.computers", "jamfplatform_device_group.computers", "jamf_pro_id"},
			{"group.mobile", "jamfplatform_device_group.mobile", "jamf_pro_id"},
		} {
			v, err := ebookStateAttr(s, f.addr, f.attr)
			if err != nil {
				return err
			}
			ids[f.key] = v
		}
		return testhelpers.CheckLiveObject(ebookResourceAddr,
			func(ctx context.Context, id string) (*proclassic.Ebook, error) {
				return c.GetEbookByID(ctx, id)
			},
			func(e *proclassic.Ebook) error {
				if e.General == nil || e.General.Category == nil {
					return fmt.Errorf("general.category: absent")
				}
				if err := testhelpers.RequireEqual("general.author", "Omit Retains", testhelpers.Deref(e.General.Author)); err != nil {
					return err
				}
				if err := testhelpers.RequireEqual("general.category.id", ids["category"], strconv.Itoa(testhelpers.Deref(e.General.Category.ID))); err != nil {
					return err
				}
				if err := ebookScopeRetained(e.Scope, ids); err != nil {
					return err
				}
				return ebookSelfServiceRetained(e.SelfService, ids["category"])
			})(s)
	}
}

// ebookScopeRetained checks every scope category the omit-retains config
// declared across all three tabs.
func ebookScopeRetained(sc *proclassic.EbookScope, ids map[string]string) error {
	if sc == nil {
		return fmt.Errorf("scope: absent")
	}
	if sc.Departments == nil || sc.Buildings == nil || sc.ComputerGroups == nil || sc.MobileDeviceGroups == nil {
		return fmt.Errorf("scope.targets: a category is absent: %+v", sc)
	}
	if err := ebookRequireSingleIDName("scope.targets.departments", ids["department.target"], sc.Departments.Department); err != nil {
		return err
	}
	if err := ebookRequireSingleIDName("scope.targets.buildings", ids["building.target"], sc.Buildings.Building); err != nil {
		return err
	}
	if err := ebookRequireSingleIDName("scope.targets.computer_groups", ids["group.computers"], sc.ComputerGroups.ComputerGroup); err != nil {
		return err
	}
	if err := ebookRequireSingleIDName("scope.targets.mobile_device_groups", ids["group.mobile"], sc.MobileDeviceGroups.MobileDeviceGroup); err != nil {
		return err
	}

	l := sc.Limitations
	if l == nil || l.NetworkSegments == nil || l.Users == nil {
		return fmt.Errorf("scope.limitations: a category is absent: %+v", l)
	}
	if err := ebookRequireSingleIDName("scope.limitations.network_segments", ids["segment.limited"], l.NetworkSegments.NetworkSegment); err != nil {
		return err
	}
	if l.Users.User == nil || len(*l.Users.User) != 1 {
		return fmt.Errorf("scope.limitations.users: want exactly one user, got %+v", l.Users)
	}
	if err := testhelpers.RequireEqual("scope.limitations.users[0].name", "tf-acc-omit-limited-user", testhelpers.Deref((*l.Users.User)[0].Name)); err != nil {
		return err
	}

	x := sc.Exclusions
	if x == nil || x.Departments == nil || x.Buildings == nil || x.NetworkSegments == nil || x.Users == nil {
		return fmt.Errorf("scope.exclusions: a category is absent: %+v", x)
	}
	if err := ebookRequireSingleIDName("scope.exclusions.departments", ids["department.excluded"], x.Departments.Department); err != nil {
		return err
	}
	if err := ebookRequireSingleIDName("scope.exclusions.buildings", ids["building.excluded"], x.Buildings.Building); err != nil {
		return err
	}
	if err := ebookRequireSingleID("scope.exclusions.network_segments", ids["segment.excluded"], x.NetworkSegments.NetworkSegment,
		func(i proclassic.EbookScopeExclusionsNetworkSegmentsNetworkSegmentItem) *int { return i.ID }); err != nil {
		return err
	}
	if x.Users.User == nil || len(*x.Users.User) != 1 {
		return fmt.Errorf("scope.exclusions.users: want exactly one user, got %+v", x.Users)
	}
	return testhelpers.RequireEqual("scope.exclusions.users[0].name", "tf-acc-omit-excluded-user", testhelpers.Deref((*x.Users.User)[0].Name))
}

// ebookSelfServiceRetained checks every self_service leaf and the category set
// the omit-retains config declared. The notification_* leaves are deliberately
// not declared or checked: the /ebooks GET never echoes <notification>,
// <notification_subject> or <notification_message> (wire-observed 2026-09-06 —
// the POST carried all three, the GET after it carried none), so their storage
// cannot be witnessed on the wire and PreferCurrent is all that holds them in
// state.
func ebookSelfServiceRetained(ss *proclassic.EbookSelfService, categoryID string) error {
	if ss == nil {
		return fmt.Errorf("self_service: absent")
	}
	if err := testhelpers.RequireEqual("self_service.display_name", "Omit-retains display name", testhelpers.Deref(ss.SelfServiceDisplayName)); err != nil {
		return err
	}
	if err := testhelpers.RequireEqual("self_service.install_button_text", "Retain me", testhelpers.Deref(ss.InstallButtonText)); err != nil {
		return err
	}
	if err := testhelpers.RequireEqual("self_service.self_service_description", "Omit-retains contract description.", testhelpers.Deref(ss.SelfServiceDescription)); err != nil {
		return err
	}
	if err := testhelpers.RequireEqual("self_service.force_users_to_view_description", true, testhelpers.Deref(ss.ForceUsersToViewDescription)); err != nil {
		return err
	}
	if err := testhelpers.RequireEqual("self_service.feature_on_main_page", true, testhelpers.Deref(ss.FeatureOnMainPage)); err != nil {
		return err
	}
	if ss.SelfServiceCategories == nil || ss.SelfServiceCategories.Category == nil || len(*ss.SelfServiceCategories.Category) != 1 {
		return fmt.Errorf("self_service.categories: want exactly one category, got %+v", ss.SelfServiceCategories)
	}
	cat := (*ss.SelfServiceCategories.Category)[0]
	if err := testhelpers.RequireEqual("self_service.categories[0].id", categoryID, strconv.Itoa(testhelpers.Deref(cat.ID))); err != nil {
		return err
	}
	if err := testhelpers.RequireEqual("self_service.categories[0].display_in", true, testhelpers.Deref(cat.DisplayIn)); err != nil {
		return err
	}
	return testhelpers.RequireEqual("self_service.categories[0].feature_in", true, testhelpers.Deref(cat.FeatureIn))
}

// TestAccResource_ProEbook_OmittedBlocksRetained pins the omit-retains contract
// the plan output cannot show: dropping scope.limitations, scope.exclusions and
// self_service.categories, then the whole scope and self_service, from config
// plans them as removed, but the classic PUT omits the elements and the server
// keeps every value. Step 2 keeps scope.targets so the scope goes through the
// granular read-merge-write, which must fold the two live tabs back in rather
// than emitting them empty, and keeps self_service without categories so the
// element is sent with no self_service_categories at all. Step 3 drops both
// blocks so the PUT carries <general> alone. Each step's implicit post-apply
// plan must be empty, which is what makes the contract usable. If this test
// fails on content, the endpoint no longer merges and nothing that suppresses
// the removal plan may ship for this resource. CheckDestroy is the file's
// documented no-op: the /ebooks delete is asynchronous and GET-sensitive.
func TestAccResource_ProEbook_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ebook-omit-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEbookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ebookOmitRetainsConfig(name, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.categories.#", "1"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "scope.limitations.network_segment_ids.#", "1"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "scope.exclusions.department_ids.#", "1"),
					ebookRetainedOnServer(t),
				),
			},
			{
				Config: ebookOmitRetainsParentsOnlyConfig(name, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(ebookResourceAddr, "self_service.categories.#"),
					resource.TestCheckNoResourceAttr(ebookResourceAddr, "scope.limitations.network_segment_ids.#"),
					resource.TestCheckNoResourceAttr(ebookResourceAddr, "scope.exclusions.department_ids.#"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "scope.targets.department_ids.#", "1"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "self_service.display_name", "Omit-retains display name"),
					ebookRetainedOnServer(t),
				),
			},
			{
				Config: ebookOmitRetainsGeneralOnlyConfig(name, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(ebookResourceAddr, "scope.targets.department_ids.#"),
					resource.TestCheckNoResourceAttr(ebookResourceAddr, "self_service.install_button_text"),
					ebookRetainedOnServer(t),
				),
			},
		},
	})
}

// ebookClassScopeFixtures declares the objects the class_ids coverage needs: a
// class to scope to, and two departments so a step can change a target category
// OTHER than classes — the transition that destroyed the stored class before
// the two-request delivery landed (issue #428).
func ebookClassScopeFixtures(suffix string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_class" "scope" {
			name        = "tf-acc-ebook-class-%[1]s"
			description = "tf-acc ebook class scope fixture"
		}

		resource "jamfplatform_pro_department" "class_a" {
			name = "tf-acc-ebook-class-dept-a-%[1]s"
		}

		resource "jamfplatform_pro_department" "class_b" {
			name = "tf-acc-ebook-class-dept-b-%[1]s"
		}
	`, suffix)
}

// ebookClassScopeConfig declares an ebook scoped to the class fixture plus the
// departments named in depts. classIDs is the literal `class_ids` line, so a
// step can declare the category, clear it with `[]`, or omit it entirely. A
// step that does not reference the class gets an explicit depends_on: nothing
// else then orders the destroy, and the ebook must go before the class it may
// still name on the wire.
func ebookClassScopeConfig(name, suffix, depts, classIDs string) string {
	dependsOn := ""
	if !strings.Contains(classIDs, "jamfplatform_pro_class.scope") {
		dependsOn = "depends_on = [jamfplatform_pro_class.scope]"
	}
	return ebookClassScopeFixtures(suffix) + fmt.Sprintf(`
		resource "jamfplatform_pro_ebook" "test" {
			%s
			general = {
				name            = %q
				url             = "https://www.rd.usda.gov/sites/default/files/pdf-sample_0.pdf"
				file_type       = "PDF"
				version         = "1.0"
				deployment_type = "Make Available in Self Service"
			}
			scope = {
				targets = {
					department_ids = [%s]
					%s
				}
			}
		}
	`, dependsOn, name, depts, classIDs)
}

// ebookClassesOnServer asserts the ebook's stored scope classes against the
// class fixture's allocated id. wantClass false demands the category be empty.
// This is the check the state cannot make: a class the server dropped is a
// server-side loss, and the pre-fix failure mode cleared it while reporting the
// department change as applied.
func ebookClassesOnServer(t *testing.T, wantClass bool) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return func(s *terraform.State) error {
		classID, err := ebookStateAttr(s, "jamfplatform_pro_class.scope", "id")
		if err != nil {
			return err
		}
		return testhelpers.CheckLiveObject(ebookResourceAddr,
			func(ctx context.Context, id string) (*proclassic.Ebook, error) {
				return c.GetEbookByID(ctx, id)
			},
			func(e *proclassic.Ebook) error {
				if e.Scope == nil {
					return fmt.Errorf("scope: absent")
				}
				if !wantClass {
					if e.Scope.Classes != nil && e.Scope.Classes.Class != nil && len(*e.Scope.Classes.Class) != 0 {
						return fmt.Errorf("scope.targets.classes: want empty, got %+v", *e.Scope.Classes.Class)
					}
					return nil
				}
				if e.Scope.Classes == nil {
					return fmt.Errorf("scope.targets.classes: absent — the server dropped the class")
				}
				return ebookRequireSingleIDName("scope.targets.classes", classID, e.Scope.Classes.Class)
			})(s)
	}
}

// TestAccResource_ProEbook_ScopeClassesSurviveUpdate is the regression test for
// issue #428: an ebook whose scope holds a class could not survive a scope
// update. Jamf Pro stores <classes> only while the stored list is empty, so
// every later scope write cleared it — the apply failed the post-apply
// consistency check AND destroyed the class server-side, and the next apply
// restored it, so an unchanged config alternated pass/fail forever. The
// provider now delivers such a scope in two requests (see
// ebookScopeClassesCleared).
//
// Step 2 is the one that used to fail: it changes department_ids and leaves
// class_ids alone. Step 3 drops class_ids so the category goes unmanaged and
// the granular merge has to carry it through the same two requests. Step 4
// clears it with `[]`, which needs one request because the server accepts an
// empty list. Every step's implicit post-apply plan must be empty, which is the
// half the alternating failure broke. CheckDestroy is the file's documented
// no-op: the /ebooks delete is asynchronous and GET-sensitive.
func TestAccResource_ProEbook_ScopeClassesSurviveUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-ebook-class-" + suffix

	const (
		deptA     = "jamfplatform_pro_department.class_a.id"
		deptAB    = "jamfplatform_pro_department.class_a.id, jamfplatform_pro_department.class_b.id"
		declared  = "class_ids = [jamfplatform_pro_class.scope.id]"
		cleared   = "class_ids = []"
		unmanaged = ""
	)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEbookDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: ebookClassScopeConfig(name, suffix, deptA, declared),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "scope.targets.class_ids.#", "1"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "scope.targets.department_ids.#", "1"),
					ebookClassesOnServer(t, true),
				),
			},
			{
				Config: ebookClassScopeConfig(name, suffix, deptAB, declared),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "scope.targets.class_ids.#", "1"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "scope.targets.department_ids.#", "2"),
					ebookClassesOnServer(t, true),
				),
			},
			{
				Config: ebookClassScopeConfig(name, suffix, deptA, unmanaged),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(ebookResourceAddr, "scope.targets.class_ids.#"),
					resource.TestCheckResourceAttr(ebookResourceAddr, "scope.targets.department_ids.#", "1"),
					ebookClassesOnServer(t, true),
				),
			},
			{
				Config: ebookClassScopeConfig(name, suffix, deptA, cleared),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ebookResourceAddr, "scope.targets.class_ids.#", "0"),
					ebookClassesOnServer(t, false),
				),
			},
		},
	})
}
