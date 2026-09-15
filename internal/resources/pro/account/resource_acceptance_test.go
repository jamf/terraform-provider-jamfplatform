// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file create real Jamf Pro administrator accounts via the Pro API
// and write the privilege grid via the classic API. Base-field updates route
// through Pro PUT (now accepted via the platform gateway) and are exercised by
// TestAccResource_ProAccount_BaseUpdate.

package account_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

func testAccCheckAccountDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_account" {
				continue
			}
			_, err := c.GetAccountV1(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking account %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("account %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// importedPrivilegeCheck asserts that an imported account carries a populated
// Jamf-Pro-server-objects privilege category including the named privilege. It
// matches on the element values rather than on an index, because a Set lands in
// the shimmed import state under keys the provider does not choose, and it
// asserts membership rather than an exact count, because Jamf Pro adds
// dependency privileges of its own to the grid an import materialises.
func importedPrivilegeCheck(want string) resource.ImportStateCheckFunc {
	const category = "privileges.jamf_pro_server_objects."
	return func(states []*terraform.InstanceState) error {
		if len(states) != 1 {
			return fmt.Errorf("expected one imported instance, got %d", len(states))
		}
		attrs := states[0].Attributes
		if count := attrs[category+"#"]; count == "" || count == "0" {
			return fmt.Errorf("imported account has no jamf_pro_server_objects privileges; the grid the classic endpoint returns must reach state on import")
		}
		for key, value := range attrs {
			if strings.HasPrefix(key, category) && !strings.HasSuffix(key, "#") && value == want {
				return nil
			}
		}
		return fmt.Errorf("imported jamf_pro_server_objects does not contain %q", want)
	}
}

func customAccountConfig(suffix string, objects ...string) string {
	var quoted strings.Builder
	for i, p := range objects {
		if i > 0 {
			quoted.WriteString(", ")
		}
		fmt.Fprintf(&quoted, "%q", p)
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_account" "test" {
  username      = "tf-acc-acct-%[1]s"
  full_name     = "TF Acc Account"
  email_address = "tf-acc-acct-%[1]s@example.invalid"
  access_level  = "Full Access"
  privilege_set = "Custom"

  password            = "Pr0bePassw0rd-%[1]s"
  password_wo_version = 1

  privileges = {
    jamf_pro_server_objects = [%[2]s]
  }
}
`, suffix, quoted.String())
}

// TestAccResource_ProAccount covers the create + privilege-only update + import
// lifecycle, all of which work through the gateway today (Pro create, classic
// privilege write, Pro delete). The privilege set is grown then shrunk to prove
// intersect-on-read (server-added dependency privileges do not leak; removals
// are honoured).
func TestAccResource_ProAccount(t *testing.T) {
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAccountDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: customAccountConfig(suffix, "Read Computers", "Update Computers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_account.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "username", "tf-acc-acct-"+suffix),
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "access_level", "Full Access"),
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "privilege_set", "Custom"),
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "account_type", "DEFAULT"),
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "privileges.jamf_pro_server_objects.#", "2"),
				),
			},
			{
				// Privilege-only update (classic write; no base change ⇒ no Pro PUT).
				Config: customAccountConfig(suffix, "Read Computers"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "privileges.jamf_pro_server_objects.#", "1"),
					resource.TestCheckTypeSetElemAttr("jamfplatform_pro_account.test", "privileges.jamf_pro_server_objects.*", "Read Computers"),
				),
			},
			{
				// Import smoke. ImportStateVerify omitted: password is WriteOnly
				// (never returned) and import materialises the full server
				// privilege grid, so a full-fidelity compare would not match.
				// ImportStateCheck carries the assertion instead. The grid is
				// what an earlier revision dropped on this path, and a compare
				// that cannot run is no cover for it (issue #372).
				ResourceName:      "jamfplatform_pro_account.test",
				ImportState:       true,
				ImportStateVerify: false,
				ImportStateCheck:  importedPrivilegeCheck("Read Computers"),
			},
		},
	})
}

// baseAccountConfig renders a non-Custom (Auditor) account so the base-update
// path is exercised in isolation: no Custom privilege grid means no classic
// write, so only the Pro PUT base-field path runs.
func baseAccountConfig(suffix, fullName, accessStatus string, passwordWOVersion int) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_account" "test" {
  username      = "tf-acc-base-%[1]s"
  full_name     = %[2]q
  email_address = "tf-acc-base-%[1]s@example.invalid"
  access_level  = "Full Access"
  privilege_set = "Auditor"
  access_status = %[3]q

  password            = "Pr0bePassw0rd-%[1]s-v%[4]d"
  password_wo_version = %[4]d
}
`, suffix, fullName, accessStatus, passwordWOVersion)
}

// TestAccResource_ProAccount_BaseUpdate exercises in-place base-field updates,
// which route through Pro PUT. Step 2 changes plain base fields (full name,
// access status); step 3 rotates the WriteOnly password (bumped wo_version →
// password re-sent on the same PUT). Both confirm the gateway now accepts the
// Pro update that previously returned 403 BAD_PERMISSIONS.
func TestAccResource_ProAccount_BaseUpdate(t *testing.T) {
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testhelpers.AccPreCheck(t) },
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAccountDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: baseAccountConfig(suffix, "TF Acc Base", "Enabled", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("jamfplatform_pro_account.test", "id"),
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "full_name", "TF Acc Base"),
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "access_status", "Enabled"),
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "privilege_set", "Auditor"),
				),
			},
			{
				// In-place base-field update (no password change ⇒ Pro PUT without
				// re-sending the password).
				Config: baseAccountConfig(suffix, "TF Acc Base Renamed", "Disabled", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "full_name", "TF Acc Base Renamed"),
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "access_status", "Disabled"),
				),
			},
			{
				// Password rotation: bump password_wo_version ⇒ password re-sent on
				// the Pro PUT.
				Config: baseAccountConfig(suffix, "TF Acc Base Renamed", "Disabled", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "password_wo_version", "2"),
					resource.TestCheckResourceAttr("jamfplatform_pro_account.test", "full_name", "TF Acc Base Renamed"),
				),
			},
		},
	})
}

const accountOmitRetainsAddr = "jamfplatform_pro_account.omit"

// accountOmitRetainsConfig is the fully declared shape for the omit-retains
// contract: three privilege categories plus the Optional+Computed site_id and
// force_password_change, each carrying a distinctive value so a server that
// stopped retaining an omitted element is caught on content, not presence.
func accountOmitRetainsConfig(suffix string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_account" "omit" {
  username      = "tf-acc-acct-omit-%[1]s"
  full_name     = "TF Acc Account Omit"
  email_address = "tf-acc-acct-omit-%[1]s@example.invalid"
  access_level  = "Full Access"
  privilege_set = "Custom"

  site_id               = -1
  force_password_change = true

  password            = "Pr0bePassw0rd-%[1]s"
  password_wo_version = 1

  privileges = {
    jamf_pro_server_objects  = ["Read Buildings", "Read Departments"]
    jamf_pro_server_settings = ["Read SMTP Server"]
    jamf_pro_server_actions  = ["Send Computer Remote Lock Command"]
  }
}
`, suffix)
}

// accountOmitRetainsObjectsOnlyConfig keeps the privileges block but declares
// only jamf_pro_server_objects, and drops site_id and force_password_change, so
// the classic PUT carries a <privileges> element missing two categories and the
// Pro side is not written at all.
func accountOmitRetainsObjectsOnlyConfig(suffix string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_account" "omit" {
  username      = "tf-acc-acct-omit-%[1]s"
  full_name     = "TF Acc Account Omit"
  email_address = "tf-acc-acct-omit-%[1]s@example.invalid"
  access_level  = "Full Access"
  privilege_set = "Custom"

  password            = "Pr0bePassw0rd-%[1]s"
  password_wo_version = 1

  privileges = {
    jamf_pro_server_objects = ["Read Buildings", "Read Departments"]
  }
}
`, suffix)
}

// accountOmitRetainsHeaderOnlyConfig drops every optional block, so no classic
// PUT is issued at all and the Pro side sees no base-field change.
func accountOmitRetainsHeaderOnlyConfig(suffix string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_account" "omit" {
  username      = "tf-acc-acct-omit-%[1]s"
  full_name     = "TF Acc Account Omit"
  email_address = "tf-acc-acct-omit-%[1]s@example.invalid"
  access_level  = "Full Access"
  privilege_set = "Custom"

  password            = "Pr0bePassw0rd-%[1]s"
  password_wo_version = 1
}
`, suffix)
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

// liveAccount is the server's copy of an account read from both sides the
// resource writes: the classic /accounts/userid object carries the privilege
// grid and force_password_change, while site is reported only by Pro v1 (the
// classic response for a Full Access account carries no <site> element,
// wire-observed 2026-09-06 on Jamf Pro 11.31.1).
type liveAccount struct {
	classic *proclassic.Account
	base    *pro.UserAccount
}

// accountRetainedOnServer asserts the server's copy still carries every
// privilege, the forced password change and the site the omit-retains config
// declared in its first step.
func accountRetainedOnServer(t *testing.T) resource.TestCheckFunc {
	sdk := testhelpers.NewAcceptanceClient(t)
	classic := proclassic.New(sdk)
	base := pro.New(sdk)
	return testhelpers.CheckLiveObject(accountOmitRetainsAddr,
		func(ctx context.Context, id string) (liveAccount, error) {
			c, err := classic.GetAccountByUserID(ctx, id)
			if err != nil {
				return liveAccount{}, err
			}
			b, err := base.GetAccountV1(ctx, id)
			if err != nil {
				return liveAccount{}, err
			}
			return liveAccount{classic: c, base: b}, nil
		},
		func(got liveAccount) error {
			a := got.classic
			if a.Privileges == nil {
				return fmt.Errorf("privileges: absent")
			}
			if a.Privileges.JssObjects == nil {
				return fmt.Errorf("privileges.jamf_pro_server_objects: absent")
			}
			for _, want := range []string{"Read Buildings", "Read Departments"} {
				if !hasPrivilege(a.Privileges.JssObjects.Privilege, want) {
					return fmt.Errorf("privileges.jamf_pro_server_objects: want %q, got %v", want, testhelpers.Deref(a.Privileges.JssObjects.Privilege))
				}
			}
			if a.Privileges.JssSettings == nil || !hasPrivilege(a.Privileges.JssSettings.Privilege, "Read SMTP Server") {
				return fmt.Errorf("privileges.jamf_pro_server_settings: want %q, got %v", "Read SMTP Server", privilegeList(a.Privileges.JssSettings, func(c *proclassic.AccountPrivilegesJssSettings) *[]string { return c.Privilege }))
			}
			if a.Privileges.JssActions == nil || !hasPrivilege(a.Privileges.JssActions.Privilege, "Send Computer Remote Lock Command") {
				return fmt.Errorf("privileges.jamf_pro_server_actions: want %q, got %v", "Send Computer Remote Lock Command", privilegeList(a.Privileges.JssActions, func(c *proclassic.AccountPrivilegesJssActions) *[]string { return c.Privilege }))
			}
			if err := testhelpers.RequireEqual("force_password_change", true, testhelpers.Deref(a.ForcePasswordChange)); err != nil {
				return err
			}
			if got.base.SiteID == nil {
				return fmt.Errorf("site_id: absent")
			}
			return testhelpers.RequireEqual("site_id", -1, *got.base.SiteID)
		})
}

// TestAccResource_ProAccount_OmittedBlocksRetained pins the omit-retains
// contract the plan output cannot show: dropping two privilege categories, the
// Optional+Computed site_id and force_password_change, and finally the whole
// privileges block from config plans them as removed, but the classic
// /accounts/userid PUT omits the elements and the server keeps every value.
// Step 2 keeps the privileges block with one category so the PUT carries a
// partial <privileges>, exercising the category-level merge the resource
// documents; step 3 drops privileges too so no classic write is issued and the
// Pro side sees no base-field change. Each step's implicit post-apply plan must
// be empty, which is what makes the contract usable.
//
// Step 2 is the step the wire cannot satisfy on its own. Wire-probed 2026-09-06
// on Jamf Pro 11.31.1 through the SDK: a PUT /accounts/userid/{id} carrying only
// <jss_objects> left jss_settings as the server-injected "Read License
// Information" alone and jss_actions empty — the server treats a sent
// <privileges> as a whole-grid replace, and Read's null-means-unmanaged gate
// hid the loss (issue #385). The resource now emulates the retention it
// documents — every privilege write reads the live grid first and re-emits the
// undeclared categories through accountprivileges.MergeGrid — and this step is
// the proof: the server-side check after step 2 must still find "Read SMTP
// Server" and the remote-lock action that only the carry-over could have sent.
// The whole-block omission in step 3 retains on the wire itself, verified by
// running the steps in the order full, header-only, objects-only: the
// header-only step issued no classic PUT, the post-apply plan was empty and
// every step-1 value survived.
func TestAccResource_ProAccount_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAccountDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: accountOmitRetainsConfig(suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(accountOmitRetainsAddr, "privileges.jamf_pro_server_objects.#", "2"),
					resource.TestCheckResourceAttr(accountOmitRetainsAddr, "privileges.jamf_pro_server_settings.#", "1"),
					resource.TestCheckResourceAttr(accountOmitRetainsAddr, "privileges.jamf_pro_server_actions.#", "1"),
					resource.TestCheckResourceAttr(accountOmitRetainsAddr, "force_password_change", "true"),
					resource.TestCheckResourceAttr(accountOmitRetainsAddr, "site_id", "-1"),
					accountRetainedOnServer(t),
				),
			},
			{
				Config: accountOmitRetainsObjectsOnlyConfig(suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(accountOmitRetainsAddr, "privileges.jamf_pro_server_settings.#"),
					resource.TestCheckNoResourceAttr(accountOmitRetainsAddr, "privileges.jamf_pro_server_actions.#"),
					resource.TestCheckResourceAttr(accountOmitRetainsAddr, "privileges.jamf_pro_server_objects.#", "2"),
					resource.TestCheckResourceAttr(accountOmitRetainsAddr, "force_password_change", "true"),
					resource.TestCheckResourceAttr(accountOmitRetainsAddr, "site_id", "-1"),
					accountRetainedOnServer(t),
				),
			},
			{
				Config: accountOmitRetainsHeaderOnlyConfig(suffix),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(accountOmitRetainsAddr, "privileges.jamf_pro_server_objects.#"),
					accountRetainedOnServer(t),
				),
			},
		},
	})
}
