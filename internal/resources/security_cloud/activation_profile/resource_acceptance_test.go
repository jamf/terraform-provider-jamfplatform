// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

// Acceptance coverage for jamfplatform_security_cloud_activation_profile.
//
// Three departures from the coverage contract in TESTING.md §Writing Acceptance
// Tests are forced by the wire behaviour documented in resource.go, and each is
// restated at the point it applies:
//
// There is no import round-trip, because the resource deliberately does not
// implement ResourceWithImportState. A read returns the activation code and
// nothing else, and every other attribute is RequiresReplace, so an import would
// adopt a profile and then immediately plan to destroy it.
//
// There is no destroy verification. Deleting an activation profile is a soft
// delete the read surface does not reflect: afterwards the item GET still answers
// 200 and the collection still returns the code. A CheckDestroy that expected a
// 404 would fail on every run.
//
// Every apply that creates a profile therefore leaks a row into the tenant's
// activation profile list, permanently and irreversibly. One full run of this
// file creates exactly one profile, in TestAccResource_SecurityCloudActivationProfile_Lifecycle;
// every other test in the file fails at plan time and provisions nothing. Keep it
// that way — fold new apply-level assertions into the lifecycle test rather than
// adding a second creating test.
//
// There is also no plural data source or list resource test, because neither
// construct exists: the list operation cannot enumerate a tenant.
package activation_profile_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/securitycloud"

	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

const activationProfileAddress = "jamfplatform_security_cloud_activation_profile.test"

// activationProfileDestroyIsUnverifiable is the CheckDestroy for every test in
// this file, and it asserts nothing on purpose.
//
// Jamf Security Cloud's delete is a soft delete its read surface does not
// reflect. After a successful delete the item GET still answers 200 with the same
// activation code and the collection still lists it, and the bulk delete endpoint
// answers 204 for a code it deleted, a code already deleted, and a code that
// never existed alike. So no request this function could make would distinguish a
// destroyed profile from a live one, and a CheckDestroy asserting absence would
// fail on every run rather than catching a regression.
func activationProfileDestroyIsUnverifiable(_ *testing.T) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		return nil
	}
}

// TestAccResource_SecurityCloudActivationProfile_Lifecycle is the only test in
// this file that provisions anything, and it provisions exactly one profile.
//
// It is deliberately long, because a leaked tenant row is the cost of a second
// creating test. The steps are, in order:
//
//  1. create with a single service capability and assert what state holds;
//  2. pause, asserting the plan is an in-place update and the code survives;
//  3. resume, asserting the same in the other direction;
//  4. delete the profile out of band, then refresh — and assert the plan is
//     EMPTY, which pins the soft-delete read law rather than working around it;
//  5. to 8. plan-only steps proving each remaining attribute is RequiresReplace.
//
// Steps 5 to 8 never apply, so they mint no profile and cost no tenant row. That
// is also why the replace coverage is a plan assertion rather than an applied
// replace: applying one would mint a second code and leak a second row to prove
// something the plan already states.
//
// Note what step 1 does NOT assert. Read cannot refresh name, platforms,
// capabilities or device_group_id — Jamf Security Cloud returns only the code —
// so those checks are assertions about what the provider wrote into state, not
// round-trip verification that the server stored it. Nothing in this file can
// verify the latter.
func TestAccResource_SecurityCloudActivationProfile_Lifecycle(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-jsc-activation-" + suffix
	note := "tf-acc note " + suffix

	base := activationProfileFields{
		name:            name,
		platforms:       []string{"ios"},
		contentControls: true,
		networkSecurity: true,
		note:            note,
	}

	paused := base
	paused.paused = new(true)

	resumed := base
	resumed.paused = new(false)

	renamed := resumed
	renamed.name = name + "-renamed"

	replatformed := resumed
	replatformed.platforms = []string{"ios", "mac"}

	// Flipped off rather than on: the create enables both capabilities so that
	// both wire fields are exercised for the price of the one profile this file is
	// allowed to mint, which leaves disabling one as the way to prove a capability
	// change replaces. network_security stays on, so the config remains valid.
	recapable := resumed
	recapable.contentControls = false

	regrouped := resumed
	regrouped.deviceGroupID = "tf-acc-no-such-device-group-" + suffix

	var code string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             activationProfileDestroyIsUnverifiable(t),
		Steps: []resource.TestStep{
			{
				Config: activationProfileConfig(base),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(activationProfileAddress, "id"),
					resource.TestCheckResourceAttr(activationProfileAddress, "name", name),
					resource.TestCheckResourceAttr(activationProfileAddress, "platforms.#", "1"),
					resource.TestCheckTypeSetElemAttr(activationProfileAddress, "platforms.*", "ios"),
					resource.TestCheckResourceAttr(activationProfileAddress, "capabilities.network_security", "true"),
					resource.TestCheckResourceAttr(activationProfileAddress, "capabilities.content_controls", "true"),
					resource.TestCheckResourceAttr(activationProfileAddress, "capabilities.note", note),
					resource.TestCheckNoResourceAttr(activationProfileAddress, "device_group_id"),
					resource.TestCheckResourceAttr(activationProfileAddress, "paused", "false"),
					captureActivationCode(activationProfileAddress, &code),
				),
			},
			{
				Config: activationProfileConfig(paused),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(activationProfileAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(activationProfileAddress, "paused", "true"),
					expectActivationCodeUnchanged(activationProfileAddress, &code),
				),
			},
			{
				Config: activationProfileConfig(resumed),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(activationProfileAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(activationProfileAddress, "paused", "false"),
					expectActivationCodeUnchanged(activationProfileAddress, &code),
				),
			},
			{
				PreConfig: func() {
					c := securitycloud.New(testhelpers.NewAcceptanceClient(t))
					err := c.DeleteActivationProfilesV1(context.Background(), &securitycloud.BulkDeleteActivationProfilesRequest{
						Codes: []string{code},
					})
					if err != nil {
						t.Fatalf("out-of-band delete of activation profile %s: %v", code, err)
					}
				},
				RefreshState: true,
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					expectActivationCodeUnchanged(activationProfileAddress, &code),
				),
			},
			{
				Config:             activationProfileConfig(renamed),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(activationProfileAddress, plancheck.ResourceActionReplace),
					},
				},
			},
			{
				Config:             activationProfileConfig(replatformed),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(activationProfileAddress, plancheck.ResourceActionReplace),
					},
				},
			},
			{
				Config:             activationProfileConfig(recapable),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(activationProfileAddress, plancheck.ResourceActionReplace),
					},
				},
			},
			{
				Config:             activationProfileConfig(regrouped),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPreRefresh: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(activationProfileAddress, plancheck.ResourceActionReplace),
					},
				},
			},
		},
	})
}

// TestAccResource_SecurityCloudActivationProfile_NoCapabilityRejectedAtPlan pins
// the ConfigValidator, from both directions an operator reaches it: capabilities
// spelled out with everything false, and capabilities left empty so the schema
// defaults supply the falses.
//
// Jamf Security Cloud enforces this itself, but as a business rule in its own
// error envelope — nothing in the response names a field — so a regression that
// dropped the validator would surface as an unattributed 400 mid-apply, after the
// plan had promised a profile.
func TestAccResource_SecurityCloudActivationProfile_NoCapabilityRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: activationProfileConfig(activationProfileFields{
					name:      "tf-acc-jsc-activation-nocap-" + suffix,
					platforms: []string{"ios"},
				}),
				ExpectError: regexpNoCapability,
			},
			{
				Config: fmt.Sprintf(`
					resource "jamfplatform_security_cloud_activation_profile" "test" {
						name         = %q
						platforms    = ["ios"]
						capabilities = {}
					}
				`, "tf-acc-jsc-activation-nocap-default-"+suffix),
				ExpectError: regexpNoCapability,
			},
		},
	})
}

// TestAccResource_SecurityCloudActivationProfile_PlatformsRejectedAtPlan pins the
// platform set validators: the size bounds Jamf Security Cloud enforces as a bare
// 400, and the accepted-value list, which is derived from the SDK's own generated
// value set rather than a restated literal.
//
// The over-size case has to name a third platform Jamf Security Cloud does not
// accept, because only two exist and a set cannot hold a duplicate. That step
// therefore raises two diagnostics, and its pattern deliberately matches the size
// one: if the size bound were dropped, the remaining accepted-value error would
// not match and the step would still fail.
func TestAccResource_SecurityCloudActivationProfile_PlatformsRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()

	valid := activationProfileFields{
		name:            "tf-acc-jsc-activation-platforms-" + suffix,
		networkSecurity: true,
	}

	empty := valid
	empty.platforms = []string{}

	tooMany := valid
	tooMany.platforms = []string{"ios", "mac", "ipados"}

	unknown := valid
	unknown.platforms = []string{"windows"}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      activationProfileConfig(empty),
				ExpectError: regexpPlatformsTooFew,
			},
			{
				Config:      activationProfileConfig(tooMany),
				ExpectError: regexpPlatformsTooMany,
			},
			{
				Config:      activationProfileConfig(unknown),
				ExpectError: regexpPlatformNotAccepted,
			},
		},
	})
}

// TestAccResource_SecurityCloudActivationProfile_NameLengthRejectedAtPlan pins
// both ends of the name length bound. An empty name and a 101-character name are
// each refused by Jamf Security Cloud as an unattributed 400 mid-apply, which is
// why the schema checks the length at plan time.
func TestAccResource_SecurityCloudActivationProfile_NameLengthRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)

	valid := activationProfileFields{
		platforms:       []string{"ios"},
		networkSecurity: true,
	}

	empty := valid
	empty.name = ""

	tooLong := valid
	tooLong.name = strings.Repeat("a", 101)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      activationProfileConfig(empty),
				ExpectError: regexpNameTooShort,
			},
			{
				Config:      activationProfileConfig(tooLong),
				ExpectError: regexpNameTooLong,
			},
		},
	})
}

// TestAccResource_SecurityCloudActivationProfile_NoteLengthRejectedAtPlan pins
// the capability note length bound at 256 characters, one over the limit.
func TestAccResource_SecurityCloudActivationProfile_NoteLengthRejectedAtPlan(t *testing.T) {
	testhelpers.AccPreCheckSecurityCloud(t)
	suffix := testhelpers.RunSuffix()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: activationProfileConfig(activationProfileFields{
					name:            "tf-acc-jsc-activation-note-" + suffix,
					platforms:       []string{"ios"},
					networkSecurity: true,
					note:            strings.Repeat("n", 256),
				}),
				ExpectError: regexpNoteTooLong,
			},
		},
	})
}

// activationProfileFields is the input to activationProfileConfig. The zero value
// renders a configuration with no capability enabled, which is the shape the
// ConfigValidator test needs.
type activationProfileFields struct {
	name            string
	platforms       []string
	contentControls bool
	networkSecurity bool
	note            string
	deviceGroupID   string
	paused          *bool
}

// activationProfileConfig renders a complete activation profile configuration.
//
// Every step of the lifecycle test renders through this one function, and each
// derives its fields from the previous step's struct. That is what keeps a step
// differing from its predecessor in exactly the attribute under test: a stray
// difference anywhere else turns an in-place update into a replace, which would
// silently invert what the step claims to prove.
//
// `capabilities` is an attribute, so it is rendered as `capabilities = { ... }`.
// The block form Terraform accepts for a nested block is refused for a
// SingleNestedAttribute.
func activationProfileConfig(f activationProfileFields) string {
	var b strings.Builder

	b.WriteString("resource \"jamfplatform_security_cloud_activation_profile\" \"test\" {\n")
	fmt.Fprintf(&b, "  name      = %q\n", f.name)
	fmt.Fprintf(&b, "  platforms = %s\n\n", hclStringList(f.platforms))

	b.WriteString("  capabilities = {\n")
	fmt.Fprintf(&b, "    content_controls = %t\n", f.contentControls)
	fmt.Fprintf(&b, "    network_security = %t\n", f.networkSecurity)
	if f.note != "" {
		fmt.Fprintf(&b, "    note             = %q\n", f.note)
	}
	b.WriteString("  }\n")

	if f.deviceGroupID != "" {
		fmt.Fprintf(&b, "\n  device_group_id = %q\n", f.deviceGroupID)
	}
	if f.paused != nil {
		fmt.Fprintf(&b, "\n  paused = %t\n", *f.paused)
	}

	b.WriteString("}\n")

	return b.String()
}

// hclStringList renders a string slice as an HCL list literal.
func hclStringList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, fmt.Sprintf("%q", v))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// captureActivationCode records the applied activation code so later steps can
// act on the profile directly and assert the code did not change.
//
// PreConfig runs without access to state, so the code has to be carried out of
// the apply step this way.
func captureActivationCode(address string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("resource %s not found in state", address)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("resource %s has an empty activation code", address)
		}
		*into = rs.Primary.ID
		return nil
	}
}

// expectActivationCodeUnchanged asserts the profile still carries the activation
// code an earlier step captured.
//
// This is the assertion that makes the pause and resume steps mean something. The
// activation code is the profile's identity and the value distributed to users, so
// a regression that turned pausing into a replace would still leave a valid,
// paused profile in state — and only a changed code reveals that the code every
// enrolled user holds has been invalidated.
func expectActivationCodeUnchanged(address string, want *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[address]
		if !ok {
			return fmt.Errorf("resource %s not found in state", address)
		}
		if *want == "" {
			return fmt.Errorf("no activation code was captured before checking %s", address)
		}
		if rs.Primary.ID != *want {
			return fmt.Errorf("activation code changed: expected %s, got %s", *want, rs.Primary.ID)
		}
		return nil
	}
}

// Expected-error patterns for the plan-time refusals.
//
// Each is built with diagPattern rather than written as a literal regular
// expression, because Terraform wraps diagnostic detail at roughly 80 columns and
// every one of these messages is longer than that. A literal pattern would depend
// on no line break falling inside it.
var (
	regexpNoCapability        = diagPattern("No service capability enabled")
	regexpPlatformsTooFew     = diagPattern("set must contain at least 1 elements and at most 2 elements, got: 0")
	regexpPlatformsTooMany    = diagPattern("set must contain at least 1 elements and at most 2 elements, got: 3")
	regexpPlatformNotAccepted = diagPattern("Invalid Attribute Value Match")
	regexpNameTooShort        = diagPattern("string length must be between 1 and 100, got: 0")
	regexpNameTooLong         = diagPattern("string length must be between 1 and 100, got: 101")
	regexpNoteTooLong         = diagPattern("string length must be at most 255, got: 256")
)

// diagPattern compiles a diagnostic phrase into a pattern that survives
// Terraform's line wrapping, by matching any run of whitespace wherever the
// phrase has a space. Terraform wraps at a word boundary and re-indents the
// continuation, so the words themselves are stable and only the space between
// them is not.
func diagPattern(phrase string) *regexp.Regexp {
	return regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(phrase), " ", `\s+`))
}
