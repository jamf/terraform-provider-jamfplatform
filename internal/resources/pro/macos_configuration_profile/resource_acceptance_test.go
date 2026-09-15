// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Tests in this file talk to the Jamf ProClassic
// /osxconfigurationprofiles endpoint. Classic has known concurrency
// issues when multiple writes hit the same resource type — keep these
// tests serial with any other classic acceptance work.

package macos_configuration_profile_test

import (
	"context"
	"crypto/rand"
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

// checkDestroy verifies profiles created during the test were destroyed.
func checkDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := proclassic.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "jamfplatform_pro_macos_configuration_profile" {
				continue
			}
			_, err := c.GetOSXConfigurationProfileByID(ctx, rs.Primary.ID)
			if err != nil {
				if helpers.IsNotFoundError(err) {
					continue
				}
				return fmt.Errorf("checking macOS configuration profile %s: %s", rs.Primary.ID, err)
			}
			return fmt.Errorf("macOS configuration profile %s still exists", rs.Primary.ID)
		}
		return nil
	}
}

func configMinimal(name, payload string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
}
`, name, payload)
}

func configWithDistribution(name, payload, dist string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = %q
    payloads = <<EOF
%sEOF
  }
}
`, name, dist, payload)
}

func configWithDescription(name, payload, desc string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name        = %q
    description = %q
    payloads = <<EOF
%sEOF
  }
}
`, name, desc, payload)
}

func configWithSelfService(name, payload, dispName string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = "Make Available in Self Service"
    payloads = <<EOF
%sEOF
  }
  self_service = {
    self_service_display_name     = %q
    install_button_text           = "Install"
    self_service_description      = "Acceptance-test profile"
    ensure_users_view_description = false
    feature_on_main_page          = true
    display_notifications         = true
    notification_location         = "Self Service"
    notification_subject          = "tf-acc"
    notification_message          = "Test notification"
    removal_disallowed            = "Never"
  }
}
`, name, payload, dispName)
}

func configAllComputers(name, payload string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      all_computers = true
    }
  }
}
`, name, payload)
}

func configScopeWithComputerIDs(name, payload, computerID string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      computer_ids = [%q]
    }
  }
}
`, name, payload, computerID)
}

func configScopeWithExclusions(name, payload, computerID string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      all_computers = true
    }
    exclusions = {
      computer_ids = [%q]
    }
  }
}
`, name, payload, computerID)
}

func configScopeAddRemoveJSSUser(name, payload, jssUserID string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
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
`, name, payload, jssUserID)
}

func configScopeClearJSSUsers(name, payload string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
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

func createDummyComputer(t *testing.T, name string) string {
	t.Helper()
	c := testhelpers.NewProClassicClient(t)
	ctx := context.Background()
	if err := c.CreateComputerByID(ctx, "0", &proclassic.ComputerPost{
		General: &proclassic.ComputerPostGeneral{Name: &name},
	}); err != nil {
		t.Fatalf("CreateComputerByID(%q): %v", name, err)
	}
	id := testhelpers.ResolveComputerIDByName(ctx, t, name)
	t.Cleanup(func() {
		if err := c.DeleteComputerByID(context.Background(), id); err != nil && !helpers.IsNotFoundError(err) {
			t.Logf("cleanup DeleteComputerByID(%s): %v", id, err)
		}
	})
	return id
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

// TestAccResource_MacOSConfigurationProfile_Minimal — minimal create / rename /
// import path. Uses a small managed-login-items mobileconfig from the corpus
// because it parses cleanly through Jamf Pro and exercises both top-level
// PayloadUUID rewrite and a non-trivial PayloadContent shape.
func TestAccResource_MacOSConfigurationProfile_Minimal(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-min-" + suffix
	renamed := name + "-renamed"
	payload := readFixture(t, "1Password__managed_login_items_profile.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name, payload),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("general").AtMapKey("name"),
						knownvalue.StringExact(name),
					),
				},
			},
			{
				Config: configMinimal(renamed, payload),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("general").AtMapKey("name"),
						knownvalue.StringExact(renamed),
					),
				},
			},
			{
				// Re-apply identical config → mask must suppress every diff,
				// producing an empty plan. This is the "no ghost profile"
				// test on the Update path.
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

// TestAccResource_MacOSConfigurationProfile_PayloadByteDifferentSemanticallyEqual
// — payload bytes change (whitespace re-indent) but semantics don't. Diff
// suppression must produce no resource update.
func TestAccResource_MacOSConfigurationProfile_PayloadByteDifferentSemanticallyEqual(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-sem-" + suffix
	payload := readFixture(t, "1Password__managed_login_items_profile.mobileconfig")

	// Re-indent: tabs → 4 spaces. Keeps line breaks (heredoc EOF still
	// resolves) and the `<?xml` preamble starts the string (plist parsers
	// reject leading whitespace before the declaration). The whitespace
	// re-indent is byte-different but semantically identical — the mask
	// must neutralise it.
	reformatted := strings.ReplaceAll(payload, "\t", "    ")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name, payload),
			},
			{
				Config: configMinimal(name, reformatted),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							"jamfplatform_pro_macos_configuration_profile.test",
							plancheck.ResourceActionNoop,
						),
					},
				},
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_RealPayloadChangeProducesPlan
// — modifying a non-masked field inside PayloadContent (the Rules[].Comment)
// must NOT be suppressed. The plan must show a non-empty change.
func TestAccResource_MacOSConfigurationProfile_RealPayloadChangeProducesPlan(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-real-" + suffix
	payload := readFixture(t, "1Password__managed_login_items_profile.mobileconfig")
	// Replace the Comment string of the first Rules entry with a distinct
	// value. This survives the mask (Rules.Comment is not server-rewritten)
	// and must surface as drift.
	tampered := strings.Replace(payload,
		"<string>Allow 1Password Launch Item</string>",
		"<string>tampered comment</string>",
		1,
	)
	if tampered == payload {
		t.Skip("expected to be able to tamper with Rules[0].Comment but pattern not found")
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

// TestAccResource_MacOSConfigurationProfile_DescriptionChange — sanity check
// that envelope-level fields (general.description) produce a normal plan
// when changed. This is a non-payload diff and should never be suppressed.
func TestAccResource_MacOSConfigurationProfile_DescriptionChange(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-desc-" + suffix
	payload := readFixture(t, "1Password__notifications_profile.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configWithDescription(name, payload, "v1")},
			{
				Config: configWithDescription(name, payload, "v2"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("general").AtMapKey("description"),
						knownvalue.StringExact("v2"),
					),
				},
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_AllComputersScope — exercises
// the scope sub-block validator (all_computers=true forbidding per-computer
// IDs) and roundtrips through state.
func TestAccResource_MacOSConfigurationProfile_AllComputersScope(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-all-" + suffix
	payload := readFixture(t, "DuckDuckGo__content_filter_profile.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configAllComputers(name, payload),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("all_computers"),
						knownvalue.Bool(true),
					),
				},
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_SelfService — exercises the
// Self Service sub-block including the dual <notification> wire elements.
func TestAccResource_MacOSConfigurationProfile_SelfService(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-ss-" + suffix
	payload := readFixture(t, "1Password__notifications_profile.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configWithSelfService(name, payload, "Test SS Display Name"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("self_service").AtMapKey("display_notifications"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("self_service").AtMapKey("notification_location"),
						knownvalue.StringExact("Self Service"),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("self_service").AtMapKey("removal_disallowed"),
						knownvalue.StringExact("Never"),
					),
				},
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_DistributionMethodChange —
// flips distribution_method between Install Automatically and Make Available
// in Self Service to confirm the wire-symmetric attribute round-trips.
func TestAccResource_MacOSConfigurationProfile_DistributionMethodChange(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-dm-" + suffix
	payload := readFixture(t, "1Password__screen_recording_profile.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configWithDistribution(name, payload, "Install Automatically")},
			{
				Config: configWithDistribution(name, payload, "Make Available in Self Service"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("general").AtMapKey("distribution_method"),
						knownvalue.StringExact("Make Available in Self Service"),
					),
				},
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_ScopeWithComputerIDs — pins a
// specific per-computer target via classic ID. Asserts the ID round-trips;
// the undeclared all_computers toggle stays null (owned outside Terraform).
func TestAccResource_MacOSConfigurationProfile_ScopeWithComputerIDs(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-comp-" + suffix
	computerID := createDummyComputer(t, "tf-acc-mcp-fixture-comp-"+suffix)
	payload := readFixture(t, "1Password__notifications_profile.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configScopeWithComputerIDs(name, payload, computerID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("computer_ids"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact(computerID)}),
					),
				},
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_ScopeWithExclusions — all_computers
// combined with a per-computer exclusion. Exercises the exclusion sub-block
// wiring.
func TestAccResource_MacOSConfigurationProfile_ScopeWithExclusions(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-excl-" + suffix
	computerID := createDummyComputer(t, "tf-acc-mcp-fixture-excl-"+suffix)
	payload := readFixture(t, "1Password__screen_recording_profile.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configScopeWithExclusions(name, payload, computerID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("all_computers"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("exclusions").AtMapKey("computer_ids"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact(computerID)}),
					),
				},
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_ScopeJSSUserAddRemove — add a Jamf
// Pro user to the scope on first apply, clear the category with a declared `[]`
// on the second (omitting it would leave the user scoped, as configured outside
// Terraform), then release the whole scope block on the third (state → null;
// the live scope is left untouched).
func TestAccResource_MacOSConfigurationProfile_ScopeJSSUserAddRemove(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-jssu-" + suffix
	jssUserID := createDummyUser(t, "tf-acc-mcp-jssuser-"+suffix)
	payload := readFixture(t, "1Password__managed_login_items_profile.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configScopeAddRemoveJSSUser(name, payload, jssUserID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("user_ids"),
						knownvalue.SetExact([]knownvalue.Check{knownvalue.StringExact(jssUserID)}),
					),
				},
			},
			{
				// Declared [] clears the category on the wire.
				Config: configScopeClearJSSUsers(name, payload),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"jamfplatform_pro_macos_configuration_profile.test",
						tfjsonpath.New("scope").AtMapKey("targets").AtMapKey("user_ids"),
						knownvalue.SetExact([]knownvalue.Check{}),
					),
				},
			},
			{
				Config: configMinimal(name, payload),
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_ImportState — import by ID
// without ImportStateVerify: import hydrates every wire-present optional
// section (scope with every category, self_service), while this minimal
// config declares none of them, so a verify would legitimately diff.
func TestAccResource_MacOSConfigurationProfile_ImportState(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-import-" + suffix
	payload := readFixture(t, "KarabinerElements__system_extension_profile.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configMinimal(name, payload)},
			{
				ResourceName:                         "jamfplatform_pro_macos_configuration_profile.test",
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

// TestAccResource_MacOSConfigurationProfile_ImportThenPlan_DerivedNamesStayKnown
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
func TestAccResource_MacOSConfigurationProfile_ImportThenPlan_DerivedNamesStayKnown(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-import-noop-" + suffix
	fixture := readFixture(t, "KarabinerElements__system_extension_profile.mobileconfig")
	const addr = "jamfplatform_pro_macos_configuration_profile.test"

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

// createOOBProfile creates a minimal macOS configuration profile directly via
// the SDK (no category, no site), then reads it back and returns its ID plus
// the server-stored payload. A t.Cleanup deletes it so the object never leaks
// if the import step fails before Terraform takes over management.
func createOOBProfile(t *testing.T, name, payload string) (id, serverPayload string) {
	t.Helper()
	c := proclassic.New(testhelpers.NewAcceptanceClient(t))
	ctx := context.Background()
	pl := proclassic.PayloadsXMLText(payload)
	created, err := c.CreateOSXConfigurationProfileByID(ctx, "0", &proclassic.OsXConfigurationProfile{
		General: &proclassic.OsXConfigurationProfileGeneral{Name: &name, Payloads: &pl},
	})
	if err != nil {
		t.Fatalf("out-of-band create of macOS configuration profile %q: %v", name, err)
	}
	switch {
	case created != nil && created.ID != nil:
		id = fmt.Sprintf("%d", *created.ID)
	case created != nil && created.General != nil && created.General.ID != nil:
		id = fmt.Sprintf("%d", *created.General.ID)
	}
	if id == "" {
		t.Fatalf("out-of-band create of macOS configuration profile %q returned no ID", name)
	}
	t.Cleanup(func() { _ = c.DeleteOSXConfigurationProfileByID(context.Background(), id) })

	got, err := c.GetOSXConfigurationProfileByID(ctx, id)
	if err != nil {
		t.Fatalf("reading back out-of-band macOS configuration profile %s: %v", id, err)
	}
	if got != nil && got.General != nil && got.General.Payloads != nil {
		serverPayload = string(*got.General.Payloads)
	}
	if serverPayload == "" {
		t.Fatalf("out-of-band macOS configuration profile %s returned an empty payload on read-back", id)
	}
	// The server returns the payload as a single line with no trailing newline;
	// configMinimal embeds it in a <<EOF heredoc, which needs the closing EOF on
	// its own line.
	if !strings.HasSuffix(serverPayload, "\n") {
		serverPayload += "\n"
	}
	return id, serverPayload
}

// mutatePPPCProfileChangeIdentifier simulates an out-of-band admin UI
// edit by fetching a PPPC profile, parsing its mobileconfig payload, and
// changing the Identifier of the first existing TCC service entry. The
// modified payload is then PUT back via the SDK. Mutating an existing
// service value rather than adding a new service avoids Jamf's
// invalid-service sanitisation pass (Jamf silently strips unknown or
// malformed service entries — see the DEVONthink Location case
// documented in helpers.go), so the modification is guaranteed to
// survive the round-trip and surface as drift on the next plan.
func mutatePPPCProfileChangeIdentifier(t *testing.T, profileID, newIdentifierValue string) {
	t.Helper()
	c := testhelpers.NewProClassicClient(t)
	ctx := context.Background()

	got, err := c.GetOSXConfigurationProfileByID(ctx, profileID)
	if err != nil {
		t.Fatalf("GetOSXConfigurationProfileByID(%s): %v", profileID, err)
	}
	if got == nil || got.General == nil || got.General.Payloads == nil {
		t.Fatalf("profile %s missing payload in GET response", profileID)
	}
	currentPayload := []byte(string(*got.General.Payloads))

	parsed, _, err := plisthelpers.ParsePlist(currentPayload)
	if err != nil {
		t.Fatalf("ParsePlist for profile %s: %v", profileID, err)
	}
	pcAny, ok := parsed["PayloadContent"]
	if !ok {
		t.Fatalf("profile %s payload missing PayloadContent", profileID)
	}
	pc, ok := pcAny.([]any)
	if !ok || len(pc) == 0 {
		t.Fatalf("profile %s PayloadContent not a non-empty array", profileID)
	}
	first, ok := pc[0].(map[string]any)
	if !ok {
		t.Fatalf("profile %s PayloadContent[0] not a dict", profileID)
	}
	servicesAny, ok := first["Services"]
	if !ok {
		t.Fatalf("profile %s PayloadContent[0] missing Services (not a PPPC payload)", profileID)
	}
	services, ok := servicesAny.(map[string]any)
	if !ok {
		t.Fatalf("profile %s PayloadContent[0].Services not a dict", profileID)
	}
	mutated := false
	for serviceKey, entriesAny := range services {
		entries, ok := entriesAny.([]any)
		if !ok || len(entries) == 0 {
			continue
		}
		entry, ok := entries[0].(map[string]any)
		if !ok {
			continue
		}
		entry["Identifier"] = newIdentifierValue
		t.Logf("admin-UI simulation: mutated Services[%q][0].Identifier → %q", serviceKey, newIdentifierValue)
		mutated = true
		break
	}
	if !mutated {
		t.Fatalf("profile %s had no Services[*] entries to mutate", profileID)
	}

	newPayloadBytes, err := plisthelpers.MarshalPlist(parsed)
	if err != nil {
		t.Fatalf("MarshalPlist for profile %s: %v", profileID, err)
	}

	newName := ""
	if got.General.Name != nil {
		newName = *got.General.Name
	}
	pxt := proclassic.PayloadsXMLText(newPayloadBytes)
	update := &proclassic.OsXConfigurationProfile{
		General: &proclassic.OsXConfigurationProfileGeneral{
			Name:     &newName,
			Payloads: &pxt,
		},
	}
	if err := c.UpdateOSXConfigurationProfileByID(ctx, profileID, update); err != nil {
		t.Fatalf("UpdateOSXConfigurationProfileByID(%s) during admin-UI simulation: %v", profileID, err)
	}
}

// TestAccResource_MacOSConfigurationProfile_AdminUIEdit_SurfacesAsDrift —
// the three-way payload compare must catch an out-of-band UI edit: the
// admin adds a service to the PPPC profile's Services dict via the Jamf
// Pro UI (simulated here via a direct SDK PUT). The next terraform plan
// against unchanged HCL must produce a non-empty plan so the drift is
// surfaced and the next apply re-aligns the server with the user's HCL.
//
// Before the three-way ModifyPlan landed, the legacy two-way compare
// with intersection semantics dropped the asymmetric Services key and
// silently reported "no changes" — the bug this test pins.
func TestAccResource_MacOSConfigurationProfile_AdminUIEdit_SurfacesAsDrift(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-drift-" + suffix
	payload := readFixture(t, "1Password__screen_recording_profile.mobileconfig")

	var profileID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name, payload),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["jamfplatform_pro_macos_configuration_profile.test"]
						if !ok {
							return fmt.Errorf("resource not found in state after Create")
						}
						profileID = rs.Primary.ID
						return nil
					},
				),
			},
			{
				// Re-apply identical config immediately after Create: with the
				// three-way private-state references freshly populated, the
				// plan must be empty.
				Config: configMinimal(name, payload),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				// Out-of-band admin UI edit between this step's plan-refresh
				// and plan: the Identifier of a TCC service entry is mutated
				// server-side. The plan must report drift (non-empty), which
				// the immediately-following apply step will then reconcile
				// back to the HCL baseline.
				PreConfig: func() {
					if profileID == "" {
						t.Fatal("profileID empty; Step 1 Check should have captured it")
					}
					mutatePPPCProfileChangeIdentifier(t, profileID, "com.acceptance.injected-by-ui-simulation")
				},
				Config: configMinimal(name, payload),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectNonEmptyPlan(),
					},
				},
			},
		},
	})
}

// mutatePPPCProfileAddValidService is the add-direction counterpart to
// mutatePPPCProfileChangeIdentifier. It injects a real-shape entry under
// a known-valid TCC service key. Jamf's invalid-service sanitisation
// pass is *key-name-driven* — well-known TCC service keys
// (Accessibility, Reminders, SpeechRecognition, …) survive the round
// trip when the entry dict is well-formed.
func mutatePPPCProfileAddValidService(t *testing.T, profileID, serviceKey string) {
	t.Helper()
	c := testhelpers.NewProClassicClient(t)
	ctx := context.Background()

	got, err := c.GetOSXConfigurationProfileByID(ctx, profileID)
	if err != nil {
		t.Fatalf("GetOSXConfigurationProfileByID(%s): %v", profileID, err)
	}
	currentPayload := []byte(string(*got.General.Payloads))
	parsed, _, err := plisthelpers.ParsePlist(currentPayload)
	if err != nil {
		t.Fatalf("ParsePlist: %v", err)
	}
	services := parsed["PayloadContent"].([]any)[0].(map[string]any)["Services"].(map[string]any)
	services[serviceKey] = []any{
		map[string]any{
			"Authorization":   "Allow",
			"Identifier":      "com.acceptance.added-via-ui",
			"CodeRequirement": "anchor apple generic",
			"IdentifierType":  "bundleID",
		},
	}
	newPayloadBytes, err := plisthelpers.MarshalPlist(parsed)
	if err != nil {
		t.Fatalf("MarshalPlist: %v", err)
	}
	newName := ""
	if got.General.Name != nil {
		newName = *got.General.Name
	}
	pxt := proclassic.PayloadsXMLText(newPayloadBytes)
	if err := c.UpdateOSXConfigurationProfileByID(ctx, profileID, &proclassic.OsXConfigurationProfile{
		General: &proclassic.OsXConfigurationProfileGeneral{Name: &newName, Payloads: &pxt},
	}); err != nil {
		t.Fatalf("UpdateOSXConfigurationProfileByID add-service: %v", err)
	}
	t.Logf("admin-UI simulation: added Services[%q]", serviceKey)
}

// mutatePPPCProfileRemoveFirstService removes the first service entry
// from PayloadContent[0].Services.
func mutatePPPCProfileRemoveFirstService(t *testing.T, profileID string) {
	t.Helper()
	c := testhelpers.NewProClassicClient(t)
	ctx := context.Background()

	got, err := c.GetOSXConfigurationProfileByID(ctx, profileID)
	if err != nil {
		t.Fatalf("GetOSXConfigurationProfileByID(%s): %v", profileID, err)
	}
	currentPayload := []byte(string(*got.General.Payloads))
	parsed, _, err := plisthelpers.ParsePlist(currentPayload)
	if err != nil {
		t.Fatalf("ParsePlist: %v", err)
	}
	services := parsed["PayloadContent"].([]any)[0].(map[string]any)["Services"].(map[string]any)
	if len(services) < 2 {
		t.Fatalf("profile %s has fewer than 2 services; remove-test needs >= 2 to leave a non-empty Services dict", profileID)
	}
	for k := range services {
		t.Logf("admin-UI simulation: removed Services[%q]", k)
		delete(services, k)
		break
	}
	newPayloadBytes, err := plisthelpers.MarshalPlist(parsed)
	if err != nil {
		t.Fatalf("MarshalPlist: %v", err)
	}
	newName := ""
	if got.General.Name != nil {
		newName = *got.General.Name
	}
	pxt := proclassic.PayloadsXMLText(newPayloadBytes)
	if err := c.UpdateOSXConfigurationProfileByID(ctx, profileID, &proclassic.OsXConfigurationProfile{
		General: &proclassic.OsXConfigurationProfileGeneral{Name: &newName, Payloads: &pxt},
	}); err != nil {
		t.Fatalf("UpdateOSXConfigurationProfileByID remove-service: %v", err)
	}
}

// TestAccResource_MacOSConfigurationProfile_AdminUIAdd_SurfacesAsDrift —
// admin adds a new service to a PPPC profile via the UI; plan must
// produce non-empty.
func TestAccResource_MacOSConfigurationProfile_AdminUIAdd_SurfacesAsDrift(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-add-" + suffix
	payload := readFixture(t, "1Password__screen_recording_profile.mobileconfig")

	var profileID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name, payload),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["jamfplatform_pro_macos_configuration_profile.test"]
						if !ok {
							return fmt.Errorf("resource not found in state after Create")
						}
						profileID = rs.Primary.ID
						return nil
					},
				),
			},
			{
				PreConfig: func() {
					if profileID == "" {
						t.Fatal("profileID empty")
					}
					mutatePPPCProfileAddValidService(t, profileID, "Accessibility")
				},
				Config: configMinimal(name, payload),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectNonEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_AdminUIRemove_SurfacesAsDrift —
// admin removes a service from a PPPC profile via the UI; plan must
// produce non-empty.
func TestAccResource_MacOSConfigurationProfile_AdminUIRemove_SurfacesAsDrift(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-rm-" + suffix
	// DEVONthink fixture carries multiple services so removing one still
	// leaves a non-empty Services dict on the wire.
	payload := readFixture(t, "DEVONthink__pppcp_profile.mobileconfig")

	var profileID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name, payload),
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["jamfplatform_pro_macos_configuration_profile.test"]
						if !ok {
							return fmt.Errorf("resource not found in state after Create")
						}
						profileID = rs.Primary.ID
						return nil
					},
				),
			},
			{
				PreConfig: func() {
					if profileID == "" {
						t.Fatal("profileID empty")
					}
					mutatePPPCProfileRemoveFirstService(t, profileID)
				},
				Config: configMinimal(name, payload),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectNonEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_ScopeLimitationsClearWithEmptySet
// verifies that a declared-but-empty category clears its members. Under
// granular per-category scope ownership a declared `[]` is the clear gesture:
// the build emits an explicit empty element, and the update's read-merge-write
// re-emits every undeclared category so only the declared one changes.
// Omitting the category instead would leave it as configured outside
// Terraform. Uses a network-segment fixture (no LDAP needed).
func TestAccResource_MacOSConfigurationProfile_ScopeLimitationsClearWithEmptySet(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-macoscp-limclear-" + suffix
	seg := "tf-acc-netseg-macoscp-" + suffix
	payload := readFixture(t, "1Password__notifications_profile.mobileconfig")
	cfg := func(segs string) string {
		return fmt.Sprintf(`
resource "jamfplatform_pro_network_segment" "fixture" {
  name             = %q
  starting_address = "10.96.0.0"
  ending_address   = "10.96.0.255"
}

resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name     = %q
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      all_computers = true
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
				Check:  resource.TestCheckResourceAttr("jamfplatform_pro_macos_configuration_profile.test", "scope.limitations.network_segment_ids.#", "1"),
			},
			{
				// Clear to [] — the declared-but-empty category must be emitted
				// as an explicit empty element so the subtree replace clears it.
				// Implicit post-step empty-plan enforces the round-trip.
				Config: cfg(``),
				Check:  resource.TestCheckResourceAttr("jamfplatform_pro_macos_configuration_profile.test", "scope.limitations.network_segment_ids.#", "0"),
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_ReservedCharacterCorpus proves
// the full reserved-character matrix round-trips without perpetual diffs
// through an MCX custom-settings payload — the payload family the Classic
// API stores byte-exact under the SDK's escaped-CDATA wire form (PI-827;
// see jamfplatform-go-sdk's acc_proclassic_profile_payloads_test.go for
// the wire-level matrix). Covers every reserved XML character in every
// legal representation (raw, named entity, decimal and hex refs), literal
// entity text, entity-bearing dict keys via all_five_mixed, a Santa-style
// CEL expression, and an embedded CDATA section. The server canonicalises
// representation (raw ">" is stored as "&gt;", numeric refs collapse, keys
// sort) — the resource's semantic comparison must absorb all of it. Steps:
// apply, identical re-apply (must be an empty plan), a genuine value change
// (must plan as an update, not be suppressed by the mask), then re-apply.
func TestAccResource_MacOSConfigurationProfile_ReservedCharacterCorpus(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-corpus-" + suffix
	payload := readFixture(t, "reserved_character_corpus.mobileconfig")
	changed := strings.Replace(payload, "target.team_id == \"EQHXZ8M8AV\"", "target.team_id == \"CHANGED000\"", 1)
	if changed == payload {
		t.Fatal("fixture edit did not apply — corpus fixture drifted?")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name, payload),
			},
			{
				Config: configMinimal(name, payload),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: configMinimal(name, changed),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(
							"jamfplatform_pro_macos_configuration_profile.test",
							plancheck.ResourceActionUpdate,
						),
					},
				},
			},
			{
				Config: configMinimal(name, changed),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_RealWorldAmpersandFixtures
// exercises real admin-supplied profiles (unsigned copies of the fixtures
// that drove the PI-827 investigation):
//   - setup_manager_ampersand: Jamf Setup Manager MCX profile with "R&D"
//     in a department picker — MCX-family storage, must round-trip
//     byte-exact with no perpetual diff.
//   - cli_escaping_test: the full reserved-character corpus as exported
//     from Jamf Pro (all five characters, every representation) — same
//     clean round-trip expectation.
func TestAccResource_MacOSConfigurationProfile_RealWorldAmpersandFixtures(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()

	for _, tc := range []struct {
		slug, fixture string
	}{
		{"setupmgr", "setup_manager_ampersand.mobileconfig"},
		{"escaping", "cli_escaping_test.mobileconfig"},
	} {
		payload := readFixture(t, tc.fixture)
		name := "tf-acc-mcp-" + tc.slug + "-" + suffix
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
			CheckDestroy:             checkDestroy(t),
			Steps: []resource.TestStep{
				{
					Config: configMinimal(name, payload),
				},
				{
					Config: configMinimal(name, payload),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
				},
			},
		})
	}
}

// TestAccResource_MacOSConfigurationProfile_LoginWindowAmpersandRejected
// documents the server defect for macOS payload types the Classic API
// stores verbatim: this real Login Window profile carries "&" in
// LoginwindowText, which the server persists with an extra entity layer
// (PI-827 — a device would display "Here is an &amp;"). The provider
// verifies the stored payload, rolls the create back, and fails with an
// actionable diagnostic. If this apply starts SUCCEEDING, Jamf fixed the
// ingest defect — convert this into a clean round-trip test.
func TestAccResource_MacOSConfigurationProfile_LoginWindowAmpersandRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-loginwin-" + suffix
	payload := readFixture(t, "login_window_ampersand.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name, payload),
				// Single tokens only: the diagnostic pre-wraps its bullet text,
				// so any pattern containing a space can land on a line break.
				ExpectError: regexp.MustCompile(`PayloadContent\[3\]\.LoginwindowText`),
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_LineWrappedConsentTextRejected
// covers the second fidelity class with a real mSCP-generated profile: its
// top-level ConsentText is decoratively line-wrapped, and Jamf Pro deletes
// every line feed and indent tab outside PayloadContent, so the words either
// side merge. The apply must fail naming that value — not blame PI-827, which
// this payload cannot trigger (it holds no "&" or "<").
func TestAccResource_MacOSConfigurationProfile_LineWrappedConsentTextRejected(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-consentwrap-" + suffix
	payload := readFixture(t, "mscp_consent_linewrapped.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config:      configMinimal(name, payload),
				ExpectError: regexp.MustCompile(`ConsentText\.default`),
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_ConsentTextCarriageReturns is the
// paired positive case: the same mSCP profile with its line wrapping expressed
// as `&#13;` character references. Jamf Pro keeps carriage returns in every
// slot, so this must apply cleanly and re-plan empty — the round trip the SDK's
// CR-reference preservation and the mask's line-ending normalisation exist to
// make work. If this starts failing, one of those two halves has regressed.
func TestAccResource_MacOSConfigurationProfile_ConsentTextCarriageReturns(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-consentcr-" + suffix
	payload := readFixture(t, "mscp_consent_cr_refs.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configMinimal(name, payload),
			},
			{
				Config: configMinimal(name, payload),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// fidelitySentinelName is a permanent profile kept on the CI tenant that holds
// "&" in a payload value Jamf Pro stores verbatim. The provider cannot create
// such a profile — Create detects the mangling and rolls back — so the import
// fidelity gate can only be exercised against a fixture authored outside
// Terraform, which is exactly the real-world case the gate exists for.
//
// Looked up by name, never by ID: the sentinel may be deleted and recreated,
// and a hardcoded ID would then silently test the wrong profile (or a profile
// that no longer exists).
const fidelitySentinelName = "Fidelity Test Sentinel [DO NOT DELETE]"

// lookupFidelitySentinelID resolves the sentinel to its current ID, skipping the
// test with an explicit message when it is absent. Skipping rather than failing
// keeps a tenant without the fixture usable, but the message has to name the
// fixture so an accidental deletion is not mistaken for the gate being untested.
func lookupFidelitySentinelID(t *testing.T) string {
	t.Helper()
	client := testhelpers.NewProClassicClient(t)
	got, err := client.GetOSXConfigurationProfileByName(context.Background(), fidelitySentinelName)
	if err != nil || got == nil || got.General == nil || got.General.ID == nil {
		t.Skipf("macOS profile %q not present on this tenant — recreate it (a Login Window payload whose "+
			"LoginwindowText contains \"&\") to cover the import fidelity gate; lookup error: %v",
			fidelitySentinelName, err)
	}
	return helpers.StringValueFromIntPtr(got.General.ID).ValueString()
}

// TestAccResource_MacOSConfigurationProfile_ImportFidelityGateRefusesSentinel is
// the regression test for the import trap: a profile holding "&" in a
// verbatim-stored payload imports and plans clean, then the first apply that
// touches it rewrites the payload into a corrupted form the Classic API will
// not accept the original back over. Import must therefore be refused outright.
//
// This cannot be covered by a unit test. The predicate has unit coverage; what
// only an acceptance test can prove is that Read actually consults it on the
// import path — an earlier revision gated on req.State.Raw.IsNull(), which is
// false for a config-driven import block, so every profile sailed through while
// every unit test still passed.
func TestAccResource_MacOSConfigurationProfile_ImportFidelityGateRefusesSentinel(t *testing.T) {
	testhelpers.AccPreCheck(t)
	id := lookupFidelitySentinelID(t)
	payload := readFixture(t, "KarabinerElements__system_extension_profile.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// A throwaway config supplies the resource address; the import
				// targets the sentinel by ID and must never reach state.
				Config:            configMinimal("tf-acc-mcp-gate-"+testhelpers.RunSuffix(), payload),
				ResourceName:      "jamfplatform_pro_macos_configuration_profile.test",
				ImportState:       true,
				ImportStateId:     id,
				ImportStateVerify: false,
				// Terraform re-wraps diagnostic prose at roughly 80 columns, so the
				// pattern must be short enough to never straddle a line break.
				ExpectError: regexp.MustCompile(`cannot be managed by Terraform`),
			},
		},
	})
}

// TestAccResource_MacOSConfigurationProfile_ImportFidelityGateNamesTheValue
// checks the refusal is actionable: on a tenant-wide import Terraform prints no
// resource address for a provider Read error, so the diagnostic itself has to
// identify both the profile and the offending value or the operator cannot act.
func TestAccResource_MacOSConfigurationProfile_ImportFidelityGateNamesTheValue(t *testing.T) {
	testhelpers.AccPreCheck(t)
	id := lookupFidelitySentinelID(t)
	payload := readFixture(t, "KarabinerElements__system_extension_profile.mobileconfig")

	// Single tokens only, for the line-wrap reason above. The PayloadContent
	// index is deliberately not asserted: it shifts if the sentinel is rebuilt
	// with a different payload order, and the key name is the actionable part.
	for _, want := range []*regexp.Regexp{
		regexp.MustCompile(`LoginwindowText`),
		regexp.MustCompile(`PI-827`),
		regexp.MustCompile(`Sentinel`),
	} {
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
			Steps: []resource.TestStep{
				{
					Config:        configMinimal("tf-acc-mcp-gatemsg-"+testhelpers.RunSuffix(), payload),
					ResourceName:  "jamfplatform_pro_macos_configuration_profile.test",
					ImportState:   true,
					ImportStateId: id,
					ExpectError:   want,
				},
			},
		})
	}
}

// TestAccResource_MacOSConfigurationProfile_RefreshIsNeverGated is the other half
// of the gate's contract, and the more important one to protect: the gate must
// fire ONLY on import. A profile that is already under management and then
// acquires an unstorable value out-of-band — an admin edits it in the UI — has
// to keep refreshing, or the operator can neither see the drift nor remove the
// resource, which would be worse than the trap the gate prevents.
//
// The out-of-band write goes through the SDK rather than the provider precisely
// because the provider refuses to write such a value.
func TestAccResource_MacOSConfigurationProfile_RefreshIsNeverGated(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-refreshgate-" + suffix
	clean := readFixture(t, "KarabinerElements__system_extension_profile.mobileconfig")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{Config: configMinimal(name, clean)},
			{
				// Inject "&" into the managed profile behind Terraform's back, then
				// re-plan. The plan must succeed and report drift; a gated refresh
				// would error here instead.
				PreConfig: func() { injectAmpersandOutOfBand(t, name) },
				Config:    configMinimal(name, clean),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectNonEmptyPlan(),
					},
				},
				// The apply that follows re-asserts the configured payload, which is
				// clean, so it must succeed — proving the resource stays manageable.
			},
		},
	})
}

// injectAmpersandOutOfBand rewrites the named profile's payload to carry "&" in
// a LoginwindowText value, simulating an admin-UI edit after the profile came
// under management. Uses the SDK directly: this is a write the provider will
// not perform.
func injectAmpersandOutOfBand(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	client := testhelpers.NewProClassicClient(t)

	got, err := client.GetOSXConfigurationProfileByName(ctx, name)
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
	if err := client.UpdateOSXConfigurationProfileByID(ctx, id,
		&proclassic.OsXConfigurationProfile{
			General: &proclassic.OsXConfigurationProfileGeneral{Payloads: &payload},
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
	suffix          string
	targetComputer  string
	excludeComputer string
	targetUser      string
	excludeUser     string
}

// omitRetainsFixtureHCL declares one inline fixture per scope category and one
// Self Service category. Targets and exclusions each get their own fixture so
// the wire assertion can tell a retained exclusion from a leaked target.
func omitRetainsFixtureHCL(suffix string) string {
	return fmt.Sprintf(`
resource "jamfplatform_pro_category" "omit" {
  name     = "tf-acc-mcp-omit-cat-%[1]s"
  priority = 7
}

resource "jamfplatform_pro_building" "target" {
  name = "tf-acc-mcp-omit-bld-t-%[1]s"
}

resource "jamfplatform_pro_building" "exclude" {
  name = "tf-acc-mcp-omit-bld-x-%[1]s"
}

resource "jamfplatform_pro_department" "target" {
  name = "tf-acc-mcp-omit-dep-t-%[1]s"
}

resource "jamfplatform_pro_department" "exclude" {
  name = "tf-acc-mcp-omit-dep-x-%[1]s"
}

resource "jamfplatform_device_group" "target" {
  name        = "tf-acc-mcp-omit-grp-t-%[1]s"
  description = "tf-acc omit-retains scope fixture"
  group_type  = "static"
  device_type = "computer"
}

resource "jamfplatform_device_group" "exclude" {
  name        = "tf-acc-mcp-omit-grp-x-%[1]s"
  description = "tf-acc omit-retains scope fixture"
  group_type  = "static"
  device_type = "computer"
}

resource "jamfplatform_pro_user_group" "target" {
  name       = "tf-acc-mcp-omit-ug-t-%[1]s"
  group_type = "static"
}

resource "jamfplatform_pro_user_group" "exclude" {
  name       = "tf-acc-mcp-omit-ug-x-%[1]s"
  group_type = "static"
}

resource "jamfplatform_pro_network_segment" "limit" {
  name             = "tf-acc-mcp-omit-seg-l-%[1]s"
  starting_address = "10.94.0.0"
  ending_address   = "10.94.0.255"
}

resource "jamfplatform_pro_network_segment" "exclude" {
  name             = "tf-acc-mcp-omit-seg-x-%[1]s"
  starting_address = "10.94.1.0"
  ending_address   = "10.94.1.255"
}

resource "jamfplatform_pro_ibeacon" "limit" {
  name                    = "tf-acc-mcp-omit-ib-l-%[1]s"
  uuid                    = "759b0599-64e0-416a-8d31-d8e93482a4d7"
  include_any_major_value = true
  include_any_minor_value = true
}

resource "jamfplatform_pro_ibeacon" "exclude" {
  name                    = "tf-acc-mcp-omit-ib-x-%[1]s"
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
// directory-service user-group categories, because the server refuses a group
// name that does not resolve against the tenant's directory integration.
//
// scope.targets.user_group_ids used to be left out too: the SDK tagged
// OsXConfigurationProfileScopeJssUserGroups' child as <jss_user_group> while
// the server answers <user_group>, so the category never read back and would
// have failed the create rather than the contract under test. SDK v0.22.1
// corrected the tag — it was the last one of the sixteen still mis-spelled —
// and the category is covered here now.
func omitRetainsConfig(name, payload string, f omitRetainsFixtures) string {
	return omitRetainsFixtureHCL(f.suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = "Make Available in Self Service"
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      computer_ids       = [%q]
      computer_group_ids = [jamfplatform_device_group.target.jamf_pro_id]
      building_ids       = [jamfplatform_pro_building.target.id]
      department_ids     = [jamfplatform_pro_department.target.id]
      user_ids           = [%q]
      user_group_ids     = [jamfplatform_pro_user_group.target.id]
    }
    limitations = {
      network_segment_ids                   = [jamfplatform_pro_network_segment.limit.id]
      ibeacon_ids                           = [jamfplatform_pro_ibeacon.limit.id]
      directory_service_or_local_user_names = ["tf-acc-omit-retains-limit-user"]
    }
    exclusions = {
      computer_ids                          = [%q]
      computer_group_ids                    = [jamfplatform_device_group.exclude.jamf_pro_id]
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
    self_service_display_name     = "Omit retains display name"
    install_button_text           = "Retain me"
    self_service_description      = "Omit-retains contract description."
    ensure_users_view_description = true
    feature_on_main_page          = true
    display_notifications         = true
    notification_location         = "Self Service"
    notification_subject          = "Omit retains subject"
    notification_message          = "Omit retains message"
    removal_disallowed            = "Never"
    categories = [
      {
        id         = jamfplatform_pro_category.omit.id
        display_in = true
        feature_in = true
      },
    ]
  }
  depends_on = [
    jamfplatform_pro_category.omit,
    jamfplatform_pro_building.target, jamfplatform_pro_building.exclude,
    jamfplatform_pro_department.target, jamfplatform_pro_department.exclude,
    jamfplatform_device_group.target, jamfplatform_device_group.exclude,
    jamfplatform_pro_user_group.target, jamfplatform_pro_user_group.exclude,
    jamfplatform_pro_network_segment.limit, jamfplatform_pro_network_segment.exclude,
    jamfplatform_pro_ibeacon.limit, jamfplatform_pro_ibeacon.exclude,
  ]
}
`, name, payload, f.targetComputer, f.targetUser, f.excludeComputer, f.excludeUser)
}

// omitRetainsParentsOnlyConfig keeps the scope and self_service parents but
// drops their gated children: scope loses limitations, exclusions and the
// user_ids target category, so the PUT re-emits the scope from the granular
// merge; self_service loses the Optional+Computed ensure_users_view_description
// leaf, and self_service.categories.
//
// Dropping categories is the point of the step rather than an omission: the
// categories stay on the server (omitRetainedOnServer asserts it on the wire)
// while state drops them, so this is the acceptance proof of the ownership gate
// from issue #392. Before that gate flattenSelfService hydrated whatever the
// wire carried, and this shape failed the apply with "Provider produced
// inconsistent result after apply" instead of testing the contract.
func omitRetainsParentsOnlyConfig(name, payload string, f omitRetainsFixtures) string {
	return omitRetainsFixtureHCL(f.suffix) + fmt.Sprintf(`
resource "jamfplatform_pro_macos_configuration_profile" "test" {
  general = {
    name                = %q
    distribution_method = "Make Available in Self Service"
    payloads = <<EOF
%sEOF
  }
  scope = {
    targets = {
      computer_ids       = [%q]
      computer_group_ids = [jamfplatform_device_group.target.jamf_pro_id]
      building_ids       = [jamfplatform_pro_building.target.id]
      department_ids     = [jamfplatform_pro_department.target.id]
    }
  }
  self_service = {
    self_service_display_name = "Omit retains display name"
    install_button_text       = "Retain me"
    self_service_description  = "Omit-retains contract description."
    feature_on_main_page      = true
    display_notifications     = true
    notification_location     = "Self Service"
    notification_subject      = "Omit retains subject"
    notification_message      = "Omit retains message"
    removal_disallowed        = "Never"
  }
  depends_on = [
    jamfplatform_pro_category.omit,
    jamfplatform_pro_building.target, jamfplatform_pro_building.exclude,
    jamfplatform_pro_department.target, jamfplatform_pro_department.exclude,
    jamfplatform_device_group.target, jamfplatform_device_group.exclude,
    jamfplatform_pro_user_group.target, jamfplatform_pro_user_group.exclude,
    jamfplatform_pro_network_segment.limit, jamfplatform_pro_network_segment.exclude,
    jamfplatform_pro_ibeacon.limit, jamfplatform_pro_ibeacon.exclude,
  ]
}
`, name, payload, f.targetComputer)
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
resource "jamfplatform_pro_macos_configuration_profile" "test" {
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
    jamfplatform_device_group.target, jamfplatform_device_group.exclude,
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

// stateID returns the primary id of a fixture resource from Terraform state,
// which is how the wire assertion learns the ids Jamf allocated to the inline
// fixtures.
func stateID(s *terraform.State, addr, attr string) (string, error) {
	rs, ok := s.RootModule().Resources[addr]
	if !ok {
		return "", fmt.Errorf("fixture %s not found in state", addr)
	}
	v, ok := rs.Primary.Attributes[attr]
	if !ok || v == "" {
		return "", fmt.Errorf("fixture %s has no %s in state", addr, attr)
	}
	return v, nil
}

// omitRetainedOnServer asserts the server's copy still carries every value the
// omit-retains config declared in its first step. Inline fixture ids are read
// from state because Jamf allocates them at apply.
//
// The four notification attributes are declared but not asserted here. An
// earlier reading had them down as never echoed — "the classic GET never
// echoes <notification>, <notification_subject> or <notification_message> for
// a macOS profile" — which is not so: a re-probe on 2026-09-07 read all three
// back on every GET. What is true is subtler, and belongs to
// buildSelfServiceNotification rather than to this contract: the flag and the
// location share one wire element, so a write carries only one of them and the
// pair cannot be asserted as declared. notification_location is set to
// "Self Service" above for that reason, the location being the half this
// provider's write order loses.
func omitRetainedOnServer(t *testing.T, f omitRetainsFixtures) resource.TestCheckFunc {
	c := testhelpers.NewProClassicClient(t)
	const addr = "jamfplatform_pro_macos_configuration_profile.test"
	return func(s *terraform.State) error {
		want := map[string]string{}
		for _, fx := range []struct{ key, addr, attr string }{
			{"category", "jamfplatform_pro_category.omit", "id"},
			{"bldT", "jamfplatform_pro_building.target", "id"},
			{"bldX", "jamfplatform_pro_building.exclude", "id"},
			{"depT", "jamfplatform_pro_department.target", "id"},
			{"depX", "jamfplatform_pro_department.exclude", "id"},
			{"grpT", "jamfplatform_device_group.target", "jamf_pro_id"},
			{"grpX", "jamfplatform_device_group.exclude", "jamf_pro_id"},
			{"ugT", "jamfplatform_pro_user_group.target", "id"},
			{"ugX", "jamfplatform_pro_user_group.exclude", "id"},
			{"segL", "jamfplatform_pro_network_segment.limit", "id"},
			{"segX", "jamfplatform_pro_network_segment.exclude", "id"},
			{"ibL", "jamfplatform_pro_ibeacon.limit", "id"},
			{"ibX", "jamfplatform_pro_ibeacon.exclude", "id"},
		} {
			v, err := stateID(s, fx.addr, fx.attr)
			if err != nil {
				return err
			}
			want[fx.key] = v
		}
		return testhelpers.CheckLiveObject(addr,
			func(ctx context.Context, id string) (*proclassic.OsXConfigurationProfile, error) {
				return c.GetOSXConfigurationProfileByID(ctx, id)
			},
			func(p *proclassic.OsXConfigurationProfile) error {
				sc := p.Scope
				if sc == nil {
					return fmt.Errorf("scope: absent")
				}
				computerID := func(i proclassic.OsXConfigurationProfileScopeComputersComputerItem) *int { return i.ID }
				checks := []error{
					requireOnlyID("scope.targets.computer_ids", derefComputers(sc.Computers), computerID, f.targetComputer),
					requireOnlyIDName("scope.targets.computer_group_ids", derefField(sc.ComputerGroups, func(g *proclassic.OsXConfigurationProfileScopeComputerGroups) *[]proclassic.IDName {
						return g.ComputerGroup
					}), want["grpT"]),
					requireOnlyIDName("scope.targets.building_ids", derefField(sc.Buildings, func(b *proclassic.OsXConfigurationProfileScopeBuildings) *[]proclassic.IDName { return b.Building }), want["bldT"]),
					requireOnlyIDName("scope.targets.department_ids", derefField(sc.Departments, func(d *proclassic.OsXConfigurationProfileScopeDepartments) *[]proclassic.IDName { return d.Department }), want["depT"]),
					requireOnlyIDName("scope.targets.user_ids", derefField(sc.JssUsers, func(u *proclassic.OsXConfigurationProfileScopeJssUsers) *[]proclassic.IDName { return u.User }), f.targetUser),
					requireOnlyIDName("scope.targets.user_group_ids", derefField(sc.JssUserGroups, func(u *proclassic.OsXConfigurationProfileScopeJssUserGroups) *[]proclassic.IDName {
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
					requireOnlyID("scope.limitations.network_segment_ids", derefField(l.NetworkSegments, func(n *proclassic.OsXConfigurationProfileScopeLimitationsNetworkSegments) *[]proclassic.OsXConfigurationProfileScopeLimitationsNetworkSegmentsNetworkSegmentItem {
						return n.NetworkSegment
					}), func(i proclassic.OsXConfigurationProfileScopeLimitationsNetworkSegmentsNetworkSegmentItem) *int {
						return i.ID
					}, want["segL"]),
					requireOnlyIDName("scope.limitations.ibeacon_ids", derefField(l.Ibeacons, func(i *proclassic.OsXConfigurationProfileScopeLimitationsIbeacons) *[]proclassic.IDName {
						return i.Ibeacon
					}), want["ibL"]),
					requireOnlyName("scope.limitations.directory_service_or_local_user_names", derefField(l.Users, func(u *proclassic.OsXConfigurationProfileScopeLimitationsUsers) *[]proclassic.IDName { return u.User }), func(i proclassic.IDName) *string { return i.Name }, "tf-acc-omit-retains-limit-user"),
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
					requireOnlyID("scope.exclusions.computer_ids", derefField(e.Computers, func(c *proclassic.OsXConfigurationProfileScopeExclusionsComputers) *[]proclassic.OsXConfigurationProfileScopeExclusionsComputersComputerItem {
						return c.Computer
					}), func(i proclassic.OsXConfigurationProfileScopeExclusionsComputersComputerItem) *int { return i.ID }, f.excludeComputer),
					requireOnlyIDName("scope.exclusions.computer_group_ids", derefField(e.ComputerGroups, func(g *proclassic.OsXConfigurationProfileScopeExclusionsComputerGroups) *[]proclassic.IDName {
						return g.ComputerGroup
					}), want["grpX"]),
					requireOnlyIDName("scope.exclusions.building_ids", derefField(e.Buildings, func(b *proclassic.OsXConfigurationProfileScopeExclusionsBuildings) *[]proclassic.IDName {
						return b.Building
					}), want["bldX"]),
					requireOnlyIDName("scope.exclusions.department_ids", derefField(e.Departments, func(d *proclassic.OsXConfigurationProfileScopeExclusionsDepartments) *[]proclassic.IDName {
						return d.Department
					}), want["depX"]),
					requireOnlyIDName("scope.exclusions.user_ids", derefField(e.JssUsers, func(u *proclassic.OsXConfigurationProfileScopeExclusionsJssUsers) *[]proclassic.IDName { return u.User }), f.excludeUser),
					requireOnlyIDName("scope.exclusions.user_group_ids", derefField(e.JssUserGroups, func(u *proclassic.OsXConfigurationProfileScopeExclusionsJssUserGroups) *[]proclassic.IDName {
						return u.UserGroup
					}), want["ugX"]),
					requireOnlyID("scope.exclusions.network_segment_ids", derefField(e.NetworkSegments, func(n *proclassic.OsXConfigurationProfileScopeExclusionsNetworkSegments) *[]proclassic.OsXConfigurationProfileScopeExclusionsNetworkSegmentsNetworkSegmentItem {
						return n.NetworkSegment
					}), func(i proclassic.OsXConfigurationProfileScopeExclusionsNetworkSegmentsNetworkSegmentItem) *int {
						return i.ID
					}, want["segX"]),
					requireOnlyIDName("scope.exclusions.ibeacon_ids", derefField(e.Ibeacons, func(i *proclassic.OsXConfigurationProfileScopeExclusionsIbeacons) *[]proclassic.IDName {
						return i.Ibeacon
					}), want["ibX"]),
					requireOnlyName("scope.exclusions.directory_service_or_local_user_names", derefField(e.Users, func(u *proclassic.OsXConfigurationProfileScopeExclusionsUsers) *[]proclassic.OsXConfigurationProfileScopeExclusionsUsersUserItem {
						return u.User
					}), func(i proclassic.OsXConfigurationProfileScopeExclusionsUsersUserItem) *string { return i.Name }, "tf-acc-omit-retains-exclude-user"),
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
				checks = []error{
					testhelpers.RequireEqual("self_service.self_service_display_name", "Omit retains display name", testhelpers.Deref(ss.SelfServiceDisplayName)),
					testhelpers.RequireEqual("self_service.install_button_text", "Retain me", testhelpers.Deref(ss.InstallButtonText)),
					testhelpers.RequireEqual("self_service.self_service_description", "Omit-retains contract description.", testhelpers.Deref(ss.SelfServiceDescription)),
					testhelpers.RequireEqual("self_service.ensure_users_view_description", true, testhelpers.Deref(ss.ForceUsersToViewDescription)),
					testhelpers.RequireEqual("self_service.feature_on_main_page", true, testhelpers.Deref(ss.FeatureOnMainPage)),
				}
				for _, err := range checks {
					if err != nil {
						return err
					}
				}
				if ss.Security == nil {
					return fmt.Errorf("self_service.security: absent")
				}
				if err := testhelpers.RequireEqual("self_service.removal_disallowed", "Never", testhelpers.Deref(ss.Security.RemovalDisallowed)); err != nil {
					return err
				}
				cats := derefField(ss.SelfServiceCategories, func(c *proclassic.OsXConfigurationProfileSelfServiceSelfServiceCategories) *[]proclassic.OsXConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem {
					return c.Category
				})
				if err := requireOnlyID("self_service.categories", cats, func(i proclassic.OsXConfigurationProfileSelfServiceSelfServiceCategoriesCategoryItem) *int {
					return i.ID
				}, want["category"]); err != nil {
					return err
				}
				cat := (*cats)[0]
				if err := testhelpers.RequireEqual("self_service.categories[0].display_in", true, testhelpers.Deref(cat.DisplayIn)); err != nil {
					return err
				}
				return testhelpers.RequireEqual("self_service.categories[0].feature_in", true, testhelpers.Deref(cat.FeatureIn))
			})(s)
	}
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

func derefComputers(c *proclassic.OsXConfigurationProfileScopeComputers) *[]proclassic.OsXConfigurationProfileScopeComputersComputerItem {
	return derefField(c, func(c *proclassic.OsXConfigurationProfileScopeComputers) *[]proclassic.OsXConfigurationProfileScopeComputersComputerItem {
		return c.Computer
	})
}

// TestAccResource_MacOSConfigurationProfile_OmittedBlocksRetained pins the
// omit-retains contract the plan output cannot show: dropping scope
// limitations, exclusions, target categories, self_service and its categories
// from config plans them as removed, but the classic PUT either omits the
// element or re-emits it from the granular scope merge, and the server keeps
// every value. Step 2 keeps the scope and self_service parents while dropping
// their gated children; step 3 drops every optional block so the PUT carries
// <general> alone. Each step's implicit post-apply plan must be empty. If this
// test fails on content, the endpoint no longer merges and nothing that
// suppresses the removal plan may ship for this resource. The payload is never
// touched between steps so a payload diff here is a finding of its own.
func TestAccResource_MacOSConfigurationProfile_OmittedBlocksRetained(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-omit-" + suffix
	payload := readFixture(t, "1Password__notifications_profile.mobileconfig")
	f := omitRetainsFixtures{
		suffix:          suffix,
		targetComputer:  createDummyComputer(t, "tf-acc-mcp-omit-comp-t-"+suffix),
		excludeComputer: createDummyComputer(t, "tf-acc-mcp-omit-comp-x-"+suffix),
		targetUser:      createDummyUser(t, "tf-acc-mcp-omit-user-t-"+suffix),
		excludeUser:     createDummyUser(t, "tf-acc-mcp-omit-user-x-"+suffix),
	}
	const addr = "jamfplatform_pro_macos_configuration_profile.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: omitRetainsConfig(name, payload, f),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "self_service.install_button_text", "Retain me"),
					resource.TestCheckResourceAttr(addr, "self_service.categories.#", "1"),
					resource.TestCheckResourceAttr(addr, "scope.exclusions.ibeacon_ids.#", "1"),
					omitRetainedOnServer(t, f),
				),
			},
			{
				Config: omitRetainsParentsOnlyConfig(name, payload, f),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(addr, "self_service.categories.#"),
					resource.TestCheckResourceAttr(addr, "self_service.ensure_users_view_description", "true"),
					resource.TestCheckNoResourceAttr(addr, "scope.limitations.network_segment_ids.#"),
					resource.TestCheckNoResourceAttr(addr, "scope.exclusions.computer_ids.#"),
					resource.TestCheckNoResourceAttr(addr, "scope.targets.user_ids.#"),
					omitRetainedOnServer(t, f),
				),
			},
			{
				Config: omitRetainsGeneralOnlyConfig(name, payload, f),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(addr, "scope.targets.computer_ids.#"),
					resource.TestCheckNoResourceAttr(addr, "self_service.install_button_text"),
					omitRetainedOnServer(t, f),
				),
			},
		},
	})
}

// ── Web clip icons (issue #418) ───────────────────────────────────────────────

// TestAccResource_MacOSConfigurationProfile_WebClipIconSurvivesAWrite mirrors
// the mobile-device test of the same name. Jamf Pro re-renders a web clip icon
// on this endpoint too — wire-probed 2026-09-09, a 64x64 PNG came back as a
// 180x180 one — and the tolerance that makes that survive a write lives in
// shared code (payloadhelpers.LenientEqualPlist), so both resources need the
// regression, not just the one the bug was reported against.
//
// The steps are the reported reproduction: create, change a field unrelated to
// the payload, then re-apply the same configuration and expect an empty plan.
func TestAccResource_MacOSConfigurationProfile_WebClipIconSurvivesAWrite(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcp-webclip-" + suffix
	payload := freshWebClipPayload(t)
	const addr = "jamfplatform_pro_macos_configuration_profile.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             checkDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: configWithDescription(name, payload, "before"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "general.description", "before"),
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

// freshWebClipPayload reads the icon-bearing fixture and gives it fresh
// top-level identifiers, so repeated runs do not collide on Jamf Pro's
// duplicate-UUID check.
func freshWebClipPayload(t *testing.T) string {
	t.Helper()
	raw := readFixture(t, "profile_webclip_icon.mobileconfig")
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("generating UUID: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	out, err := payloadhelpers.InjectTopLevelIdentifierValues([]byte(raw), uuid, uuid)
	if err != nil {
		t.Fatalf("injecting identifiers: %v", err)
	}
	s := string(out)
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}
