// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic
// /mobiledeviceconfigurationprofiles endpoint. Classic has known concurrency
// issues when multiple writes hit the same resource type — keep these
// tests serial with any other classic acceptance work.

package mobile_device_configuration_profile_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/proclassic"

	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/payloadhelpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/plisthelpers"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// ── Fixture helpers ───────────────────────────────────────────────────────────

func testdataFile(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("could not resolve caller path")
	}
	abs, err := filepath.Abs(filepath.Join(filepath.Dir(file), "testdata", name))
	if err != nil {
		t.Fatalf("resolving testdata path %q: %v", name, err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("testdata fixture %q not present at %q: %v", name, abs, err)
	}
	return abs
}

// readFixture returns a testdata payload, newline-terminated. Every config
// helper here splices the payload straight into an HCL heredoc whose terminator
// follows it ("%sEOF"), so a fixture saved without a trailing newline puts EOF
// on the same line as the last tag, the heredoc never closes, and HCL fails the
// step with "Unterminated template string" before the provider is reached.
// Normalised here rather than in the fixtures: those mirror real exported
// profiles, which are not reliably newline-terminated, and are more useful
// byte-faithful.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(testdataFile(t, name))
	if err != nil {
		t.Fatalf("reading testdata fixture %q: %v", name, err)
	}
	s := string(b)
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

// TestAccFixtureHeredocTermination guards the whole fixture corpus against the
// break above. It talks to nothing — it just checks that each payload still
// closes its heredoc once spliced into a config — so it costs nothing to run
// and fails with the offending fixture named, instead of every step of every
// test that happens to use it reporting an HCL parse error.
func TestAccFixtureHeredocTermination(t *testing.T) {
	for _, path := range fixturePaths(t) {
		name := filepath.Base(path)
		if cfg := configMinimal("heredoc-check", readFixture(t, name)); !strings.Contains(cfg, "\nEOF\n") {
			t.Errorf("fixture %s does not terminate its heredoc — the payload must end with a newline", name)
		}
	}
}

// fixturePaths lists every .mobileconfig in testdata.
func fixturePaths(t *testing.T) []string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("could not resolve caller path")
	}
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(file), "testdata", "*.mobileconfig"))
	if err != nil {
		t.Fatalf("globbing testdata fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no .mobileconfig fixtures found in testdata")
	}
	return paths
}

// newUUID generates a random v4 UUID string.
func newUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("generating UUID: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// freshPayload reads a fixture and injects a fresh top-level PayloadUUID +
// PayloadIdentifier so the same on-disk fixture can be used across multiple
// test runs without hitting Jamf Pro's duplicate-UUID rejection. Always
// returns a string ending with \n so Terraform heredoc EOF is on its own line.
func freshPayload(t *testing.T, fixture string) string {
	t.Helper()
	raw := readFixture(t, fixture)
	uuid := newUUID(t)
	out, err := payloadhelpers.InjectTopLevelIdentifierValues([]byte(raw), uuid, uuid)
	if err != nil {
		t.Fatalf("freshPayload(%q): %v", fixture, err)
	}
	s := string(out)
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

// ── SDK helpers ───────────────────────────────────────────────────────────────

func checkDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testhelpers.NewProClassicClient(t)
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_mobile_device_configuration_profile" {
				continue
			}
			_, err := c.GetMobileDeviceConfigurationProfileByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("checking mobile device configuration profile %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("mobile device configuration profile %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func createDummyUser(t *testing.T, name string) string {
	t.Helper()
	c := testhelpers.NewProClassicClient(t)
	ctx := context.Background()
	got, err := c.CreateUserByID(ctx, "0", &proclassic.UserPost{Name: &name})
	if err != nil || got == nil || got.ID == nil {
		t.Fatalf("CreateUserByID(%q): %v", name, err)
	}
	id := fmt.Sprintf("%d", *got.ID)
	t.Cleanup(func() {
		if err := c.DeleteUserByID(context.Background(), id); err != nil && !helpers.IsNotFoundError(err) {
			t.Logf("cleanup DeleteUserByID(%s): %v", id, err)
		}
	})
	return id
}

func createDummyMobileDeviceGroup(t *testing.T, name string) string {
	t.Helper()
	c := testhelpers.NewProClassicClient(t)
	ctx := context.Background()
	isSmart := false
	got, err := c.CreateMobileDeviceGroupByID(ctx, "0", &proclassic.MobileDeviceGroup{
		Name:    &name,
		IsSmart: &isSmart,
	})
	if err != nil || got == nil || got.ID == nil {
		t.Fatalf("CreateMobileDeviceGroupByID(%q): %v", name, err)
	}
	id := fmt.Sprintf("%d", *got.ID)
	t.Cleanup(func() {
		if err := c.DeleteMobileDeviceGroupByID(context.Background(), id); err != nil && !helpers.IsNotFoundError(err) {
			t.Logf("cleanup DeleteMobileDeviceGroupByID(%s): %v", id, err)
		}
	})
	return id
}

// ── Config builders ───────────────────────────────────────────────────────────

func configMinimal(name, payload string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
}
`, name, payload)
}

func configWithDescription(name, payload, desc string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name        = %q
    description = %q
    payloads = <<EOF
%sEOF
  }
}
`, name, desc, payload)
}

func configWithDistribution(name, payload, dist string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = %q
    payloads = <<EOF
%sEOF
  }
}
`, name, dist, payload)
}

func configAllMobileDevices(name, payload string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      all_mobile_devices = true
    }
  }
}
`, name, payload)
}

func configScopeWithExclusions(name, payload string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      all_mobile_devices = true
    }
    exclusions = {
      directory_service_or_local_user_names = ["nonexistent-acc-test-user"]
    }
  }
}
`, name, payload)
}

func configScopeWithMobileDeviceGroupIDs(name, payload, groupID string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      mobile_device_group_ids = [%q]
    }
  }
}
`, name, payload, groupID)
}

func configScopeJSSUser(name, payload, userID string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      user_ids = [%q]
    }
  }
}
`, name, payload, userID)
}

// configScopeJSSUserCleared declares user_ids = [] — the granular-ownership
// clear gesture: a declared empty category is Terraform-owned and empties it,
// where omitting the category would leave it as configured outside Terraform.
func configScopeJSSUserCleared(name, payload string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      user_ids = []
    }
  }
}
`, name, payload)
}

func configWithSelfServiceAuthPassword(name, payload, password string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = "Make Available in Self Service"
    payloads = <<EOF
%sEOF
  }
  self_service = {
    feature_on_main_page     = true
    removal_disallowed       = "With Authorization"
    authorization_password   = %q
  }
}
`, name, payload, password)
}

func configWithSelfService(name, payload string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = "Make Available in Self Service"
    payloads = <<EOF
%sEOF
  }
  self_service = {
    feature_on_main_page     = true
    removal_disallowed       = "Never"
  }
}
`, name, payload)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestAccResource_MobileDeviceConfigurationProfile_Minimal — minimal create /
// rename / idempotent re-apply. Uses a real Exchange ActiveSync payload
// (profile_50) to exercise PayloadUUID injection on update and verify the
// no-ghost-profile path.
func TestAccResource_MobileDeviceConfigurationProfile_Minimal(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-min-" + suffix
	renamed := name + "-renamed"
	payload := freshPayload(t, "profile_50.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name, payload),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("general").AtMapKey("name"),
						knownvalue.StringExact(name),
					),
				},
			},
			{
				Config: configMinimal(renamed, payload),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("general").AtMapKey("name"),
						knownvalue.StringExact(renamed),
					),
				},
			},
			{
				// Re-apply identical config → must produce empty plan.
				Config: configMinimal(renamed, payload),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_PayloadByteDifferentSemanticallyEqual
// — compact XML reformatted by inserting newlines between tags. Bytes differ
// but semantics don't; diff suppression must produce ResourceActionNoop.
func TestAccResource_MobileDeviceConfigurationProfile_PayloadByteDifferentSemanticallyEqual(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-sem-" + suffix
	payload := freshPayload(t, "profile_48.mobileconfig")

	// Compact XML has no whitespace between tags. Insert newlines at every
	// ><  boundary — byte-different, semantically identical to a plist parser.
	reformatted := strings.ReplaceAll(payload, "><", ">\n<")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configMinimal(name, payload)},
			{
				Config: configMinimal(name, reformatted),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							"jamfplatform_pro_mobile_device_configuration_profile.test",
							plancheck.ResourceActionNoop,
						),
					},
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_RealPayloadChangeProducesPlan
// — increments maxFailedAttempts in profile_48 (Passcode policy). The field
// is not in the masked set so the change must surface as drift and produce
// a non-empty plan. Uses a small payload to avoid Jamf PUT timeouts.
func TestAccResource_MobileDeviceConfigurationProfile_RealPayloadChangeProducesPlan(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-real-" + suffix
	payload := freshPayload(t, "profile_48.mobileconfig")

	// freshPayload marshals via the plist library; use its output format.
	// Skip guard catches format drift if the fixture value changes.
	const before = "<key>maxFailedAttempts</key>\n\t\t\t\t<integer>2</integer>"
	const after = "<key>maxFailedAttempts</key>\n\t\t\t\t<integer>10</integer>"
	tampered := strings.Replace(payload, before, after, 1)
	if tampered == payload {
		t.Skip("expected to tamper with maxFailedAttempts in profile_48 but pattern not found")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configMinimal(name, payload)},
			{
				Config: configMinimal(name, tampered),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectNonEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_DescriptionChange — envelope
// field change (general.description) must produce a normal plan and round-trip
// through state. Uses the Notifications payload (profile_44).
func TestAccResource_MobileDeviceConfigurationProfile_DescriptionChange(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-desc-" + suffix
	payload := freshPayload(t, "profile_44.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configWithDescription(name, payload, "v1")},
			{
				Config: configWithDescription(name, payload, "v2"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("general").AtMapKey("description"),
						knownvalue.StringExact("v2"),
					),
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_AllMobileDevicesScope —
// exercises all_mobile_devices=true and verifies the flag round-trips through
// state. Uses the Wi-Fi payload (profile_70).
func TestAccResource_MobileDeviceConfigurationProfile_AllMobileDevicesScope(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-all-" + suffix
	payload := freshPayload(t, "profile_70.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configAllMobileDevices(name, payload),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("all_mobile_devices"),
						knownvalue.Bool(true),
					),
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_ScopeWithExclusions —
// all_mobile_devices + a directory-user name exclusion. Exercises the
// exclusion sub-block wiring without requiring an enrolled device.
func TestAccResource_MobileDeviceConfigurationProfile_ScopeWithExclusions(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-excl-" + suffix
	payload := freshPayload(t, "profile_70.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configScopeWithExclusions(name, payload),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("all_mobile_devices"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("exclusions").AtMapKey("directory_service_or_local_user_names"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact("nonexistent-acc-test-user")}),
					),
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_ScopeWithMobileDeviceGroupIDs
// — creates a static mobile device group, pins it in the profile scope, and
// verifies the group ID round-trips through state.
func TestAccResource_MobileDeviceConfigurationProfile_ScopeWithMobileDeviceGroupIDs(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-grp-" + suffix
	groupID := createDummyMobileDeviceGroup(t, "tf-acc-mdcp-fixture-grp-"+suffix)
	payload := freshPayload(t, "profile_70.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configScopeWithMobileDeviceGroupIDs(name, payload, groupID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("mobile_device_group_ids"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact(groupID)}),
					),
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_ScopeJSSUserAddRemove —
// adds a Jamf Pro user to scope, then clears the category by declaring `[]`.
// Granular ownership: omitting the category would leave the user scoped
// outside Terraform, so the clear must be an explicit empty set. The implicit
// post-step empty-plan check enforces that the clear round-tripped.
// Uses the Exchange ActiveSync payload (profile_50).
func TestAccResource_MobileDeviceConfigurationProfile_ScopeJSSUserAddRemove(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-jssu-" + suffix
	jssUserID := createDummyUser(t, "tf-acc-mdcp-jssuser-"+suffix)
	payload := freshPayload(t, "profile_50.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configScopeJSSUser(name, payload, jssUserID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("user_ids"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact(jssUserID)}),
					),
				},
			},
			{
				Config: configScopeJSSUserCleared(name, payload),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("user_ids"),
						knownvalue.SetExact([]knownvalue.Check{}),
					),
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_SelfService — exercises the
// self_service sub-block (mobile variant: no notification fields). Uses the
// Notifications payload (profile_44) with Make Available in Self Service.
func TestAccResource_MobileDeviceConfigurationProfile_SelfService(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-ss-" + suffix
	payload := freshPayload(t, "profile_44.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configWithSelfService(name, payload),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("self_service").AtMapKey("feature_on_main_page"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("self_service").AtMapKey("removal_disallowed"),
						knownvalue.StringExact("Never"),
					),
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_SelfServiceAuthorizationPassword —
// exercises self_service.security end-to-end: round-trip of plaintext
// `authorization_password` (Jamf Pro echoes it on read) plus a step that
// rotates the password. Also exercises the
// payloadhelpers.serverInjectedPayloadTypes path: Jamf injects a
// com.apple.profileRemovalPassword PayloadContent entry into the stored
// mobileconfig when authorization_password is set, and the mask filter must
// drop it so PayloadsSemanticallyEqual treats plan and server as equal and
// the self-healing payload branch in flattenGeneral keeps the user-authored
// payload bytes in state.
func TestAccResource_MobileDeviceConfigurationProfile_SelfServiceAuthorizationPassword(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-ssauth-" + suffix
	payload := freshPayload(t, "profile_44.mobileconfig")

	const pw1 = "tf-acc-secret-aaa"
	const pw2 = "tf-acc-secret-bbb"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configWithSelfServiceAuthPassword(name, payload, pw1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("self_service").AtMapKey("removal_disallowed"),
						knownvalue.StringExact("With Authorization"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("self_service").AtMapKey("authorization_password"),
						knownvalue.StringExact(pw1),
					),
				},
			},
			{
				Config: configWithSelfServiceAuthPassword(name, payload, pw2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("self_service").AtMapKey("authorization_password"),
						knownvalue.StringExact(pw2),
					),
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_DistributionMethodChange —
// flips distribution_method; wire uses deployment_method but TF exposes
// distribution_method (UI-canonical). Confirms symmetric round-trip.
// Uses the Passcode policy payload (profile_48).
func TestAccResource_MobileDeviceConfigurationProfile_DistributionMethodChange(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-dm-" + suffix
	payload := freshPayload(t, "profile_48.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configWithDistribution(name, payload, "Install Automatically")},
			{
				Config: configWithDistribution(name, payload, "Make Available in Self Service"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_mobile_device_configuration_profile.test",
						tfjsonpath.New("general").AtMapKey("distribution_method"),
						knownvalue.StringExact("Make Available in Self Service"),
					),
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_ImportState — import by ID.
// No ImportStateVerify: import hydrates every wire-present section (scope with
// all categories, self_service), which legitimately differs from the created
// state of this general-only config, where those sections stay null/undeclared.
// Uses the Wi-Fi payload (profile_70).
func TestAccResource_MobileDeviceConfigurationProfile_ImportState(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-import-" + suffix
	payload := freshPayload(t, "profile_70.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configMinimal(name, payload)},
			{
				ResourceName:                         "jamfplatform_pro_mobile_device_configuration_profile.test",
				ImportState:                          true,
				ImportStateVerify:                    false,
				ImportStateVerifyIdentifierAttribute: "id",
			},
		},
	})
}

// expectGeneralAttrKnown asserts that general.<attr> is NOT planned as
// unknown ("known after apply"). plancheck.ExpectKnownValue cannot express
// this: it reads Change.After, where an unknown value is serialised as null
// and is therefore indistinguishable from a known null. This check reads the
// parallel Change.AfterUnknown tree (the same source ExpectUnknownValue uses)
// and fails when the attribute is flagged unknown.
type expectGeneralAttrKnown struct {
	resourceAddress string
	attr            string
}

func (e expectGeneralAttrKnown) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	for _, rc := range req.Plan.ResourceChanges {
		if rc.Address != e.resourceAddress {
			continue
		}
		unknown, err := tfjsonpath.Traverse(rc.Change.AfterUnknown, tfjsonpath.New("general").AtMapKey(e.attr))
		if err != nil {
			// Path absent from the AfterUnknown tree ⇒ not flagged unknown ⇒ known.
			return
		}
		if isUnknown, ok := unknown.(bool); ok && isUnknown {
			resp.Error = fmt.Errorf(
				"%s: general.%s is planned unknown (\"known after apply\") on the first post-import plan; "+
					"it must stay known. Regression of the §886 derived-name restore in ModifyPlan's two-way "+
					"fallback (0bfb64b follow-up).",
				e.resourceAddress, e.attr)
		}
		return
	}
	resp.Error = fmt.Errorf("%s - resource not found in plan ResourceChanges", e.resourceAddress)
}

// TestAccResource_MobileDeviceConfigurationProfile_ImportThenPlan_DerivedNamesStayKnown
// pins the fix for the phantom in-place update seen on the first plan after a
// fresh import. On import the payload is stored in the server-canonical form,
// which is byte-different from (but semantically equal to) the user's HCL, so
// the next plan proposes an update on `general`; that marks category_name /
// site_name Unknown (they are Computed without UseStateForUnknown per §886).
// Import leaves the three-way payload private-state refs empty, so ModifyPlan
// takes its two-way fallback — the branch that, before this fix, suppressed the
// payload diff but forgot to restore the derived names, leaving them Unknown
// and surfacing a spurious update (which for a config profile issues a PUT that
// can re-deploy the profile). The names must stay known.
func TestAccResource_MobileDeviceConfigurationProfile_ImportThenPlan_DerivedNamesStayKnown(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-import-noop-" + suffix
	fixture := freshPayload(t, "profile_44.mobileconfig")
	const addr = "jamfplatform_pro_mobile_device_configuration_profile.test"

	// Create the object out-of-band so import is the first Terraform action, as
	// a user importing a pre-existing profile does. A Terraform-managed create
	// step would populate the three-way payload private-state refs and route the
	// next plan through the three-way compare (already correct since 0bfb64b) —
	// the regression lives on the import-only two-way fallback path. The config
	// uses the payload read back from the server so it is semantically equal to
	// (but byte-different from) the form import canonicalises into state, which
	// is what drives ModifyPlan into that fallback's suppression branch.
	profileID, serverPayload := createOOBProfile(t, name, fixture)
	cfg := configMinimal(name, serverPayload)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			// 1. Import the out-of-band object and persist it into the working
			//    state. Import's Read never writes payload_last_input /
			//    payload_last_canonical, so the next plan takes the two-way
			//    fallback.
			{
				Config:             cfg,
				ResourceName:       addr,
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateVerify:  false,
				ImportStateIdFunc:  func(*terraform.State) (string, error) { return profileID, nil },
			},
			// 2. Plan the imported state against the same HCL. The pre-apply plan
			//    is legitimately non-empty — import hydrates scope / self_service
			//    (includeUnmanaged) that this minimal config omits — but the two
			//    derived names in `general` must NOT churn to "known after apply".
			//    The plan checks run on that pre-apply plan; the apply then
			//    reconciles the hydrated sections and the framework's built-in
			//    post-apply idempotency check confirms the resource settles.
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						expectGeneralAttrKnown{addr, "category_name"},
						expectGeneralAttrKnown{addr, "site_name"},
					},
				},
			},
		},
	})
}

// createOOBProfile creates a minimal mobile device configuration profile
// directly via the SDK (no category, no site), then reads it back and returns
// its ID plus the server-stored payload. A t.Cleanup deletes it so the object
// never leaks if the import step fails before Terraform takes over management.
func createOOBProfile(t *testing.T, name, payload string) (id, serverPayload string) {
	t.Helper()
	c := testhelpers.NewProClassicClient(t)
	ctx := context.Background()
	pl := proclassic.PayloadsXMLText(payload)
	created, err := c.CreateMobileDeviceConfigurationProfileByID(ctx, "0", &proclassic.MobileDeviceConfigurationProfile{
		General: &proclassic.MobileDeviceConfigurationProfileGeneral{Name: &name, Payloads: &pl},
	})
	if err != nil {
		t.Fatalf("out-of-band create of mobile device configuration profile %q: %v", name, err)
	}
	switch {
	case created != nil && created.ID != nil:
		id = fmt.Sprintf("%d", *created.ID)
	case created != nil && created.General != nil && created.General.ID != nil:
		id = fmt.Sprintf("%d", *created.General.ID)
	}
	if id == "" {
		t.Fatalf("out-of-band create of mobile device configuration profile %q returned no ID", name)
	}
	t.Cleanup(func() { _ = c.DeleteMobileDeviceConfigurationProfileByID(context.Background(), id) })

	got, err := c.GetMobileDeviceConfigurationProfileByID(ctx, id)
	if err != nil {
		t.Fatalf("reading back out-of-band mobile device configuration profile %s: %v", id, err)
	}
	if got != nil && got.General != nil && got.General.Payloads != nil {
		serverPayload = string(*got.General.Payloads)
	}
	if serverPayload == "" {
		t.Fatalf("out-of-band mobile device configuration profile %s returned an empty payload on read-back", id)
	}
	// The server returns the payload as a single line with no trailing newline;
	// configMinimal embeds it in a <<EOF heredoc, which needs the closing EOF on
	// its own line.
	if !strings.HasSuffix(serverPayload, "\n") {
		serverPayload += "\n"
	}
	return id, serverPayload
}

// TestAccResource_MobileDeviceConfigurationProfile_ScopeLimitationsClearWithEmptySet
// verifies that a declared-empty limitations category clears its members.
// Granular ownership (wire-probed 2026-07-08): a scope write replaces the whole
// subtree once any category element is present, and `[]` must be emitted as an
// explicit empty element to clear — omitting the category instead preserves it
// via the Update read-merge. Uses a network-segment fixture (no LDAP needed).
func TestAccResource_MobileDeviceConfigurationProfile_ScopeLimitationsClearWithEmptySet(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-limclear-" + suffix
	seg := "tf-acc-netseg-mdcp-" + suffix
	payload := freshPayload(t, "profile_44.mobileconfig")
	cfg := func(segs string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_network_segment" "fixture" {
  name             = %q
  starting_address = "10.95.0.0"
  ending_address   = "10.95.0.255"
}

resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      all_mobile_devices = true
    }
    limitations = {
      network_segment_ids = [%s]
    }
  }
}
`, seg, name, payload, segs)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: cfg(`jamfplatform_pro_network_segment.fixture.id`),
				Check:  resource.TestCheckResourceAttr("jamfplatform_pro_mobile_device_configuration_profile.test", "scope.limitations.network_segment_ids.#", "1"),
			},
			{
				// Clear via declared [] — Terraform owns the category and emits the
				// explicit empty element. Implicit post-step empty-plan enforces it.
				Config: cfg(``),
				Check:  resource.TestCheckResourceAttr("jamfplatform_pro_mobile_device_configuration_profile.test", "scope.limitations.network_segment_ids.#", "0"),
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_ReservedCharacterCorpusRejected
// documents a server defect, not provider behaviour: the Classic API
// stores mobile device payload fragments VERBATIM after validating the
// escaped wire form (PI-827; see jamfplatform-go-sdk's
// acc_proclassic_profile_payloads_test.go for the wire-level matrix), so
// every entity-bearing value ("&"/"<" in a string) would be persisted
// with one extra entity layer — a device would see `&amp;` where `&` was
// meant. No client can store such values faithfully on this endpoint.
// The provider verifies the stored payload after create, rolls the
// profile back, and fails with an actionable diagnostic. If this test
// starts failing because the apply SUCCEEDS, Jamf fixed the ingest
// defect — replace this with a byte-exact corpus round-trip like the
// macOS resource has.
func TestAccResource_MobileDeviceConfigurationProfile_ReservedCharacterCorpusRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-corpus-" + suffix
	payload := readFixture(t, "reserved_character_corpus.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      configMinimal(name, payload),
				ExpectError: regexp.MustCompile(`cannot store this payload faithfully`),
			},
		},
	})
}

// fidelitySentinelName is a permanent profile kept on the CI tenant carrying "&"
// in Web Clip Label and URL values. Jamf Pro stores com.apple.webClip.managed
// verbatim in a mobile device profile, so those values cannot be written back
// unchanged. The provider cannot create such a profile — Create detects the
// mangling and rolls back — so the import gate can only be exercised against a
// fixture authored outside Terraform, which is the real-world case it guards.
//
// Looked up by name, never by ID: the sentinel may be deleted and recreated, and
// a hardcoded ID would then test the wrong profile or a missing one.
const fidelitySentinelName = "Fidelity Test Sentinel [DO NOT DELETE]"

// lookupFidelitySentinelID resolves the sentinel to its current ID, skipping with
// an explicit message when absent so an accidental deletion is not mistaken for
// the gate being covered.
func lookupFidelitySentinelID(t *testing.T) string {
	t.Helper()
	c := testhelpers.NewProClassicClient(t)
	got, err := c.GetMobileDeviceConfigurationProfileByName(context.Background(), fidelitySentinelName)
	if err != nil || got == nil || got.General == nil || got.General.ID == nil {
		t.Skipf("mobile device profile %q not present on this tenant — recreate it (a Web Clip payload whose "+
			"Label and URL contain \"&\") to cover the import fidelity gate; lookup error: %v",
			fidelitySentinelName, err)
	}
	return helpers.StringValueFromIntPtr(got.General.ID).ValueString()
}

// TestAccResource_MobileDeviceConfigurationProfile_ImportFidelityGateRefusesSentinel
// is the regression test for the import trap on the mobile endpoint, which is the
// more exposed of the two: Jamf Pro stores nearly every mobile payload type
// verbatim — including com.apple.ManagedClient.preferences, which is faithful on
// macOS — so an everyday web clip URL with a query string is enough to trip it.
//
// Only an acceptance test can prove Read consults the gate on the import path.
// An earlier revision gated on req.State.Raw.IsNull(), which is false for a
// config-driven import block, so every profile imported while unit tests passed.
func TestAccResource_MobileDeviceConfigurationProfile_ImportFidelityGateRefusesSentinel(t *testing.T) {
	testhelpers.AccPreCheck(t)
	id := lookupFidelitySentinelID(t)
	payload := readFixture(t, "profile_44.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// A throwaway config supplies the resource address; the import targets
				// the sentinel by ID and must never reach state.
				Config:            configMinimal("tf-acc-mdcp-gate-"+testhelpers.RunSuffix(), payload),
				ResourceName:      "jamfplatform_pro_mobile_device_configuration_profile.test",
				ImportState:       true,
				ImportStateId:     id,
				ImportStateVerify: false,
				// Terraform re-wraps diagnostic prose at roughly 80 columns, so the
				// pattern must never straddle a line break.
				ExpectError: regexp.MustCompile(`cannot be managed by Terraform`),
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_ImportFidelityGateNamesTheValue
// checks the refusal is actionable. Both offending values must be named, not just
// the first: a tenant-wide import prints no resource address for a provider Read
// error, so the diagnostic is the operator's only handle on what to fix.
func TestAccResource_MobileDeviceConfigurationProfile_ImportFidelityGateNamesTheValue(t *testing.T) {
	testhelpers.AccPreCheck(t)
	id := lookupFidelitySentinelID(t)
	payload := readFixture(t, "profile_44.mobileconfig")

	// Single tokens only, for the line-wrap reason above. The PayloadContent index
	// is deliberately not asserted — it shifts if the sentinel is rebuilt.
	for _, want := range []*regexp.Regexp{
		regexp.MustCompile(`\.Label`),
		regexp.MustCompile(`\.URL`),
		regexp.MustCompile(`Sentinel`),
	} {
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config:        configMinimal("tf-acc-mdcp-gatemsg-"+testhelpers.RunSuffix(), payload),
					ResourceName:  "jamfplatform_pro_mobile_device_configuration_profile.test",
					ImportState:   true,
					ImportStateId: id,
					ExpectError:   want,
				},
			},
		})
	}
}

// TestAccResource_MobileDeviceConfigurationProfile_RefreshIsNeverGated protects the
// other half of the gate's contract: it must fire ONLY on import. A profile
// already under management that acquires an unstorable value out-of-band has to
// keep refreshing, or the operator can neither see the drift nor remove the
// resource — worse than the trap the gate prevents.
func TestAccResource_MobileDeviceConfigurationProfile_RefreshIsNeverGated(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-refreshgate-" + suffix
	clean := readFixture(t, "profile_44.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configMinimal(name, clean)},
			{
				PreConfig: func() { injectAmpersandOutOfBand(t, name) },
				Config:    configMinimal(name, clean),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectNonEmptyPlan(),
					},
				},
			},
		},
	})
}

// injectAmpersandOutOfBand rewrites the named profile's payload to carry "&",
// simulating an admin-UI edit after the profile came under management. Uses the
// SDK directly: this is a write the provider itself refuses to perform.
func injectAmpersandOutOfBand(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	c := testhelpers.NewProClassicClient(t)

	got, err := c.GetMobileDeviceConfigurationProfileByName(ctx, name)
	if err != nil || got == nil || got.General == nil || got.General.ID == nil || got.General.Payloads == nil {
		t.Fatalf("reading %q before out-of-band edit: %v", name, err)
	}
	id := helpers.StringValueFromIntPtr(got.General.ID).ValueString()

	current := string(*got.General.Payloads)
	edited := injectPlistTailKey(current)
	if edited == current {
		t.Fatalf("could not locate the plist tail in %q to inject the out-of-band edit", name)
	}

	payload := proclassic.PayloadsXMLText(edited)
	if err := c.UpdateMobileDeviceConfigurationProfileByID(ctx, id,
		&proclassic.MobileDeviceConfigurationProfile{
			General: &proclassic.MobileDeviceConfigurationProfileGeneral{Payloads: &payload},
		}); err != nil {
		t.Fatalf("out-of-band payload edit of %q: %v", name, err)
	}
}

// injectPlistTailKey appends a key holding "&" to the top-level dict, trying both
// the indented and single-line document shapes Jamf Pro may return. Returns the
// input unchanged when neither tail is found, so the caller fails loudly rather
// than asserting against an unmodified profile.
func injectPlistTailKey(plist string) string {
	const inject = `<key>ZZOutOfBandEdit</key><string>injected &amp; value</string>`
	for _, tail := range []string{"</dict>\n</plist>", "</dict></plist>"} {
		if strings.Contains(plist, tail) {
			return strings.Replace(plist, tail, inject+tail, 1)
		}
	}
	return plist
}

// omitRetainsFixtures carries the out-of-band (SDK-created) fixture IDs the
// omit-retains configs reference, plus the run suffix that names the inline
// HCL fixtures. The inline fixture IDs are only known after apply, so the
// wire assertion resolves them from Terraform state rather than from here.
type omitRetainsFixtures struct {
	suffix       string
	targetGroup  string
	excludeGroup string
	targetUser   string
	excludeUser  string
}

// omitRetainsFixtureHCL declares one inline fixture per scope category. Targets
// and exclusions each get their own fixture so the wire assertion can tell a
// retained exclusion from a leaked target.
func omitRetainsFixtureHCL(suffix string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_category" "omit" {
  name     = "tf-acc-mdcp-omit-cat-%[1]s"
  priority = 7
}

resource "jamfplatform_pro_building" "target" {
  name = "tf-acc-mdcp-omit-bld-t-%[1]s"
}

resource "jamfplatform_pro_building" "exclude" {
  name = "tf-acc-mdcp-omit-bld-x-%[1]s"
}

resource "jamfplatform_pro_department" "target" {
  name = "tf-acc-mdcp-omit-dep-t-%[1]s"
}

resource "jamfplatform_pro_department" "exclude" {
  name = "tf-acc-mdcp-omit-dep-x-%[1]s"
}

resource "jamfplatform_pro_user_group" "target" {
  name       = "tf-acc-mdcp-omit-ug-t-%[1]s"
  group_type = "static"
}

resource "jamfplatform_pro_user_group" "exclude" {
  name       = "tf-acc-mdcp-omit-ug-x-%[1]s"
  group_type = "static"
}

resource "jamfplatform_pro_network_segment" "limit" {
  name             = "tf-acc-mdcp-omit-seg-l-%[1]s"
  starting_address = "10.93.0.0"
  ending_address   = "10.93.0.255"
}

resource "jamfplatform_pro_network_segment" "exclude" {
  name             = "tf-acc-mdcp-omit-seg-x-%[1]s"
  starting_address = "10.93.1.0"
  ending_address   = "10.93.1.255"
}

resource "jamfplatform_pro_ibeacon" "limit" {
  name                    = "tf-acc-mdcp-omit-ib-l-%[1]s"
  uuid                    = "759b0599-64e0-416a-8d31-d8e93482a4d7"
  include_any_major_value = true
  include_any_minor_value = true
}

resource "jamfplatform_pro_ibeacon" "exclude" {
  name                    = "tf-acc-mdcp-omit-ib-x-%[1]s"
  uuid                    = "759b0599-64e0-416a-8d31-d8e93482a4d7"
  major                   = 42
  include_any_minor_value = true
}
`, suffix)
}

// omitRetainsConfig is the fully declared shape for the omit-retains contract:
// every state-gated block and every scope category the resource has carries a
// distinctive value so that a server which stopped retaining an omitted
// element is caught on content, not on presence. Left out: the two
// directory-service user-group categories (the server refuses a group name
// that does not resolve against the tenant's directory integration), the
// per-device categories (no enrolled mobile device fixture exists),
// authorization_password (its removal_disallowed pairing injects a payload the
// mask must strip, which is a separate test's concern).
//
// self_service.categories used to be left out: a POST or PUT carrying
// <self_service_categories> answered 2xx and every GET afterwards returned
// <self_service_categories/>, so the attribute's Computed name could never
// become known after apply. The cause was the payload rather than the
// endpoint — the SDK type could not express <display_in>, which is what makes
// Jamf Pro store the category at all, and an id-only <category> is discarded.
// SDK v0.22.2 added the field, the write now sends it, and the category is
// covered here: wire-probed 2026-09-07, a create carrying display_in stores the
// category, and a PUT omitting <self_service_categories> retains it while a
// control field changed in the same request lands.
func omitRetainsConfig(name, payload string, f omitRetainsFixtures) string {
	return omitRetainsFixtureHCL(f.suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = "Make Available in Self Service"
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      mobile_device_group_ids = [%q]
      building_ids            = [jamfplatform_pro_building.target.id]
      department_ids          = [jamfplatform_pro_department.target.id]
      user_ids                = [%q]
      user_group_ids          = [jamfplatform_pro_user_group.target.id]
    }
    limitations = {
      network_segment_ids                   = [jamfplatform_pro_network_segment.limit.id]
      ibeacon_ids                           = [jamfplatform_pro_ibeacon.limit.id]
      directory_service_or_local_user_names = ["tf-acc-omit-retains-limit-user"]
    }
    exclusions = {
      mobile_device_group_ids               = [%q]
      building_ids                          = [jamfplatform_pro_building.exclude.id]
      department_ids                        = [jamfplatform_pro_department.exclude.id]
      user_ids                              = [%q]
      user_group_ids                        = [jamfplatform_pro_user_group.exclude.id]
      network_segment_ids                   = [jamfplatform_pro_network_segment.exclude.id]
      ibeacon_ids                           = [jamfplatform_pro_ibeacon.exclude.id]
      directory_service_or_local_user_names = ["tf-acc-omit-retains-exclude-user"]
    }
  }
  self_service = {
    feature_on_main_page     = true
    removal_disallowed       = "Never"
    categories = [
      {
        id = jamfplatform_pro_category.omit.id
      },
    ]
  }
  depends_on = [
    jamfplatform_pro_category.omit,
    jamfplatform_pro_building.target, jamfplatform_pro_building.exclude,
    jamfplatform_pro_department.target, jamfplatform_pro_department.exclude,
    jamfplatform_pro_user_group.target, jamfplatform_pro_user_group.exclude,
    jamfplatform_pro_network_segment.limit, jamfplatform_pro_network_segment.exclude,
    jamfplatform_pro_ibeacon.limit, jamfplatform_pro_ibeacon.exclude,
  ]
}
`, name, payload, f.targetGroup, f.targetUser, f.excludeGroup, f.excludeUser)
}

// omitRetainsParentsOnlyConfig keeps the scope and self_service parents but
// drops their gated children: scope loses limitations, exclusions and the two
// user target categories, so the PUT re-emits the scope from the granular
// merge; self_service loses the Optional+Computed feature_on_main_page leaf.
//
// self_service.categories is dropped here too, which is the point of the step
// rather than an omission: the categories stay on the server (omitRetainedOnServer
// asserts it on the wire) while state drops them, so this is the acceptance
// proof of the ownership gate from issue #392. Before that gate the hydration
// tested only what the wire carried, and this shape failed the apply with
// "Provider produced inconsistent result after apply".
func omitRetainsParentsOnlyConfig(name, payload string, f omitRetainsFixtures) string {
	return omitRetainsFixtureHCL(f.suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = "Make Available in Self Service"
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      mobile_device_group_ids = [%q]
      building_ids            = [jamfplatform_pro_building.target.id]
      department_ids          = [jamfplatform_pro_department.target.id]
    }
  }
  self_service = {
    removal_disallowed       = "Never"
  }
  depends_on = [
    jamfplatform_pro_category.omit,
    jamfplatform_pro_building.target, jamfplatform_pro_building.exclude,
    jamfplatform_pro_department.target, jamfplatform_pro_department.exclude,
    jamfplatform_pro_user_group.target, jamfplatform_pro_user_group.exclude,
    jamfplatform_pro_network_segment.limit, jamfplatform_pro_network_segment.exclude,
    jamfplatform_pro_ibeacon.limit, jamfplatform_pro_ibeacon.exclude,
  ]
}
`, name, payload, f.targetGroup)
}

// omitRetainsGeneralOnlyConfig drops every optional block, so the PUT carries
// <general> alone. The fixtures stay declared so nothing the server still
// references is destroyed underneath it, and every config lists them in
// depends_on: once a step stops referencing a fixture from the profile, the
// dependency edge is gone and Terraform would otherwise destroy the fixture in
// parallel with the profile, which the server refuses while the retained scope
// still names it.
func omitRetainsGeneralOnlyConfig(name, payload string, f omitRetainsFixtures) string {
	return omitRetainsFixtureHCL(f.suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_mobile_device_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = "Make Available in Self Service"
    payloads = <<EOF
%sEOF
  }
  depends_on = [
    jamfplatform_pro_category.omit,
    jamfplatform_pro_building.target, jamfplatform_pro_building.exclude,
    jamfplatform_pro_department.target, jamfplatform_pro_department.exclude,
    jamfplatform_pro_user_group.target, jamfplatform_pro_user_group.exclude,
    jamfplatform_pro_network_segment.limit, jamfplatform_pro_network_segment.exclude,
    jamfplatform_pro_ibeacon.limit, jamfplatform_pro_ibeacon.exclude,
  ]
}
`, name, payload)
}

// requireOnlyID asserts a classic member list holds exactly one entry whose
// id is want. Membership is checked on count as well as content so a server
// that appended rather than retained is caught too.
func requireOnlyID[T any](field string, items *[]T, id func(T) *int, want string) error {
	if items == nil || len(*items) != 1 {
		n := 0
		if items != nil {
			n = len(*items)
		}
		return fmt.Errorf("%s: want exactly one member (%s), got %d", field, want, n)
	}
	return testhelpers.RequireEqual(field, want, fmt.Sprint(testhelpers.Deref(id((*items)[0]))))
}

// requireOnlyIDName is requireOnlyID for the SDK's shared IDName element.
func requireOnlyIDName(field string, items *[]proclassic.IDName, want string) error {
	return requireOnlyID(field, items, func(i proclassic.IDName) *int { return i.ID }, want)
}

// requireOnlyName is requireOnlyID for name-keyed classic members.
func requireOnlyName[T any](field string, items *[]T, name func(T) *string, want string) error {
	if items == nil || len(*items) != 1 {
		n := 0
		if items != nil {
			n = len(*items)
		}
		return fmt.Errorf("%s: want exactly one member (%s), got %d", field, want, n)
	}
	return testhelpers.RequireEqual(field, want, testhelpers.Deref(name((*items)[0])))
}

// derefField reads a member slice out of an optional classic wrapper element,
// yielding nil when the wrapper itself is absent.
func derefField[W any, T any](w *W, get func(*W) *[]T) *[]T {
	if w == nil {
		return nil
	}
	return get(w)
}

// stateID returns the primary id of a fixture resource from Terraform state,
// which is how the wire assertion learns the ids Jamf allocated to the inline
// fixtures.
func stateID(s *terraform.State, addr string) (string, error) {
	rs, ok := s.RootModule().Resources[addr]
	if !ok {
		return "", fmt.Errorf("fixture %s not found in state", addr)
	}
	if rs.Primary.ID == "" {
		return "", fmt.Errorf("fixture %s has no id in state", addr)
	}
	return rs.Primary.ID, nil
}

// omitRetainedOnServer asserts the server's copy still carries every value the
// omit-retains config declared in its first step. Inline fixture ids are read
// from state because Jamf allocates them at apply. There is no
// self_service_description to assert: the attribute is gone, because the classic
// API wrote it into general.description and never echoed it back (issue #393).
func omitRetainedOnServer(t *testing.T, f omitRetainsFixtures) resource.TestCheckFunc {
	c := testhelpers.NewProClassicClient(t)
	const addr = "jamfplatform_pro_mobile_device_configuration_profile.test"
	return func(s *terraform.State) error {
		want := map[string]string{}
		for _, fx := range []struct{ key, addr string }{
			{"catO", "jamfplatform_pro_category.omit"},
			{"bldT", "jamfplatform_pro_building.target"},
			{"bldX", "jamfplatform_pro_building.exclude"},
			{"depT", "jamfplatform_pro_department.target"},
			{"depX", "jamfplatform_pro_department.exclude"},
			{"ugT", "jamfplatform_pro_user_group.target"},
			{"ugX", "jamfplatform_pro_user_group.exclude"},
			{"segL", "jamfplatform_pro_network_segment.limit"},
			{"segX", "jamfplatform_pro_network_segment.exclude"},
			{"ibL", "jamfplatform_pro_ibeacon.limit"},
			{"ibX", "jamfplatform_pro_ibeacon.exclude"},
		} {
			v, err := stateID(s, fx.addr)
			if err != nil {
				return err
			}
			want[fx.key] = v
		}
		return testhelpers.CheckLiveObject(addr,
			func(ctx context.Context, id string) (*proclassic.MobileDeviceConfigurationProfile, error) {
				return c.GetMobileDeviceConfigurationProfileByID(ctx, id)
			},
			func(p *proclassic.MobileDeviceConfigurationProfile) error {
				sc := p.Scope
				if sc == nil {
					return fmt.Errorf("scope: absent")
				}
				checks := []error{
					requireOnlyIDName("scope.targets.mobile_device_group_ids", derefField(sc.MobileDeviceGroups, func(g *proclassic.MobileDeviceConfigurationProfileScopeMobileDeviceGroups) *[]proclassic.IDName {
						return g.MobileDeviceGroup
					}), f.targetGroup),
					requireOnlyIDName("scope.targets.building_ids", derefField(sc.Buildings, func(b *proclassic.MobileDeviceConfigurationProfileScopeBuildings) *[]proclassic.IDName {
						return b.Building
					}), want["bldT"]),
					requireOnlyIDName("scope.targets.department_ids", derefField(sc.Departments, func(d *proclassic.MobileDeviceConfigurationProfileScopeDepartments) *[]proclassic.IDName {
						return d.Department
					}), want["depT"]),
					requireOnlyIDName("scope.targets.user_ids", derefField(sc.JssUsers, func(u *proclassic.MobileDeviceConfigurationProfileScopeJssUsers) *[]proclassic.IDName { return u.User }), f.targetUser),
					requireOnlyIDName("scope.targets.user_group_ids", derefField(sc.JssUserGroups, func(u *proclassic.MobileDeviceConfigurationProfileScopeJssUserGroups) *[]proclassic.IDName {
						return u.UserGroup
					}), want["ugT"]),
				}
				for _, err := range checks {
					if err != nil {
						return err
					}
				}
				l := sc.Limitations
				if l == nil {
					return fmt.Errorf("scope.limitations: absent")
				}
				checks = []error{
					requireOnlyIDName("scope.limitations.network_segment_ids", derefField(l.NetworkSegments, func(n *proclassic.MobileDeviceConfigurationProfileScopeLimitationsNetworkSegments) *[]proclassic.IDName {
						return n.NetworkSegment
					}), want["segL"]),
					requireOnlyIDName("scope.limitations.ibeacon_ids", derefField(l.Ibeacons, func(i *proclassic.MobileDeviceConfigurationProfileScopeLimitationsIbeacons) *[]proclassic.IDName {
						return i.Ibeacon
					}), want["ibL"]),
					requireOnlyName("scope.limitations.directory_service_or_local_user_names", derefField(l.Users, func(u *proclassic.MobileDeviceConfigurationProfileScopeLimitationsUsers) *[]proclassic.IDName {
						return u.User
					}), func(i proclassic.IDName) *string { return i.Name }, "tf-acc-omit-retains-limit-user"),
				}
				for _, err := range checks {
					if err != nil {
						return err
					}
				}
				e := sc.Exclusions
				if e == nil {
					return fmt.Errorf("scope.exclusions: absent")
				}
				checks = []error{
					requireOnlyIDName("scope.exclusions.mobile_device_group_ids", derefField(e.MobileDeviceGroups, func(g *proclassic.MobileDeviceConfigurationProfileScopeExclusionsMobileDeviceGroups) *[]proclassic.IDName {
						return g.MobileDeviceGroup
					}), f.excludeGroup),
					requireOnlyIDName("scope.exclusions.building_ids", derefField(e.Buildings, func(b *proclassic.MobileDeviceConfigurationProfileScopeExclusionsBuildings) *[]proclassic.IDName {
						return b.Building
					}), want["bldX"]),
					requireOnlyIDName("scope.exclusions.department_ids", derefField(e.Departments, func(d *proclassic.MobileDeviceConfigurationProfileScopeExclusionsDepartments) *[]proclassic.IDName {
						return d.Department
					}), want["depX"]),
					requireOnlyIDName("scope.exclusions.user_ids", derefField(e.JssUsers, func(u *proclassic.MobileDeviceConfigurationProfileScopeExclusionsJssUsers) *[]proclassic.IDName {
						return u.User
					}), f.excludeUser),
					requireOnlyIDName("scope.exclusions.user_group_ids", derefField(e.JssUserGroups, func(u *proclassic.MobileDeviceConfigurationProfileScopeExclusionsJssUserGroups) *[]proclassic.IDName {
						return u.UserGroup
					}), want["ugX"]),
					requireOnlyID("scope.exclusions.network_segment_ids", derefField(e.NetworkSegments, func(n *proclassic.MobileDeviceConfigurationProfileScopeExclusionsNetworkSegments) *[]proclassic.MobileDeviceConfigurationProfileScopeExclusionsNetworkSegmentsNetworkSegmentItem {
						return n.NetworkSegment
					}), func(i proclassic.MobileDeviceConfigurationProfileScopeExclusionsNetworkSegmentsNetworkSegmentItem) *int {
						return i.ID
					}, want["segX"]),
					requireOnlyIDName("scope.exclusions.ibeacon_ids", derefField(e.Ibeacons, func(i *proclassic.MobileDeviceConfigurationProfileScopeExclusionsIbeacons) *[]proclassic.IDName {
						return i.Ibeacon
					}), want["ibX"]),
					requireOnlyName("scope.exclusions.directory_service_or_local_user_names", derefField(e.Users, func(u *proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsers) *[]proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsersUserItem {
						return u.User
					}), func(i proclassic.MobileDeviceConfigurationProfileScopeExclusionsUsersUserItem) *string { return i.Name }, "tf-acc-omit-retains-exclude-user"),
				}
				for _, err := range checks {
					if err != nil {
						return err
					}
				}
				ss := p.SelfService
				if ss == nil {
					return fmt.Errorf("self_service: absent")
				}
				if err := testhelpers.RequireEqual("self_service.feature_on_main_page", true, testhelpers.Deref(ss.FeatureOnMainPage)); err != nil {
					return err
				}
				if ss.Security == nil {
					return fmt.Errorf("self_service.security: absent")
				}
				if err := testhelpers.RequireEqual("self_service.removal_disallowed", "Never", testhelpers.Deref(ss.Security.RemovalDisallowed)); err != nil {
					return err
				}
				if ss.SelfServiceCategories == nil {
					return fmt.Errorf("self_service.categories: absent")
				}
				return requireOnlyID("self_service.categories", ss.SelfServiceCategories.Category,
					func(c proclassic.MobileDeviceConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem) *int {
						return c.ID
					},
					want["catO"])
			})(s)
	}
}

// TestAccResource_MobileDeviceConfigurationProfile_OmittedBlocksRetained pins
// the omit-retains contract the plan output cannot show: dropping scope
// limitations, exclusions, target categories and self_service from config
// plans them as removed, but the classic PUT either omits the
// element or re-emits it from the granular scope merge, and the server keeps
// every value. Step 2 keeps the scope and self_service parents while dropping
// their gated children; step 3 drops every optional block so the PUT carries
// <general> alone. Each step's implicit post-apply plan must be empty. If this
// test fails on content, the endpoint no longer merges and nothing that
// suppresses the removal plan may ship for this resource. The payload is never
// touched between steps so a payload diff here is a finding of its own.
func TestAccResource_MobileDeviceConfigurationProfile_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-omit-" + suffix
	payload := freshPayload(t, "profile_44.mobileconfig")
	f := omitRetainsFixtures{
		suffix:       suffix,
		targetGroup:  createDummyMobileDeviceGroup(t, "tf-acc-mdcp-omit-grp-t-"+suffix),
		excludeGroup: createDummyMobileDeviceGroup(t, "tf-acc-mdcp-omit-grp-x-"+suffix),
		targetUser:   createDummyUser(t, "tf-acc-mdcp-omit-user-t-"+suffix),
		excludeUser:  createDummyUser(t, "tf-acc-mdcp-omit-user-x-"+suffix),
	}
	const addr = "jamfplatform_pro_mobile_device_configuration_profile.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: omitRetainsConfig(name, payload, f),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "self_service.feature_on_main_page", "true"),
					resource.TestCheckResourceAttr(addr, "self_service.categories.#", "1"),
					resource.TestCheckResourceAttr(addr, "scope.exclusions.ibeacon_ids.#", "1"),
					omitRetainedOnServer(t, f),
				),
			},
			{
				Config: omitRetainsParentsOnlyConfig(name, payload, f),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "self_service.feature_on_main_page", "true"),
					resource.TestCheckNoResourceAttr(addr, "self_service.categories.#"),
					resource.TestCheckNoResourceAttr(addr, "scope.limitations.network_segment_ids.#"),
					resource.TestCheckNoResourceAttr(addr, "scope.exclusions.mobile_device_group_ids.#"),
					resource.TestCheckNoResourceAttr(addr, "scope.targets.user_ids.#"),
					omitRetainedOnServer(t, f),
				),
			},
			{
				Config: omitRetainsGeneralOnlyConfig(name, payload, f),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(addr, "scope.targets.mobile_device_group_ids.#"),
					omitRetainedOnServer(t, f),
				),
			},
		},
	})
}

// ── Web clip icons (issue #418) ───────────────────────────────────────────────

// webClipIconFixture is the sample icon embedded in
// profile_webclip_icon.mobileconfig: a 64x64 PNG, deliberately not the 180px
// form Jamf Pro stores, so every one of these tests exercises the re-render.
const (
	webClipIconFixture          = "webclip_icon.png"
	webClipIconAlternateFixture = "webclip_icon_alternate.png"
	webClipProfileFixture       = "profile_webclip_icon.mobileconfig"
)

// iconDataBlock matches the <data> element holding the web clip's icon, so a
// test can swap the picture without maintaining a second copy of the payload.
var iconDataBlock = regexp.MustCompile(`(?s)(<key>Icon</key>\s*<data>)(.*?)(</data>)`)

// replaceIcon swaps the payload's icon for the named fixture, base64-encoded
// and line-wrapped the way a plist writer emits it.
func replaceIcon(t *testing.T, payload, fixture string) string {
	t.Helper()
	raw, err := os.ReadFile(testdataFile(t, fixture))
	if err != nil {
		t.Fatalf("reading icon fixture %q: %v", fixture, err)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	var b strings.Builder
	for i := 0; i < len(encoded); i += 68 {
		b.WriteString("\n\t\t\t")
		b.WriteString(encoded[i:min(i+68, len(encoded))])
	}
	b.WriteString("\n\t\t\t")
	out := iconDataBlock.ReplaceAllString(payload, "${1}"+b.String()+"${3}")
	if out == payload {
		t.Fatalf("payload does not carry an <key>Icon</key> data block to replace")
	}
	return out
}

// TestAccFixtureWebClipIconMatchesItsSource keeps the two fixtures honest: the
// icon embedded in the profile must be the sample .png beside it, so a
// regenerated icon cannot silently disagree with the payload that carries it.
// Talks to nothing.
func TestAccFixtureWebClipIconMatchesItsSource(t *testing.T) {
	payload := readFixture(t, webClipProfileFixture)
	tree, _, err := plisthelpers.ParsePlist([]byte(payload))
	if err != nil {
		t.Fatalf("parsing %s: %v", webClipProfileFixture, err)
	}
	entries, ok := tree["PayloadContent"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("%s has no PayloadContent entries", webClipProfileFixture)
	}
	entry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("%s PayloadContent[0] is not a dictionary", webClipProfileFixture)
	}
	embedded, ok := entry["Icon"].([]byte)
	if !ok {
		t.Fatalf("%s PayloadContent[0] carries no Icon data blob", webClipProfileFixture)
	}
	source, err := os.ReadFile(testdataFile(t, webClipIconFixture))
	if err != nil {
		t.Fatalf("reading %s: %v", webClipIconFixture, err)
	}
	if !bytes.Equal(embedded, source) {
		t.Errorf("the icon embedded in %s (%d bytes) is not %s (%d bytes)",
			webClipProfileFixture, len(embedded), webClipIconFixture, len(source))
	}
}

// TestAccResource_MobileDeviceConfigurationProfile_WebClipIconSurvivesAWrite is
// the regression for issue #418. Jamf Pro never stores a web clip icon as
// submitted: it rescales it to 180px on the longest side and re-encodes it,
// which used to fail the post-write payload verification and abort every write
// to an icon-bearing profile — the create rolled back, and an update left the
// profile changed in Jamf Pro with the error making the plan repeat forever.
//
// The three steps are the reported reproduction: create, then a change to a
// field that has nothing to do with the payload, then the same configuration
// again. All three must apply, and the third must plan empty — the icon
// difference has to be tolerated on refresh as well as on write, or the plan
// never settles.
func TestAccResource_MobileDeviceConfigurationProfile_WebClipIconSurvivesAWrite(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-webclip-" + suffix
	payload := freshPayload(t, webClipProfileFixture)
	const addr = "jamfplatform_pro_mobile_device_configuration_profile.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configWithDescription(name, payload, "before"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "general.description", "before"),
					// The authored bytes are kept: the icon Jamf Pro holds is its own
					// re-render, and overwriting state with it would put a picture the
					// configuration never wrote into the payload attribute.
					resource.TestCheckResourceAttr(addr, "general.payloads", payload),
				),
			},
			{
				Config: configWithDescription(name, payload, "after"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "general.description", "after"),
					resource.TestCheckResourceAttr(addr, "general.payloads", payload),
				),
			},
			{
				Config: configWithDescription(name, payload, "after"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_WebClipIconChangeApplies
// covers the other direction: tolerating the re-render must not stop a genuine
// icon change from being written. Swapping the icon changes the payloads string
// itself, so Terraform plans it directly — the point of the test is that the
// apply then succeeds and the new icon sticks, which it cannot do if the
// comparison rejects Jamf Pro's re-render of the replacement.
func TestAccResource_MobileDeviceConfigurationProfile_WebClipIconChangeApplies(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-webclip-swap-" + suffix
	payload := freshPayload(t, webClipProfileFixture)
	swapped := replaceIcon(t, payload, webClipIconAlternateFixture)
	const addr = "jamfplatform_pro_mobile_device_configuration_profile.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configMinimal(name, payload)},
			{
				Config: configMinimal(name, swapped),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectNonEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr(addr, "general.payloads", swapped),
			},
			{
				Config: configMinimal(name, swapped),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionNoop),
					},
				},
			},
		},
	})
}

// TestAccResource_MobileDeviceConfigurationProfile_WebClipIconNotAnImageIsReported
// pins the failure direction. Jamf Pro substitutes a fixed placeholder image
// for an icon it cannot decode, so the stored profile does not carry what was
// authored — and the operator has to be told which value, by name. Before the
// non-string leaves were diffed, a difference in a data blob produced no
// findings at all and the diagnostic fell back to a list of causes about line
// feeds and ampersands that could not apply.
func TestAccResource_MobileDeviceConfigurationProfile_WebClipIconNotAnImageIsReported(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mdcp-webclip-bad-" + suffix
	payload := freshPayload(t, webClipProfileFixture)
	notAnImage := iconDataBlock.ReplaceAllString(payload,
		"${1}"+base64.StdEncoding.EncodeToString([]byte("this is not an image at all"))+"${3}")
	if notAnImage == payload {
		t.Fatal("payload does not carry an <key>Icon</key> data block to replace")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name, notAnImage),
				// The diagnostic wraps at roughly 80 columns, so match the path alone.
				ExpectError: regexp.MustCompile(`PayloadContent\[0\]\.Icon`),
			},
		},
	})
}
