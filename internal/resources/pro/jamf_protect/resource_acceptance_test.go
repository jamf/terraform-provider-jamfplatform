// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package jamf_protect_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// protectCreds gates every Protect-touching test: all of
// JAMFPLATFORM_ACC_PRO_PROTECT_URL / _CLIENT_ID / _PASSWORD must be set (matching the
// SDK acceptance-test precedent). Values are read trimmed, and a bare Protect
// console URL is accepted — the register endpoint expects the GraphQL
// endpoint, so /graphql is appended when missing.
//
// NOTE: these tests register the env-var credentials over whatever
// registration the tenant currently holds and leave the tenant UNREGISTERED
// at the end. A pre-existing foreign registration cannot be restored (its
// password is write-only) — re-register manually if that matters.
func protectCreds(t *testing.T) (protectURL, clientID, password string) {
	t.Helper()
	protectURL = strings.TrimSpace(testhelpers.AccEnv("JAMFPLATFORM_ACC_PRO_PROTECT_URL"))
	clientID = strings.TrimSpace(testhelpers.AccEnv("JAMFPLATFORM_ACC_PRO_PROTECT_CLIENT_ID"))
	password = strings.TrimSpace(testhelpers.AccEnv("JAMFPLATFORM_ACC_PRO_PROTECT_PASSWORD"))
	if protectURL == "" || clientID == "" || password == "" {
		t.Skip("JAMFPLATFORM_ACC_PRO_PROTECT_{URL,CLIENT_ID,PASSWORD} not all set — Jamf Protect tests need a Jamf Protect API client")
	}
	if !strings.HasSuffix(protectURL, "/graphql") {
		protectURL = strings.TrimRight(protectURL, "/") + "/graphql"
	}
	t.Log("warning: this test overwrites any existing Jamf Protect registration and leaves the tenant unregistered; a pre-existing registration cannot be restored (password is write-only)")
	return protectURL, clientID, password
}

func protectClient(t *testing.T) *pro.Client {
	t.Helper()
	return pro.New(testhelpers.NewAcceptanceClient(t))
}

// checkUnregistered is the CheckDestroy: after Terraform destroys the
// resource, the real unregister must have run, so the settings GET returns
// 404.
func checkUnregistered(t *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		_, err := protectClient(t).GetJamfProtectSettingsV1(context.Background())
		if err == nil {
			return fmt.Errorf("Jamf Protect is still registered after destroy")
		}
		if !helpers.IsNotFoundError(err) {
			return fmt.Errorf("reading Jamf Protect registration post-destroy: %w", err)
		}
		return nil
	}
}

// protectConfig renders the resource config from the env-var credentials —
// never hardcode Protect URLs or secrets. autoInstall is an optional extra
// attribute line (e.g. "auto_install = true") so the first step can exercise
// the server default.
func protectConfig(protectURL, clientID, password string, woVersion int, autoInstall string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_jamf_protect" "test" {
  api_url             = %q
  client_id           = %q
  password            = %q
  password_wo_version = %d
  %s
}
`, protectURL, clientID, password, woVersion, autoInstall)
}

// TestAccResource_ProJamfProtect exercises the full registration lifecycle:
// register (server-default auto_install) → toggle auto_install (PUT-only
// path) → password_wo_version bump (in-place re-register path, same
// credentials) → import. CheckDestroy asserts the real unregister ran
// (GET → 404).
func TestAccResource_ProJamfProtect(t *testing.T) {
	testhelpers.AccPreCheck(t)
	protectURL, clientID, password := protectCreds(t)

	const rn = "jamfplatform_pro_jamf_protect.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkUnregistered(t),
		Steps: []resource.TestStep{
			{
				// Register. auto_install omitted — adopts the server default.
				Config: protectConfig(protectURL, clientID, password, 1, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "id", "singleton"),
					resource.TestCheckResourceAttr(rn, "api_url", protectURL),
					resource.TestCheckResourceAttr(rn, "client_id", clientID),
					resource.TestCheckResourceAttr(rn, "auto_install", "false"),
					resource.TestCheckResourceAttrSet(rn, "registration_id"),
					resource.TestCheckResourceAttrSet(rn, "api_client_name"),
					resource.TestCheckResourceAttrSet(rn, "platform_plan_sync"),
					resource.TestCheckResourceAttrSet(rn, "sync_status"),
					resource.TestCheckNoResourceAttr(rn, "password"),
				),
			},
			{
				// PUT-only path: toggle the sole mutable settings field.
				Config: protectConfig(protectURL, clientID, password, 1, "auto_install = true"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "auto_install", "true"),
				),
			},
			{
				// Re-register path: a password_wo_version bump re-POSTs the
				// (same) credentials in place; auto_install must survive.
				Config: protectConfig(protectURL, clientID, password, 2, "auto_install = true"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "auto_install", "true"),
					resource.TestCheckResourceAttrSet(rn, "registration_id"),
				),
			},
			{
				ResourceName:      rn,
				ImportState:       true,
				ImportStateId:     "singleton",
				ImportStateVerify: true,
				// password is WriteOnly (never in state, never returned);
				// password_wo_version round-trips from prior state only;
				// last_sync_time / sync_status are volatile; timeouts has no
				// remote equivalent.
				ImportStateVerifyIgnore: []string{"password", "password_wo_version", "last_sync_time", "sync_status", "timeouts"},
			},
		},
	})
}

// TestAccResource_ProJamfProtect_RejectsNonSingletonImport verifies the
// ImportState guard.
func TestAccResource_ProJamfProtect_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	protectURL, clientID, password := protectCreds(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkUnregistered(t),
		Steps: []resource.TestStep{
			{Config: protectConfig(protectURL, clientID, password, 1, "")},
			{
				ResourceName:  "jamfplatform_pro_jamf_protect.test",
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccDataSource_ProJamfProtectPlans reads the synced plans catalog. The
// config includes the registration resource so a registration (and the
// Create-time plans sync) exists before the data source reads. Gated on the
// same Protect env vars — the data source itself would work unregistered
// (the catalog persists), but exercising it without a registration would
// depend on undefined tenant history.
func TestAccDataSource_ProJamfProtectPlans(t *testing.T) {
	testhelpers.AccPreCheck(t)
	protectURL, clientID, password := protectCreds(t)

	config := protectConfig(protectURL, clientID, password, 1, "") + `
data "jamfplatform_pro_jamf_protect_plans" "all" {
  depends_on = [jamfplatform_pro_jamf_protect.test]
}
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkUnregistered(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_jamf_protect_plans.all", "id", "jamf_protect_plans"),
					// An empty catalog is not an error — assert the list
					// materialised, not its size.
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_jamf_protect_plans.all", "plans.#"),
				),
			},
		},
	})
}
