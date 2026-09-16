// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

// legacyPayloadsInStep decodes a step's legacy configuration profile component into payload type to
// the whole payload map, so a test can assert on what was sent rather than on a JSON string.
func legacyPayloadsInStep(t *testing.T, step blueprints.BlueprintStep) map[string]map[string]any {
	t.Helper()

	payloads := make(map[string]map[string]any)
	for _, component := range step.Components {
		if component.Identifier != legacyConfigProfileIdentifier {
			continue
		}
		var configuration struct {
			PayloadContent []map[string]any `json:"payloadContent"`
		}
		if err := json.Unmarshal(component.Configuration, &configuration); err != nil {
			t.Fatalf("unmarshal legacy component configuration: %v", err)
		}
		for _, payload := range configuration.PayloadContent {
			payloadType, _ := payload["payloadType"].(string)
			payloads[payloadType] = payload
		}
	}
	return payloads
}

// blockWithLegacyPayload builds a one-payload component block, optionally named.
func blockWithLegacyPayload(name, payloadType, settings string) ComponentBlockModel {
	block := ComponentBlockModel{
		LegacyPayloads: []BlockLegacyPayloadModel{{
			PayloadType: types.StringValue(payloadType),
			Settings:    types.StringValue(settings),
		}},
	}
	if name != "" {
		block.Name = types.StringValue(name)
	}
	return block
}

func storedBlueprint(steps ...blueprints.BlueprintStep) *blueprints.BlueprintDetail {
	return &blueprints.BlueprintDetail{Steps: steps}
}

func storedStep(name string, payloadTypeToIdentifier map[string]string) blueprints.BlueprintStep {
	payloads := make([]map[string]any, 0, len(payloadTypeToIdentifier))
	for payloadType, identifier := range payloadTypeToIdentifier {
		payloads = append(payloads, map[string]any{"payloadType": payloadType, "payloadIdentifier": identifier})
	}
	configuration, err := json.Marshal(map[string]any{"payloadContent": payloads})
	if err != nil {
		panic(err)
	}
	return blueprints.BlueprintStep{
		Name:       &name,
		Components: []blueprints.Component{{Identifier: legacyConfigProfileIdentifier, Configuration: configuration}},
	}
}

// TestBuildSteps_CreateOmitsPayloadIdentifier covers the create path, where the service mints the
// identifier. The Blueprints API specification requires only payloadType in a payloadContent entry
// and never mentions payloadIdentifier, so the provider sends no value for the service to have to
// honour.
func TestBuildSteps_CreateOmitsPayloadIdentifier(t *testing.T) {
	t.Parallel()

	data := &BlueprintResourceModel{
		Name:            types.StringValue("BP"),
		ComponentBlocks: []ComponentBlockModel{blockWithLegacyPayload("Block 1", "com.apple.domains", `{"EmailDomains":["example.com"]}`)},
	}

	steps, diags := (&BlueprintResource{}).buildSteps(context.Background(), data, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	payload := legacyPayloadsInStep(t, steps[0])["com.apple.domains"]
	if _, present := payload["payloadIdentifier"]; present {
		t.Errorf("create sent payloadIdentifier %q, want the key omitted", payload["payloadIdentifier"])
	}
	if got := payload["EmailDomains"]; got == nil {
		t.Error("authored settings did not survive")
	}
}

// TestBuildSteps_UpdateWritesStoredPayloadIdentifierBack is the whole point of the read-merge-write:
// the service mints a replacement identifier for any payload sent without one, on every write, and
// a replaced identifier reinstalls the profile on every scoped device.
func TestBuildSteps_UpdateWritesStoredPayloadIdentifierBack(t *testing.T) {
	t.Parallel()

	data := &BlueprintResourceModel{
		Name:            types.StringValue("BP"),
		ComponentBlocks: []ComponentBlockModel{blockWithLegacyPayload("Block 1", "com.apple.domains", `{"EmailDomains":["example.com"]}`)},
	}
	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(storedStep("Block 1", map[string]string{"com.apple.domains": "STORED-IDENT"})))

	steps, diags := (&BlueprintResource{}).buildSteps(context.Background(), data, stored)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got := legacyPayloadsInStep(t, steps[0])["com.apple.domains"]["payloadIdentifier"]; got != "STORED-IDENT" {
		t.Errorf("payloadIdentifier = %v, want the stored STORED-IDENT", got)
	}
}

// TestBuildSteps_AuthoredPayloadIdentifierIsDiscarded covers the key reaching the wire from
// configuration, which it can because settings is a free-form object and the service honours
// whatever arrives. The service owns the field, so an authored value loses to the stored one on an
// update and is dropped entirely on a create — the same treatment the configuration profile
// resources give an authored top-level identifier.
func TestBuildSteps_AuthoredPayloadIdentifierIsDiscarded(t *testing.T) {
	t.Parallel()

	settings := `{"EmailDomains":["example.com"],"payloadIdentifier":"AUTHORED"}`

	for _, tc := range []struct {
		name   string
		stored *storedLegacyPayloadIdentifiers
		want   any
	}{
		{name: "create drops it", stored: nil, want: nil},
		{
			name:   "update prefers the stored value",
			stored: newStoredLegacyPayloadIdentifiers(storedBlueprint(storedStep("Block 1", map[string]string{"com.apple.domains": "STORED-IDENT"}))),
			want:   "STORED-IDENT",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data := &BlueprintResourceModel{
				Name:            types.StringValue("BP"),
				ComponentBlocks: []ComponentBlockModel{blockWithLegacyPayload("Block 1", "com.apple.domains", settings)},
			}

			steps, diags := (&BlueprintResource{}).buildSteps(context.Background(), data, tc.stored)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got := legacyPayloadsInStep(t, steps[0])["com.apple.domains"]["payloadIdentifier"]; got != tc.want {
				t.Errorf("payloadIdentifier = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestBuildSteps_StoredIdentifiersFollowTheBlockName covers inserting a block ahead of an existing
// one. Matching stored steps by position alone would hand the new block the existing block's
// identifier and leave the existing block minting a fresh one, so an insert would re-identify
// payloads it never touched.
func TestBuildSteps_StoredIdentifiersFollowTheBlockName(t *testing.T) {
	t.Parallel()

	data := &BlueprintResourceModel{
		Name: types.StringValue("BP"),
		ComponentBlocks: []ComponentBlockModel{
			blockWithLegacyPayload("Inserted", "com.apple.domains", `{"EmailDomains":["new.example.com"]}`),
			blockWithLegacyPayload("Original", "com.apple.domains", `{"EmailDomains":["example.com"]}`),
		},
	}
	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(storedStep("Original", map[string]string{"com.apple.domains": "ORIGINAL-IDENT"})))

	steps, diags := (&BlueprintResource{}).buildSteps(context.Background(), data, stored)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got, present := legacyPayloadsInStep(t, steps[0])["com.apple.domains"]["payloadIdentifier"]; present {
		t.Errorf("the inserted block was given payloadIdentifier %v, want the key omitted", got)
	}
	if got := legacyPayloadsInStep(t, steps[1])["com.apple.domains"]["payloadIdentifier"]; got != "ORIGINAL-IDENT" {
		t.Errorf("the original block's payloadIdentifier = %v, want ORIGINAL-IDENT", got)
	}
}

// TestBuildSteps_UnstoredPayloadTypeOmitsTheIdentifier covers a payload added to an existing block.
// The service stamps the new payload and leaves its siblings' identifiers alone — wire-verified
// 2026-09-16, a component carrying one authored identifier and one omitted returning the authored
// value untouched beside a freshly minted one.
func TestBuildSteps_UnstoredPayloadTypeOmitsTheIdentifier(t *testing.T) {
	t.Parallel()

	block := blockWithLegacyPayload("Block 1", "com.apple.domains", `{"EmailDomains":["example.com"]}`)
	block.LegacyPayloads = append(block.LegacyPayloads, BlockLegacyPayloadModel{
		PayloadType: types.StringValue("com.apple.universalaccess"),
		Settings:    types.StringValue(`{"closeViewFarPoint":1}`),
	})
	data := &BlueprintResourceModel{Name: types.StringValue("BP"), ComponentBlocks: []ComponentBlockModel{block}}
	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(storedStep("Block 1", map[string]string{"com.apple.domains": "STORED-IDENT"})))

	steps, diags := (&BlueprintResource{}).buildSteps(context.Background(), data, stored)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	payloads := legacyPayloadsInStep(t, steps[0])
	if got := payloads["com.apple.domains"]["payloadIdentifier"]; got != "STORED-IDENT" {
		t.Errorf("the existing payload's payloadIdentifier = %v, want STORED-IDENT", got)
	}
	if got, present := payloads["com.apple.universalaccess"]["payloadIdentifier"]; present {
		t.Errorf("the added payload was given payloadIdentifier %v, want the key omitted", got)
	}
}

// TestStoredLegacyPayloadIdentifiers_DuplicateStepNamesFallBackToPosition covers two stored steps
// sharing a name, which the schema permits. An ambiguous name must not resolve to whichever step was
// read first, so both drop to a positional match.
//
// The second case is the one that holds newStoredLegacyPayloadIdentifiers to its dedupe: where both
// blocks carry the shared name, resolve declines it on the block side anyway and the positional pass
// reproduces the deduped answer, so that case cannot tell whether the stored side deduped. With only
// one block carrying it, a name that still resolved would hand block 1 the first step and push block
// 0 onto the second, inverting both.
func TestStoredLegacyPayloadIdentifiers_DuplicateStepNamesFallBackToPosition(t *testing.T) {
	t.Parallel()

	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(
		storedStep("Same", map[string]string{"com.apple.domains": "FIRST"}),
		storedStep("Same", map[string]string{"com.apple.domains": "SECOND"}),
	))

	for _, tc := range []struct {
		name       string
		blockNames []types.String
	}{
		{"both blocks carry the shared name", []types.String{types.StringValue("Same"), types.StringValue("Same")}},
		{"only the second block carries it", []types.String{types.StringValue("Other"), types.StringValue("Same")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resolved := stored.resolve(tc.blockNames)
			if got := resolved[0]["com.apple.domains"]; got != "FIRST" {
				t.Errorf("block 0 resolved to %q, want the positional FIRST", got)
			}
			if got := resolved[1]["com.apple.domains"]; got != "SECOND" {
				t.Errorf("block 1 resolved to %q, want the positional SECOND", got)
			}
		})
	}
}

// TestStoredLegacyPayloadIdentifiers_NilIsTheCreatePath keeps the create path free of a special
// case: a nil receiver answers for every step.
func TestStoredLegacyPayloadIdentifiers_NilIsTheCreatePath(t *testing.T) {
	t.Parallel()

	if got := newStoredLegacyPayloadIdentifiers(nil); got != nil {
		t.Errorf("newStoredLegacyPayloadIdentifiers(nil) = %v, want nil", got)
	}

	var stored *storedLegacyPayloadIdentifiers
	resolved := stored.resolve([]types.String{types.StringValue("Block 1"), types.StringNull()})
	if len(resolved) != 2 {
		t.Fatalf("resolve on a nil receiver returned %d entries, want one per block", len(resolved))
	}
	for i, identifiers := range resolved {
		if identifiers != nil {
			t.Errorf("block %d resolved to %v on a nil receiver, want nil", i, identifiers)
		}
	}
}

// TestMaskServerStampedPayloadKeys_PayloadUUIDIsMaskedEvenWhenAuthored covers the one metadata key
// the service never honours: it overwrites payloadUUID with the payload's identifier, or mints one
// where no identifier was sent. Keeping an authored value in state would leave the plan unable to
// settle, so the mask is unconditional for that key while the keys the service does preserve stay
// conditional.
func TestMaskServerStampedPayloadKeys_PayloadUUIDIsMaskedEvenWhenAuthored(t *testing.T) {
	t.Parallel()

	settings := map[string]any{
		"EmailDomains":        []any{"example.com"},
		"payloadUUID":         "SERVER-ASSIGNED",
		"payloadVersion":      float64(7),
		"payloadDisplayName":  "Server Stamped",
		"payloadOrganization": "JAMF Software",
	}
	prior := map[string]any{
		"payloadUUID":    "AUTHORED-UUID",
		"payloadVersion": float64(7),
	}

	maskServerStampedPayloadKeys(settings, prior)

	if _, present := settings["payloadUUID"]; present {
		t.Error("an authored payloadUUID survived the mask; the service always reassigns it")
	}
	if _, present := settings["payloadVersion"]; !present {
		t.Error("an authored payloadVersion was masked; the service preserves it")
	}
	for _, key := range []string{"payloadDisplayName", "payloadOrganization"} {
		if _, present := settings[key]; present {
			t.Errorf("unauthored %s survived the mask", key)
		}
	}
}

// TestBuildSteps_MultipleStepsEachKeepTheirOwnIdentifiers covers the shape the resource is most
// often authored in: several blocks carrying legacy payloads at once, some sharing a payload type.
// A payload is located by step and by type, so the same type in two blocks is two payloads with two
// identifiers, and no block may be handed another's.
func TestBuildSteps_MultipleStepsEachKeepTheirOwnIdentifiers(t *testing.T) {
	t.Parallel()

	data := &BlueprintResourceModel{
		Name: types.StringValue("BP"),
		ComponentBlocks: []ComponentBlockModel{
			blockWithLegacyPayload("Accessibility", "com.apple.universalaccess", `{"closeViewFarPoint":1}`),
			blockWithLegacyPayload("Domains", "com.apple.domains", `{"EmailDomains":["example.com"]}`),
			blockWithLegacyPayload("Second Domains", "com.apple.domains", `{"EmailDomains":["other.example.com"]}`),
		},
	}
	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(
		storedStep("Accessibility", map[string]string{"com.apple.universalaccess": "IDENT-ACCESS"}),
		storedStep("Domains", map[string]string{"com.apple.domains": "IDENT-DOMAINS-1"}),
		storedStep("Second Domains", map[string]string{"com.apple.domains": "IDENT-DOMAINS-2"}),
	))

	steps, diags := (&BlueprintResource{}).buildSteps(context.Background(), data, stored)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(steps))
	}

	want := []struct {
		payloadType string
		identifier  string
	}{
		{"com.apple.universalaccess", "IDENT-ACCESS"},
		{"com.apple.domains", "IDENT-DOMAINS-1"},
		{"com.apple.domains", "IDENT-DOMAINS-2"},
	}
	for i, expected := range want {
		if got := legacyPayloadsInStep(t, steps[i])[expected.payloadType]["payloadIdentifier"]; got != expected.identifier {
			t.Errorf("step %d %s payloadIdentifier = %v, want %s", i, expected.payloadType, got, expected.identifier)
		}
	}
}

// TestBuildSteps_MultipleStepsWithPayloadsOnlyInSome covers blocks that carry no legacy payloads
// sitting between ones that do. A block without them still occupies a step, so it still consumes a
// position, and the blocks that do carry payloads must not be shifted onto each other's stored
// steps by it.
func TestBuildSteps_MultipleStepsWithPayloadsOnlyInSome(t *testing.T) {
	t.Parallel()

	data := &BlueprintResourceModel{
		Name: types.StringValue("BP"),
		ComponentBlocks: []ComponentBlockModel{
			{Name: types.StringValue("No payloads")},
			blockWithLegacyPayload("Has payloads", "com.apple.domains", `{"EmailDomains":["example.com"]}`),
		},
	}
	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(
		emptyStoredStep("No payloads"),
		storedStep("Has payloads", map[string]string{"com.apple.domains": "IDENT-DOMAINS"}),
	))

	steps, diags := (&BlueprintResource{}).buildSteps(context.Background(), data, stored)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got := legacyPayloadsInStep(t, steps[1])["com.apple.domains"]["payloadIdentifier"]; got != "IDENT-DOMAINS" {
		t.Errorf("payloadIdentifier = %v, want IDENT-DOMAINS", got)
	}
}

// TestStoredLegacyPayloadIdentifiers_ResolveNeverReusesAStep pins the whole pairing, block by block,
// across the mixes of named, renamed, inserted and unnamed blocks an apply produces. The invariant
// behind the two-pass match is that a stored step feeds at most one block — handing one step to two
// blocks would put one identifier in two profiles, which collides on the device rather than merely
// churning — but the expected identifier per block is asserted alongside it, since a resolve that
// matched nothing at all would satisfy the invariant on its own while re-identifying every payload
// in the blueprint. An empty want is a block that must mint.
//
// Where a leftover block sits alongside a step no name claimed, the two are paired: an insert and a
// rename are indistinguishable from outside, so the inserted block in the first case inherits the
// departed Third step's identifier rather than leaving it stranded. That is the trade the positional
// pass exists to make — a block that really is new is one the operator has not deployed yet, while a
// renamed one is installed on every scoped device.
func TestStoredLegacyPayloadIdentifiers_ResolveNeverReusesAStep(t *testing.T) {
	t.Parallel()

	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(
		storedStep("First", map[string]string{"com.apple.domains": "IDENT-1"}),
		storedStep("Second", map[string]string{"com.apple.domains": "IDENT-2"}),
		storedStep("Third", map[string]string{"com.apple.domains": "IDENT-3"}),
	))

	for _, tc := range []struct {
		name       string
		blockNames []types.String
		want       []string
	}{
		{
			name:       "insert ahead of a named block",
			blockNames: []types.String{types.StringValue("Inserted"), types.StringValue("First"), types.StringValue("Second")},
			want:       []string{"IDENT-3", "IDENT-1", "IDENT-2"},
		},
		{
			name:       "reordered",
			blockNames: []types.String{types.StringValue("Third"), types.StringValue("First"), types.StringValue("Second")},
			want:       []string{"IDENT-3", "IDENT-1", "IDENT-2"},
		},
		{
			name:       "renamed middle block",
			blockNames: []types.String{types.StringValue("First"), types.StringValue("Renamed"), types.StringValue("Third")},
			want:       []string{"IDENT-1", "IDENT-2", "IDENT-3"},
		},
		{
			name:       "all unnamed",
			blockNames: []types.String{types.StringNull(), types.StringNull(), types.StringNull()},
			want:       []string{"IDENT-1", "IDENT-2", "IDENT-3"},
		},
		{
			name:       "more blocks than steps",
			blockNames: []types.String{types.StringValue("First"), types.StringNull(), types.StringNull(), types.StringNull()},
			want:       []string{"IDENT-1", "IDENT-2", "IDENT-3", ""},
		},
		{
			name:       "fewer blocks than steps",
			blockNames: []types.String{types.StringValue("Third")},
			want:       []string{"IDENT-3"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resolved := stored.resolve(tc.blockNames)
			if len(resolved) != len(tc.want) {
				t.Fatalf("resolve returned %d entries, want one per block (%d)", len(resolved), len(tc.want))
			}

			seen := make(map[string]int)
			for i, identifiers := range resolved {
				identifier := identifiers["com.apple.domains"]
				if identifier != tc.want[i] {
					t.Errorf("block %d resolved to %q, want %q", i, identifier, tc.want[i])
				}
				if identifier == "" {
					continue
				}
				if previous, reused := seen[identifier]; reused {
					t.Errorf("%s went to block %d and block %d", identifier, previous, i)
					continue
				}
				seen[identifier] = i
			}
		})
	}
}

// TestStoredLegacyPayloadIdentifiers_RenamedAndMovedBlockKeepsItsStep covers one apply that both
// renames a block and moves it. The name pass claims the step the sibling still names, out of
// position, and the renamed block names nothing — so the positional pass has to offer it the step
// left unclaimed rather than the step at its own index, which the sibling has already taken. Getting
// that wrong mints a fresh identifier for every payload in the renamed block, and Apple reinstalls
// those profiles on every scoped device.
func TestStoredLegacyPayloadIdentifiers_RenamedAndMovedBlockKeepsItsStep(t *testing.T) {
	t.Parallel()

	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(
		storedStep("A", map[string]string{"com.apple.domains": "IDENT-A"}),
		storedStep("B", map[string]string{"com.apple.domains": "IDENT-B"}),
	))

	resolved := stored.resolve([]types.String{types.StringValue("B"), types.StringValue("A renamed")})
	if got := resolved[0]["com.apple.domains"]; got != "IDENT-B" {
		t.Errorf("block 0 resolved to %q, want IDENT-B matched by name", got)
	}
	if got := resolved[1]["com.apple.domains"]; got != "IDENT-A" {
		t.Errorf("block 1 resolved to %q, want the unclaimed IDENT-A", got)
	}
}

// TestStoredLegacyPayloadIdentifiers_DuplicateBlockNamesFallBackToPosition covers renaming a block
// onto a sibling's name, which the schema permits: component_blocks[].name is optional and carries
// no uniqueness validator. A name two blocks carry can name neither step, or the first block to ask
// takes the step its namesake was continuing and writes that identifier onto a different profile's
// payloads while the namesake mints a replacement.
func TestStoredLegacyPayloadIdentifiers_DuplicateBlockNamesFallBackToPosition(t *testing.T) {
	t.Parallel()

	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(
		storedStep("A", map[string]string{"com.apple.domains": "IDENT-A"}),
		storedStep("B", map[string]string{"com.apple.domains": "IDENT-B"}),
	))

	resolved := stored.resolve([]types.String{types.StringValue("B"), types.StringValue("B")})
	if got := resolved[0]["com.apple.domains"]; got != "IDENT-A" {
		t.Errorf("block 0 resolved to %q, want the positional IDENT-A", got)
	}
	if got := resolved[1]["com.apple.domains"]; got != "IDENT-B" {
		t.Errorf("block 1 resolved to %q, want the positional IDENT-B", got)
	}
}

// emptyStoredStep is a stored step carrying no legacy configuration profile component, the shape a
// block without legacy payloads leaves behind.
func emptyStoredStep(name string) blueprints.BlueprintStep {
	return blueprints.BlueprintStep{Name: &name}
}

// storedStepSharingOneIdentifier is a stored step whose payloads all carry the same identifier, the
// shape an out-of-band edit can leave behind and the one macOS refuses outright.
func storedStepSharingOneIdentifier(name, identifier string, payloadTypes ...string) blueprints.BlueprintStep {
	payloads := make([]map[string]any, 0, len(payloadTypes))
	for _, payloadType := range payloadTypes {
		payloads = append(payloads, map[string]any{
			"payloadType":       payloadType,
			"payloadIdentifier": identifier,
			"payloadUUID":       identifier,
		})
	}
	configuration, err := json.Marshal(map[string]any{"payloadContent": payloads})
	if err != nil {
		panic(err)
	}
	return blueprints.BlueprintStep{
		Name:       &name,
		Components: []blueprints.Component{{Identifier: legacyConfigProfileIdentifier, Configuration: configuration}},
	}
}

// TestStoredLegacyPayloadIdentifiers_DuplicateIdentifierIsDropped covers a stored step holding one
// identifier on two payload types. macOS rejects such a profile whole — "the PayloadIdentifier is
// used more than once in the profile", ConfigProfilePluginDomain:-107, wire-verified on macOS 26.6
// on 2026-09-16, where neither payload installed and the device retried while the blueprint
// reported DEPLOYED and SUCCEEDED.
//
// The provider reads stored identifiers by payload type, so without this guard both types would map
// to the duplicate and the next update would write that unusable profile straight back, on every
// apply, for as long as the blueprint existed. Dropping both lets the service mint a distinct value
// for each, which costs nothing: a rotated identifier has no device effect.
func TestStoredLegacyPayloadIdentifiers_DuplicateIdentifierIsDropped(t *testing.T) {
	t.Parallel()

	const shared = "AAAA1111-2222-4333-8444-555566667777"
	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(
		storedStepSharingOneIdentifier("Shared", shared, "com.apple.domains", "com.apple.universalaccess"),
	))

	resolved := stored.resolve([]types.String{types.StringValue("Shared")})
	for _, payloadType := range []string{"com.apple.domains", "com.apple.universalaccess"} {
		if got, present := resolved[0][payloadType]; present {
			t.Errorf("%s resolved to %q, want the duplicate dropped so the service reissues it", payloadType, got)
		}
	}
	if len(stored.ambiguous) != 1 || stored.ambiguous[0] != shared {
		t.Errorf("ambiguous = %v, want exactly [%s] so the operator is told", stored.ambiguous, shared)
	}
}

// TestStoredLegacyPayloadIdentifiers_DistinctIdentifiersAreKept is the other half: the guard must
// not fire on the ordinary case. Two payload types with two identifiers is what every healthy
// blueprint holds, and dropping those would re-identify a whole blueprint on every update.
func TestStoredLegacyPayloadIdentifiers_DistinctIdentifiersAreKept(t *testing.T) {
	t.Parallel()

	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(storedStep("Shared", map[string]string{
		"com.apple.domains":         "IDENT-DOMAINS",
		"com.apple.universalaccess": "IDENT-ACCESS",
	})))

	resolved := stored.resolve([]types.String{types.StringValue("Shared")})
	if got := resolved[0]["com.apple.domains"]; got != "IDENT-DOMAINS" {
		t.Errorf("com.apple.domains resolved to %q, want IDENT-DOMAINS", got)
	}
	if got := resolved[0]["com.apple.universalaccess"]; got != "IDENT-ACCESS" {
		t.Errorf("com.apple.universalaccess resolved to %q, want IDENT-ACCESS", got)
	}
	if len(stored.ambiguous) != 0 {
		t.Errorf("ambiguous = %v, want empty: two distinct identifiers are the healthy case", stored.ambiguous)
	}
}
