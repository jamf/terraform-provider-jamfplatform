// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic /mobiledeviceenrollmentprofiles
// endpoint. Classic has known concurrency issues when multiple writes hit the same
// resource type — keep these tests serial with any future classic acceptance work
// in this package.
//
// Writes are a MERGE (omit=retain, empty=clear); the update step mutates scalars,
// clears a field, and moves the site to exercise that path. Attachments are
// read-only (the upload endpoint rejects bearer auth for this resource) so no
// attachment write is exercised. location/purchasing are hydrated on first-time
// import and go through the same flattener as a refresh, so ImportStateVerify
// covers them rather than ignoring them (#391).

package mobile_device_enrollment_profile_test

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

const resAddr = "jamfplatform_pro_mobile_device_enrollment_profile.test"

func testAccCheckEnrollmentProfileDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_mobile_device_enrollment_profile" {
				continue
			}
			_, err := c.GetMobileDeviceEnrollmentProfileByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking enrollment profile %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("enrollment profile %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// config builds a profile that references a managed site (tenant-agnostic).
// An empty room omits the line entirely (clear-by-omission — the real Terraform
// path); a merge write then clears the field on the server and state reconciles
// to null. (Setting room = "" explicitly would be a known plan value the server
// drops to null, which is an inconsistency — users omit to clear.)
func config(name, desc, room string) string {
	roomLine := ""
	if room != "" {
		roomLine = fmt.Sprintf("    room      = %q\n", room)
	}
	return fmt.Sprintf(`
resource "jamfplatform_pro_site" "s" {
  name = "%s-site"
}

resource "jamfplatform_pro_mobile_device_enrollment_profile" "test" {
  name        = %q
  description = %q
  site_id     = jamfplatform_pro_site.s.id

  location = {
    username  = "alice"
    real_name = "Alice A"
%s  }

  purchasing = {
    is_purchased = true
    vendor       = "Acme"
  }
}
`, name, name, desc, roomLine)
}

func TestAccResource_ProMobileDeviceEnrollmentProfile(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdep-" + suffix
	renamed := "tf-acc-mdep-renamed-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(name, "first", "R1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(resAddr, "id"),
					resource.TestCheckResourceAttr(resAddr, "name", name),
					resource.TestCheckResourceAttr(resAddr, "description", "first"),
					resource.TestCheckResourceAttrSet(resAddr, "invitation"),
					resource.TestCheckResourceAttrSet(resAddr, "uuid"),
					resource.TestCheckResourceAttrSet(resAddr, "site_id"),
					resource.TestCheckResourceAttrSet(resAddr, "site_name"),
					resource.TestCheckResourceAttr(resAddr, "location.username", "alice"),
					resource.TestCheckResourceAttr(resAddr, "location.room", "R1"),
					resource.TestCheckResourceAttr(resAddr, "purchasing.vendor", "Acme"),
					resource.TestCheckResourceAttr(resAddr, "purchasing.is_purchased", "true"),
				),
			},
			{
				// Merge update: rename, change description, clear room (empty=clear), keep invitation/uuid stable.
				Config: config(renamed, "second", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "name", renamed),
					resource.TestCheckResourceAttr(resAddr, "description", "second"),
					resource.TestCheckNoResourceAttr(resAddr, "location.room"),
					resource.TestCheckResourceAttr(resAddr, "location.username", "alice"),
				),
			},
			{
				ResourceName:            resAddr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func TestAccDataSource_ProMobileDeviceEnrollmentProfile_BySelectors(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-mdep-ds-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(name, "ds", "R1") + `
data "jamfplatform_pro_mobile_device_enrollment_profile" "by_id" {
  id = jamfplatform_pro_mobile_device_enrollment_profile.test.id
}
data "jamfplatform_pro_mobile_device_enrollment_profile" "by_name" {
  name = jamfplatform_pro_mobile_device_enrollment_profile.test.name
}
data "jamfplatform_pro_mobile_device_enrollment_profile" "by_invitation" {
  invitation = jamfplatform_pro_mobile_device_enrollment_profile.test.invitation
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_mobile_device_enrollment_profile.by_id", "name", resAddr, "name"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_mobile_device_enrollment_profile.by_name", "id", resAddr, "id"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_mobile_device_enrollment_profile.by_invitation", "id", resAddr, "id"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_mobile_device_enrollment_profile.by_id", "location.username", "alice"),
				),
			},
		},
	})
}

func TestAccResource_ProMobileDeviceEnrollmentProfile_DriftRecovery(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-mdep-drift-" + testhelpers.RunSuffix()
	cfg := config(name, "drift", "R1")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentProfileDestroy(t),
		Steps: []resource.TestStep{
			{Config: cfg, Check: resource.TestCheckResourceAttrSet(resAddr, "invitation")},
			{
				PreConfig: func() {
					c := proclassic.New(testhelpers.NewAcceptanceClient(t))
					ctx := context.Background()
					id, err := c.ResolveMobileDeviceEnrollmentProfileIDByName(ctx, name)
					if err != nil {
						t.Fatalf("drift preconfig resolve: %v", err)
					}
					if err := c.DeleteMobileDeviceEnrollmentProfileByID(ctx, id); err != nil {
						t.Fatalf("drift preconfig delete: %v", err)
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

func TestAccResource_ProMobileDeviceEnrollmentProfile_EmptyNameRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "jamfplatform_pro_mobile_device_enrollment_profile" "test" {
  name = ""
}
`,
				ExpectError: regexp.MustCompile(`string length must be at least 1`),
			},
		},
	})
}

func TestAccDataSource_ProMobileDeviceEnrollmentProfile_AmbiguousSelector(t *testing.T) {
	testhelpers.AccPreCheck(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
data "jamfplatform_pro_mobile_device_enrollment_profile" "bad" {
  id   = "1"
  name = "x"
}
`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Combination`),
			},
		},
	})
}

// mdepOmitRetainsFixtures declares the site, department and building the
// omit-retains configs reference. The department and building are real objects
// rather than free text so the location leaves name something the tenant knows.
func mdepOmitRetainsFixtures(name string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_site" "omit" {
  name = "%[1]s-site"
}

resource "jamfplatform_pro_department" "omit" {
  name = "%[1]s-dept"
}

resource "jamfplatform_pro_building" "omit" {
  name = "%[1]s-bldg"
}
`, name)
}

// mdepOmitRetainsConfig is the fully declared shape for the omit-retains
// contract: both state-gated blocks (location, purchasing) carry a distinctive
// value in every writable leaf so that a server which stopped retaining an
// omitted element is caught on content, not on presence. is_purchased is false
// and is_leased true because the server treats the pair as exclusive: a write
// carrying both true is stored with is_leased reset to false (wire-observed
// 2026-09-06), which the provider does not yet reject at plan time.
func mdepOmitRetainsConfig(name string) string {
	return mdepOmitRetainsFixtures(name) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_enrollment_profile" "test" {
  depends_on  = [jamfplatform_pro_site.omit, jamfplatform_pro_department.omit, jamfplatform_pro_building.omit]
  name        = %[1]q
  description = "Omit-retains contract profile."
  site_id     = jamfplatform_pro_site.omit.id

  location = {
    username      = "omit.retains"
    real_name     = "Omit Retains"
    email_address = "omit.retains@example.com"
    phone_number  = "+1 555 0100"
    department    = jamfplatform_pro_department.omit.name
    building      = jamfplatform_pro_building.omit.name
    room          = "R-omit"
    position      = "Retained Position"
  }

  purchasing = {
    is_purchased       = false
    is_leased          = true
    po_number          = "PO-OMIT-1"
    po_date            = "2024-02-29"
    vendor             = "Omit Vendor"
    warranty_expires   = "2027-03-31"
    applecare_id       = "AC-OMIT-1"
    lease_expires      = "2028-04-30"
    purchase_price     = "1234.56"
    life_expectancy    = 7
    purchasing_account = "ACCT-OMIT"
    purchasing_contact = "Contact Omit"
  }
}
`, name)
}

// mdepOmitRetainsLeavesDroppedConfig keeps both blocks and drops two leaves
// that the merge treats differently: life_expectancy is Optional+Computed, so
// the builder omits it and the server retains 7; room is plain Optional, so
// the builder emits <room></room> and the server clears it. The second is the
// documented clear-by-omission half of this endpoint's contract, not a retain
// case.
func mdepOmitRetainsLeavesDroppedConfig(name string) string {
	return mdepOmitRetainsFixtures(name) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_enrollment_profile" "test" {
  depends_on  = [jamfplatform_pro_site.omit, jamfplatform_pro_department.omit, jamfplatform_pro_building.omit]
  name        = %[1]q
  description = "Omit-retains contract profile."
  site_id     = jamfplatform_pro_site.omit.id

  location = {
    username      = "omit.retains"
    real_name     = "Omit Retains"
    email_address = "omit.retains@example.com"
    phone_number  = "+1 555 0100"
    department    = jamfplatform_pro_department.omit.name
    building      = jamfplatform_pro_building.omit.name
    position      = "Retained Position"
  }

  purchasing = {
    is_purchased       = false
    is_leased          = true
    po_number          = "PO-OMIT-1"
    po_date            = "2024-02-29"
    vendor             = "Omit Vendor"
    warranty_expires   = "2027-03-31"
    applecare_id       = "AC-OMIT-1"
    lease_expires      = "2028-04-30"
    purchase_price     = "1234.56"
    purchasing_account = "ACCT-OMIT"
    purchasing_contact = "Contact Omit"
  }
}
`, name)
}

// mdepOmitRetainsGeneralOnlyConfig drops both optional blocks, so the PUT
// carries <general> alone.
func mdepOmitRetainsGeneralOnlyConfig(name string) string {
	return mdepOmitRetainsFixtures(name) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_enrollment_profile" "test" {
  depends_on  = [jamfplatform_pro_site.omit, jamfplatform_pro_department.omit, jamfplatform_pro_building.omit]
  name        = %[1]q
  description = "Omit-retains contract profile."
  site_id     = jamfplatform_pro_site.omit.id
}
`, name)
}

// mdepRetainedOnServer asserts the server's copy still carries every value the
// omit-retains config declared in its first step. wantRoom is the one leaf the
// contract test deliberately clears mid-run (step 2 omits it inside a kept
// block), so the caller states what the server should hold after each step.
func mdepRetainedOnServer(t *testing.T, name, wantRoom string) resource.TestCheckFunc {
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	return testhelpers.CheckLiveObject(resAddr,
		func(ctx context.Context, id string) (*proclassic.MobileDeviceEnrollmentProfile, error) {
			return c.GetMobileDeviceEnrollmentProfileByID(ctx, id)
		},
		func(p *proclassic.MobileDeviceEnrollmentProfile) error {
			if p.Location == nil {
				return fmt.Errorf("location: absent")
			}
			l := p.Location
			for _, f := range []struct{ field, want, got string }{
				{"location.username", "omit.retains", testhelpers.Deref(l.Username)},
				{"location.real_name", "Omit Retains", testhelpers.Deref(l.RealName)},
				{"location.email_address", "omit.retains@example.com", testhelpers.Deref(l.EmailAddress)},
				{"location.phone_number", "+1 555 0100", testhelpers.Deref(l.PhoneNumber)},
				{"location.department", name + "-dept", testhelpers.Deref(l.Department)},
				{"location.building", name + "-bldg", testhelpers.Deref(l.Building)},
				{"location.room", wantRoom, testhelpers.Deref(l.Room)},
				{"location.position", "Retained Position", testhelpers.Deref(l.Position)},
			} {
				if err := testhelpers.RequireEqual(f.field, f.want, f.got); err != nil {
					return err
				}
			}
			if p.Purchasing == nil {
				return fmt.Errorf("purchasing: absent")
			}
			u := p.Purchasing
			if err := testhelpers.RequireEqual("purchasing.is_purchased", false, testhelpers.Deref(u.IsPurchased)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("purchasing.is_leased", true, testhelpers.Deref(u.IsLeased)); err != nil {
				return err
			}
			if err := testhelpers.RequireEqual("purchasing.life_expectancy", 7, testhelpers.Deref(u.LifeExpectancy)); err != nil {
				return err
			}
			for _, f := range []struct{ field, want, got string }{
				{"purchasing.po_number", "PO-OMIT-1", testhelpers.Deref(u.PoNumber)},
				{"purchasing.po_date", "2024-02-29", testhelpers.Deref(u.PoDate)},
				{"purchasing.vendor", "Omit Vendor", testhelpers.Deref(u.Vendor)},
				{"purchasing.warranty_expires", "2027-03-31", testhelpers.Deref(u.WarrantyExpires)},
				{"purchasing.applecare_id", "AC-OMIT-1", testhelpers.Deref(u.ApplecareID)},
				{"purchasing.lease_expires", "2028-04-30", testhelpers.Deref(u.LeaseExpires)},
				{"purchasing.purchase_price", "1234.56", testhelpers.Deref(u.PurchasePrice)},
				{"purchasing.purchasing_account", "ACCT-OMIT", testhelpers.Deref(u.PurchasingAccount)},
				{"purchasing.purchasing_contact", "Contact Omit", testhelpers.Deref(u.PurchasingContact)},
			} {
				if err := testhelpers.RequireEqual(f.field, f.want, f.got); err != nil {
					return err
				}
			}
			return nil
		})
}

// TestAccResource_ProMobileDeviceEnrollmentProfile_OmittedBlocksRetained pins
// the omit-retains contract the plan output cannot show: dropping a gated
// block from config plans it as removed, but the classic merge PUT omits the
// element and the server keeps every value. This endpoint's merge has a second
// half — an element that is present but empty clears — and the builder relies
// on it by always emitting every plain-Optional leaf inside a declared block.
// Step 2 keeps both blocks and drops one leaf of each kind: the
// Optional+Computed life_expectancy (omitted, so retained) and the
// plain-Optional room (emitted empty, so cleared); step 3 drops both blocks so
// the PUT carries <general> alone and every step-1 value must survive. Each
// step's implicit post-apply plan must be empty. If this test fails on content
// the endpoint no longer merges and nothing that suppresses the removal plan
// may ship for this resource.
func TestAccResource_ProMobileDeviceEnrollmentProfile_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-mdep-omit-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: mdepOmitRetainsConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resAddr, "location.room", "R-omit"),
					resource.TestCheckResourceAttr(resAddr, "purchasing.life_expectancy", "7"),
					mdepRetainedOnServer(t, name, "R-omit"),
				),
			},
			{
				Config: mdepOmitRetainsLeavesDroppedConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(resAddr, "location.room"),
					resource.TestCheckResourceAttr(resAddr, "purchasing.life_expectancy", "7"),
					mdepRetainedOnServer(t, name, ""),
				),
			},
			{
				Config: mdepOmitRetainsGeneralOnlyConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(resAddr, "location.username"),
					resource.TestCheckNoResourceAttr(resAddr, "purchasing.vendor"),
					mdepRetainedOnServer(t, name, ""),
				),
			},
		},
	})
}

// TestAccListResource_ProMobileDeviceEnrollmentProfile_HydratesBeyondTheSummaryRow
// pins that a list result carries the attributes the
// /mobiledeviceenrollmentprofiles summary row does not.
//
// The summary carries id, name and invitation. The list resource used to emit
// nulls for description and for the whole location and purchasing blocks, which
// no query test caught because there was no query test at all — but
// `terraform query -generate-config-out` wrote a profile without them, and
// applying that configuration back cleared them on the tenant.
//
// This asserts fields that can only have come from the per-item GET. It belongs
// here rather than on testhelpers.GenerateConfigStep, which adopts through the
// singular read and never touches the list resource.
func TestAccListResource_ProMobileDeviceEnrollmentProfile_HydratesBeyondTheSummaryRow(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-pro-mdep-list-" + suffix

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_14_0),
		},
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEnrollmentProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_pro_mobile_device_enrollment_profile" "test" {
						name        = %q
						description = "list hydration fixture"

						location = {
							username  = "tf-acc-mdep-list-user"
							real_name = "TF Acc List User"
							room      = "101"
						}

						purchasing = {
							is_purchased    = true
							vendor          = "Apple"
							life_expectancy = 3
						}
					}
				`, name),
				Check: resource.TestCheckResourceAttrSet(resAddr, "id"),
			},
			{
				Query: true,
				Config: fmt.Sprintf(`
					provider "jamfplatform" {}

					list "jamfplatform_pro_mobile_device_enrollment_profile" "test" {
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
					querycheck.ExpectLength("jamfplatform_pro_mobile_device_enrollment_profile.test", 1),
					querycheck.ExpectResourceKnownValues(
						"jamfplatform_pro_mobile_device_enrollment_profile.test",
						queryfilter.ByDisplayName(knownvalue.StringExact(name)),
						[]querycheck.KnownValueCheck{
							{Path: tfjsonpath.New("name"), KnownValue: knownvalue.StringExact(name)},
							// Not in the summary row: these are the fix.
							{Path: tfjsonpath.New("description"), KnownValue: knownvalue.StringExact("list hydration fixture")},
							{Path: tfjsonpath.New("location").AtMapKey("real_name"), KnownValue: knownvalue.StringExact("TF Acc List User")},
							{Path: tfjsonpath.New("purchasing").AtMapKey("vendor"), KnownValue: knownvalue.StringExact("Apple")},
						},
					),
				},
			},
		},
	})
}
