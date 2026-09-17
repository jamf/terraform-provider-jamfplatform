// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

//go:build acceptance

package blueprint_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	bpSDK "github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
	"github.com/jamf/terraform-provider-jamfplatform/internal/testhelpers"
)

// The tests in this file cover one property, and it is the only property that cannot be observed
// from Terraform: the `payloadIdentifier` the blueprints service holds for each stored legacy
// configuration profile payload stays put across updates the author did not aim at it.
//
// The field is the service's. It is absent from the Blueprints API specification, which requires
// only `payloadType` in a `payloadContent` entry, and wire probing on 2026-09-16 established that a
// payload sent without one is accepted and stamped with a fresh uppercase UUID **on every write** —
// `payloadContent` is an array and a merge-patch replaces an array wholesale, so omitting the key
// re-identifies every payload in the blueprint whenever any part of it changed, 4 of 4 payloads.
// A payload sent *with* an identifier keeps it verbatim, and `payloadUUID` is forced to equal
// `payloadIdentifier` either way. The provider therefore reads the blueprint before an update and
// writes each stored identifier back, which is what Jamf's own web app does.
//
// None of that is visible in state: both keys are masked from both sides of the diff,
// case-insensitively, so every assertion below reads the blueprint back through the SDK. Unit
// coverage in legacy_payload_identifiers_test.go pins the resolution logic against a fabricated
// blueprint; only these tests pin it against the service that owns the field, and only these would
// notice the day Jamf changes its mind.

// legacyConfigProfileComponent is the blueprint component every legacy payload in a block folds
// into. Restated here rather than imported because these tests are `package blueprint_test` behind
// the acceptance build tag and cannot reach the provider's own unexported constant.
const legacyConfigProfileComponent = "com.jamf.ddm-configuration-profile"

// The payload types these tests author. Each is declared by the embedded Apple schema table, so
// plan-time validation passes, each was probed live against the blueprints service, and each carries
// a value of a different JSON kind — integer, array of strings, boolean — so a test using two of
// them cannot pass on a coincidence of shape. All three are macOS payloads; an iOS-only type such
// as com.apple.app.lock is refused by the service.
const (
	accessibilityPayloadType = "com.apple.universalaccess"
	domainsPayloadType       = "com.apple.domains"
	fileProviderPayloadType  = "com.apple.fileproviderd"
	mcxPayloadTypeName       = "com.apple.ManagedClient.preferences"
)

// The preference domains the custom settings tests author, one payload each. They are throwaway
// reverse-domain names no real application reads, so a run that leaves a profile behind changes
// nothing on a device.
const (
	firstPreferenceDomain  = "com.tf-acc-safe-to-delete.first"
	secondPreferenceDomain = "com.tf-acc-safe-to-delete.second"
)

// The settings each payload type is authored with. They are HCL expressions rather than JSON so the
// configurations below read as something an operator would write.
const (
	accessibilitySettings        = `jsonencode({ closeViewFarPoint = 5 })`
	accessibilitySettingsEdited  = `jsonencode({ closeViewFarPoint = 9 })`
	domainsSettings              = `jsonencode({ EmailDomains = ["tf-acc-safe-to-delete.example"] })`
	fileProviderSettings         = `jsonencode({ ManagementAllowsRemoteSyncing = true })`
	flatAccessibilitySettingsHCL = `{ closeViewFarPoint = 5 }`
	flatDomainsSettingsHCL       = `{ EmailDomains = ["tf-acc-safe-to-delete.example"] }`
)

// legacyPayloadSpec is one authored legacy payload: its Apple payload type and the HCL expression
// for its settings.
type legacyPayloadSpec struct {
	payloadType string
	settings    string
}

// legacyPayloadBlock is one component block carrying nothing but legacy payloads, which is all
// these tests need — the identifier lives on the payload, so any other component would only add
// noise to the read-back.
type legacyPayloadBlock struct {
	// name is the block's name. Empty renders no `name` attribute at all, which is how the unnamed
	// authoring style is expressed: the attribute is optional, and a block without one has nothing
	// but its position for the provider to match it by.
	name     string
	payloads []legacyPayloadSpec
}

// legacyIdentifierConfig renders a blueprint in the current component_blocks authoring style,
// scoped to the throwaway smart group every blueprint acceptance test builds.
func legacyIdentifierConfig(groupSuffix, resourceLabel, name, description string, blocks ...legacyPayloadBlock) string {
	var rendered strings.Builder
	for _, block := range blocks {
		rendered.WriteString("\t\t\t\t{\n")
		if block.name != "" {
			fmt.Fprintf(&rendered, "\t\t\t\t\tname = %q\n", block.name)
		}
		rendered.WriteString("\t\t\t\t\tlegacy_payloads = [\n")
		for _, payload := range block.payloads {
			fmt.Fprintf(&rendered, "\t\t\t\t\t\t{\n\t\t\t\t\t\t\tpayload_type = %q\n\t\t\t\t\t\t\tsettings     = %s\n\t\t\t\t\t\t},\n", payload.payloadType, payload.settings)
		}
		rendered.WriteString("\t\t\t\t\t]\n\t\t\t\t},\n")
	}

	return testBlueprintConfig(smartGroupHCL(groupSuffix), fmt.Sprintf(`
			resource "jamfplatform_blueprints_blueprint" %q {
				name          = %q
				description   = %q
				deployed      = false
				device_groups = [jamfplatform_device_group.scope.id]

				component_blocks = [
%s			]
			}
		`, resourceLabel, name, description, rendered.String()))
}

// storedPayloadKey returns the key one stored legacy payload is followed by, which is the payload
// type for most payloads and the payload type plus its preference domain for a custom settings
// payload. It restates the provider's own legacyPayloadIdentity for the same reason
// legacyConfigProfileComponent restates a constant: these tests are `package blueprint_test` behind
// the acceptance build tag and cannot reach an unexported function.
//
// The domain has to be part of the key. A block may carry a custom settings payload per preference
// domain, because a Mac applies one domain of a payload that sets several and discards the rest, so
// keying by payload type alone would collapse every one of them onto a single entry — and a
// collapsed map is one an assertion about "the identifier did not change" passes vacuously.
func storedPayloadKey(payload map[string]any) string {
	payloadType, _ := payload["payloadType"].(string)
	if payloadType != mcxPayloadTypeName {
		return payloadType
	}

	domains, ok := payload["PayloadContent"].(map[string]any)
	if !ok || len(domains) != 1 {
		return payloadType
	}
	for domain := range domains {
		return payloadType + " (" + domain + ")"
	}
	return payloadType
}

// readLegacyPayloadIdentifiers reads a blueprint back through the SDK and returns the
// `payloadIdentifier` the service holds for each of its legacy payloads, keyed by storedPayloadKey.
// The key is unique across the whole blueprint here — a payload is unique within a block, and no
// test below gives two blocks the same one — which is what lets an identifier be followed through a
// reorder or a rename, where position and block name both move.
//
// Reading the wire is the only way to assert any of this: neither `payloadIdentifier` nor
// `payloadUUID` ever reaches Terraform state.
//
// Every absence is an error rather than a missing map entry. A blueprint storing no legacy payload,
// or a payload the service has not stamped, would otherwise let a test that asserts "the identifier
// did not change" pass by having nothing to compare — the failure an acceptance test must never
// take for a result. The `payloadUUID` equality rides along on the same pass: the provider masks
// that key on the premise that the service overwrites it with the identifier on every write, and a
// divergence is that premise failing rather than any one test's business, so it is reported at the
// point it is seen.
func readLegacyPayloadIdentifiers(t *testing.T, blueprintID string) (map[string]string, error) {
	t.Helper()

	if blueprintID == "" {
		return nil, errors.New("no blueprint id was captured, so there is nothing to read the stored identifiers from")
	}

	client := bpSDK.New(testhelpers.NewAcceptanceClient(t))
	blueprint, err := client.GetBlueprint(context.Background(), blueprintID)
	if err != nil {
		return nil, fmt.Errorf("reading blueprint %s: %w", blueprintID, err)
	}

	identifiers := make(map[string]string)
	for _, step := range blueprint.Steps {
		for _, component := range step.Components {
			if component.Identifier != legacyConfigProfileComponent {
				continue
			}

			var configuration struct {
				PayloadContent []map[string]any `json:"payloadContent"`
			}
			if err := json.Unmarshal(component.Configuration, &configuration); err != nil {
				return nil, fmt.Errorf("decoding the stored %s component of blueprint %s: %w", legacyConfigProfileComponent, blueprintID, err)
			}

			for _, payload := range configuration.PayloadContent {
				key := storedPayloadKey(payload)
				identifier, _ := payload["payloadIdentifier"].(string)
				uuid, _ := payload["payloadUUID"].(string)
				switch {
				case key == "":
					return nil, fmt.Errorf("blueprint %s stores a legacy payload with no payloadType", blueprintID)
				case identifier == "":
					return nil, fmt.Errorf("blueprint %s stores the %s payload with no payloadIdentifier, which the service assigns on every write", blueprintID, key)
				case uuid != identifier:
					return nil, fmt.Errorf(
						"blueprint %s stores the %s payload with payloadUUID %s against payloadIdentifier %s; the provider masks payloadUUID because the service overwrites it with the identifier",
						blueprintID, key, uuid, identifier,
					)
				}
				identifiers[key] = identifier
			}
		}
	}

	if len(identifiers) == 0 {
		return nil, fmt.Errorf("blueprint %s stores no legacy payload identifiers, so nothing this file asserts could be observed", blueprintID)
	}
	return identifiers, nil
}

// blueprintIDFromState reads the managed blueprint's id out of Terraform state, which is how every
// check here reaches the API.
func blueprintIDFromState(s *terraform.State, addr string) (string, error) {
	rs, ok := s.RootModule().Resources[addr]
	if !ok {
		return "", fmt.Errorf("%s is absent from state", addr)
	}
	if rs.Primary.ID == "" {
		return "", fmt.Errorf("%s carries no id in state", addr)
	}
	return rs.Primary.ID, nil
}

// captureStoredIdentifiers returns a Check that records the identifier the service has stored for
// each of the blueprint's legacy payloads. The map passed in is filled rather than replaced, so a
// later step's check — built before this one runs — can close over it directly.
func captureStoredIdentifiers(t *testing.T, addr string, into map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		blueprintID, err := blueprintIDFromState(s, addr)
		if err != nil {
			return err
		}

		current, err := readLegacyPayloadIdentifiers(t, blueprintID)
		if err != nil {
			return err
		}
		maps.Copy(into, current)
		return nil
	}
}

// captureBlueprintID returns a Check that records the blueprint's id for a PreConfig, which runs
// before the step and so sees no Terraform state of its own.
func captureBlueprintID(addr string, into *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		blueprintID, err := blueprintIDFromState(s, addr)
		if err != nil {
			return err
		}
		*into = blueprintID
		return nil
	}
}

// checkStoredIdentifiers returns a Check that reads the blueprint back and asserts three things
// about what the service now holds: every identifier an earlier step recorded is still on the same
// payload type and byte-identical; no identifier has moved onto a different type; and every type
// named in alsoStored carries one of its own.
//
// The middle clause is the one that matters most. A rotated identifier is churn, but a transplanted
// one puts two profiles' payloads under a single Apple identifier — and Apple keys an installed
// payload on exactly that, so the two would then fight over one payload on the device.
func checkStoredIdentifiers(t *testing.T, addr string, recorded map[string]string, alsoStored ...string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		blueprintID, err := blueprintIDFromState(s, addr)
		if err != nil {
			return err
		}

		current, err := readLegacyPayloadIdentifiers(t, blueprintID)
		if err != nil {
			return err
		}
		if len(recorded) == 0 {
			return errors.New("the earlier step recorded no identifiers, so an unchanged identifier here would prove nothing")
		}

		for payloadType, want := range recorded {
			got, stored := current[payloadType]
			if !stored {
				return fmt.Errorf("the service no longer stores a %s payload, so its identifier cannot be compared", payloadType)
			}
			if got != want {
				return fmt.Errorf("payload %s: the service reassigned payloadIdentifier %s to %s, so the update did not write the stored value back", payloadType, want, got)
			}
		}

		for payloadType, got := range current {
			for recordedType, want := range recorded {
				if recordedType != payloadType && got == want {
					return fmt.Errorf("payload %s now carries %s, which is the identifier recorded for %s", payloadType, got, recordedType)
				}
			}
		}

		for _, payloadType := range alsoStored {
			if _, stored := current[payloadType]; !stored {
				return fmt.Errorf("the service stores no %s payload, so the identifier it should have minted is missing", payloadType)
			}
		}
		return nil
	}
}

// checkSettingsCarryNoServerOwnedKeys returns a Check asserting a payload's `settings` in state
// carries neither key the service owns, in any casing. Jamf writes its own metadata with a
// lowercase leading `p` while Apple declares `PayloadIdentifier` and `PayloadUUID`, so an
// exact-match assertion would pass over the spelling an author is most likely to have tried.
func checkSettingsCarryNoServerOwnedKeys(addr, attribute string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(addr, attribute, func(value string) error {
		var settings map[string]any
		if err := json.Unmarshal([]byte(value), &settings); err != nil {
			return fmt.Errorf("%s holds %q, which is not a JSON object: %w", attribute, value, err)
		}
		if len(settings) == 0 {
			return fmt.Errorf("%s holds no keys at all, so the absence of a server-owned one proves nothing", attribute)
		}

		for key := range settings {
			switch strings.ToLower(key) {
			case "payloadidentifier", "payloaduuid":
				return fmt.Errorf("%s carries %q, which the service owns and the provider masks from both sides of the diff", attribute, key)
			}
		}
		return nil
	})
}

// TestAccResource_Blueprint_LegacyPayloads_IdentifiersSurviveUnrelatedUpdate is the test the whole
// read-merge-write exists for: an update aimed at the blueprint's metadata must leave every legacy
// payload's identifier alone. Step 1 authors two payloads and no identifier, so a stored value is
// necessarily one the service minted, and records them. Step 2 changes the description and nothing
// else, and every identifier must come back byte-identical.
//
// Without this test a regression to omitting the key, or to deriving an identifier provider-side,
// is invisible: the apply succeeds, the plan stays empty, state is unchanged, and the only casualty
// is a stored blueprint whose payload identities rotate under the operator on every edit.
func TestAccResource_Blueprint_LegacyPayloads_IdentifiersSurviveUnrelatedUpdate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-lpid-unrelated-" + suffix
	addr := "jamfplatform_blueprints_blueprint.lpid_unrelated"
	recorded := make(map[string]string)

	config := func(description string) string {
		return legacyIdentifierConfig("lpidunrelated", "lpid_unrelated", name, description, legacyPayloadBlock{
			name: "Legacy Payloads",
			payloads: []legacyPayloadSpec{
				{payloadType: accessibilityPayloadType, settings: accessibilitySettings},
				{payloadType: domainsPayloadType, settings: domainsSettings},
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
					resource.TestCheckResourceAttrSet(addr, "id"),
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.#", "2"),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: config("Acceptance test — safe to delete, description edited"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "description", "Acceptance test — safe to delete, description edited"),
					checkStoredIdentifiers(t, addr, recorded),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_IdentifiersSurviveSettingsEdit covers the case the web
// app was observed in: editing one payload's setting and saving. The web app left the identifier of
// the very payload it edited untouched, wire-verified 2026-09-16, so an operator who edits the same
// setting through Terraform must get the same result.
//
// It is the edited payload's own identifier that makes this test more than a repeat of the
// unrelated-update one. `payloadContent` is an array and a merge-patch replaces it wholesale, so the
// write that carries the new setting carries the whole array — a write path that preserved only the
// payloads it was not touching would pass the previous test and fail here.
func TestAccResource_Blueprint_LegacyPayloads_IdentifiersSurviveSettingsEdit(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-lpid-settings-" + suffix
	addr := "jamfplatform_blueprints_blueprint.lpid_settings"
	recorded := make(map[string]string)

	config := func(accessibility string) string {
		return legacyIdentifierConfig("lpidsettings", "lpid_settings", name, "Acceptance test — safe to delete", legacyPayloadBlock{
			name: "Legacy Payloads",
			payloads: []legacyPayloadSpec{
				{payloadType: accessibilityPayloadType, settings: accessibility},
				{payloadType: domainsPayloadType, settings: domainsSettings},
			},
		})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config(accessibilitySettings),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "id"),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: config(accessibilitySettingsEdited),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(addr, "component_blocks.0.legacy_payloads.0.settings"),
					checkStoredIdentifiers(t, addr, recorded),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_IdentifiersFollowRenamedAndMovedBlock covers one apply
// that both reorders two blocks and renames one of them. The name pass claims the step the sibling
// still names, out of position, so the renamed block names nothing and the positional pass has to
// offer it the step left unclaimed rather than the step at its own index — which the sibling has
// already taken.
//
// Keying the leftover block on its own index strands the renamed block's step and mints a fresh
// identifier for every payload in it, which is why the two blocks here carry different payload
// types: a same-type pair would let a swap pass unnoticed.
func TestAccResource_Blueprint_LegacyPayloads_IdentifiersFollowRenamedAndMovedBlock(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-lpid-moved-" + suffix
	addr := "jamfplatform_blueprints_blueprint.lpid_moved"
	recorded := make(map[string]string)

	accessibilityBlockNamed := func(blockName string) legacyPayloadBlock {
		return legacyPayloadBlock{
			name:     blockName,
			payloads: []legacyPayloadSpec{{payloadType: accessibilityPayloadType, settings: accessibilitySettings}},
		}
	}
	domainsBlock := legacyPayloadBlock{
		name:     "B",
		payloads: []legacyPayloadSpec{{payloadType: domainsPayloadType, settings: domainsSettings}},
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: legacyIdentifierConfig("lpidmoved", "lpid_moved", name, "Acceptance test — safe to delete",
					accessibilityBlockNamed("A"), domainsBlock),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.#", "2"),
					resource.TestCheckResourceAttr(addr, "component_blocks.0.name", "A"),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: legacyIdentifierConfig("lpidmoved", "lpid_moved", name, "Acceptance test — safe to delete",
					domainsBlock, accessibilityBlockNamed("A renamed")),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.name", "B"),
					resource.TestCheckResourceAttr(addr, "component_blocks.1.name", "A renamed"),
					checkStoredIdentifiers(t, addr, recorded),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_DuplicateBlockNamesDoNotTransplantIdentifiers covers
// renaming a block onto a sibling's name, which the schema permits: component_blocks[].name is
// optional and carries no uniqueness validator. A name two blocks share can name neither step, or
// the first block to ask takes the step its namesake was continuing.
//
// The assertion that matters is that neither block received the other's identifier. A rotated
// identifier costs a stored blueprint's tidiness; a transplanted one writes one profile's identity
// onto another profile's payloads, and Apple keys an installed payload on exactly that. Both blocks
// here fall back to position, so each also keeps its own — asserted too, because the weaker claim
// alone would pass while both identifiers rotated.
func TestAccResource_Blueprint_LegacyPayloads_DuplicateBlockNamesDoNotTransplantIdentifiers(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-lpid-dupe-" + suffix
	addr := "jamfplatform_blueprints_blueprint.lpid_dupe"
	recorded := make(map[string]string)

	accessibilityBlockNamed := func(blockName string) legacyPayloadBlock {
		return legacyPayloadBlock{
			name:     blockName,
			payloads: []legacyPayloadSpec{{payloadType: accessibilityPayloadType, settings: accessibilitySettings}},
		}
	}
	domainsBlock := legacyPayloadBlock{
		name:     "B",
		payloads: []legacyPayloadSpec{{payloadType: domainsPayloadType, settings: domainsSettings}},
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: legacyIdentifierConfig("lpiddupe", "lpid_dupe", name, "Acceptance test — safe to delete",
					accessibilityBlockNamed("A"), domainsBlock),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.name", "A"),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: legacyIdentifierConfig("lpiddupe", "lpid_dupe", name, "Acceptance test — safe to delete",
					accessibilityBlockNamed("B"), domainsBlock),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.name", "B"),
					resource.TestCheckResourceAttr(addr, "component_blocks.1.name", "B"),
					checkStoredIdentifiers(t, addr, recorded),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_AddedPayloadMintsWhileSiblingsHold covers adding a
// payload to a block that already has some. The stored identifiers are looked up per payload type,
// so the added type has none and is the service's to mint, while its siblings must be written back
// — and because the whole `payloadContent` array goes out on that write, a path that sent the new
// payload without the old ones' identifiers would re-identify the lot.
//
// The added identifier is asserted distinct from both recorded ones as well as merely present,
// since minting is only correct if it mints something new.
func TestAccResource_Blueprint_LegacyPayloads_AddedPayloadMintsWhileSiblingsHold(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-lpid-added-" + suffix
	addr := "jamfplatform_blueprints_blueprint.lpid_added"
	recorded := make(map[string]string)

	block := func(payloads ...legacyPayloadSpec) legacyPayloadBlock {
		return legacyPayloadBlock{name: "Legacy Payloads", payloads: payloads}
	}
	accessibility := legacyPayloadSpec{payloadType: accessibilityPayloadType, settings: accessibilitySettings}
	domains := legacyPayloadSpec{payloadType: domainsPayloadType, settings: domainsSettings}
	fileProvider := legacyPayloadSpec{payloadType: fileProviderPayloadType, settings: fileProviderSettings}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: legacyIdentifierConfig("lpidadded", "lpid_added", name, "Acceptance test — safe to delete",
					block(accessibility, domains)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.#", "2"),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: legacyIdentifierConfig("lpidadded", "lpid_added", name, "Acceptance test — safe to delete",
					block(accessibility, domains, fileProvider)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.#", "3"),
					checkStoredIdentifiers(t, addr, recorded, fileProviderPayloadType),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_IdentifiersNeverReachState is the other half of the
// contract. The service stamps an identifier onto every payload it stores, and the provider masks
// it from both sides of the diff — so state must carry neither key while the wire carries both.
//
// Both halves are asserted on the same step, because either alone is satisfiable by a broken
// provider: a state with no identifier proves nothing if the service never stored one, and a stored
// identifier is no use if it leaks into `settings`, where it becomes a perpetual diff against the
// author's `jsonencode` string and, on apply, an inconsistent-result error. The second step pins
// that: the same configuration must plan empty.
func TestAccResource_Blueprint_LegacyPayloads_IdentifiersNeverReachState(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-lpid-state-" + suffix
	addr := "jamfplatform_blueprints_blueprint.lpid_state"
	recorded := make(map[string]string)

	config := legacyIdentifierConfig("lpidstate", "lpid_state", name, "Acceptance test — safe to delete", legacyPayloadBlock{
		name: "Legacy Payloads",
		payloads: []legacyPayloadSpec{
			{payloadType: accessibilityPayloadType, settings: accessibilitySettings},
			{payloadType: domainsPayloadType, settings: domainsSettings},
		},
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.0.payload_type", accessibilityPayloadType),
					resource.TestCheckResourceAttr(addr, "component_blocks.0.legacy_payloads.1.payload_type", domainsPayloadType),
					checkSettingsCarryNoServerOwnedKeys(addr, "component_blocks.0.legacy_payloads.0.settings"),
					checkSettingsCarryNoServerOwnedKeys(addr, "component_blocks.0.legacy_payloads.1.settings"),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: checkStoredIdentifiers(t, addr, recorded),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_FlatModePreservesIdentifiers covers the deprecated
// top-level `legacy_payloads` attribute, which had no coverage of this at any level. It resolves its
// stored identifiers differently from a block: flat mode writes and reads exactly one step, so it
// asks for the identifiers of a single unnamed block and takes step 0 positionally. Matching on the
// provider's own "Declaration group" step name instead would hand flat mode the identifiers of a
// step it does not manage whenever a stored step elsewhere carried that name.
//
// Deprecated is not the same as unused, and an operator who has not yet migrated has the same claim
// on a stable payload identity as one who has.
func TestAccResource_Blueprint_LegacyPayloads_FlatModePreservesIdentifiers(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-lpid-flat-" + suffix
	addr := "jamfplatform_blueprints_blueprint.lpid_flat"
	recorded := make(map[string]string)

	config := func(description string) string {
		return testBlueprintConfig(smartGroupHCL("lpidflat"), fmt.Sprintf(`
			resource "jamfplatform_blueprints_blueprint" "lpid_flat" {
				name          = %q
				description   = %q
				deployed      = false
				device_groups = [jamfplatform_device_group.scope.id]

				legacy_payloads = [
					{
						payload_type = %q
						settings     = %s
					},
					{
						payload_type = %q
						settings     = %s
					},
				]
			}
		`, name, description,
			accessibilityPayloadType, flatAccessibilitySettingsHCL,
			domainsPayloadType, flatDomainsSettingsHCL))
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
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				Config: config("Acceptance test — safe to delete, description edited"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "description", "Acceptance test — safe to delete, description edited"),
					checkStoredIdentifiers(t, addr, recorded),
				),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_ServiceRemintsWhenIdentifierOmitted pins the wire law the
// rest of this file rests on, rather than leaving it as a premise nobody rechecks. Step 1 records
// the stored identifiers; the PreConfig then writes the blueprint's own steps straight back through
// the SDK with `payloadIdentifier` and `payloadUUID` stripped from every payload — the shape a
// provider that omitted the key would send — and asserts the service minted different ones.
//
// If Jamf ever starts preserving an identifier across an omission, this test fails and says so,
// which is the signal that the read-before-update can go. Step 2 then re-applies the unchanged
// configuration and asserts the provider settles on what the service now holds, so an out-of-band
// re-mint is not something the provider fights.
func TestAccResource_Blueprint_LegacyPayloads_ServiceRemintsWhenIdentifierOmitted(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-lpid-remint-" + suffix
	addr := "jamfplatform_blueprints_blueprint.lpid_remint"

	var blueprintID string
	recorded := make(map[string]string)
	reminted := make(map[string]string)

	config := legacyIdentifierConfig("lpidremint", "lpid_remint", name, "Acceptance test — safe to delete", legacyPayloadBlock{
		name: "Legacy Payloads",
		payloads: []legacyPayloadSpec{
			{payloadType: accessibilityPayloadType, settings: accessibilitySettings},
			{payloadType: domainsPayloadType, settings: domainsSettings},
		},
	})

	// stripIdentifiersOutOfBand simulates a write that lets the service own the identifier: the
	// blueprint's stored steps go back verbatim except that every payload loses the two keys.
	stripIdentifiersOutOfBand := func() {
		client := bpSDK.New(testhelpers.NewAcceptanceClient(t))
		ctx := context.Background()

		blueprint, err := client.GetBlueprint(ctx, blueprintID)
		if err != nil {
			t.Fatalf("out-of-band GET: %v", err)
		}

		steps := blueprint.Steps
		stripped := 0
		for stepIndex, step := range steps {
			for componentIndex, component := range step.Components {
				if component.Identifier != legacyConfigProfileComponent {
					continue
				}

				var configuration map[string]any
				if err := json.Unmarshal(component.Configuration, &configuration); err != nil {
					t.Fatalf("out-of-band decode of the stored %s component: %v", legacyConfigProfileComponent, err)
				}
				payloads, ok := configuration["payloadContent"].([]any)
				if !ok {
					t.Fatalf("out-of-band: the stored %s component carries no payloadContent array", legacyConfigProfileComponent)
				}
				for _, item := range payloads {
					payload, ok := item.(map[string]any)
					if !ok {
						t.Fatal("out-of-band: a payloadContent entry is not an object")
					}
					for key := range payload {
						if strings.EqualFold(key, "payloadIdentifier") || strings.EqualFold(key, "payloadUUID") {
							delete(payload, key)
							stripped++
						}
					}
				}

				encoded, err := json.Marshal(configuration)
				if err != nil {
					t.Fatalf("out-of-band encode: %v", err)
				}
				steps[stepIndex].Components[componentIndex].Configuration = encoded
			}
		}
		if stripped == 0 {
			t.Fatal("out-of-band: no identifier was stripped, so the re-mint this test asserts could not have been triggered")
		}

		if err := client.UpdateBlueprint(ctx, blueprintID, &bpSDK.UpdateBlueprintRequest{Steps: &steps}); err != nil {
			t.Fatalf("out-of-band identifier strip: %v", err)
		}

		current, err := readLegacyPayloadIdentifiers(t, blueprintID)
		if err != nil {
			t.Fatalf("reading back the stripped blueprint: %v", err)
		}
		for payloadType, want := range recorded {
			got, stored := current[payloadType]
			if !stored {
				t.Fatalf("payload %s is no longer stored, so the out-of-band write lost it rather than re-identifying it", payloadType)
			}
			if got == want {
				t.Fatalf("payload %s kept payloadIdentifier %s across a write that omitted the key; the service now preserves it, so the provider's read-before-update is no longer needed", payloadType, got)
			}
			reminted[payloadType] = got
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					captureBlueprintID(addr, &blueprintID),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				PreConfig: stripIdentifiersOutOfBand,
				Config:    config,
				Check:     checkStoredIdentifiers(t, addr, reminted),
			},
		},
	})
}

// legacyDerivedIdentifier reproduces the identifier every released provider version wrote before the
// service was given the field: the first sixteen bytes of sha256(payloadType), formatted as a UUID.
// It depended on the payload type alone, so every blueprint in a tenant carrying one payload type
// held the same value. Reproduced here so a test can assert the provider no longer sends it.
func legacyDerivedIdentifier(payloadType string) string {
	sum := sha256.Sum256([]byte(payloadType))
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// checkStoredIdentifiersAllChanged asserts every recorded payload type is still stored and now
// carries a different identifier. It is the inverse of checkStoredIdentifiers, for the one case
// where re-minting is the correct outcome rather than a regression.
func checkStoredIdentifiersAllChanged(t *testing.T, addr string, recorded map[string]string) resource.TestCheckFunc {
	t.Helper()

	return func(s *terraform.State) error {
		if len(recorded) == 0 {
			return errors.New("no identifiers were recorded, so there is nothing to compare against")
		}

		blueprintID, err := blueprintIDFromState(s, addr)
		if err != nil {
			return err
		}
		current, err := readLegacyPayloadIdentifiers(t, blueprintID)
		if err != nil {
			return err
		}

		for payloadType, before := range recorded {
			after, stored := current[payloadType]
			if !stored {
				return fmt.Errorf("payload %s is no longer stored at all, rather than re-identified", payloadType)
			}
			if after == before {
				return fmt.Errorf("payload %s kept identifier %s, but a positional match cannot have found its step", payloadType, before)
			}
		}
		return nil
	}
}

// TestAccResource_Blueprint_LegacyPayloads_UnnamedBlocksResolveByPosition covers the authoring style
// the schema permits but no other test in this file uses: component_blocks[].name is optional, and a
// block without one has nothing but its position for the provider to match it by.
//
// Both halves are asserted, because the guarantee and its boundary are one behaviour. An update the
// author did not aim at the blocks keeps every identifier, so an unnamed block is not second-class.
// Reordering two of them re-mints both, because each block then sits over the step the other one
// left and finds no entry for its own payload type there. That is the cost of leaving blocks
// unnamed, and pinning it stops the cost being discovered by an operator instead.
func TestAccResource_Blueprint_LegacyPayloads_UnnamedBlocksResolveByPosition(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-lpid-unnamed-" + suffix
	addr := "jamfplatform_blueprints_blueprint.lpid_unnamed"
	recorded := make(map[string]string)

	accessibilityBlock := legacyPayloadBlock{
		payloads: []legacyPayloadSpec{{payloadType: accessibilityPayloadType, settings: accessibilitySettings}},
	}
	domainsBlock := legacyPayloadBlock{
		payloads: []legacyPayloadSpec{{payloadType: domainsPayloadType, settings: domainsSettings}},
	}

	config := func(description string, blocks ...legacyPayloadBlock) string {
		return legacyIdentifierConfig("lpidunnamed", "lpid_unnamed", name, description, blocks...)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config("Acceptance test — safe to delete", accessibilityBlock, domainsBlock),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "component_blocks.#", "2"),
					captureStoredIdentifiers(t, addr, recorded),
				),
			},
			{
				// An unrelated edit. Position is unchanged, so a positional match finds every step.
				Config: config("Acceptance test — safe to delete, edited", accessibilityBlock, domainsBlock),
				Check:  checkStoredIdentifiers(t, addr, recorded),
			},
			{
				// Reordered. Neither block can be matched by name, so both take the other's step and
				// the service mints for both.
				Config: config("Acceptance test — safe to delete, edited", domainsBlock, accessibilityBlock),
				Check:  checkStoredIdentifiersAllChanged(t, addr, recorded),
			},
		},
	})
}

// TestAccResource_Blueprint_LegacyPayloads_ServiceMintsOnCreate isolates what the service does on
// create, which no other test here can: every one of them observes the create path only as the
// starting point for an update, and a manual probe through jamf-cli cannot answer it either, because
// `pro bp apply` randomizes payload identifiers client-side before the request is sent.
//
// Two blueprints, created in one apply, carrying the same payload type. The provider sends no
// identifier for either — legacy_payload_identifiers_test.go's create case pins that against the
// request body — so whatever comes back was assigned by the service. The two must differ, which is
// what separates a value minted per payload from any value derived from the payload type, and
// neither may equal the sha256 derivation the provider itself used to send.
func TestAccResource_Blueprint_LegacyPayloads_ServiceMintsOnCreate(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	firstAddr := "jamfplatform_blueprints_blueprint.lpid_mint_one"
	secondAddr := "jamfplatform_blueprints_blueprint.lpid_mint_two"
	first := make(map[string]string)
	second := make(map[string]string)

	blueprintHCL := func(label, name string) string {
		return fmt.Sprintf(`
			resource "jamfplatform_blueprints_blueprint" %q {
				name          = %q
				description   = "Acceptance test — safe to delete"
				deployed      = false
				device_groups = [jamfplatform_device_group.scope.id]

				component_blocks = [{
					name = "Minting"
					legacy_payloads = [{
						payload_type = %q
						settings     = %s
					}]
				}]
			}
		`, label, name, domainsPayloadType, domainsSettings)
	}

	config := smartGroupHCL("lpidmint") +
		blueprintHCL("lpid_mint_one", "tf-acc-lpid-mint-one-"+suffix) +
		blueprintHCL("lpid_mint_two", "tf-acc-lpid-mint-two-"+suffix)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					captureStoredIdentifiers(t, firstAddr, first),
					captureStoredIdentifiers(t, secondAddr, second),
					func(*terraform.State) error {
						one, two := first[domainsPayloadType], second[domainsPayloadType]
						if one == two {
							return fmt.Errorf(
								"both blueprints stored %s for %s; an identifier shared by two separate creates is derived from the payload type rather than minted per payload",
								one, domainsPayloadType,
							)
						}
						derived := legacyDerivedIdentifier(domainsPayloadType)
						for label, got := range map[string]string{firstAddr: one, secondAddr: two} {
							if strings.EqualFold(got, derived) {
								return fmt.Errorf(
									"%s stored %s, which is sha256(%s) — the provider is still sending the identifier it used to derive",
									label, got, domainsPayloadType,
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

// TestAccResource_Blueprint_LegacyPayloads_DuplicateIdentifierIsReissued covers the one duplicate a
// device refuses: two payloads in a single step sharing an identifier. macOS rejects the whole
// profile and every payload in it — "the PayloadIdentifier is used more than once in the profile",
// ConfigProfilePluginDomain:-107, wire-verified on macOS 26.6 on 2026-09-16, retrying indefinitely
// while the blueprint reported DEPLOYED and SUCCEEDED.
//
// The provider never writes that shape itself, but it would propagate one, because stored
// identifiers are read by payload type: a blueprint edited outside Terraform into holding one
// identifier on two types would have both types map to it and be written back unusable on every
// apply. So the duplicate is planted out of band here, exactly as an admin or a raw_component author
// could, and the update that follows must reissue rather than preserve.
//
// The step changes the description as well, because an identifier is masked from state: with the
// configuration untouched the plan is empty, nothing is written, and the guard is never reached.
func TestAccResource_Blueprint_LegacyPayloads_DuplicateIdentifierIsReissued(t *testing.T) {
	testhelpers.AccPreCheck(t)
	suffix := testhelpers.RunSuffix()
	name := "tf-acc-lpid-dupident-" + suffix
	addr := "jamfplatform_blueprints_blueprint.lpid_dupident"

	var blueprintID string
	const planted = "AAAA1111-2222-4333-8444-555566667777"

	block := legacyPayloadBlock{
		name: "Shared",
		payloads: []legacyPayloadSpec{
			{payloadType: accessibilityPayloadType, settings: accessibilitySettings},
			{payloadType: domainsPayloadType, settings: domainsSettings},
		},
	}
	config := func(description string) string {
		return legacyIdentifierConfig("lpiddupident", "lpid_dupident", name, description, block)
	}

	// plantDuplicateOutOfBand rewrites both payloads of the stored step to one identifier, the shape
	// an edit outside Terraform can leave and the provider must not carry forward.
	plantDuplicateOutOfBand := func() {
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
		for _, payloadType := range []string{accessibilityPayloadType, domainsPayloadType} {
			if got := current[payloadType]; !strings.EqualFold(got, planted) {
				t.Fatalf("payload %s stores %s after the plant, want the shared %s; the service refused the duplicate and this test can prove nothing", payloadType, got, planted)
			}
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testhelpers.AccTestProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckBlueprintResourcesDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config("Acceptance test — safe to delete"),
				Check:  captureBlueprintID(addr, &blueprintID),
			},
			{
				PreConfig: plantDuplicateOutOfBand,
				Config:    config("Acceptance test — safe to delete, edited"),
				Check: func(s *terraform.State) error {
					current, err := readLegacyPayloadIdentifiers(t, blueprintID)
					if err != nil {
						return err
					}
					accessibility, domains := current[accessibilityPayloadType], current[domainsPayloadType]
					if strings.EqualFold(accessibility, planted) || strings.EqualFold(domains, planted) {
						return fmt.Errorf(
							"the planted identifier %s survived on at least one payload (%s=%s, %s=%s); the provider wrote back a profile a device refuses whole",
							planted, accessibilityPayloadType, accessibility, domainsPayloadType, domains,
						)
					}
					if accessibility == domains {
						return fmt.Errorf(
							"both payloads were reissued %s, so the profile is still one a device refuses; the service must assign a distinct identifier per payload",
							accessibility,
						)
					}
					return nil
				},
			},
		},
	})
}

// checkStoredIdentifiersAreDistinct returns a Check asserting the blueprint stores wantCount legacy
// payloads and that no two of them carry the same identifier.
//
// checkStoredIdentifiers cannot say this. It compares each recorded identifier against what is
// stored now, so two payloads that shared one identifier from the first apply would agree with
// themselves forever. The count is asserted in the same place because a collapse and a duplicate
// have the same cause: several payloads keyed as one. A block's payloads install as a single
// profile, and macOS refuses a profile whose payload identifiers are not unique — every payload in
// it, not the duplicated ones.
func checkStoredIdentifiersAreDistinct(t *testing.T, addr string, wantCount int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		blueprintID, err := blueprintIDFromState(s, addr)
		if err != nil {
			return err
		}

		current, err := readLegacyPayloadIdentifiers(t, blueprintID)
		if err != nil {
			return err
		}
		if len(current) != wantCount {
			return fmt.Errorf("the service stores %d identified legacy payloads, want %d: %v", len(current), wantCount, current)
		}

		keysByIdentifier := make(map[string][]string, len(current))
		for key, identifier := range current {
			keysByIdentifier[identifier] = append(keysByIdentifier[identifier], key)
		}
		for identifier, keys := range keysByIdentifier {
			if len(keys) > 1 {
				return fmt.Errorf("payloadIdentifier %s is shared by %s, which makes a Mac refuse the whole profile", identifier, strings.Join(keys, " and "))
			}
		}
		return nil
	}
}
