// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests talk to the Jamf ProClassic /vppinvitations endpoint (user-based VPP).
// Keep serial with other classic acceptance work in this domain.
//
// Writes are a MERGE; scope follows per-category granular ownership: a declared
// category (including explicit `[]`, which clears) is owned by Terraform, an
// omitted category is preserved via read-merge-write on update. The update
// steps mutate scalars, toggle auto_register, and add/remove a scope group to
// exercise the declared-category full-replace path. Email-mode fields only
// persist for distribution_method = "Send emails"; a dedicated test toggles in
// and out.
//
// Apply tests provision their own VPP account via a jamfplatform_pro_volume_-
// purchasing_location fixture, so they are gated on JAMFPLATFORM_ACC_PRO_VPP_TOKEN (a real
// ABM/ASM .vpptoken — same gate as the location + mac_app VPP tests). The
// location's id is the VPP account id the invitation references. Token material
// MUST come from env — never commit it.
//
// The pure plan-time validation tests (ExpectError) need no account or token —
// they use a literal vpp_account_id and never apply.
//
// Directory-service-group tests stand up the shared Okta LDAP server fixture via
// the SDK (so the directory exists before the plan-time scope preflight) and use
// JAMFPLATFORM_ACC_PRO_LDAP_GROUP_NAME for the real group name.
//
// NOTE on email message: the classic API form-decodes the <message> field (a
// bare `%` 500s), so the provider form-URL-encodes it; the email test uses a `%@`
// + newline + literal-`%` message to exercise the verbatim round-trip.

package vpp_invitation_test

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const resAddr = "jamfplatform_pro_vpp_invitation.test"

// vppTokenEnvVar holds the base64 `.vpptoken` contents used to stand up a VPP
// location fixture (which owns the VPP account the invitation references).
const vppTokenEnvVar = "JAMFPLATFORM_ACC_PRO_VPP_TOKEN"

func vppToken(t *testing.T) string {
	v := testhelpers.AccEnv(vppTokenEnvVar)
	if v == "" {
		t.Skipf("%s not set; skipping VPP invitation acceptance test (needs a VPP location fixture)", vppTokenEnvVar)
	}
	return v
}

func testAccCheckVPPInvitationDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_vpp_invitation" {
				continue
			}
			_, err := c.GetVPPInvitationByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking VPP invitation %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("VPP invitation %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// vppLocationFixture stands up a VPP location from the token; its id is the VPP
// account the invitation registers against. auto_register_managed_users is
// enabled on the location because an invitation can only set
// auto_register_managed_users = true when its location has it enabled (the
// server otherwise 409s "not enabled on Vpp Location").
func vppLocationFixture(token, suffix string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_volume_purchasing_location" "vpp" {
  name                                     = "tf-acc-vpp-loc-%[2]s"
  service_token                            = %[1]q
  service_token_wo_version                 = 1
  automatically_populate_purchased_content = true
  auto_register_managed_users              = true
}
`, token, suffix)
}

// lifecycleConfig builds an invitation plus the location fixture and two static
// user-group fixtures so scope targets reference real IDs. groupCount selects how
// many target groups are in scope (1 or 2) to exercise add/remove.
func lifecycleConfig(token, suffix, name, distMethod string, autoReg bool, groupCount int) string {
	target := "jamfplatform_pro_user_group.a.id"
	if groupCount == 2 {
		target = "jamfplatform_pro_user_group.a.id, jamfplatform_pro_user_group.b.id"
	}
	return vppLocationFixture(token, suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_user_group" "a" {
  name       = "%[1]s-grp-a"
  group_type = "static"
}

resource "jamfplatform_pro_user_group" "b" {
  name       = "%[1]s-grp-b"
  group_type = "static"
}

resource "jamfplatform_pro_vpp_invitation" "test" {
  name                        = %[1]q
  vpp_account_id              = jamfplatform_pro_volume_purchasing_location.vpp.id
  distribution_method         = %[2]q
  auto_register_managed_users = %[3]t

  scope = {
    targets = {
      jss_user_group_ids = [%[4]s]
    }

    exclusions = {
      jss_user_group_ids = [jamfplatform_pro_user_group.b.id]
    }
  }
}
`, name, distMethod, autoReg, target)
}

func TestAccResource_ProVPPInvitation(t *testing.T) {
	token := vppToken(t)
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-vpp-" + suffix
	renamed := "tf-acc-vpp-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVPPInvitationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: lifecycleConfig(token, suffix, name, "Make available in Self Service only", true, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resAddr, "id"),
					resource.TestCheckResourceAttr(resAddr, "name", name),
					resource.TestCheckResourceAttrPair(resAddr, "vpp_account_id", "jamfplatform_pro_volume_purchasing_location.vpp", "id"),
					resource.TestCheckResourceAttr(resAddr, "distribution_method", "Make available in Self Service only"),
					resource.TestCheckResourceAttr(resAddr, "auto_register_managed_users", "true"),
					resource.TestCheckResourceAttr(resAddr, "scope.targets.jss_user_group_ids.#", "1"),
					resource.TestCheckResourceAttr(resAddr, "scope.exclusions.jss_user_group_ids.#", "1"),
				),
			},
			{
				// Merge update: rename, change distribution_method, toggle auto_register,
				// add the second target group (nested-set growth).
				Config: lifecycleConfig(token, suffix, renamed, "Prompt users to accept/make available in Self Service", false, 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "name", renamed),
					resource.TestCheckResourceAttr(resAddr, "distribution_method", "Prompt users to accept/make available in Self Service"),
					resource.TestCheckResourceAttr(resAddr, "auto_register_managed_users", "false"),
					resource.TestCheckResourceAttr(resAddr, "scope.targets.jss_user_group_ids.#", "2"),
				),
			},
			{
				// Shrink the target set back to one (nested-set removal — the declared
				// category is owned, so its members full-replace).
				Config: lifecycleConfig(token, suffix, renamed, "Prompt users to accept/make available in Self Service", false, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "scope.targets.jss_user_group_ids.#", "1"),
				),
			},
			{
				ResourceName:      resAddr,
				ImportState:       true,
				ImportStateVerify: true,
				// Import hydrates every scope category; apply keeps declared-only, so
				// verify against this subset-scope config must ignore scope.
				ImportStateVerifyIgnore: []string{"timeouts", "scope"},
			},
		},
	})
}

// emailConfig builds an email-distribution invitation (all_jss_users scope, no
// real recipients → no dispatch). When sendEmails is false it switches to a
// Self-Service method and drops the email fields to verify the server clears
// them without a perpetual diff. The message may contain `%@` and literal `%`
// (the provider form-URL-encodes it for the server's form-decode).
func emailConfig(token, suffix, name, message string, sendEmails bool) string {
	if !sendEmails {
		return vppLocationFixture(token, suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_vpp_invitation" "test" {
  name                = %[1]q
  vpp_account_id      = jamfplatform_pro_volume_purchasing_location.vpp.id
  distribution_method = "Make available in Self Service only"

  scope = {
    targets = {
      all_jss_users = true
    }
  }
}
`, name)
	}
	return vppLocationFixture(token, suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_vpp_invitation" "test" {
  name                 = %[1]q
  vpp_account_id       = jamfplatform_pro_volume_purchasing_location.vpp.id
  distribution_method  = "Send emails"
  sender_name          = "IT Support"
  sender_email_address = "it-support@example.com"
  subject              = "Register with Volume Purchasing"
  message              = %[2]q
  require_login        = true

  scope = {
    targets = {
      all_jss_users = true
    }
  }
}
`, name, message)
}

func TestAccResource_ProVPPInvitation_EmailMode(t *testing.T) {
	token := vppToken(t)
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-vpp-email-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVPPInvitationDestroy(t),
		Steps: []resource.TestStep{
			{
				// The message carries the `%@` registration-URL placeholder and a
				// newline + literal `%` — the provider form-URL-encodes it so the
				// server's form-decode round-trips it verbatim (the field 500s on a
				// bare `%`). State must reflect the exact original.
				Config: emailConfig(token, suffix, name, "Click to register:\n\n%@\n\n(100% done)", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "distribution_method", "Send emails"),
					resource.TestCheckResourceAttr(resAddr, "sender_name", "IT Support"),
					resource.TestCheckResourceAttr(resAddr, "sender_email_address", "it-support@example.com"),
					resource.TestCheckResourceAttr(resAddr, "subject", "Register with Volume Purchasing"),
					resource.TestCheckResourceAttr(resAddr, "message", "Click to register:\n\n%@\n\n(100% done)"),
					resource.TestCheckResourceAttr(resAddr, "require_login", "true"),
				),
			},
			{
				// Mutate the email body (every non-RequiresReplace email field is
				// updatable); keep a %@ placeholder to re-exercise the encode path.
				Config: emailConfig(token, suffix, name, "Updated — register here: %@", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "message", "Updated — register here: %@"),
				),
			},
			{
				// Toggle out of email mode: the server drops the email fields; state
				// must reconcile to null with no perpetual diff.
				Config: emailConfig(token, suffix, name, "", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "distribution_method", "Make available in Self Service only"),
					resource.TestCheckNoResourceAttr(resAddr, "sender_name"),
					resource.TestCheckNoResourceAttr(resAddr, "subject"),
					resource.TestCheckNoResourceAttr(resAddr, "message"),
				),
			},
		},
	})
}

func TestAccDataSource_ProVPPInvitation_BySelectors(t *testing.T) {
	token := vppToken(t)
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-vpp-ds-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVPPInvitationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: lifecycleConfig(token, suffix, name, "Make available in Self Service only", true, 1) + `
data "jamfplatform_pro_vpp_invitation" "by_id" {
  id = jamfplatform_pro_vpp_invitation.test.id
}
data "jamfplatform_pro_vpp_invitation" "by_name" {
  name = jamfplatform_pro_vpp_invitation.test.name
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_vpp_invitation.by_id", "name", resAddr, "name"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_vpp_invitation.by_name", "id", resAddr, "id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_vpp_invitation.by_id", "distribution_method", "Make available in Self Service only"),
				),
			},
		},
	})
}

func TestAccResource_ProVPPInvitation_DriftRecovery(t *testing.T) {
	token := vppToken(t)
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-vpp-drift-" + suffix
	cfg := lifecycleConfig(token, suffix, name, "Make available in Self Service only", true, 1)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVPPInvitationDestroy(t),
		Steps: []resource.TestStep{
			{Config: cfg, Check: resource.TestCheckResourceAttrSet(resAddr, "id")},
			{
				PreConfig: func() {
					c := proclassic.New(testhelpers.NewAcceptanceClient(t))
					ctx := context.Background()
					listed, err := c.ListVPPInvitations(ctx)
					if err != nil {
						t.Fatalf("drift preconfig list: %v", err)
					}
					for _, item := range listed.VppInvitations {
						if item.Name != nil && *item.Name == name && item.ID != nil {
							if err := c.DeleteVPPInvitationByID(ctx, helpers.StringValueFromIntPtr(item.ID).ValueString()); err != nil {
								t.Fatalf("drift preconfig delete: %v", err)
							}
						}
					}
				},
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resAddr, "id"),
					resource.TestCheckResourceAttr(resAddr, "name", name),
				),
			},
		},
	})
}

// TestAccResource_ProVPPInvitation_DSGroupScope exercises a real directory-service
// group in both limitations and exclusions. Requires both a VPP token (for the
// account fixture) and a real LDAP group name.
func TestAccResource_ProVPPInvitation_DSGroupScope(t *testing.T) {
	token := vppToken(t)
	ldapEnv := testhelpers.RequireOktaLdapEnv(t)
	group := testhelpers.RequireLdapGroupName(t)
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-vpp-dsscope-" + suffix
	// Plan-time scope preflight resolves the group, so the directory must exist
	// before plan — pre-create it via the SDK (not an in-config fixture).
	testhelpers.EnsureLdapServerFixture(t, name, ldapEnv)
	// Wait until the fresh fixture's bind is up so the plan-time scope preflight
	// resolves the group instead of failing "not found".
	testhelpers.WaitForLdapGroupResolvable(t, group)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVPPInvitationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: vppLocationFixture(token, suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_vpp_invitation" "test" {
  name                = %[1]q
  vpp_account_id      = jamfplatform_pro_volume_purchasing_location.vpp.id
  distribution_method = "Make available in Self Service only"

  scope = {
    limitations = {
      directory_service_user_group_names = [%[2]q]
    }
    exclusions = {
      directory_service_user_group_names = [%[2]q]
    }
  }
}
`, name, group),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "scope.limitations.directory_service_user_group_names.#", "1"),
					resource.TestCheckResourceAttr(resAddr, "scope.exclusions.directory_service_user_group_names.#", "1"),
				),
			},
			{
				// Detach the DS group from both limitations and exclusions BEFORE
				// the framework destroys the invitation, via an empty set `[]` (the
				// natural "remove all" gesture). Destroying while a DS group is still
				// scoped can leave an orphaned scope->LDAP association that blocks the
				// LDAP server's deletion (a server-side data-integrity bug). `[]` is
				// the declared-clear gesture (plans as an empty set, emits an explicit
				// empty element — omission would preserve the group); the post-step
				// empty-plan check enforces the clear round-tripped.
				Config: vppLocationFixture(token, suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_vpp_invitation" "test" {
  name                = %[1]q
  vpp_account_id      = jamfplatform_pro_volume_purchasing_location.vpp.id
  distribution_method = "Make available in Self Service only"

  scope = {
    limitations = {
      directory_service_user_group_names = []
    }
    exclusions = {
      directory_service_user_group_names = []
    }
  }
}
`, name),
				Check: resource.TestCheckResourceAttrSet(resAddr, "id"),
			},
		},
	})
}

// vppiOmitRetainsConfig is the fully declared shape for the omit-retains
// contract: the one state-gated block this resource has is scope, so it
// carries a target group and an exclusion group, each a distinctive tenant id
// so that a server which stopped retaining an omitted element is caught on
// content, not on presence. The directory-service limitation category is not
// declared because it needs a real LDAP group and leaving one scoped through
// destroy orphans the scope->LDAP association (see
// TestAccResource_ProVPPInvitation_DSGroupScope). The group fixtures are
// carried by every step so the server never sees a scoped group disappear
// underneath the invitation.
func vppiOmitRetainsConfig(token, suffix, name string) string {
	return vppiOmitRetainsFixtures(token, suffix, name) + fmt.Sprintf(`
resource "jamfplatform_pro_vpp_invitation" "test" {
  name                        = %[1]q
  vpp_account_id              = jamfplatform_pro_volume_purchasing_location.vpp.id
  distribution_method         = "Prompt users to accept/make available in Self Service"
  auto_register_managed_users = true

  scope = {
    targets = {
      jss_user_group_ids = [jamfplatform_pro_user_group.a.id]
    }
    exclusions = {
      jss_user_group_ids = [jamfplatform_pro_user_group.b.id]
    }
  }

  depends_on = [jamfplatform_pro_user_group.a, jamfplatform_pro_user_group.b]
}
`, name)
}

// vppiOmitRetainsChildrenDroppedConfig keeps the scope block but drops its
// exclusions, which the granular merge re-emits from the server's copy.
func vppiOmitRetainsChildrenDroppedConfig(token, suffix, name string) string {
	return vppiOmitRetainsFixtures(token, suffix, name) + fmt.Sprintf(`
resource "jamfplatform_pro_vpp_invitation" "test" {
  name                        = %[1]q
  vpp_account_id              = jamfplatform_pro_volume_purchasing_location.vpp.id
  distribution_method         = "Prompt users to accept/make available in Self Service"
  auto_register_managed_users = true

  scope = {
    targets = {
      jss_user_group_ids = [jamfplatform_pro_user_group.a.id]
    }
  }

  depends_on = [jamfplatform_pro_user_group.a, jamfplatform_pro_user_group.b]
}
`, name)
}

// vppiOmitRetainsGeneralOnlyConfig drops the scope, so the PUT carries
// <general> alone.
func vppiOmitRetainsGeneralOnlyConfig(token, suffix, name string) string {
	return vppiOmitRetainsFixtures(token, suffix, name) + fmt.Sprintf(`
resource "jamfplatform_pro_vpp_invitation" "test" {
  name                        = %[1]q
  vpp_account_id              = jamfplatform_pro_volume_purchasing_location.vpp.id
  distribution_method         = "Prompt users to accept/make available in Self Service"
  auto_register_managed_users = true

  depends_on = [jamfplatform_pro_user_group.a, jamfplatform_pro_user_group.b]
}
`, name)
}

// vppiOmitRetainsFixtures is the location + two static user groups every
// omit-retains step shares.
//
// Every omit-retains step's invitation carries an explicit depends_on over both
// groups. The interpolations that would imply the ordering are exactly what the
// later steps drop, and the retention this test exists to prove is what makes
// that fatal: the server keeps scoping the invitation to a group the
// configuration no longer mentions, so Terraform sees no edge, destroys the
// group first, and Jamf Pro refuses it with "The following items are dependent
// on this Static User Group" (USERS_VPP_INVITATIONS). The run then fails in
// CheckDestroy having leaked the group. depends_on is ordering metadata only
// and reaches no payload, so it cannot weaken what the steps assert.
func vppiOmitRetainsFixtures(token, suffix, name string) string {
	return vppLocationFixture(token, suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_user_group" "a" {
  name       = "%[1]s-grp-a"
  group_type = "static"
}

resource "jamfplatform_pro_user_group" "b" {
  name       = "%[1]s-grp-b"
  group_type = "static"
}
`, name)
}

// vppiOmitRetainsWant holds the tenant-minted group ids step 1 scoped, read
// from Terraform state so the wire assertion compares real ids.
type vppiOmitRetainsWant struct {
	targetGroup, excl int
}

// vppiOmitRetainsCapture records the step-1 group ids the later steps must
// find on the wire. Runs before vppiRetainedOnServer in the same composed check.
func vppiOmitRetainsCapture(want *vppiOmitRetainsWant) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		var err error
		if want.targetGroup, err = vppiStateInt(s, "jamfplatform_pro_user_group.a", "id"); err != nil {
			return err
		}
		want.excl, err = vppiStateInt(s, "jamfplatform_pro_user_group.b", "id")
		return err
	}
}

func vppiStateInt(s *terraform.State, addr, key string) (int, error) {
	rs, ok := s.RootModule().Resources[addr]
	if !ok {
		return 0, fmt.Errorf("resource %s not found in state", addr)
	}
	raw, ok := rs.Primary.Attributes[key]
	if !ok {
		return 0, fmt.Errorf("%s: attribute %s not in state", addr, key)
	}
	return strconv.Atoi(raw)
}

// vppiRequireIDs compares a wire IDName list against the ids step 1 wrote,
// order-insensitively. A nil wrapper is an empty list.
func vppiRequireIDs(field string, want []int, got *[]proclassic.IDName) error {
	ids := make([]int, 0)
	if got != nil {
		for _, it := range *got {
			if it.ID != nil {
				ids = append(ids, *it.ID)
			}
		}
	}
	sort.Ints(ids)
	if !slices.Equal(want, ids) {
		return fmt.Errorf("%s: want %v, got %v", field, want, ids)
	}
	return nil
}

// vppiRetainedOnServer asserts the server's copy still carries every scope
// value the omit-retains config declared in its first step.
func vppiRetainedOnServer(t *testing.T, want *vppiOmitRetainsWant) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return testhelpers.CheckLiveObject(resAddr,
		func(ctx context.Context, id string) (*proclassic.VppInvitation, error) {
			return c.GetVPPInvitationByID(ctx, id)
		},
		func(inv *proclassic.VppInvitation) error {
			if inv.Scope == nil {
				return fmt.Errorf("scope: absent")
			}
			if err := testhelpers.RequireEqual("scope.all_jss_users", false, testhelpers.Deref(inv.Scope.AllJssUsers)); err != nil {
				return err
			}
			var targetGroups *[]proclassic.IDName
			if inv.Scope.JssUserGroups != nil {
				targetGroups = inv.Scope.JssUserGroups.UserGroup
			}
			if err := vppiRequireIDs("scope.jss_user_groups", []int{want.targetGroup}, targetGroups); err != nil {
				return err
			}
			if inv.Scope.Exclusions == nil {
				return fmt.Errorf("scope.exclusions: absent")
			}
			var exclGroups *[]proclassic.IDName
			if inv.Scope.Exclusions.JssUserGroups != nil {
				exclGroups = inv.Scope.Exclusions.JssUserGroups.UserGroup
			}
			return vppiRequireIDs("scope.exclusions.jss_user_groups", []int{want.excl}, exclGroups)
		})
}

// TestAccResource_ProVPPInvitation_OmittedBlocksRetained pins the omit-retains
// contract the plan output cannot show: dropping the gated scope block from
// config plans it as removed, but the classic PUT omits the element and the
// server keeps every value. Step 2 keeps the scope and drops its exclusions
// (through the granular merge); step 3 drops the scope so the PUT carries
// <general> alone. Each step's implicit post-apply plan must be empty, which
// is what makes the contract usable. If this test fails on content, the
// endpoint no longer merges and nothing that suppresses the removal plan may
// ship for this resource. Gated on JAMFPLATFORM_ACC_PRO_VPP_TOKEN like every
// apply test here.
func TestAccResource_ProVPPInvitation_OmittedBlocksRetained(t *testing.T) {
	token := vppToken(t)
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-vpp-omit-" + suffix
	var want vppiOmitRetainsWant

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVPPInvitationDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: vppiOmitRetainsConfig(token, suffix, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "scope.targets.jss_user_group_ids.#", "1"),
					resource.TestCheckResourceAttr(resAddr, "scope.exclusions.jss_user_group_ids.#", "1"),
					vppiOmitRetainsCapture(&want),
					vppiRetainedOnServer(t, &want),
				),
			},
			{
				Config: vppiOmitRetainsChildrenDroppedConfig(token, suffix, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "scope.targets.jss_user_group_ids.#", "1"),
					resource.TestCheckNoResourceAttr(resAddr, "scope.exclusions.jss_user_group_ids.#"),
					vppiRetainedOnServer(t, &want),
				),
			},
			{
				Config: vppiOmitRetainsGeneralOnlyConfig(token, suffix, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(resAddr, "scope.targets.jss_user_group_ids.#"),
					vppiRetainedOnServer(t, &want),
				),
			},
		},
	})
}

// ---- ExpectError: plan-time validators (no account / token needed) -------------

// litAccount is a placeholder vpp_account_id for validation-only tests that fail
// at plan time before any API call — they never apply, so the id need not exist.
const litAccount = "1"

func TestAccResource_ProVPPInvitation_InvalidDistributionMethod(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jamfplatform_pro_vpp_invitation" "test" {
  name                = "tf-acc-vpp-bad"
  vpp_account_id      = %q
  distribution_method = "bogus"
}
`, litAccount),
				ExpectError: regexp.MustCompile(`value must be one of`),
			},
		},
	})
}

func TestAccResource_ProVPPInvitation_EmailModeMissingFields(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jamfplatform_pro_vpp_invitation" "test" {
  name                = "tf-acc-vpp-email-bad"
  vpp_account_id      = %q
  distribution_method = "Send emails"
}
`, litAccount),
				ExpectError: regexp.MustCompile(`Missing required field`),
			},
		},
	})
}

func TestAccResource_ProVPPInvitation_AllJSSUsersConflict(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "jamfplatform_pro_user_group" "a" {
  name       = "tf-acc-vpp-conflict-grp"
  group_type = "static"
}

resource "jamfplatform_pro_vpp_invitation" "test" {
  name                = "tf-acc-vpp-conflict"
  vpp_account_id      = %q
  distribution_method = "Make available in Self Service only"

  scope = {
    targets = {
      all_jss_users      = true
      jss_user_group_ids = [jamfplatform_pro_user_group.a.id]
    }
  }
}
`, litAccount),
				ExpectError: regexp.MustCompile(`all-flag`),
			},
		},
	})
}

// NOTE: the directory-service-group preflight is intentionally advisory now (a
// no-match warns, never errors at plan) so a same-apply directory+scope bootstrap
// isn't blocked — see helpers.RetryOnDirectoryGroupMatchConflict and the
// criteria/scope unit tests. A wrong group name therefore surfaces at apply (after a
// bounded retry), not plan, so there is no plan-time "not found" acc test here.

func TestAccDataSource_ProVPPInvitation_AmbiguousSelector(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "jamfplatform_pro_vpp_invitation" "bad" {
  id   = "1"
  name = "x"
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}
