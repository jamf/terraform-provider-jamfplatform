// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic JSON Web Token configurations
// endpoint. The server allows AT MOST ONE configuration per Jamf Pro instance
// (a second create is rejected), so every test in this package must reuse the
// single tenant slot: one resource per step, tests run serially.

package pki_json_web_token_configuration_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const jwtResourceAddr = "jamfplatform_pro_pki_json_web_token_configuration.test"

// Dummy throwaway keys — base64 of nothing meaningful, encoded at runtime so
// the literal plaintext stays low-entropy (secret scanners flag pre-encoded
// base64 constants as credentials).
var (
	jwtAccKeyV1 = base64.StdEncoding.EncodeToString([]byte("tf-acc-dummy-jwt-key-v1"))
	jwtAccKeyV2 = base64.StdEncoding.EncodeToString([]byte("tf-acc-dummy-jwt-key-v2"))
)

// testAccCheckJSONWebTokenConfigurationDestroy verifies records created during
// the test were destroyed.
func testAccCheckJSONWebTokenConfigurationDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_pki_json_web_token_configuration" {
				continue
			}
			_, err := c.GetJsonWebTokenConfigurationByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("error checking Jamf Pro JSON Web Token configuration %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("Jamf Pro JSON Web Token configuration %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

// jwtConfig renders the single-slot resource config. enabled is rendered only
// when non-nil so the create step exercises the server default absorption.
func jwtConfig(name, key string, woVersion, tokenExpiry int, enabled *bool) string {
	enabledLine := ""
	if enabled != nil {
		enabledLine = fmt.Sprintf("enabled = %t", *enabled)
	}
	return fmt.Sprintf(`
		resource "jamfplatform_pro_pki_json_web_token_configuration" "test" {
			name                      = %q
			encryption_key_wo         = %q
			encryption_key_wo_version = %d
			token_expiry              = %d
			%s
		}
	`, name, key, woVersion, tokenExpiry, enabledLine)
}

// TestAccResource_ProPkiJSONWebTokenConfiguration_Lifecycle walks the full
// lifecycle on the single tenant slot: create → update name + token_expiry →
// rotate the key via a bumped encryption_key_wo_version → toggle enabled
// false→true → import-by-id.
func TestAccResource_ProPkiJSONWebTokenConfiguration_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheck(t)
	name := "tf-acc-pro-jwt-" + testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckJSONWebTokenConfigurationDestroy(t),
		Steps: []resource.TestStep{
			{
				// Create: name + key + token_expiry; enabled omitted so the
				// server default (enabled) is absorbed.
				Config: jwtConfig(name, jwtAccKeyV1, 1, 30, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(jwtResourceAddr, "id"),
					resource.TestCheckResourceAttr(jwtResourceAddr, "name", name),
					resource.TestCheckResourceAttr(jwtResourceAddr, "token_expiry", "30"),
					resource.TestCheckResourceAttr(jwtResourceAddr, "enabled", "true"),
					// encryption_key_wo is WriteOnly — never surfaced in state.
					resource.TestCheckNoResourceAttr(jwtResourceAddr, "encryption_key_wo"),
				),
			},
			{
				// Update name + token_expiry (same key, version unchanged →
				// key omitted on the wire; the stored key is retained).
				Config: jwtConfig(name+"-renamed", jwtAccKeyV1, 1, 60, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(jwtResourceAddr, "name", name+"-renamed"),
					resource.TestCheckResourceAttr(jwtResourceAddr, "token_expiry", "60"),
				),
			},
			{
				// Rotate the key: bump encryption_key_wo_version → re-sent.
				Config: jwtConfig(name+"-renamed", jwtAccKeyV2, 2, 60, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(jwtResourceAddr, "encryption_key_wo_version", "2"),
					resource.TestCheckResourceAttr(jwtResourceAddr, "name", name+"-renamed"),
				),
			},
			{
				// Toggle enabled → false.
				Config: jwtConfig(name+"-renamed", jwtAccKeyV2, 2, 60, new(false)),
				Check:  resource.TestCheckResourceAttr(jwtResourceAddr, "enabled", "false"),
			},
			{
				// Toggle enabled → true.
				Config: jwtConfig(name+"-renamed", jwtAccKeyV2, 2, 60, new(true)),
				Check:  resource.TestCheckResourceAttr(jwtResourceAddr, "enabled", "true"),
			},
			{
				ResourceName:            jwtResourceAddr,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts", "encryption_key_wo", "encryption_key_wo_version"},
			},
		},
	})
}
