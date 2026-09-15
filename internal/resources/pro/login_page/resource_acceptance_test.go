// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package login_page_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// checkSingletonRecordStillExists verifies the Jamf Pro login page settings record persists
// on the tenant after Terraform destroys the resource from state. Canonical singleton
// acceptance check: the remote Delete is a no-op, so the API must still return the record.
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "login page settings", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetLoginCustomizationV1(ctx)
	})
}

// TestAccResource_ProLoginPageSettings_Basic mutates every field across two Update steps
// against a real tenant. Each step is independently valid: the three text fields are
// non-empty in BOTH steps (the server requires them on every write regardless of the
// toggle), and the toggle flips between steps. Singleton resources have no remote Delete,
// so CheckDestroy verifies the record PERSISTS on the tenant after Terraform stops managing
// it.
func TestAccResource_ProLoginPageSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const addr = "jamfplatform_pro_login_page_settings.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_login_page_settings" "test" {
						include_custom_disclaimer = true
						disclaimer_heading         = "Welcome"
						disclaimer_main_text       = "By signing in you agree to the acceptable use policy."
						action_text                = "I Agree"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "singleton"),
					resource.TestCheckResourceAttr(addr, "include_custom_disclaimer", "true"),
					resource.TestCheckResourceAttr(addr, "disclaimer_heading", "Welcome"),
					resource.TestCheckResourceAttr(addr, "disclaimer_main_text", "By signing in you agree to the acceptable use policy."),
					resource.TestCheckResourceAttr(addr, "action_text", "I Agree"),
				),
			},
			{
				// Toggle off + edit all three text fields. Text remains non-empty (still
				// required on the wire even with the toggle off).
				Config: `
					resource "jamfplatform_pro_login_page_settings" "test" {
						include_custom_disclaimer = false
						disclaimer_heading         = "Notice"
						disclaimer_main_text       = "Updated disclaimer body text."
						action_text                = "Continue"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "include_custom_disclaimer", "false"),
					resource.TestCheckResourceAttr(addr, "disclaimer_heading", "Notice"),
					resource.TestCheckResourceAttr(addr, "disclaimer_main_text", "Updated disclaimer body text."),
					resource.TestCheckResourceAttr(addr, "action_text", "Continue"),
				),
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"timeouts",
				},
			},
		},
	})
}

// TestAccResource_ProLoginPageSettings_CreateAdopt proves the singleton GET-on-create-adopt:
// when a field is omitted from HCL on the FIRST apply, the provider reads the live settings
// and re-sends the existing value rather than failing the all-fields-required PUT. Here only
// the toggle is declared; the three text fields are omitted and must adopt the out-of-band
// baseline rather than being sent empty (which the server rejects with 400).
func TestAccResource_ProLoginPageSettings_CreateAdopt(t *testing.T) {
	testhelpers.AccPreCheck(t)

	const addr = "jamfplatform_pro_login_page_settings.test"

	setBaselineOutOfBand := func() {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		body := &pro.LoginContentPut{
			IncludeCustomDisclaimer: true,
			DisclaimerHeading:       new("Baseline Heading"),
			DisclaimerMainText:      new("Baseline main disclaimer body."),
			ActionText:              new("Acknowledge"),
		}
		if _, err := c.UpdateLoginCustomizationV1(context.Background(), body); err != nil {
			t.Fatalf("out-of-band baseline PUT: %v", err)
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				// Declare only the toggle; text fields omitted. GET-on-create-adopt must
				// preserve the out-of-band text rather than send empty strings (→ 400).
				PreConfig: setBaselineOutOfBand,
				Config: `
					resource "jamfplatform_pro_login_page_settings" "test" {
						include_custom_disclaimer = false
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "include_custom_disclaimer", "false"),
					// Adopted from the live settings (not reset / not emptied).
					resource.TestCheckResourceAttr(addr, "disclaimer_heading", "Baseline Heading"),
					resource.TestCheckResourceAttr(addr, "disclaimer_main_text", "Baseline main disclaimer body."),
					resource.TestCheckResourceAttr(addr, "action_text", "Acknowledge"),
				),
			},
		},
	})
}

// TestAccResource_ProLoginPageSettings_RejectsNonSingletonImport verifies the ImportState
// guard: any identifier other than "singleton" must fail with a clear error.
func TestAccResource_ProLoginPageSettings_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_login_page_settings" "test" {
						include_custom_disclaimer = true
						disclaimer_heading         = "Welcome"
						disclaimer_main_text       = "Body."
						action_text                = "OK"
					}
				`,
			},
			{
				ResourceName:  "jamfplatform_pro_login_page_settings.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccResource_ProLoginPageSettings_RejectsOverLengthHeading verifies the plan-time
// LengthBetween validator rejects a disclaimer_heading over the 20-character cap
// (wire-proven limit). The "21-character" token avoids whitespace at the ~80-col error wrap.
func TestAccResource_ProLoginPageSettings_RejectsOverLengthHeading(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_login_page_settings" "test" {
						include_custom_disclaimer = true
						disclaimer_heading         = "THIS-HEADING-IS-WAY-TOO-LONG-FOR-THE-FIELD"
						disclaimer_main_text       = "Body."
						action_text                = "OK"
					}
				`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Value Length`),
			},
		},
	})
}

func TestAccDataSource_ProLoginPageSettings_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_login_page_settings" "src" {
						include_custom_disclaimer = true
						disclaimer_heading         = "DS Heading"
						disclaimer_main_text       = "DS body."
						action_text                = "DS Action"
					}

					data "jamfplatform_pro_login_page_settings" "lookup" {
						depends_on = [jamfplatform_pro_login_page_settings.src]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_login_page_settings.lookup", "id", "singleton"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_login_page_settings.lookup", "disclaimer_heading", "jamfplatform_pro_login_page_settings.src", "disclaimer_heading"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_login_page_settings.lookup", "include_custom_disclaimer", "jamfplatform_pro_login_page_settings.src", "include_custom_disclaimer"),
				),
			},
		},
	})
}
