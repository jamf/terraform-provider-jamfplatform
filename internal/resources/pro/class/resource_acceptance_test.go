// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /classes endpoint.
//
// Design notes the acc run verifies (each is load-bearing — see CLASS_SPIKE.md):
//   - Authoritative always-emit membership: students/teachers are cleared by
//     OMITTING the attribute (or `= []`). The build always sends the full set, so
//     removing an element removes the member and omitting clears all members. The
//     clear step relies on the framework's automatic post-apply empty-plan check
//     to prove no perma-diff.
//   - Username -> ID resolution: supplying students by username populates the
//     Computed student_ids echo. Unknown usernames are AUTO-CREATED as Jamf Pro
//     users by the server, so this test creates throwaway users named
//     tf-acc-class-*; they persist after the class is deleted (acceptable on a
//     test tenant — clean up manually if desired).
//   - GET-after-Update: every Update step implicitly verifies the GET-after path
//     (classic Update returns 201 + empty body).
//   - students/teachers are Sets: membership is order-independent, so
//     TestCheckTypeSetElemAttr is used.
//
// The *_group_ids membership (student/teacher user groups, mobile device groups)
// is exercised by TestAccResource_ProClass_GroupMembership, which mints the
// referenced groups inline as fixtures (jamfplatform_pro_user_group and a
// jamfplatform_device_group bridged via jamf_pro_id) — no pre-existing tenant
// groups or env vars required.

package class_test

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
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const classResource = "jamfplatform_pro_class.test"

func testAccCheckClassDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_class" {
				continue
			}
			_, err := c.GetClassByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking class %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("class %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// Step 1: create with 2 students + 1 teacher + a description, site NONE.
func classConfigCreate(name, studentA, studentB, teacher string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_class" "test" {
			name        = %q
			description = "created by acc test"
			students    = [%q, %q]
			teachers    = [%q]
		}
	`, name, studentA, studentB, teacher)
}

// Step 2: rename, change description, drop a student (2->1), add a teacher (1->2).
func classConfigGrow(name, studentA, teacherA, teacherB string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_class" "test" {
			name        = %q
			description = "updated by acc test"
			students    = [%q]
			teachers    = [%q, %q]
		}
	`, name, studentA, teacherA, teacherB)
}

// Step 3: clear all membership and description by OMITTING the attributes.
func classConfigCleared(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_class" "test" {
			name = %q
		}
	`, name)
}

func TestAccResource_ProClass_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-class-" + suffix
	renamed := "tf-acc-pro-class-renamed-" + suffix
	// Throwaway usernames; unknown usernames are auto-created by Jamf Pro.
	studentA := "tf-acc-class-student-a-" + suffix
	studentB := "tf-acc-class-student-b-" + suffix
	teacherA := "tf-acc-class-teacher-a-" + suffix
	teacherB := "tf-acc-class-teacher-b-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckClassDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: classConfigCreate(name, studentA, studentB, teacherA),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(classResource, "id"),
					resource.TestCheckResourceAttr(classResource, "name", name),
					resource.TestCheckResourceAttr(classResource, "description", "created by acc test"),
					resource.TestCheckResourceAttr(classResource, "site_id", "-1"),
					// No site: site_name is null (absent) on the "-1" sentinel — the
					// server echo of "NONE" is flaky, so DerivedRefName nulls it.
					resource.TestCheckNoResourceAttr(classResource, "site_name"),
					resource.TestCheckResourceAttrSet(classResource, "source"),
					resource.TestCheckResourceAttr(classResource, "students.#", "2"),
					resource.TestCheckTypeSetElemAttr(classResource, "students.*", studentA),
					resource.TestCheckTypeSetElemAttr(classResource, "students.*", studentB),
					resource.TestCheckResourceAttr(classResource, "teachers.#", "1"),
					resource.TestCheckTypeSetElemAttr(classResource, "teachers.*", teacherA),
					// Server resolved usernames to user IDs.
					resource.TestCheckResourceAttr(classResource, "student_ids.#", "2"),
					resource.TestCheckResourceAttr(classResource, "teacher_ids.#", "1"),
				),
			},
			{
				Config: classConfigGrow(renamed, studentA, teacherA, teacherB),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(classResource, "name", renamed),
					resource.TestCheckResourceAttr(classResource, "description", "updated by acc test"),
					resource.TestCheckResourceAttr(classResource, "students.#", "1"),
					resource.TestCheckTypeSetElemAttr(classResource, "students.*", studentA),
					resource.TestCheckResourceAttr(classResource, "teachers.#", "2"),
					resource.TestCheckTypeSetElemAttr(classResource, "teachers.*", teacherB),
					resource.TestCheckResourceAttr(classResource, "student_ids.#", "1"),
					resource.TestCheckResourceAttr(classResource, "teacher_ids.#", "2"),
				),
			},
			{
				// Import the POPULATED resource — the highest-risk round-trip.
				ResourceName:            classResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				Config: classConfigCleared(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(classResource, "name", renamed),
					// Cleared: omitted membership flattens to null; the automatic
					// post-apply empty-plan check is the real assertion.
					resource.TestCheckNoResourceAttr(classResource, "students.0"),
					resource.TestCheckNoResourceAttr(classResource, "teachers.0"),
				),
			},
			{
				ResourceName:            classResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// classGroupFixtures returns HCL for two static Jamf Pro user groups (student /
// teacher) plus a static Platform Services mobile device group, so the
// group-membership test is fully self-contained. The device group's
// server-derived `jamf_pro_id` bridges into the classic class's
// mobile_device_group_ids set (same pattern as jamfplatform_pro_policy scope).
func classGroupFixtures(suffix string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "student_grp" {
			name       = "tf-acc-class-student-grp-%[1]s"
			group_type = "static"
		}

		resource "jamfplatform_pro_user_group" "teacher_grp" {
			name       = "tf-acc-class-teacher-grp-%[1]s"
			group_type = "static"
		}

		resource "jamfplatform_device_group" "md_grp" {
			name        = "tf-acc-class-md-grp-%[1]s"
			description = "tf-acc class fixture"
			group_type  = "static"
			device_type = "mobile"
		}
	`, suffix)
}

// TestAccResource_ProClass_GroupMembership exercises the student_group_ids,
// teacher_group_ids, and mobile_device_group_ids sets using fixtures created in
// the same config (two jamfplatform_pro_user_group resources and one
// jamfplatform_device_group whose jamf_pro_id bridges to the classic mobile
// device group). Self-contained — runs on every acceptance pass. Step 2 removes
// the teacher group and clears the mobile device group to prove the
// authoritative remove path; the populated state is then imported.
func TestAccResource_ProClass_GroupMembership(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-class-groups-" + suffix
	fixtures := classGroupFixtures(suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckClassDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fixtures + fmt.Sprintf(`
					resource "jamfplatform_pro_class" "test" {
						name                    = %q
						student_group_ids       = [jamfplatform_pro_user_group.student_grp.id]
						teacher_group_ids       = [jamfplatform_pro_user_group.teacher_grp.id]
						mobile_device_group_ids = [jamfplatform_device_group.md_grp.jamf_pro_id]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(classResource, "student_group_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(classResource, "student_group_ids.*", "jamfplatform_pro_user_group.student_grp", "id"),
					resource.TestCheckResourceAttr(classResource, "teacher_group_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(classResource, "teacher_group_ids.*", "jamfplatform_pro_user_group.teacher_grp", "id"),
					resource.TestCheckResourceAttr(classResource, "mobile_device_group_ids.#", "1"),
					resource.TestCheckTypeSetElemAttrPair(classResource, "mobile_device_group_ids.*", "jamfplatform_device_group.md_grp", "jamf_pro_id"),
				),
			},
			{
				// Remove the teacher group and clear the mobile device group; keep
				// the student group.
				Config: fixtures + fmt.Sprintf(`
					resource "jamfplatform_pro_class" "test" {
						name              = %q
						student_group_ids = [jamfplatform_pro_user_group.student_grp.id]
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(classResource, "student_group_ids.#", "1"),
					resource.TestCheckNoResourceAttr(classResource, "teacher_group_ids.0"),
					resource.TestCheckNoResourceAttr(classResource, "mobile_device_group_ids.0"),
				),
			},
			{
				ResourceName:            classResource,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProClass_EmptyNameRejected exercises the name LengthAtLeast(1)
// validator.
func TestAccResource_ProClass_EmptyNameRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_class" "test" {
						name = ""
					}
				`,
				ExpectError: regexp.MustCompile(`string length must be at least 1`),
			},
		},
	})
}

func TestAccDataSource_ProClass_ByIDAndName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-class-ds-" + suffix
	student := "tf-acc-class-ds-student-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckClassDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_class" "test" {
						name        = %q
						description = "ds fixture"
						students    = [%q]
					}

					data "jamfplatform_pro_class" "by_id" {
						id = jamfplatform_pro_class.test.id
					}

					data "jamfplatform_pro_class" "by_name" {
						name = jamfplatform_pro_class.test.name
					}
				`, name, student),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_class.by_id", "name", classResource, "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_class.by_id", "description", "ds fixture"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_class.by_id", "students.#", "1"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_class.by_name", "id", classResource, "id"),
				),
			},
		},
	})
}

func TestAccListResource_ProClass_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-class-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckClassDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_class" "test" {
						name        = %q
						description = "list hydration fixture"

						students = ["tf-acc-class-list-student@example.com"]
					}
				`, name),
				Check: resource.TestCheckResourceAttrSet(classResource, "id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_class" "test" {
						provider         = jamfplatform
						include_resource = true

						config {
							filter = {
								name_substring = %q
							}
						}
					}
				`, name),
				QueryResultChecks: []querycheck.QueryResultCheck{
					querycheck.ExpectLength("jamfplatform_pro_class.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_class.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							// The /classes summary row carries id, name and description
							// only — never membership. Asserting name alone is what let a
							// list resource emitting null students/teachers/groups pass,
							// while `terraform query -generate-config-out` wrote a class
							// with no members. Membership is authoritative on write, so
							// applying that back emptied the class. This pins the
							// per-item GET.
							{Path: tfjsonpath.New("students"), KnownValue: knownvalue.SetExact([]knownvalue.Check{
								knownvalue.StringExact("tf-acc-class-list-student@example.com"),
							})},
						},
					),
				},
			},
		},
	})
}
