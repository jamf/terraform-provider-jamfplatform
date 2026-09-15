// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package macos_onboarding_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/pro"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// onboardingFixtures returns HCL for three Self-Service-eligible objects the test
// self-mints to reference from onboarding_items — one of each managed entity type:
// a policy (OS_X_POLICY), a configuration profile (OS_X_CONFIG_PROFILE), and a Mac
// App Store app (OS_X_MAC_APP). Each is flagged "Make Available in Self Service" /
// use_for_self_service so it is onboarding-eligible, and left UNSCOPED so it targets
// no real devices (wire-probed: an unscoped Self Service object is still eligible).
// Reference them as jamfplatform_pro_policy.fixture_policy.id, etc.
func onboardingFixtures(suffix string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_policy" "fixture_policy" {
  general = {
    name    = "tf-acc-onboard-policy-%[1]s"
    enabled = true
  }
  self_service = {
    use_for_self_service      = true
    self_service_display_name = "tf-acc-onboard-policy-%[1]s"
  }
}

resource "jamfplatform_pro_macos_configuration_profile" "fixture_profile" {
  general = {
    name                = "tf-acc-onboard-profile-%[1]s"
    distribution_method = "Make Available in Self Service"
    payloads            = <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array/>
	<key>PayloadDisplayName</key>
	<string>tf-acc-onboard-profile-%[1]s</string>
	<key>PayloadIdentifier</key>
	<string>com.example.tfacc.onboarding.%[1]s</string>
	<key>PayloadScope</key>
	<string>System</string>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>4C3E5B1A-0000-4000-8000-000000000001</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>
EOF
  }
}

resource "jamfplatform_pro_mac_app_store_app" "fixture_app" {
  general = {
    name            = "tf-acc-onboard-app-%[1]s"
    version         = "1.0"
    bundle_id       = "com.example.tfacc.onboarding"
    url             = "https://apps.apple.com/app/id000000001"
    deployment_type = "Make Available in Self Service"
  }
}
`, suffix)
}

// snapshotAndRestoreOnboarding captures the tenant's current onboarding configuration
// and registers a cleanup that restores it (writable fields only). This runs AFTER the
// test framework destroys the resources: destroying the singleton leaves the
// last-applied config on the tenant (no remote delete), and the fixture objects it
// referenced are then deleted — so the restore re-points onboarding at the tenant's
// original, still-existing items, leaving onboarding exactly as it was found.
func snapshotAndRestoreOnboarding(t *testing.T) {
	t.Helper()
	c := pro.New(testhelpers.NewAcceptanceClient(t))
	before, err := c.GetOnboardingV1(context.Background())
	if err != nil {
		t.Fatalf("snapshot onboarding config: %v", err)
	}
	t.Cleanup(func() {
		items := make([]pro.OnboardingItem, 0, len(before.OnboardingItems))
		for _, it := range before.OnboardingItems {
			items = append(items, pro.OnboardingItem{
				EntityID:              it.EntityID,
				SelfServiceEntityType: it.SelfServiceEntityType,
				Priority:              it.Priority,
			})
		}
		restore := &pro.OnboardingConfiguration{Enabled: before.Enabled, OnboardingItems: items}
		if _, err := c.UpdateOnboardingV1(context.Background(), restore); err != nil {
			t.Errorf("restore onboarding config: %v", err)
		}
	})
}

// checkSingletonRecordStillExists verifies the onboarding record persists on the tenant
// after Terraform destroys the resource from state (singleton — remote delete is a no-op).
func checkSingletonRecordStillExists(t *testing.T) resource.TestCheckFunc {
	return testhelpers.RequireSingletonStillExists(t, "onboarding", func(ctx context.Context) (any, error) {
		return pro.New(testhelpers.NewAcceptanceClient(t)).GetOnboardingV1(ctx)
	})
}

const (
	addrOnboarding = "jamfplatform_pro_macos_onboarding.test"
	addrPolicy     = "jamfplatform_pro_policy.fixture_policy"
	addrProfile    = "jamfplatform_pro_macos_configuration_profile.fixture_profile"
	addrApp        = "jamfplatform_pro_mac_app_store_app.fixture_app"
)

// TestAccResource_ProMacosOnboarding_Basic exercises add / reorder / remove / enum /
// enabled-toggle across steps plus import, referencing self-minted Self-Service
// fixtures. The tenant's prior onboarding config is snapshotted and restored.
func TestAccResource_ProMacosOnboarding_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreOnboarding(t)
	suffix := testhelpers.RunSuffix()
	fx := onboardingFixtures(suffix)

	// step1: policy + profile, enabled.
	step1 := fx + `
		resource "jamfplatform_pro_macos_onboarding" "test" {
			enabled = true
			onboarding_items = [
				{
					entity_id                = jamfplatform_pro_policy.fixture_policy.id
					self_service_entity_type = "OS_X_POLICY"
				},
				{
					entity_id                = jamfplatform_pro_macos_configuration_profile.fixture_profile.id
					self_service_entity_type = "OS_X_CONFIG_PROFILE"
				},
			]
		}
	`

	// stepReorder: same two items, swapped — pure reorder, nothing else changed. This
	// isolates the load-bearing behaviour: the Computed-only nested echoes are modelled
	// as plain Computed with NO plan modifier, so a reordered element must go
	// Unknown→fill on apply and never trip "inconsistent result after apply". A clean
	// apply here validates the reorder path (see the package design notes / spike).
	stepReorder := fx + `
		resource "jamfplatform_pro_macos_onboarding" "test" {
			enabled = true
			onboarding_items = [
				{
					entity_id                = jamfplatform_pro_macos_configuration_profile.fixture_profile.id
					self_service_entity_type = "OS_X_CONFIG_PROFILE"
				},
				{
					entity_id                = jamfplatform_pro_policy.fixture_policy.id
					self_service_entity_type = "OS_X_POLICY"
				},
			]
		}
	`

	// step2: add the Mac app (third entity type), reorder, and disable.
	step2 := fx + `
		resource "jamfplatform_pro_macos_onboarding" "test" {
			enabled = false
			onboarding_items = [
				{
					entity_id                = jamfplatform_pro_macos_configuration_profile.fixture_profile.id
					self_service_entity_type = "OS_X_CONFIG_PROFILE"
				},
				{
					entity_id                = jamfplatform_pro_mac_app_store_app.fixture_app.id
					self_service_entity_type = "OS_X_MAC_APP"
				},
				{
					entity_id                = jamfplatform_pro_policy.fixture_policy.id
					self_service_entity_type = "OS_X_POLICY"
				},
			]
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: step1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addrOnboarding, "id", "singleton"),
					resource.TestCheckResourceAttr(addrOnboarding, "enabled", "true"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.#", "2"),
					resource.TestCheckResourceAttrPair(addrOnboarding, "onboarding_items.0.entity_id", addrPolicy, "id"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.0.self_service_entity_type", "OS_X_POLICY"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.0.priority", "1"),
					resource.TestCheckResourceAttrPair(addrOnboarding, "onboarding_items.1.entity_id", addrProfile, "id"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.1.priority", "2"),
					resource.TestCheckResourceAttrSet(addrOnboarding, "onboarding_items.0.entity_name"),
				),
			},
			{
				Config: stepReorder,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.#", "2"),
					resource.TestCheckResourceAttrPair(addrOnboarding, "onboarding_items.0.entity_id", addrProfile, "id"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.0.self_service_entity_type", "OS_X_CONFIG_PROFILE"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.0.priority", "1"),
					resource.TestCheckResourceAttrPair(addrOnboarding, "onboarding_items.1.entity_id", addrPolicy, "id"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.1.priority", "2"),
				),
			},
			{
				Config: step2,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addrOnboarding, "enabled", "false"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.#", "3"),
					resource.TestCheckResourceAttrPair(addrOnboarding, "onboarding_items.0.entity_id", addrProfile, "id"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.0.priority", "1"),
					resource.TestCheckResourceAttrPair(addrOnboarding, "onboarding_items.1.entity_id", addrApp, "id"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.1.self_service_entity_type", "OS_X_MAC_APP"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.1.priority", "2"),
					resource.TestCheckResourceAttrPair(addrOnboarding, "onboarding_items.2.entity_id", addrPolicy, "id"),
					resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.2.priority", "3"),
				),
			},
			{
				ResourceName:            addrOnboarding,
				ImportState:             true,
				ImportStateId:           "singleton",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// TestAccResource_ProMacosOnboarding_ClearItems proves an empty onboarding_items list
// removes every item.
func TestAccResource_ProMacosOnboarding_ClearItems(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreOnboarding(t)
	suffix := testhelpers.RunSuffix()
	fx := onboardingFixtures(suffix)

	withItem := fx + `
		resource "jamfplatform_pro_macos_onboarding" "test" {
			enabled = false
			onboarding_items = [
				{
					entity_id                = jamfplatform_pro_policy.fixture_policy.id
					self_service_entity_type = "OS_X_POLICY"
				},
			]
		}
	`

	cleared := fx + `
		resource "jamfplatform_pro_macos_onboarding" "test" {
			enabled          = false
			onboarding_items = []
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkSingletonRecordStillExists(t),
		Steps: []resource.TestStep{
			{
				Config: withItem,
				Check:  resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.#", "1"),
			},
			{
				Config: cleared,
				Check:  resource.TestCheckResourceAttr(addrOnboarding, "onboarding_items.#", "0"),
			},
		},
	})
}

// TestAccResource_ProMacosOnboarding_RejectsInvalidEntityType verifies the plan-time
// OneOf validator rejects an unsupported self_service_entity_type. No tenant write
// occurs, so no fixtures are needed. The "OS_X_EBOOK" token avoids whitespace at the
// ~80-col error wrap.
func TestAccResource_ProMacosOnboarding_RejectsInvalidEntityType(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					resource "jamfplatform_pro_macos_onboarding" "test" {
						enabled = true
						onboarding_items = [
							{
								entity_id                = "1"
								self_service_entity_type = "OS_X_EBOOK"
							},
						]
					}
				`,
				ExpectError: regexp.MustCompile(`Invalid Attribute Value Match`),
			},
		},
	})
}

// TestAccResource_ProMacosOnboarding_RejectsNonSingletonImport verifies the ImportState
// guard rejects any identifier other than "singleton".
func TestAccResource_ProMacosOnboarding_RejectsNonSingletonImport(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreOnboarding(t)
	suffix := testhelpers.RunSuffix()
	fx := onboardingFixtures(suffix)

	cfg := fx + `
		resource "jamfplatform_pro_macos_onboarding" "test" {
			enabled = false
			onboarding_items = [
				{
					entity_id                = jamfplatform_pro_policy.fixture_policy.id
					self_service_entity_type = "OS_X_POLICY"
				},
			]
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				ResourceName:  addrOnboarding,
				ImportState:   true,
				ImportStateId: "not-the-singleton",
				ExpectError:   regexp.MustCompile(`Invalid singleton import identifier`),
			},
		},
	})
}

// TestAccDataSource_ProMacosOnboarding_Basic reads the singleton via the data source.
func TestAccDataSource_ProMacosOnboarding_Basic(t *testing.T) {
	testhelpers.AccPreCheck(t)
	snapshotAndRestoreOnboarding(t)
	suffix := testhelpers.RunSuffix()
	fx := onboardingFixtures(suffix)

	cfg := fx + `
		resource "jamfplatform_pro_macos_onboarding" "test" {
			enabled = true
			onboarding_items = [
				{
					entity_id                = jamfplatform_pro_policy.fixture_policy.id
					self_service_entity_type = "OS_X_POLICY"
				},
			]
		}

		data "jamfplatform_pro_macos_onboarding" "lookup" {
			depends_on = [jamfplatform_pro_macos_onboarding.test]
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_macos_onboarding.lookup", "id", "singleton"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_macos_onboarding.lookup", "enabled", "true"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_macos_onboarding.lookup", "onboarding_items.#", "1"),
					resource.TestCheckResourceAttrPair("data.jamfplatform_pro_macos_onboarding.lookup", "onboarding_items.0.entity_id", addrPolicy, "id"),
				),
			},
		},
	})
}

// TestAccDataSource_ProMacosOnboarding_EligibleItems reads the parameterised eligible
// items discovery data source for each entity_type.
func TestAccDataSource_ProMacosOnboarding_EligibleItems(t *testing.T) {
	testhelpers.AccPreCheck(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					data "jamfplatform_pro_macos_onboarding_eligible_items" "policies" {
						entity_type = "policies"
					}
					data "jamfplatform_pro_macos_onboarding_eligible_items" "profiles" {
						entity_type = "configuration_profiles"
					}
					data "jamfplatform_pro_macos_onboarding_eligible_items" "apps" {
						entity_type = "apps"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.jamfplatform_pro_macos_onboarding_eligible_items.policies", "id", "policies"),
					resource.TestCheckResourceAttrSet("data.jamfplatform_pro_macos_onboarding_eligible_items.policies", "items.#"),
					resource.TestCheckResourceAttr("data.jamfplatform_pro_macos_onboarding_eligible_items.apps", "entity_type", "apps"),
				),
			},
		},
	})
}
