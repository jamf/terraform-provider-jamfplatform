// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /usergroups endpoint. Classic
// has known concurrency issues when multiple writes hit the same resource
// type — keep these tests serial with any other classic acceptance work in
// this package.

package user_group_test

import (
	"context"
	"fmt"
	"slices"
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
	"github.com/jamf/terraform-provider-jamfplatform/internal/resources/pro/user_group"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// testAccCheckUserGroupDestroy verifies user groups created during the test
// were destroyed.
func testAccCheckUserGroupDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_user_group" {
				continue
			}
			_, err := c.GetUserGroupByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro user group %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro user group %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func userGroupStaticConfigMinimal(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "test" {
			name       = %q
			group_type = "static"
		}
	`, name)
}

func userGroupStaticConfigRenamed(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "test" {
			name                = %q
			group_type          = "static"
			notify_on_membership_change = true
		}
	`, name)
}

// userGroupSmartConfig exercises a smart group with two criteria — mirrors
// the example payload supplied during design (User Group → member of, VPP
// Invitation Status → is). Both criteria use non-empty `value` strings:
// classic strips empty <value/> elements from the criterion on round-trip,
// which would surface as ImportStateVerify drift even though the post-create
// state matches the wire payload.
func userGroupSmartConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "test" {
			name       = %q
			group_type = "smart"

			criteria = [
				{
					name        = "Email Address"
					search_type = "like"
					value       = "@example.com"
				},
				{
					name        = "Full Name"
					search_type = "is not"
					value       = "Test User"
					and_or      = "and"
				},
			]
		}
	`, name)
}

// TestAccResource_ProUserGroup_Static_Basic exercises create, in-place rename
// + toggle of notify_on_membership_change, and import for a static user group with no
// members. The rename step implicitly verifies the GET-after-Update path
// (classic Update returns 201 + empty body).
func TestAccResource_ProUserGroup_Static_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	original := "tf-acc-pro-ug-" + suffix
	renamed := "tf-acc-pro-ug-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: userGroupStaticConfigMinimal(original),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_user_group.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "name", original),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "group_type", "static"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "notify_on_membership_change", "false"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "site_id", "-1"),
				),
			},
			{
				Config: userGroupStaticConfigRenamed(renamed),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "name", renamed),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "notify_on_membership_change", "true"),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_user_group.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProUserGroup_Smart_Basic exercises a smart group with
// criteria — covers the criteria expand/flatten symmetry and confirms the
// SDK's UserGroupCriteria wrapper round-trips correctly.
func TestAccResource_ProUserGroup_Smart_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ug-smart-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: userGroupSmartConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_user_group.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "group_type", "smart"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "criteria.#", "2"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "criteria.0.name", "Email Address"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "criteria.0.search_type", "like"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "criteria.1.search_type", "is not"),
				),
			},
			{
				ResourceName:            "jamfplatform_pro_user_group.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func TestAccDataSource_ProUserGroup_ByID(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ug-ds-id-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_user_group" "src" {
						name       = %q
						group_type = "static"
					}

					data "jamfplatform_pro_user_group" "lookup" {
						id = jamfplatform_pro_user_group.src.id
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_user_group.lookup", "name", "jamfplatform_pro_user_group.src", "name"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_user_group.lookup", "group_type", "static"),
				),
			},
		},
	})
}

func TestAccDataSource_ProUserGroup_ByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ug-ds-name-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_user_group" "src" {
						name       = %q
						group_type = "static"
					}

					data "jamfplatform_pro_user_group" "lookup" {
						name = jamfplatform_pro_user_group.src.name
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_user_group.lookup", "id", "jamfplatform_pro_user_group.src", "id"),
				),
			},
		},
	})
}

func TestAccDataSource_ProUserGroups_FilterByName(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ug-filter-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_user_group" "src" {
						name       = %q
						group_type = "static"
					}

					data "jamfplatform_pro_user_groups" "lookup" {
						filter = {
							name_substring = jamfplatform_pro_user_group.src.name
						}
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_user_groups.lookup", "user_groups.#", "1"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_user_groups.lookup", "user_groups.0.name", name),
				),
			},
		},
	})
}

// TestAccResource_ProUserGroup_Smart_AllOperators sweeps every operator in
// user_group.ValidOperators by creating a smart group whose single criterion
// uses that operator. Verifies the operator string round-trips through the
// classic XML payload — no transformation, no normalisation, no rejection
// at write-time. Subtests run sequentially with isolated CheckDestroy.
//
// Classic rejects date/numeric operators against built-in attributes that
// it considers string-typed. Two user extension attributes (one Date, one
// Integer) are created via the SDK before the sweep and referenced by name
// for those operator families. member_of / not_member_of use a fixture
// user group created inline in the HCL for those iterations.
func TestAccResource_ProUserGroup_Smart_AllOperators(t *testing.T) {
	testhelpers.AccPreCheck(t)

	suffix := testhelpers.RunSuffix()
	dateEAName := "tf-acc-pro-ug-date-ea-" + suffix
	intEAName := "tf-acc-pro-ug-int-ea-" + suffix

	ctx := context.Background()
	client := proclassic.New(testhelpers.NewAcceptanceClient(t))

	textInput := &proclassic.UserExtensionAttributeInputType{Type: new("Text Field")}

	dateEA, err := client.CreateUserExtensionAttributeByID(ctx, "0", &proclassic.UserExtensionAttribute{
		Name:      new(dateEAName),
		DataType:  new("Date"),
		InputType: textInput,
	})
	if err != nil {
		t.Fatalf("create date EA fixture: %v", err)
	}
	intEA, err := client.CreateUserExtensionAttributeByID(ctx, "0", &proclassic.UserExtensionAttribute{
		Name:      new(intEAName),
		DataType:  new("Integer"),
		InputType: textInput,
	})
	if err != nil {
		if dateEA != nil && dateEA.ID != nil {
			_ = client.DeleteUserExtensionAttributeByID(context.Background(), fmt.Sprintf("%d", *dateEA.ID))
		}
		t.Fatalf("create integer EA fixture: %v", err)
	}
	t.Cleanup(func() {
		cleanCtx := context.Background()
		if dateEA != nil && dateEA.ID != nil {
			_ = client.DeleteUserExtensionAttributeByID(cleanCtx, fmt.Sprintf("%d", *dateEA.ID))
		}
		if intEA != nil && intEA.ID != nil {
			_ = client.DeleteUserExtensionAttributeByID(cleanCtx, fmt.Sprintf("%d", *intEA.ID))
		}
	})

	type opSpec struct {
		op    string
		name  string // criterion's `name` (inventory attribute or EA name)
		value string // criterion's `value`
	}

	// Build the case table from the canonical ValidOperators slice — adding
	// an operator there auto-extends this sweep. Date/numeric operators
	// reference the EA fixtures above; member_of / not_member_of get a
	// placeholder value that is filled in per-iteration with the fixture
	// group name.
	specs := make([]opSpec, 0, len(user_group.ValidOperators))
	for _, op := range user_group.ValidOperators {
		spec := opSpec{op: op, name: "Full Name", value: "tf-acc-sweep"}
		switch op {
		case "before (yyyy-mm-dd)", "after (yyyy-mm-dd)":
			spec.name = dateEAName
			spec.value = "2025-01-01"
		case "more than x days ago", "less than x days ago":
			spec.name = dateEAName
			spec.value = "7"
		case "greater than", "more than", "less than", "greater than or equal", "less than or equal":
			spec.name = intEAName
			spec.value = "10"
		case "matches regex", "does not match regex":
			spec.value = ".*tf-acc.*"
		case "member of", "not member of":
			spec.name = "User Group"
			spec.value = "" // placeholder; populated below
		}
		specs = append(specs, spec)
	}

	for _, spec := range specs {
		t.Run(spec.op, func(t *testing.T) {
			suffix := testhelpers.RunSuffix()
			groupName := "tf-acc-pro-ug-op-" + suffix
			fixtureName := "tf-acc-pro-ug-fixture-" + suffix

			needsFixture := spec.op == "member of" || spec.op == "not member of"
			if needsFixture {
				// The group "member of" / "not member of" operators are rejected
				// before Jamf Pro 11.29; skip just these operators on older tenants.
				testhelpers.RequireMinJamfProVersion(t, "11.29.0")
			}
			value := spec.value
			if needsFixture {
				value = fixtureName
			}

			var cfg string
			if needsFixture {
				cfg = fmt.Sprintf(`
					resource "jamfplatform_pro_user_group" "fixture" {
						name       = %q
						group_type = "static"
					}

					resource "jamfplatform_pro_user_group" "test" {
						name       = %q
						group_type = "smart"

						criteria = [
							{
								name        = %q
								search_type = %q
								value       = %q
							},
						]

						depends_on = [jamfplatform_pro_user_group.fixture]
					}
				`, fixtureName, groupName, spec.name, spec.op, value)
			} else {
				cfg = fmt.Sprintf(`
					resource "jamfplatform_pro_user_group" "test" {
						name       = %q
						group_type = "smart"

						criteria = [
							{
								name        = %q
								search_type = %q
								value       = %q
							},
						]
					}
				`, groupName, spec.name, spec.op, value)
			}

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
				CheckDestroy:             testAccCheckUserGroupDestroy(t),
				Steps: []resource.TestStep{
					{
						Config: cfg,
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "group_type", "smart"),
							resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "criteria.#", "1"),
							resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "criteria.0.search_type", spec.op),
							resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "criteria.0.name", spec.name),
							resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "criteria.0.value", value),
						),
					},
				},
			})
		})
	}
}

// TestAccResource_ProUserGroup_Static_MembersNullVsEmpty asserts the
// null-vs-empty members semantics on static groups:
//
//   - members omitted (null) → SDK sends Users: nil → server preserves
//     existing membership. Critical: the user MUST be able to manage the
//     group's name/notify flags without touching membership.
//   - members = []           → SDK sends an empty Users wrapper → server
//     clears membership.
//   - members = ["id"]       → SDK sends explicit user list → server sets.
//
// Two fixture users are created via the SDK before the resource.Test runs
// (no jamfplatform_pro_user resource exists yet) and cleaned up after. The
// test sequence:
//
//  1. Create group with members = ["u1", "u2"]      → count=2
//  2. Omit members (null)                            → count=2  (preserved!)
//  3. Switch to members = []                         → count=0  (cleared)
//  4. Switch to members = ["u1"]                     → count=1  (set)
func TestAccResource_ProUserGroup_Static_MembersNullVsEmpty(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	groupName := "tf-acc-pro-ug-members-" + suffix
	u1Name := "tf-acc-pro-ug-member-1-" + suffix
	u2Name := "tf-acc-pro-ug-member-2-" + suffix

	// Create fixture users via the SDK — no jamfplatform_pro_user resource
	// exists yet. Cleanup runs after the test regardless of outcome.
	ctx := context.Background()
	client := proclassic.New(testhelpers.NewAcceptanceClient(t))

	u1, err := client.CreateUserByID(ctx, "0", &proclassic.UserPost{Name: new(u1Name)})
	if err != nil {
		t.Fatalf("create fixture user 1: %v", err)
	}
	u2, err := client.CreateUserByID(ctx, "0", &proclassic.UserPost{Name: new(u2Name)})
	if err != nil {
		// Best-effort cleanup of u1 before failing.
		if u1 != nil && u1.ID != nil {
			_ = client.DeleteUserByID(ctx, fmt.Sprintf("%d", *u1.ID))
		}
		t.Fatalf("create fixture user 2: %v", err)
	}
	t.Cleanup(func() {
		if u1.ID != nil {
			_ = client.DeleteUserByID(context.Background(), fmt.Sprintf("%d", *u1.ID))
		}
		if u2.ID != nil {
			_ = client.DeleteUserByID(context.Background(), fmt.Sprintf("%d", *u2.ID))
		}
	})

	u1ID := fmt.Sprintf("%d", *u1.ID)
	u2ID := fmt.Sprintf("%d", *u2.ID)

	configBothMembers := fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "test" {
			name       = %q
			group_type = "static"
			members    = [%q, %q]
		}
	`, groupName, u1ID, u2ID)

	configMembersOmitted := fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "test" {
			name                = %q
			group_type          = "static"
			notify_on_membership_change = true
		}
	`, groupName)

	configMembersEmpty := fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "test" {
			name       = %q
			group_type = "static"
			members    = []
		}
	`, groupName)

	configMembersOne := fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "test" {
			name       = %q
			group_type = "static"
			members    = [%q]
		}
	`, groupName, u1ID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configBothMembers,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "members.#", "2"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "member_count", "2"),
				),
			},
			{
				// CRITICAL: members omitted must NOT clear server-side membership.
				// SDK sends Users: nil → classic preserves the previous list.
				Config: configMembersOmitted,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "notify_on_membership_change", "true"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "member_count", "2"),
					resource.TestCheckNoResourceAttr("jamfplatform_pro_user_group.test", "members"),
				),
			},
			{
				// Explicit empty set must clear membership.
				Config: configMembersEmpty,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "members.#", "0"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "member_count", "0"),
				),
			},
			{
				// Single-member set replaces.
				Config: configMembersOne,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "members.#", "1"),
					resource.TestCheckResourceAttr("jamfplatform_pro_user_group.test", "member_count", "1"),
					resource.TestCheckTypeSetElemAttr("jamfplatform_pro_user_group.test", "members.*", u1ID),
				),
			},
		},
	})
}

const userGroupResourceAddr = "jamfplatform_pro_user_group.test"

// userGroupOmitRetainsConfig is the fully declared shape for the omit-retains
// contract on a static group: members is the one Read-gated attribute this
// resource has (refreshed only when prior state holds it), and it carries a
// real user id rather than an empty set so a server that stopped retaining an
// omitted <users> element is caught on content, not on presence. The
// notify flag is a non-default Optional+Computed leaf kept alongside it.
func userGroupOmitRetainsConfig(name, memberID string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "test" {
			name                        = %q
			group_type                  = "static"
			notify_on_membership_change = true
			members                     = [%q]
		}
	`, name, memberID)
}

// userGroupOmitRetainsMembersDroppedConfig drops members from config, so the
// plan removes it from state and buildUsersWrapper omits <users> from the PUT.
func userGroupOmitRetainsMembersDroppedConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "test" {
			name                        = %q
			group_type                  = "static"
			notify_on_membership_change = true
		}
	`, name)
}

// userGroupOmitRetainsMembersClearedConfig is the documented clear path: an
// explicit empty set sends an empty <users/> wrapper, which the server
// applies. members is Optional-only, so `[]` is the only way to clear it.
func userGroupOmitRetainsMembersClearedConfig(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "test" {
			name                        = %q
			group_type                  = "static"
			notify_on_membership_change = true
			members                     = []
		}
	`, name)
}

// userGroupMembersOnServer asserts the server's copy of the group lists
// exactly wantIDs as members and still carries the step-1 notify flag.
func userGroupMembersOnServer(t *testing.T, wantIDs ...int) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return testhelpers.CheckLiveObject(userGroupResourceAddr,
		func(ctx context.Context, id string) (*proclassic.UserGroup, error) {
			return c.GetUserGroupByID(ctx, id)
		},
		func(ug *proclassic.UserGroup) error {
			if err := testhelpers.RequireEqual("is_notify_on_change", true, testhelpers.Deref(ug.IsNotifyOnChange)); err != nil {
				return err
			}
			var gotIDs []int
			if ug.Users != nil && ug.Users.User != nil {
				for _, u := range *ug.Users.User {
					gotIDs = append(gotIDs, testhelpers.Deref(u.ID))
				}
			}
			if err := testhelpers.RequireEqual("users.size", len(wantIDs), len(gotIDs)); err != nil {
				return err
			}
			for _, want := range wantIDs {
				if !slices.Contains(gotIDs, want) {
					return fmt.Errorf("users: want member %d, got %v", want, gotIDs)
				}
			}
			return nil
		})
}

// TestAccResource_ProUserGroup_OmittedBlocksRetained pins the omit-retains
// contract on the one Read-gated attribute a static user group has: dropping
// members from config plans it as removed, the classic PUT omits <users>, and
// the server keeps the membership. Terraform state cannot witness that, so the
// assertion runs against the wire object after every step. Step 3 then
// asserts the documented clear path — an explicit `members = []` — really does
// empty the group, so the two shapes are pinned side by side. Each step's
// implicit post-apply plan must be empty. If step 2 fails on content, the
// endpoint no longer merges and nothing that suppresses the removal plan may
// ship for this resource.
//
// The member fixture is a classic user created through the SDK, as
// TestAccResource_ProUserGroup_Static_MembersNullVsEmpty does: there is no
// jamfplatform_pro_user resource. t.Cleanup runs after the framework's destroy,
// so the group is gone before the user is deleted.
func TestAccResource_ProUserGroup_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	groupName := "tf-acc-pro-ug-omit-" + suffix
	userName := "tf-acc-pro-ug-omit-member-" + suffix

	ctx := context.Background()
	client := proclassic.New(testhelpers.NewAcceptanceClient(t))

	member, err := client.CreateUserByID(ctx, "0", &proclassic.UserPost{Name: new(userName)})
	if err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	if member == nil || member.ID == nil {
		t.Fatal("create fixture user: response carried no id")
	}
	t.Cleanup(func() {
		_ = client.DeleteUserByID(context.Background(), fmt.Sprintf("%d", *member.ID))
	})
	memberID := fmt.Sprintf("%d", *member.ID)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: userGroupOmitRetainsConfig(groupName, memberID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(userGroupResourceAddr, "members.#", "1"),
					resource.TestCheckTypeSetElemAttr(userGroupResourceAddr, "members.*", memberID),
					resource.TestCheckResourceAttr(userGroupResourceAddr, "member_count", "1"),
					userGroupMembersOnServer(t, *member.ID),
				),
			},
			{
				Config: userGroupOmitRetainsMembersDroppedConfig(groupName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(userGroupResourceAddr, "members"),
					resource.TestCheckResourceAttr(userGroupResourceAddr, "member_count", "1"),
					userGroupMembersOnServer(t, *member.ID),
				),
			},
			{
				Config: userGroupOmitRetainsMembersClearedConfig(groupName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(userGroupResourceAddr, "members.#", "0"),
					resource.TestCheckResourceAttr(userGroupResourceAddr, "member_count", "0"),
					userGroupMembersOnServer(t),
				),
			},
		},
	})
}

// TestAccListResource_ProUserGroup_Basic exercises the jamfplatform_pro_user_group
// list resource via the `terraform query` workflow.
func TestAccListResource_ProUserGroup_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-ug-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_user_group" "src" {
						name       = %q
						group_type = "static"
					}
				`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_user_group.src", "id"),
				),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_user_group" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_user_group.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_user_group.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							{Path: tfjsonpath.New("group_type"), KnownValue: knownvalue.StringExact("static")},
						},
					),
				},
			},
		},
	})
}
