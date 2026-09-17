// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package blueprint_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	bpSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// The tests in this file cover one rule and everything the rule re-keyed: a custom settings payload
// (`com.apple.ManagedClient.preferences`, the managed preferences envelope the Jamf Pro profile
// editor calls "Application & Custom Settings") carries **exactly one** preference domain, so a
// block covering several domains carries one payload per domain — which is what the Jamf Pro editor
// itself produces.
//
// Each count the rule refuses is refused on its own evidence.
//
// More than one is lost on the device, and nothing between Terraform and the device objects to it.
// Jamf stores a payload setting several domains faithfully, the deploy reports SUCCEEDED, the stored
// blueprint round-trips, and every later plan settles. The loss happens on the Mac, which applies
// one of the domains and discards the rest: wire probing on macOS 26.6 on 2026-09-17 found a trio
// stored as Safari, SoftwareUpdate and Terminal applying SoftwareUpdate, and a quad stored as
// Accessibility, Safari, Terminal and dock applying dock — so neither first nor last fits both, and
// com.apple.ManagedClient logged nothing either time. Splitting the same domains one payload per
// domain applied all of them. That is why the check is an error rather than a warning, and why these
// tests exist at all: a green apply delivering two of three domains is the outcome the rule prevents,
// and no assertion available to Terraform would have noticed it.
//
// None at all is lost on the wire, which is the newer half of the rule and the one an acceptance run
// found. Jamf discards an empty dictionary: a payload sent with `PayloadContent: {}` reads back on
// the EU environment, 2026-09-17, carrying only the stamped metadata keys and no PayloadContent at
// all, so the read finds no settings and the apply fails with "Provider produced inconsistent result
// after apply". An earlier version of this file applied that shape as a boundary case and is what
// disproved it. It is now a plan-time refusal alongside the multi-domain cases, and no step here
// applies it.
//
// One payload per domain then made a payload type repeatable within a block, which every keying in
// this resource had assumed it was not. `legacyPayloadIdentity` is the answer — the bare payload type
// for everything else, the type plus its single preference domain for a custom settings payload —
// and the stored `payloadIdentifier` read-merge-write, the duplicate check, the settings mask and the
// discard check all key on it. The identifier half is what most of this file asserts, because it is
// the half that cannot be observed from Terraform (see
// legacy_payload_identifiers_acceptance_test.go, whose helpers these tests reuse) and because
// getting it wrong is not cosmetic: several custom settings payloads keyed on their shared type
// collapse to one map entry, every one of them is written with the same stored identifier, and macOS
// refuses the whole profile and every payload in it — "the PayloadIdentifier is used more than once
// in the profile", ConfigProfilePluginDomain:-107 — retrying forever while the blueprint reports
// DEPLOYED and SUCCEEDED.
//
// Two limits on what a green run here proves, so nobody reads these tests as more than they are.
//
// First, they prove the **provider** no longer emits a multi-domain custom settings payload, and
// that identities, identifiers and state survive the re-keying. They do **not** prove multi-domain
// delivery is broken on a device. That observation needs a Mac, an enrolled blueprint and a look at
// /Library/Managed Preferences, so it is recorded by hand in issue #436 rather than automated here.
// If Apple or Jamf ever makes a multi-domain payload deliver every domain, no test in this file
// fails; #436 is what has to be re-probed.
//
// Second, every device observation behind the rule is **macOS 26.6**. Nothing here speaks to iOS,
// where managed preferences are not delivered this way at all, and nothing here speaks to an earlier
// or later macOS. A regression in the rule is a wire probe, not a test run.

// mcxStoredKey returns the key readLegacyPayloadIdentifiers files a custom settings payload under:
// the payload type and its single preference domain, which is what the provider's own
// legacyPayloadIdentity produces. Restated here for the reason the sibling file restates
// storedPayloadKey — these tests are `package blueprint_test` behind the acceptance build tag and
// cannot reach an unexported function.
func mcxStoredKey(domain string) string {
	return mcxPayloadTypeName + " (" + domain + ")"
}

// The preference domains these tests author beyond the two the sibling file already declares. They
// are throwaway reverse-domain names no real application reads, and no Jamf-, Apple- or
// vendor-owned domain can collide with them, so a run that leaves a profile behind on a shared
// estate changes nothing on a device.
const (
	mcxThirdPreferenceDomain = "com.tf-acc-safe-to-delete.third"
)

// mcxForcedKey is the one key every custom settings payload below forces. Managed preferences are
// passthrough — Apple declares the dictionary under a preference domain as free-form, Jamf stores
// whatever arrives there — so the key need not be a real Apple preference, but it is named and
// shaped like one so a stored profile reads as something an operator could have written.
const mcxForcedKey = "tfAccSafeToDelete"

// mcxPayloadContentHCL renders the HCL object for a custom settings payload's settings, forcing
// mcxForcedKey to value in each named preference domain. Naming no domain renders the empty
// dictionary, which the provider refuses at plan time because Jamf discards it.
//
// The value is a parameter so a test can edit one payload's settings without touching its domain,
// which is the edit that has to leave the payload's identity — and therefore its stored identifier —
// alone.
func mcxPayloadContentHCL(value int, domains ...string) string {
	var b strings.Builder
	b.WriteString("{ PayloadContent = {")
	for i, domain := range domains {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, " %q = { Forced = [{ mcx_preference_settings = { %s = %d } }] }", domain, mcxForcedKey, value)
	}
	b.WriteString(" } }")
	return b.String()
}

// mcxBlockSettings renders the settings expression for a component block's legacy payload, where
// settings is a JSON string.
func mcxBlockSettings(value int, domains ...string) string {
	return "jsonencode(" + mcxPayloadContentHCL(value, domains...) + ")"
}

// mcxBlockPayload is one custom settings payload for a component block, covering the named domains.
// More than one domain is the shape the rule refuses, and a test authors it deliberately.
func mcxBlockPayload(value int, domains ...string) legacyPayloadSpec {
	return legacyPayloadSpec{payloadType: mcxPayloadTypeName, settings: mcxBlockSettings(value, domains...)}
}

// mcxNullDomainPayload is one custom settings payload forcing a real preference domain beside a
// second domain set to null. The platform discards a null-valued key before it stores the payload,
// wire-probed on the EU environment 2026-09-17, so this stores as a single domain and must plan and
// apply as one — the count the provider reports has to agree with the count the platform keeps.
func mcxNullDomainPayload(value int, domain, nullDomain string) legacyPayloadSpec {
	settings := fmt.Sprintf(
		"jsonencode({ PayloadContent = { %q = { Forced = [{ mcx_preference_settings = { %s = %d } }] }, %q = null } })",
		domain, mcxForcedKey, value, nullDomain,
	)
	return legacyPayloadSpec{payloadType: mcxPayloadTypeName, settings: settings}
}

// mcxFlatPayload is the same payload for the deprecated top-level attribute, where settings is an
// object rather than a JSON string.
func mcxFlatPayload(value int, domains ...string) legacyPayloadSpec {
	return legacyPayloadSpec{payloadType: mcxPayloadTypeName, settings: mcxPayloadContentHCL(value, domains...)}
}

// mcxSettingsWithoutPayloadContent is a custom settings payload carrying no preference dictionary at
// all. Apple declares the dictionary required, so this is Apple's finding to report rather than the
// domain count's — a distinction one test below pins.
const mcxSettingsWithoutPayloadContent = `jsonencode({ PayloadDescription = "Acceptance test — safe to delete" })`

// mcxAuthoredServerKeysSettings is a single-domain custom settings payload that also declares both
// keys the service owns, in Apple's casing, which is the spelling Apple's schemas accept and so the
// one an author is most likely to copy from a .mobileconfig. It is assembled as literal JSON rather
// than through jsonencode so a test can assert the bytes in state are the bytes that were authored,
// and assembled from the same constants every other payload here uses so the two cannot drift.
var mcxAuthoredServerKeysSettings = fmt.Sprintf(
	`{"PayloadContent":{%q:{"Forced":[{"mcx_preference_settings":{%q:1}}]}},"PayloadIdentifier":"tf-acc-authored-identifier-safe-to-delete","PayloadUUID":"tf-acc-authored-uuid-safe-to-delete"}`,
	firstPreferenceDomain, mcxForcedKey,
)

// mcxFlatConfig renders a blueprint in the deprecated top-level `legacy_payloads` style, scoped to
// the throwaway smart group every blueprint acceptance test builds. legacyIdentifierConfig cannot
// express it: the flat attribute is a dynamic value whose settings are objects, not JSON strings.
func mcxFlatConfig(groupSuffix, resourceLabel, name, description string, payloads ...legacyPayloadSpec) string {
	var rendered strings.Builder
	for _, payload := range payloads {
		fmt.Fprintf(&rendered, "\t\t\t\t\t{\n\t\t\t\t\t\tpayload_type = %q\n\t\t\t\t\t\tsettings     = %s\n\t\t\t\t\t},\n", payload.payloadType, payload.settings)
	}

	return testBlueprintConfig(smartGroupHCL(groupSuffix), fmt.Sprintf(`
			resource "jamfplatform_blueprints_blueprint" %q {
				name          = %q
				description   = %q
				deployed      = false
				device_groups = [jamfplatform_device_group.scope.id]

				legacy_payloads = [
%s				]
			}
		`, resourceLabel, name, description, rendered.String()))
}

// captureStoredIdentifiersForKeys returns a Check that records the stored identifier of only the
// named payloads, so a later step can assert those survive an edit that removes or renames a
// sibling. captureStoredIdentifiers cannot serve that: it records every payload the blueprint holds,
// and checkStoredIdentifiers then insists every recorded payload is still stored, which a deliberate
// removal fails.
//
// A named payload the service does not hold is an error rather than a skipped entry. Recording
// nothing would leave the next step comparing an empty map, which is the failure an acceptance test
// must never take for a result.
func captureStoredIdentifiersForKeys(t *testing.T, addr string, into map[string]string, keys ...string) resource.TestCheckFunc {
	t.Helper()

	return func(s *terraform.State) error {
		blueprintID, err := blueprintIDFromState(s, addr)
		if err != nil {
			return err
		}

		current, err := readLegacyPayloadIdentifiers(t, blueprintID)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return errors.New("no payload was named, so this check would record nothing to compare against")
		}
		for _, key := range keys {
			identifier, stored := current[key]
			if !stored {
				return fmt.Errorf("the service stores no %s payload, so there is no identifier to carry into the next step: %v", key, current)
			}
			into[key] = identifier
		}
		return nil
	}
}

// checkMCXIdentifierIsFresh returns a Check asserting the named payload is stored and carries an
// identifier none of the recorded ones holds.
//
// It is what checkStoredIdentifiers cannot say about a payload whose identity has just changed. A
// renamed preference domain is a new identity, so the provider sends no stored identifier for it and
// the service mints one; nothing else asserts the minted value is genuinely new rather than the one
// the old identity was holding, which is the outcome a keying that ignored the domain would produce.
func checkMCXIdentifierIsFresh(t *testing.T, addr, key string, notAnyOf map[string]string) resource.TestCheckFunc {
	t.Helper()

	return func(s *terraform.State) error {
		if len(notAnyOf) == 0 {
			return errors.New("no earlier identifier was recorded, so a freshly minted one here would prove nothing")
		}

		blueprintID, err := blueprintIDFromState(s, addr)
		if err != nil {
			return err
		}
		current, err := readLegacyPayloadIdentifiers(t, blueprintID)
		if err != nil {
			return err
		}

		got, stored := current[key]
		if !stored {
			return fmt.Errorf("the service stores no %s payload, so the identifier it should have minted is missing: %v", key, current)
		}
		for recordedKey, recorded := range notAnyOf {
			if strings.EqualFold(got, recorded) {
				return fmt.Errorf("payload %s carries %s, which is the identifier recorded for %s, so its new identity was not treated as new", key, got, recordedKey)
			}
		}
		return nil
	}
}

// checkMCXForcedValue returns a Check asserting a payload's settings in state force mcxForcedKey to
// want in the named preference domain. It is how an edit to a payload's settings is proved to have
// landed, which a test asserting the identifier survived that edit has to show separately —
// otherwise a provider that dropped the edit entirely would pass by leaving everything alone.
//
// The whole path is walked explicitly so a failure names the level that broke rather than reporting
// a missing value.
func checkMCXForcedValue(addr, attribute, domain string, want float64) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(addr, attribute, func(value string) error {
		var settings map[string]any
		if err := json.Unmarshal([]byte(value), &settings); err != nil {
			return fmt.Errorf("%s holds %q, which is not a JSON object: %w", attribute, value, err)
		}

		content, ok := settings["PayloadContent"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s carries no PayloadContent dictionary: %s", attribute, value)
		}
		preferences, ok := content[domain].(map[string]any)
		if !ok {
			return fmt.Errorf("%s does not set the %s preference domain: %s", attribute, domain, value)
		}
		forced, ok := preferences["Forced"].([]any)
		if !ok || len(forced) == 0 {
			return fmt.Errorf("%s sets %s with no Forced entry: %s", attribute, domain, value)
		}
		entry, ok := forced[0].(map[string]any)
		if !ok {
			return fmt.Errorf("%s sets %s with a Forced entry that is not an object: %s", attribute, domain, value)
		}
		mcx, ok := entry["mcx_preference_settings"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s sets %s with no mcx_preference_settings dictionary: %s", attribute, domain, value)
		}
		got, ok := mcx[mcxForcedKey].(float64)
		if !ok {
			return fmt.Errorf("%s does not force %s in %s: %s", attribute, mcxForcedKey, domain, value)
		}
		if got != want {
			return fmt.Errorf("%s forces %s = %v in %s, want %v", attribute, mcxForcedKey, got, domain, want)
		}
		return nil
	})
}

// TestAccResource_Blueprint_LegacyPayloads_MCXSeveralDomainsRefusedAtPlan is the rule itself, on
// both carriers, and the boundary that keeps it out of Apple's way. Nothing is applied: each step
// plans and must be refused, which is the point — the refusal has to arrive before a write, because
// once the payload is stored nothing downstream can tell it is wrong. Jamf accepts it, the deploy
// reports SUCCEEDED, the blueprint round-trips and every later plan settles, while the Mac silently
// applies one domain of the several.
//
// The third step is the empty dictionary, which Jamf discards, so the payload cannot be stored and
// an apply of it fails with an inconsistent result. It is refused with its own summary rather than
// the multi-domain one, and the regex matches the word only that summary's detail carries.
//
// The fourth step is the one that is easy to lose. A payload carrying no preference dictionary at all
// is Apple's missing-required-key finding, which names the key and offers the per-block escape
// hatch; reporting the domain count there instead would replace a precise diagnostic with a vaguer
// one. Terraform wraps a diagnostic at about eighty columns, so each expectation matches a short
// phrase of the summary rather than a sentence that could straddle a line break. That also means
// this step asserts Apple's finding is *present* rather than that a domain-count finding is
// absent, which Go's regexp cannot express — the exclusivity is pinned by the unit test
// TestLegacyPayloadValidator_AbsentPreferenceDictionaryStaysApplesFinding.
func TestAccResource_Blueprint_LegacyPayloads_MCXSeveralDomainsRefusedAtPlan(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcx-refused-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				// A component block's typed legacy payload list.
				Config: legacyIdentifierConfig("mcxrefused", "mcx_refused", name, "Acceptance test — safe to delete", legacyPayloadBlock{
					name:     "Custom Settings",
					payloads: []legacyPayloadSpec{mcxBlockPayload(1, firstPreferenceDomain, secondPreferenceDomain)},
				}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`more than one preference`),
			},
			{
				// The deprecated top-level attribute, where the same payload arrives as an object
				// and the finding has no per-element path to land on.
				Config: mcxFlatConfig("mcxrefused", "mcx_refused", name, "Acceptance test — safe to delete",
					mcxFlatPayload(1, firstPreferenceDomain, secondPreferenceDomain)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`more than one preference`),
			},
			{
				// An empty preference dictionary, which Jamf discards.
				Config: legacyIdentifierConfig("mcxrefused", "mcx_refused", name, "Acceptance test — safe to delete", legacyPayloadBlock{
					name:     "Custom Settings",
					payloads: []legacyPayloadSpec{mcxBlockPayload(1)},
				}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`sets no preference`),
			},
			{
				// No preference dictionary at all: Apple's finding, not this one.
				Config: legacyIdentifierConfig("mcxrefused", "mcx_refused", name, "Acceptance test — safe to delete", legacyPayloadBlock{
					name:     "Custom Settings",
					payloads: []legacyPayloadSpec{{payloadType: mcxPayloadTypeName, settings: mcxSettingsWithoutPayloadContent}},
				}),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`missing a required setting`),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_MCXDuplicatePayloadsRefused covers the other half of the
// re-keying: what a component block may not declare twice is now a payload *identity*, not a payload
// type. A repeated preference domain is refused, and a repeated non-custom-settings type still is.
//
// Both refusals arrive at apply rather than at plan, and deliberately so: the check lives in
// appendLegacyConfigProfile, which assembles the request body, so it sees the payloads exactly as
// they will be written. Neither step therefore uses PlanOnly, and each expects the error from the
// create. The scoped smart group is created by both steps and destroyed by the framework.
//
// The second step is the regression guard on the identity change. Keying on identity widened what a
// block may repeat, and the easy mistake is to widen it for everything: two accessibility payloads
// in one block are still one profile with one payload type twice, which is exactly the shape the
// original check existed to stop. They carry different settings here so the block is rejected for
// the repeated type rather than for being a verbatim duplicate.
func TestAccResource_Blueprint_LegacyPayloads_MCXDuplicatePayloadsRefused(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcx-dupes-" + suffix

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				// Two custom settings payloads, one preference domain between them.
				Config: legacyIdentifierConfig("mcxdupes", "mcx_dupes", name, "Acceptance test — safe to delete", legacyPayloadBlock{
					name: "Custom Settings",
					payloads: []legacyPayloadSpec{
						mcxBlockPayload(1, firstPreferenceDomain),
						mcxBlockPayload(2, firstPreferenceDomain),
					},
				}),
				ExpectError: regexp.MustCompile(`Duplicate preference domain`),
			},
			{
				// A repeated ordinary payload type is still refused.
				Config: legacyIdentifierConfig("mcxdupes", "mcx_dupes", name, "Acceptance test — safe to delete", legacyPayloadBlock{
					name: "Legacy Payloads",
					payloads: []legacyPayloadSpec{
						{payloadType: accessibilityPayloadType, settings: accessibilitySettings},
						{payloadType: accessibilityPayloadType, settings: accessibilitySettingsEdited},
					},
				}),
				ExpectError: regexp.MustCompile(`Duplicate payload_type`),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_MCXDomainCountBoundariesApply pins the one count the rule
// must let through, which matters more than it reads: the refusal above is worthless if it also
// refuses the shape its own diagnostic tells the author to write.
//
// Step 1 is one payload, one domain — what the fix asks for, and the only authoring the Jamf Pro
// profile editor produces. It also proves the state contract on that shape: the service stamps
// `payloadIdentifier` and `payloadUUID` onto every payload it stores and the provider masks both
// from both sides of the diff, so the wire must carry them and `settings` must not, in any casing.
// Step 2 re-applies the same configuration and the plan must be empty, which is what a mask applied
// to only one side would fail.
//
// An earlier version of this test applied the empty dictionary here as the other boundary, on the
// reading that Apple declares the dictionary required and `{}` satisfies it. The run failed with
// "Provider produced inconsistent result after apply": Jamf discards an empty dictionary, so state
// cannot hold what the author wrote. That case now sits in
// TestAccResource_Blueprint_LegacyPayloads_MCXSeveralDomainsRefusedAtPlan, where nothing is applied.
//
// Steps 3 and 4 are the boundary a null domain sits on, and they are here rather than in a unit test
// because only a tenant can prove the half that matters: the platform discards a null-valued key
// before it stores the payload, so a payload naming two domains where one is null stores as one and
// must plan, apply and then re-plan empty. A provider counting the null key would refuse this
// configuration at plan while naming a domain the platform never stores, and one counting a lone
// null domain as a domain would send a payload that reads back with no PayloadContent at all and
// fail the apply with "Provider produced inconsistent result after apply".
func TestAccResource_Blueprint_LegacyPayloads_MCXDomainCountBoundariesApply(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcx-bounds-" + suffix
	addr := "jamfplatform_blueprints_blueprint.mcx_bounds"

	config := func(payload legacyPayloadSpec) string {
		return legacyIdentifierConfig("mcxbounds", "mcx_bounds", name, "Acceptance test — safe to delete", legacyPayloadBlock{
			name:     "Custom Settings",
			payloads: []legacyPayloadSpec{payload},
		})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(mcxBlockPayload(1, firstPreferenceDomain)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.#", "1"),
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.0.payload_type", mcxPayloadTypeName),
					checkMCXForcedValue(addr, "component_blocks.0.legacy_payloads.0.settings", firstPreferenceDomain, 1),
					checkSettingsCarryNoServerOwnedKeys(addr, "component_blocks.0.legacy_payloads.0.settings"),
					checkStoredIdentifiersAreDistinct(t, addr, 1),
				),
			},
			{
				Config: config(mcxBlockPayload(1, firstPreferenceDomain)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: checkStoredIdentifiersAreDistinct(t, addr, 1),
			},
			{
				Config: config(mcxNullDomainPayload(1, firstPreferenceDomain, secondPreferenceDomain)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.#", "1"),
					checkMCXForcedValue(addr, "component_blocks.0.legacy_payloads.0.settings", firstPreferenceDomain, 1),
					checkStoredIdentifiersAreDistinct(t, addr, 1),
				),
			},
			{
				Config: config(mcxNullDomainPayload(1, firstPreferenceDomain, secondPreferenceDomain)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_MCXDomainsKeepSeparateIdentifiers is the core of the
// re-keying. Two custom settings payloads in one block, one preference domain each, and the service
// must hold a distinct identifier for each of them across edits the author did not aim at them.
//
// Step 1's distinctness assertion is the load-bearing one, and it needs its own helper for a reason
// worth stating: an assertion that compares recorded identifiers against current ones agrees with
// itself if both payloads shared a single identifier from the first apply, which is precisely the
// collapse a type-keyed provider produces. checkStoredIdentifiersAreDistinct counts the stored
// payloads and rejects a shared value, so it fails on the collapse where the comparison cannot.
//
// Step 2 changes the description and nothing else: `payloadContent` is an array and a merge-patch
// replaces an array wholesale, so a write that omits a stored identifier re-identifies every payload
// in the blueprint. Step 3 edits one payload's forced value, which leaves its preference domain and
// therefore its identity alone — so both identifiers must still come back byte-identical, and the
// edit must be shown to have landed, since a provider that silently dropped it would pass an
// identifier assertion by changing nothing at all.
//
// The state contract is asserted here as well as on the single-domain shape, because two payloads is
// where a mask keyed on payload type leaks: both payloads would mask against one prior settings
// object, and the one that did not own it would keep the service's stamped keys.
func TestAccResource_Blueprint_LegacyPayloads_MCXDomainsKeepSeparateIdentifiers(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcx-domains-" + suffix
	addr := "jamfplatform_blueprints_blueprint.mcx_domains"
	recorded := make(map[string]string)

	config := func(description string, firstValue int) string {
		return legacyIdentifierConfig("mcxdomains", "mcx_domains", name, description, legacyPayloadBlock{
			name: "Custom Settings",
			payloads: []legacyPayloadSpec{
				mcxBlockPayload(firstValue, firstPreferenceDomain),
				mcxBlockPayload(2, secondPreferenceDomain),
			},
		})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config("Acceptance test — safe to delete", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.#", "2"),
					checkMCXForcedValue(addr, "component_blocks.0.legacy_payloads.0.settings", firstPreferenceDomain, 1),
					checkMCXForcedValue(addr, "component_blocks.0.legacy_payloads.1.settings", secondPreferenceDomain, 2),
					checkSettingsCarryNoServerOwnedKeys(addr, "component_blocks.0.legacy_payloads.0.settings"),
					checkSettingsCarryNoServerOwnedKeys(addr, "component_blocks.0.legacy_payloads.1.settings"),
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: config("Acceptance test — safe to delete, description edited", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "description", "Acceptance test — safe to delete, description edited"),
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					checkStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: config("Acceptance test — safe to delete, description edited", 9),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkMCXForcedValue(addr, "component_blocks.0.legacy_payloads.0.settings", firstPreferenceDomain, 9),
					checkMCXForcedValue(addr, "component_blocks.0.legacy_payloads.1.settings", secondPreferenceDomain, 2),
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					checkStoredIdentifiers(t, addr, recorded),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_MCXDomainsAddedRemovedAndReordered walks the edits an
// operator actually makes to a block of custom settings payloads, in one blueprint, because each
// step only means anything on top of the last.
//
// Adding a domain: the added identity has no stored identifier and is the service's to mint, while
// its siblings must be written back — and the whole `payloadContent` array goes out on that write,
// so a path that sent the new payload without the old ones' identifiers would re-identify the lot.
// Distinctness is asserted alongside presence, because minting is only correct if it mints something
// new.
//
// Removing a domain: the survivors keep what they had. This is the step that needs
// captureStoredIdentifiersForKeys rather than the file-wide capture, since checkStoredIdentifiers
// insists every recorded payload is still stored and a deliberate removal would fail it.
//
// Reordering the two remaining payloads in the configuration is the whole point of identity keying,
// and the one step that fails under either weaker scheme. Keying on payload type collapses both
// payloads to a single entry, so nothing can follow a domain anywhere; keying on position hands each
// payload the identifier of the one that used to sit at its index, transplanting both. Identity keys
// on the preference domain, so each identifier follows its domain and the reorder is a no-op to the
// service — a transplant being the worse of the two failures, since Apple keys an installed payload
// on its identifier and two payloads would then be fighting over one.
func TestAccResource_Blueprint_LegacyPayloads_MCXDomainsAddedRemovedAndReordered(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcx-shuffle-" + suffix
	addr := "jamfplatform_blueprints_blueprint.mcx_shuffle"
	pair := make(map[string]string)
	survivors := make(map[string]string)

	config := func(payloads ...legacyPayloadSpec) string {
		return legacyIdentifierConfig("mcxshuffle", "mcx_shuffle", name, "Acceptance test — safe to delete", legacyPayloadBlock{
			name:     "Custom Settings",
			payloads: payloads,
		})
	}
	first := mcxBlockPayload(1, firstPreferenceDomain)
	second := mcxBlockPayload(2, secondPreferenceDomain)
	third := mcxBlockPayload(3, mcxThirdPreferenceDomain)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(first, second),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.#", "2"),
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					captureStoredIdentifiers(t, addr, pair),
				),
			},
			{
				// A third domain. The two already there hold; the new one is minted and distinct.
				Config: config(first, second, third),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.#", "3"),
					checkStoredIdentifiers(t, addr, pair, mcxStoredKey(mcxThirdPreferenceDomain)),
					checkStoredIdentifiersAreDistinct(t, addr, 3),
					captureStoredIdentifiersForKeys(t, addr, survivors,
						mcxStoredKey(firstPreferenceDomain), mcxStoredKey(mcxThirdPreferenceDomain)),
				),
			},
			{
				// The middle domain removed. The survivors keep their own identifiers.
				Config: config(first, third),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.#", "2"),
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					checkStoredIdentifiers(t, addr, survivors),
				),
			},
			{
				// Reordered in the configuration. Each identifier follows its domain, not its index.
				Config: config(third, first),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkMCXForcedValue(addr, "component_blocks.0.legacy_payloads.0.settings", mcxThirdPreferenceDomain, 3),
					checkMCXForcedValue(addr, "component_blocks.0.legacy_payloads.1.settings", firstPreferenceDomain, 1),
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					checkStoredIdentifiers(t, addr, survivors),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_MCXRenamedDomainMintsWhileSiblingHolds covers the edit
// that is a rename to the author and a different payload to everything downstream. A custom settings
// payload's identity is its type plus its preference domain, so changing the domain retires one
// identity and introduces another: nothing is stored for the new one, the service mints for it, and
// the sibling that was not touched keeps what it had.
//
// Asserting the renamed payload merely *has* an identifier is not enough, which is why this test
// carries its own check. A scheme that ignored the domain would hand the renamed payload the
// identifier the old domain was holding — present, distinct from its sibling's, and wrong, because
// the payload delivering com.tf-acc-safe-to-delete.third would be identified as the one that used to
// deliver .first. checkMCXIdentifierIsFresh is what rejects that.
func TestAccResource_Blueprint_LegacyPayloads_MCXRenamedDomainMintsWhileSiblingHolds(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcx-rename-" + suffix
	addr := "jamfplatform_blueprints_blueprint.mcx_rename"
	pair := make(map[string]string)
	sibling := make(map[string]string)

	config := func(renamed string) string {
		return legacyIdentifierConfig("mcxrename", "mcx_rename", name, "Acceptance test — safe to delete", legacyPayloadBlock{
			name: "Custom Settings",
			payloads: []legacyPayloadSpec{
				mcxBlockPayload(1, renamed),
				mcxBlockPayload(2, secondPreferenceDomain),
			},
		})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(firstPreferenceDomain),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					captureStoredIdentifiers(t, addr, pair),
					captureStoredIdentifiersForKeys(t, addr, sibling, mcxStoredKey(secondPreferenceDomain)),
				),
			},
			{
				Config: config(mcxThirdPreferenceDomain),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkMCXForcedValue(addr, "component_blocks.0.legacy_payloads.0.settings", mcxThirdPreferenceDomain, 1),
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					checkStoredIdentifiers(t, addr, sibling),
					checkMCXIdentifierIsFresh(t, addr, mcxStoredKey(mcxThirdPreferenceDomain), pair),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_MCXAlongsideTypedPayloads mixes the two kinds of identity
// in one block, which is the authoring a real profile has: a couple of ordinary payload types and a
// custom settings payload per preference domain.
//
// The identities have to stay independent across both kinds. An ordinary payload keys on its bare
// type and a custom settings payload on its type plus a bracketed domain, and a payload type never
// contains " (", so a composite key cannot collide with a bare one — but that is an argument, and
// this is the only place it is measured. Four payloads, four distinct identifiers, all four
// surviving an edit aimed at the blueprint's description.
//
// The counted assertion matters as much as the comparison. Four authored payloads collapsing to
// three stored identities is the failure a per-type keying produces, and it is invisible to a
// comparison that only checks what it recorded against what is there now.
func TestAccResource_Blueprint_LegacyPayloads_MCXAlongsideTypedPayloads(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcx-mixed-" + suffix
	addr := "jamfplatform_blueprints_blueprint.mcx_mixed"
	recorded := make(map[string]string)

	config := func(description string) string {
		return legacyIdentifierConfig("mcxmixed", "mcx_mixed", name, description, legacyPayloadBlock{
			name: "Custom Settings and more",
			payloads: []legacyPayloadSpec{
				{payloadType: accessibilityPayloadType, settings: accessibilitySettings},
				mcxBlockPayload(1, firstPreferenceDomain),
				{payloadType: domainsPayloadType, settings: domainsSettings},
				mcxBlockPayload(2, secondPreferenceDomain),
			},
		})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config("Acceptance test — safe to delete"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.#", "4"),
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.0.payload_type", accessibilityPayloadType),
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.2.payload_type", domainsPayloadType),
					checkMCXForcedValue(addr, "component_blocks.0.legacy_payloads.1.settings", firstPreferenceDomain, 1),
					checkMCXForcedValue(addr, "component_blocks.0.legacy_payloads.3.settings", secondPreferenceDomain, 2),
					checkStoredIdentifiersAreDistinct(t, addr, 4),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: config("Acceptance test — safe to delete, description edited"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "description", "Acceptance test — safe to delete, description edited"),
					checkStoredIdentifiersAreDistinct(t, addr, 4),
					checkStoredIdentifiers(t, addr, recorded),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_MCXSharedIdentifierIsReissued covers the one duplicate a
// device refuses, reached the way custom settings payloads newly make reachable: two payloads of the
// **same type** in a single step carrying one identifier. macOS rejects the whole profile and every
// payload in it — "the PayloadIdentifier is used more than once in the profile",
// ConfigProfilePluginDomain:-107 — and retries indefinitely while the blueprint reports DEPLOYED and
// SUCCEEDED.
//
// The provider never writes that shape itself, but it would propagate one, because stored
// identifiers are read off the wire: a blueprint edited outside Terraform into holding one
// identifier on two custom settings payloads would have both identities map to it and be written
// back unusable on every apply. dropDuplicateIdentifiers is what stops that, by dropping any
// identifier two identities in a step share so the service reissues distinct ones. So the duplicate
// is planted out of band here, exactly as an admin or a raw_component author could, and the update
// that follows must reissue rather than preserve.
//
// Two things the step depends on. The plant is verified before the apply, because the service
// refusing to store the duplicate would leave this test proving nothing. And the configuration
// changes its description, because both identifiers are masked from state: leave the configuration
// untouched and the plan is empty, nothing is written, and the guard is never reached.
func TestAccResource_Blueprint_LegacyPayloads_MCXSharedIdentifierIsReissued(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcx-shared-" + suffix
	addr := "jamfplatform_blueprints_blueprint.mcx_shared"

	var blueprintID string
	const planted = "BBBB1111-2222-4333-8444-555566667777"

	config := func(description string) string {
		return legacyIdentifierConfig("mcxshared", "mcx_shared", name, description, legacyPayloadBlock{
			name: "Custom Settings",
			payloads: []legacyPayloadSpec{
				mcxBlockPayload(1, firstPreferenceDomain),
				mcxBlockPayload(2, secondPreferenceDomain),
			},
		})
	}

	// plantSharedIdentifierOutOfBand rewrites both stored custom settings payloads to one identifier,
	// the shape an edit outside Terraform can leave and the provider must not carry forward.
	plantSharedIdentifierOutOfBand := func() {
		client := bpSDK.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		blueprint, err := client.GetBlueprint(ctx, blueprintID)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}

		steps := blueprint.Steps
		stamped := 0
		for stepIndex, step := range steps {
			for componentIndex, component := range step.Components {
				if component.Identifier != legacyConfigProfileComponent {
					continue
				}

				var configuration map[string]any
				if err := json.Unmarshal(component.Configuration, &configuration); err != nil {
					t.Fatalf("out-of-band decode: %v", err)
				}
				payloads, ok := configuration["payloadContent"].([]any)
				if !ok || len(payloads) < 2 {
					t.Fatalf("out-of-band: expected at least two stored payloads, got %v", configuration["payloadContent"])
				}
				for _, item := range payloads {
					payload, ok := item.(map[string]any)
					if !ok {
						t.Fatal("out-of-band: a payloadContent entry is not an object")
					}
					payload["payloadIdentifier"] = planted
					payload["payloadUUID"] = planted
					stamped++
				}

				encoded, err := json.Marshal(configuration)
				if err != nil {
					t.Fatalf("out-of-band encode: %v", err)
				}
				steps[stepIndex].Components[componentIndex].Configuration = encoded
			}
		}
		if stamped < 2 {
			t.Fatalf("out-of-band: stamped the shared identifier on %d payloads, so no duplicate exists to reissue", stamped)
		}

		if err := client.UpdateBlueprint(ctx, blueprintID, &bpSDK.UpdateBlueprintRequest{Steps: &steps}); err != nil {
			t.Fatalf("out-of-band duplicate plant: %v", err)
		}

		current, err := readLegacyPayloadIdentifiers(t, blueprintID)
		if err != nil {
			t.Fatalf("reading back the planted blueprint: %v", err)
		}
		for _, domain := range []string{firstPreferenceDomain, secondPreferenceDomain} {
			key := mcxStoredKey(domain)
			if got := current[key]; !strings.EqualFold(got, planted) {
				t.Fatalf("payload %s stores %s after the plant, want the shared %s; the service refused the duplicate and this test can prove nothing", key, got, planted)
			}
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config("Acceptance test — safe to delete"),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					captureBlueprintID(addr, &blueprintID),
				),
			},
			{
				PreConfig: plantSharedIdentifierOutOfBand,
				Config:    config("Acceptance test — safe to delete, description edited"),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					func(s *terraform.State) error {
						current, err := readLegacyPayloadIdentifiers(t, blueprintID)
						if err != nil {
							return err
						}
						for _, domain := range []string{firstPreferenceDomain, secondPreferenceDomain} {
							key := mcxStoredKey(domain)
							got, stored := current[key]
							if !stored {
								return fmt.Errorf("the service no longer stores a %s payload: %v", key, current)
							}
							if strings.EqualFold(got, planted) {
								return fmt.Errorf(
									"payload %s kept the planted identifier %s, so the provider wrote back a profile a device refuses whole",
									key, planted,
								)
							}
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_MCXAuthoredServerKeysRoundTrip covers an author who
// copied a payload out of a .mobileconfig and left Apple's `PayloadIdentifier` and `PayloadUUID` on
// it. Both are the service's rather than the author's, so both are dropped from the write and the
// service assigns its own — and the settings string in state must still be the bytes that were
// authored, byte for byte.
//
// Both sides of the mask is the load-bearing part, and this is the shape that proves it. A payload's
// `settings` is Optional rather than Computed, so the planned value is the string the author wrote:
// dropping the key from the server side alone is as much a mismatch as leaving a different value on
// it, which replaces the authored string in state and fails the apply outright with "Provider
// produced inconsistent result after apply". Pruning both sides makes an authored copy of a
// Jamf-owned key a no-op, which is why step 2 asserts an empty plan as well.
//
// The settings are written as literal JSON rather than through jsonencode so the assertion can be a
// byte comparison. Nothing about the payload's identity changes: it still declares one preference
// domain, and it is still located by that domain.
func TestAccResource_Blueprint_LegacyPayloads_MCXAuthoredServerKeysRoundTrip(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcx-authored-" + suffix
	addr := "jamfplatform_blueprints_blueprint.mcx_authored"
	recorded := make(map[string]string)

	config := legacyIdentifierConfig("mcxauthored", "mcx_authored", name, "Acceptance test — safe to delete", legacyPayloadBlock{
		name: "Custom Settings",
		payloads: []legacyPayloadSpec{
			{payloadType: mcxPayloadTypeName, settings: fmt.Sprintf("%q", mcxAuthoredServerKeysSettings)},
			mcxBlockPayload(2, secondPreferenceDomain),
		},
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.#", "2"),
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.0.settings", mcxAuthoredServerKeysSettings),
					checkSettingsCarryNoServerOwnedKeys(addr, "component_blocks.0.legacy_payloads.1.settings"),
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.0.settings", mcxAuthoredServerKeysSettings),
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					checkStoredIdentifiers(t, addr, recorded),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_MCXFlatCarrierKeepsDomainsApart covers the deprecated
// top-level `legacy_payloads` attribute, which reaches every one of these code paths by a different
// route: its settings arrive as objects rather than JSON strings, its prior values are read out of a
// types.Dynamic, and it resolves its stored identifiers by asking for a single unnamed block and
// taking step 0 positionally. Each of those keys on the same payload identity, so each could have
// been left behind by the re-keying independently of the block carrier.
//
// Deprecated is not the same as unused. An operator who has not migrated to `component_blocks` sets
// custom settings the same way, hits the same device behaviour and has the same claim on a stable
// payload identity — and would have no way to notice a collapse, because both the identifier and the
// collapse are invisible from Terraform.
//
// Step 2 re-applies the same configuration for the empty plan, which is the flat carrier's half of
// the state contract: the mask, the semantic comparison and the per-identity pairing of prior
// settings all have to agree, and two payloads of one type is where pairing by type stops working.
// Step 3 then edits the description so the identifiers face an update they were not aimed at.
func TestAccResource_Blueprint_LegacyPayloads_MCXFlatCarrierKeepsDomainsApart(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-mcx-flat-" + suffix
	addr := "jamfplatform_blueprints_blueprint.mcx_flat"
	recorded := make(map[string]string)

	config := func(description string) string {
		return mcxFlatConfig("mcxflat", "mcx_flat", name, description,
			mcxFlatPayload(1, firstPreferenceDomain),
			mcxFlatPayload(2, secondPreferenceDomain))
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config("Acceptance test — safe to delete"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckNoResourceAttr(addr, "component_blocks.#"),
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: config("Acceptance test — safe to delete"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					checkStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: config("Acceptance test — safe to delete, description edited"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "description", "Acceptance test — safe to delete, description edited"),
					checkStoredIdentifiersAreDistinct(t, addr, 2),
					checkStoredIdentifiers(t, addr, recorded),
				),
			},
		},
	})
}
