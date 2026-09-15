// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// These tests create additional tenant objects as fixtures: the SMTP server (enabling App
// Requests requires a configured SMTP server), a transient static user group (the requester
// group), and a transient App Request form field (a tenant must hold at least one before App
// Requests can be enabled). The App Request settings and SMTP server are singletons with
// no remote delete, so after a run the tenant is left with App Requests disabled, a dummy
// SMTP configuration, and the test's approver emails. Restore these manually if needed.

package app_request_settings_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const settingsAddr = "jamfplatform_pro_app_request_settings.test"

// checkSettingsStillExist verifies the App Request settings singleton still resolves after
// the test — a singleton has no remote delete, so "destroy" only removes it from state.
func checkSettingsStillExist(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := pro.New(testhelpers.NewAcceptanceClient(t))
		if _, err := c.GetAppRequestSettingsV1(context.Background()); err != nil {
			return fmt.Errorf("App Request settings singleton not readable after destroy: %s", err)
		}
		return nil
	}
}

// smtpFixture is the SMTP server configuration App Requests depends on. Uses dummy
// documentation values (RFC 5737 / RFC 2606) — no real credentials.
const smtpFixture = `
	resource "jamfplatform_pro_smtp_server" "fixture" {
		enabled             = true
		authentication_type = "BASIC"
		sender_settings = {
			email_address = "notifications@example.com"
			display_name  = "Example Notifications"
		}
		connection_settings = {
			host               = "192.0.2.25"
			port               = 465
			encryption_type    = "SSL"
			connection_timeout = 30
		}
		basic_auth_credentials = {
			username            = "svc@example.com"
			password            = "dummy-password-1"
			password_wo_version = 1
		}
	}
`

// formFieldFixture is the App Request form field App Requests depends on: Jamf Pro refuses
// an enabling write while the tenant has no form fields (wire-probed 2026-09-04), and the
// prerequisite cannot be satisfied by the settings payload.
func formFieldFixture(title string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_app_request_form_field" "fixture" {
			title    = %q
			priority = 1
		}
	`, title)
}

func requesterGroupFixture(name string) string {
	return fmt.Sprintf(`
		resource "jamfplatform_pro_user_group" "fixture" {
			name       = %q
			group_type = "static"
		}
	`, name)
}

// TestAccResource_ProAppRequestSettings_Lifecycle adopts the settings (disabled), then
// enables App Requests with a static requester group and an extra approver email, then
// imports.
func TestAccResource_ProAppRequestSettings_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	groupName := fmt.Sprintf("ZTFACC App Request Requesters %s", suffix)
	fieldTitle := fmt.Sprintf("ZTFACC App Request Reason %s", suffix)
	fixtures := smtpFixture + requesterGroupFixture(groupName) + formFieldFixture(fieldTitle)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSettingsStillExist(t),
		Steps: []resource.TestStep{
			{
				// Adopt: disabled, one approver, canonical (upper-case) locale.
				Config: fixtures + `
					resource "jamfplatform_pro_app_request_settings" "test" {
						enabled          = false
						app_store_locale = "US"
						approver_emails  = ["approver-a@example.com"]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(settingsAddr, "id", "singleton"),
					resource.TestCheckResourceAttr(settingsAddr, "enabled", "false"),
					resource.TestCheckResourceAttr(settingsAddr, "app_store_locale", "US"),
					resource.TestCheckResourceAttr(settingsAddr, "approver_emails.#", "1"),
				),
			},
			{
				// Enable with the static requester group, the form field the tenant must
				// hold before App Requests can be enabled, and a second approver email.
				Config: fixtures + `
					resource "jamfplatform_pro_app_request_settings" "test" {
						enabled                 = true
						app_store_locale        = "US"
						approver_emails         = ["approver-a@example.com", "approver-b@example.com"]
						requester_user_group_id = tonumber(jamfplatform_pro_user_group.fixture.id)
						depends_on = [
							jamfplatform_pro_smtp_server.fixture,
							jamfplatform_pro_app_request_form_field.fixture,
						]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(settingsAddr, "enabled", "true"),
					resource.TestCheckResourceAttr(settingsAddr, "approver_emails.#", "2"),
					resource.TestCheckResourceAttrPair(settingsAddr, "requester_user_group_id", "jamfplatform_pro_user_group.fixture", "id"),
				),
			},
			{
				// Disable again: omitting the group clears it (the provider does not carry a
				// requester group on a disabled write). Leaves the tenant clean for teardown,
				// so destroying the group fixture cannot strand a dangling reference.
				Config: fixtures + `
					resource "jamfplatform_pro_app_request_settings" "test" {
						enabled         = false
						approver_emails = ["approver-a@example.com"]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(settingsAddr, "enabled", "false"),
					resource.TestCheckNoResourceAttr(settingsAddr, "requester_user_group_id"),
				),
			},
			{
				ResourceName:            settingsAddr,
				ImportState:             true,
				ImportStateId:           "singleton",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProAppRequestSettings_NoApprovers covers a tenant with App
// Requests switched off and therefore no approvers, which is an ordinary state and
// was not declarable until issue #379: approver_emails carried a minimum-size
// validator taken from the Jamf Pro admin UI, while Jamf Pro itself accepts an
// empty list and round-trips it (wire-probed 2026-09-05, disabled AND enabled,
// against a control that satisfied both prerequisites). Because this resource is a
// per-tenant singleton that always exists, the validator made such a tenant
// impossible to adopt at all.
//
// Step 2 is the generation guard: an empty set has to survive being written back
// out as configuration, which is where the defect surfaced.
func TestAccResource_ProAppRequestSettings_NoApprovers(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSettingsStillExist(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_app_request_settings" "test" {
						enabled         = false
						approver_emails = []
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(settingsAddr, "enabled", "false"),
					resource.TestCheckResourceAttr(settingsAddr, "approver_emails.#", "0"),
				),
			},
			testhelpers.GenerateConfigStep(settingsAddr),
		},
	})
}

// TestAccResource_ProAppRequestSettings_EnabledRequiresGroup confirms the plan-time check:
// enabling App Requests while the requester group is a known-null carried from prior state
// fails before apply. Step 1 establishes disabled settings (requester group null in state);
// step 2 flips enabled=true while omitting the group, so UseStateForUnknown carries the
// known null into the plan and the ModifyPlan check fires. (On a brand-new resource the
// group is Unknown at plan and the check correctly defers to the server 400 instead.)
func TestAccResource_ProAppRequestSettings_EnabledRequiresGroup(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSettingsStillExist(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_app_request_settings" "test" {
						enabled         = false
						approver_emails = ["approver-a@example.com"]
					}
				`,
			},
			{
				Config: `
					resource "jamfplatform_pro_app_request_settings" "test" {
						enabled         = true
						approver_emails = ["approver-a@example.com"]
					}
				`,
				ExpectError: regexp.MustCompile("requester_user_group_id"),
			},
		},
	})
}

// TestAccResource_ProAppRequestSettings_NonCanonicalLocale confirms a non-canonical locale
// (lowercase) is rejected at plan time with the canonical form, rather than failing apply
// with an inconsistent-result error.
func TestAccResource_ProAppRequestSettings_NonCanonicalLocale(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSettingsStillExist(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_app_request_settings" "test" {
						enabled          = false
						app_store_locale = "us"
						approver_emails  = ["approver-a@example.com"]
					}
				`,
				ExpectError: regexp.MustCompile("canonical"),
			},
		},
	})
}

// TestAccResource_ProAppRequestSettings_InvalidLocale confirms the plan-time country-code
// preflight rejects an unknown code (validated live against the tenant's supported list).
func TestAccResource_ProAppRequestSettings_InvalidLocale(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSettingsStillExist(t),
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_app_request_settings" "test" {
						enabled          = false
						app_store_locale = "ZZ"
						approver_emails  = ["approver-a@example.com"]
					}
				`,
				ExpectError: regexp.MustCompile("country"),
			},
		},
	})
}
