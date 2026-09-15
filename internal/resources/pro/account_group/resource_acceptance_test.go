// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /accounts/groupid endpoint and
// the Pro v1 /account-groups endpoint. Classic has known concurrency issues when
// multiple writes hit the same resource type — keep these serial.

package account_group_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func testAccCheckAccountGroupDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_account_group" {
				continue
			}
			_, err := c.GetAccountGroupByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking account group %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("account group %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// customGroupConfig builds a Custom-privilege-set group with the given
// jamf_pro_server_objects privileges.
func customGroupConfig(displayName string, objects ...string) string {
	var quoted strings.Builder
	for i, p := range objects {
		if i > 0 {
			quoted.WriteString(", ")
		}
		fmt.Fprintf(&quoted, "%q", p)
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_account_group" "test" {
  display_name  = %q
  access_level  = "Full Access"
  privilege_set = "Custom"
  privileges = {
    jamf_pro_server_objects = [%s]
  }
}
`, displayName, quoted.String())
}

// TestAccResource_ProAccountGroup exercises the full lifecycle plus the privilege
// intersect-on-read behaviour: a privilege set is grown then shrunk, and state
// must reflect exactly the declared set each time (proving server-added
// dependency privileges are reconciled out and removals are honoured).
func TestAccResource_ProAccountGroup(t *testing.T) {
	name := fmt.Sprintf("tf-acc-account-group-%d", os.Getpid())
	renamed := name + "-renamed"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAccountGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				// Create with two privileges.
				Config: customGroupConfig(name, "Read Computers", "Update Computers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_account_group.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_account_group.test", "display_name", name),
					resource.TestCheckResourceAttr("jamfplatform_pro_account_group.test", "privilege_set", "Custom"),
					resource.TestCheckResourceAttr("jamfplatform_pro_account_group.test", "privileges.jamf_pro_server_objects.#", "2"),
					resource.TestCheckTypeSetElemAttr("jamfplatform_pro_account_group.test", "privileges.jamf_pro_server_objects.*", "Read Computers"),
					resource.TestCheckTypeSetElemAttr("jamfplatform_pro_account_group.test", "privileges.jamf_pro_server_objects.*", "Update Computers"),
				),
			},
			{
				// Shrink privileges (remove Update Computers) and rename. State
				// must show exactly the declared single privilege — removals work
				// and server-added dependencies do not leak in.
				Config: customGroupConfig(renamed, "Read Computers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_account_group.test", "display_name", renamed),
					resource.TestCheckResourceAttr("jamfplatform_pro_account_group.test", "privileges.jamf_pro_server_objects.#", "1"),
					resource.TestCheckTypeSetElemAttr("jamfplatform_pro_account_group.test", "privileges.jamf_pro_server_objects.*", "Read Computers"),
				),
			},
			{
				// Import smoke. ImportStateVerify is intentionally omitted: import
				// faithfully materialises the full server privilege grid (a
				// superset of the declared subset), so a full-fidelity compare
				// against the prior managed state would not match by design.
				ResourceName:      "jamfplatform_pro_account_group.test",
				ImportState:       true,
				ImportStateVerify: false,
			},
		},
	})
}

// TestAccResource_ProAccountGroup_NonCustom verifies a preset privilege set
// (Auditor) with no privileges block, and the Pro v1 data source read.
func TestAccResource_ProAccountGroup_NonCustom(t *testing.T) {
	name := fmt.Sprintf("tf-acc-account-group-auditor-%d", os.Getpid())
	config := fmt.Sprintf(`
resource "jamfplatform_pro_account_group" "auditor" {
  display_name  = %q
  access_level  = "Full Access"
  privilege_set = "Auditor"
}

data "jamfplatform_pro_account_group" "by_name" {
  display_name = jamfplatform_pro_account_group.auditor.display_name
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAccountGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_account_group.auditor", "privilege_set", "Auditor"),
					// DS is classic-sourced (same spellings as the resource).
					resource.TestCheckResourceAttr("data.jamfplatform_pro_account_group.by_name", "privilege_set", "Auditor"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_account_group.by_name", "access_level", "Full Access"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_account_group.by_name", "id", "jamfplatform_pro_account_group.auditor", "id"),
				),
			},
			// A preset privilege_set is where the read used to commit privileges
			// the tenant will not grant. Jamf Pro expands "Auditor" into a full
			// privilege list, and on an 11.x tenant that list names "Read Knobs",
			// which the same tenant refuses — so the generated configuration was
			// rejected by this resource's own ModifyPlan validator and the plan
			// could not run. The expansion is no longer adopted at all for a
			// preset set, because no write can change it, so the generated
			// configuration carries no privileges block; GenerateConfigStep
			// requires it to plan as a no-op import, which is what both halves of
			// that (adopting nothing, validating nothing) have to agree on.
			testhelpers.GenerateConfigStep("jamfplatform_pro_account_group.auditor"),
		},
	})
}

// accountGroupMembersConfig builds an account group plus a jamfplatform_pro_account
// fixture used as a real member (withMember adds it, else members = []). The
// account is present in both steps so it is created once; the group clears the
// member before the account is destroyed. The account mirrors the proven
// jamfplatform_pro_account acceptance shape (unique email + Custom privilege set
// with a privileges block — an empty email or the Administrator set fail create).
func accountGroupMembersConfig(name, suffix string, withMember bool) string {
	members := "[]"
	if withMember {
		members = "[jamfplatform_pro_account.member.id]"
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_account" "member" {
  username      = "tf-acc-ag-member-%[2]s"
  full_name     = "TF Acc AG Member"
  email_address = "tf-acc-ag-member-%[2]s@example.invalid"
  access_level  = "Full Access"
  privilege_set = "Custom"

  password            = "Pr0bePassw0rd-%[2]s"
  password_wo_version = 1

  privileges = {
    jamf_pro_server_objects = ["Read Computers"]
  }
}

resource "jamfplatform_pro_account_group" "members" {
  display_name  = %[1]q
  access_level  = "Full Access"
  privilege_set = "Auditor"
  members       = %[3]s
}
`, name, suffix, members)
}

// TestAccResource_ProAccountGroup_Members exercises membership add and remove
// against a self-provisioned jamfplatform_pro_account member fixture.
func TestAccResource_ProAccountGroup_Members(t *testing.T) {
	name := fmt.Sprintf("tf-acc-account-group-members-%d", os.Getpid())
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAccountGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: accountGroupMembersConfig(name, suffix, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_account_group.members", "members.#", "1"),
					resource.TestCheckTypeSetElemAttrPair("jamfplatform_pro_account_group.members", "members.*", "jamfplatform_pro_account.member", "id"),
				),
			},
			{
				Config: accountGroupMembersConfig(name, suffix, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_account_group.members", "members.#", "0"),
				),
			},
		},
	})
}

const accountGroupLdapAddr = "jamfplatform_pro_account_group.ldap"

// accountGroupLdapConfig declares an anonymous Open Directory server fixture
// (Jamf Pro stores it without contacting the host) and a group that either
// references it through ldap_server_id or, when withServer is false, drops
// the attribute while keeping the fixture alive for the wire assertion.
func accountGroupLdapConfig(name, suffix string, withServer bool) string {
	ldap := ""
	if withServer {
		ldap = "ldap_server_id = jamfplatform_pro_ldap_server.fixture.id"
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_ldap_server" "fixture" {
  connection_settings = {
    display_name        = "tf-acc-ag-ldap-%[2]s"
    directory_service   = "Open Directory"
    hostname            = "ldap.acc-ag-%[2]s.example.com"
    port                = 389
    use_ssl             = false
    authentication_type = "none"
  }
}

resource "jamfplatform_pro_account_group" "ldap" {
  depends_on    = [jamfplatform_pro_ldap_server.fixture]
  display_name  = %[1]q
  access_level  = "Full Access"
  privilege_set = "Auditor"
  %[3]s
}
`, name, suffix, ldap)
}

// accountGroupLive fetches the server's copy of the LDAP-backed group under
// test and hands it to assert, so the drop step can prove on the wire that the
// -1 clear token removed the directory server — a null in state cannot tell a
// cleared association from one the classic merge retained and the state
// builder read as absent (#384).
func accountGroupLive(t *testing.T, assert func(*proclassic.Group) error) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return testhelpers.CheckLiveObject(accountGroupLdapAddr, c.GetAccountGroupByID, assert)
}

// TestAccResource_ProAccountGroup_LdapServer binds a group to a directory
// server, then drops ldap_server_id and asserts the association is gone in
// state and on the wire.
func TestAccResource_ProAccountGroup_LdapServer(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-account-group-ldap-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAccountGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: accountGroupLdapConfig(name, suffix, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(accountGroupLdapAddr, "ldap_server_id", "jamfplatform_pro_ldap_server.fixture", "id"),
					resource.TestCheckResourceAttr(accountGroupLdapAddr, "ldap_server_name", "tf-acc-ag-ldap-"+suffix),
					accountGroupLive(t, func(g *proclassic.Group) error {
						if g.LdapServer == nil || g.LdapServer.ID == nil {
							return fmt.Errorf("ldap_server: absent, want the fixture")
						}
						return nil
					}),
				),
			},
			{
				Config: accountGroupLdapConfig(name, suffix, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(accountGroupLdapAddr, "ldap_server_id"),
					resource.TestCheckNoResourceAttr(accountGroupLdapAddr, "ldap_server_name"),
					accountGroupLive(t, func(g *proclassic.Group) error {
						if g.LdapServer != nil && g.LdapServer.ID != nil && *g.LdapServer.ID > 0 {
							return fmt.Errorf("ldap_server: want none, got id %d", *g.LdapServer.ID)
						}
						return nil
					}),
				),
			},
		},
	})
}

const accountGroupOmitRetainsAddr = "jamfplatform_pro_account_group.omit"

// accountGroupOmitRetainsFixture is the jamfplatform_pro_account member every
// step of the omit-retains test carries, so the account is created once and
// outlives the group. It mirrors the proven accountGroupMembersConfig shape.
// The steps that stop managing members keep the fixture through depends_on,
// which also orders the destroy (group first, account second) so a member is
// never deleted out from under a group that still lists it.
func accountGroupOmitRetainsFixture(suffix string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_account" "omit_member" {
  username      = "tf-acc-ag-omit-%[1]s"
  full_name     = "TF Acc AG Omit Member"
  email_address = "tf-acc-ag-omit-%[1]s@example.invalid"
  access_level  = "Full Access"
  privilege_set = "Custom"

  password            = "Pr0bePassw0rd-%[1]s"
  password_wo_version = 1

  privileges = {
    jamf_pro_server_objects = ["Read Computers"]
  }
}
`, suffix)
}

// accountGroupOmitRetainsConfig is the fully declared shape for the omit-retains
// contract: three privilege categories, a managed member and the
// Optional+Computed site_id, each carrying a distinctive value so a server that
// stopped retaining an omitted element is caught on content, not presence.
func accountGroupOmitRetainsConfig(name, suffix string) string {
	return accountGroupOmitRetainsFixture(suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_account_group" "omit" {
  display_name  = %q
  access_level  = "Full Access"
  privilege_set = "Custom"
  site_id       = -1
  members       = [jamfplatform_pro_account.omit_member.id]

  privileges = {
    jamf_pro_server_objects  = ["Read Buildings", "Read Departments"]
    jamf_pro_server_settings = ["Read SMTP Server"]
    jamf_pro_server_actions  = ["Send Computer Remote Lock Command"]
  }
}
`, name)
}

// accountGroupOmitRetainsObjectsOnlyConfig keeps the privileges block but
// declares only jamf_pro_server_objects, and drops members and site_id, so the
// PUT carries a <privileges> element missing two categories and no <members>.
func accountGroupOmitRetainsObjectsOnlyConfig(name, suffix string) string {
	return accountGroupOmitRetainsFixture(suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_account_group" "omit" {
  display_name  = %q
  access_level  = "Full Access"
  privilege_set = "Custom"

  privileges = {
    jamf_pro_server_objects = ["Read Buildings", "Read Departments"]
  }

  depends_on = [jamfplatform_pro_account.omit_member]
}
`, name)
}

// accountGroupOmitRetainsHeaderOnlyConfig drops every optional attribute, so the
// PUT carries the three required scalars alone.
func accountGroupOmitRetainsHeaderOnlyConfig(name, suffix string) string {
	return accountGroupOmitRetainsFixture(suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_account_group" "omit" {
  display_name  = %q
  access_level  = "Full Access"
  privilege_set = "Custom"

  depends_on = [jamfplatform_pro_account.omit_member]
}
`, name)
}

// hasPrivilege reports whether a classic privilege category carries want. The
// server silently expands a submitted grid with dependency privileges, so the
// retained-on-server assertion is containment, never equality.
func hasPrivilege(category *[]string, want string) bool {
	if category == nil {
		return false
	}
	return slices.Contains(*category, want)
}

// privilegeList renders a classic privilege category for a diagnostic, so a
// missing category reads as "absent" rather than a struct pointer.
func privilegeList[T any](category *T, privileges func(*T) *[]string) []string {
	if category == nil {
		return nil
	}
	return testhelpers.Deref(privileges(category))
}

// accountGroupRetainedOnServer asserts the server's copy still carries every
// privilege and the member the omit-retains config declared in its first step.
// The member's id is read from the fixture's state entry because the classic
// response identifies members by account id, not by the group.
func accountGroupRetainedOnServer(t *testing.T) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return func(s *terraform.State) error {
		member, ok := s.RootModule().Resources["jamfplatform_pro_account.omit_member"]
		if !ok {
			return fmt.Errorf("member fixture jamfplatform_pro_account.omit_member not found in state")
		}
		wantMember, err := strconv.Atoi(member.Primary.ID)
		if err != nil {
			return fmt.Errorf("member fixture id %q is not an integer: %w", member.Primary.ID, err)
		}
		return testhelpers.CheckLiveObject(accountGroupOmitRetainsAddr,
			func(ctx context.Context, id string) (*proclassic.Group, error) {
				return c.GetAccountGroupByID(ctx, id)
			},
			func(g *proclassic.Group) error {
				if g.Privileges == nil {
					return fmt.Errorf("privileges: absent")
				}
				if g.Privileges.JssObjects == nil {
					return fmt.Errorf("privileges.jamf_pro_server_objects: absent")
				}
				for _, want := range []string{"Read Buildings", "Read Departments"} {
					if !hasPrivilege(g.Privileges.JssObjects.Privilege, want) {
						return fmt.Errorf("privileges.jamf_pro_server_objects: want %q, got %v", want, testhelpers.Deref(g.Privileges.JssObjects.Privilege))
					}
				}
				if g.Privileges.JssSettings == nil || !hasPrivilege(g.Privileges.JssSettings.Privilege, "Read SMTP Server") {
					return fmt.Errorf("privileges.jamf_pro_server_settings: want %q, got %v", "Read SMTP Server", privilegeList(g.Privileges.JssSettings, func(c *proclassic.GroupPrivilegesJssSettings) *[]string { return c.Privilege }))
				}
				if g.Privileges.JssActions == nil || !hasPrivilege(g.Privileges.JssActions.Privilege, "Send Computer Remote Lock Command") {
					return fmt.Errorf("privileges.jamf_pro_server_actions: want %q, got %v", "Send Computer Remote Lock Command", privilegeList(g.Privileges.JssActions, func(c *proclassic.GroupPrivilegesJssActions) *[]string { return c.Privilege }))
				}
				if g.Members == nil || g.Members.User == nil || len(*g.Members.User) != 1 {
					return fmt.Errorf("members: want exactly one member, got %+v", g.Members)
				}
				if err := testhelpers.RequireEqual("members[0].id", wantMember, testhelpers.Deref((*g.Members.User)[0].ID)); err != nil {
					return err
				}
				if g.Site == nil {
					return fmt.Errorf("site: absent")
				}
				return testhelpers.RequireEqual("site.id", -1, testhelpers.Deref(g.Site.ID))
			})(s)
	}
}

// TestAccResource_ProAccountGroup_OmittedBlocksRetained pins the omit-retains
// contract the plan output cannot show: dropping members, two privilege
// categories, and finally the whole privileges block from config plans them as
// removed, but the classic /accounts/groupid PUT omits the elements and the
// server keeps every value. Step 2 keeps the privileges block with one category
// so the PUT carries a partial <privileges>, exercising the category-level
// merge the resource documents; step 3 drops privileges too so the PUT carries
// the header alone. Each step's implicit post-apply plan must be empty, which
// is what makes the contract usable.
//
// Step 2 is the step the wire cannot satisfy on its own: probed 2026-09-06 on
// Jamf Pro 11.31.1, a sent <privileges> replaces the whole grid, so a partial
// body empties the two undeclared categories while Read's null-means-unmanaged
// gate hides the loss (issue #385). The resource now emulates the retention it
// documents — Update reads the live grid and re-emits the undeclared categories
// through accountprivileges.MergeGrid — and this step is the proof: the
// server-side check after step 2 must still find "Read SMTP Server" and the
// remote-lock action that only the carry-over could have sent. The whole-block
// omission in step 3 and the members omission retain on the wire itself.
func TestAccResource_ProAccountGroup_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-account-group-omit-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAccountGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: accountGroupOmitRetainsConfig(name, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accountGroupOmitRetainsAddr, "members.#", "1"),
					resource.TestCheckResourceAttr(accountGroupOmitRetainsAddr, "privileges.jamf_pro_server_settings.#", "1"),
					resource.TestCheckResourceAttr(accountGroupOmitRetainsAddr, "privileges.jamf_pro_server_actions.#", "1"),
					accountGroupRetainedOnServer(t),
				),
			},
			{
				Config: accountGroupOmitRetainsObjectsOnlyConfig(name, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(accountGroupOmitRetainsAddr, "members.#"),
					resource.TestCheckNoResourceAttr(accountGroupOmitRetainsAddr, "privileges.jamf_pro_server_settings.#"),
					resource.TestCheckNoResourceAttr(accountGroupOmitRetainsAddr, "privileges.jamf_pro_server_actions.#"),
					resource.TestCheckResourceAttr(accountGroupOmitRetainsAddr, "privileges.jamf_pro_server_objects.#", "2"),
					resource.TestCheckResourceAttr(accountGroupOmitRetainsAddr, "site_id", "-1"),
					accountGroupRetainedOnServer(t),
				),
			},
			{
				Config: accountGroupOmitRetainsHeaderOnlyConfig(name, suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(accountGroupOmitRetainsAddr, "privileges.jamf_pro_server_objects.#"),
					accountGroupRetainedOnServer(t),
				),
			},
		},
	})
}
