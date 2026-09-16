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

// TestStoredLegacyPayloadIdentifiers_DuplicateStepNamesFallBackToPosition covers two blocks sharing
// a name, which the schema permits. An ambiguous name must not resolve to whichever step was read
// first, so both drop to a positional match.
func TestStoredLegacyPayloadIdentifiers_DuplicateStepNamesFallBackToPosition(t *testing.T) {
	t.Parallel()

	stored := newStoredLegacyPayloadIdentifiers(storedBlueprint(
		storedStep("Same", map[string]string{"com.apple.domains": "FIRST"}),
		storedStep("Same", map[string]string{"com.apple.domains": "SECOND"}),
	))

	resolved := stored.resolve([]types.String{types.StringValue("Same"), types.StringValue("Same")})
	if got := resolved[0]["com.apple.domains"]; got != "FIRST" {
		t.Errorf("block 0 resolved to %q, want the positional FIRST", got)
	}
	if got := resolved[1]["com.apple.domains"]; got != "SECOND" {
		t.Errorf("block 1 resolved to %q, want the positional SECOND", got)
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

// TestStoredLegacyPayloadIdentifiers_ResolveNeverReusesAStep is the invariant behind the two-pass
// match: a stored step feeds at most one block, whatever mix of named, renamed, inserted and
// unnamed blocks it is asked about. Handing one step to two blocks would put one identifier in two
// profiles, which collides on the device rather than merely churning.
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
	}{
		{"insert ahead of a named block", []types.String{types.StringValue("Inserted"), types.StringValue("First"), types.StringValue("Second")}},
		{"reordered", []types.String{types.StringValue("Third"), types.StringValue("First"), types.StringValue("Second")}},
		{"renamed middle block", []types.String{types.StringValue("First"), types.StringValue("Renamed"), types.StringValue("Third")}},
		{"all unnamed", []types.String{types.StringNull(), types.StringNull(), types.StringNull()}},
		{"more blocks than steps", []types.String{types.StringValue("First"), types.StringNull(), types.StringNull(), types.StringNull()}},
		{"fewer blocks than steps", []types.String{types.StringValue("Third")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			seen := make(map[string]int)
			for i, identifiers := range stored.resolve(tc.blockNames) {
				identifier, ok := identifiers["com.apple.domains"]
				if !ok {
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

//go:fix inline
// emptyStoredStep is a stored step carrying no legacy configuration profile component, the shape a
// block without legacy payloads leaves behind.
func emptyStoredStep(name string) blueprints.BlueprintStep {
	return blueprints.BlueprintStep{Name: &name}
}
