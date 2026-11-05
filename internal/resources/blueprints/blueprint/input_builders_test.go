// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

// blockPayloadsFrom turns payloads written as plain Go values into the typed list a component
// block carries, encoding each settings object the way jsonencode() would.
func blockPayloadsFrom(t *testing.T, input []any) []BlockLegacyPayloadModel {
	t.Helper()

	payloads := make([]BlockLegacyPayloadModel, 0, len(input))
	for _, item := range input {
		object := item.(map[string]any)
		payload := BlockLegacyPayloadModel{
			PayloadType: types.StringValue(object["payload_type"].(string)),
			Settings:    types.StringNull(),
		}
		if settings, ok := object["settings"]; ok {
			encoded, err := json.Marshal(settings)
			if err != nil {
				t.Fatalf("encoding settings: %v", err)
			}
			payload.Settings = types.StringValue(string(encoded))
		}
		payloads = append(payloads, payload)
	}
	return payloads
}

func TestCollectBlockLegacyPayloads_ValidPayload(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	input := []any{
		map[string]any{
			"payload_type": "com.apple.applicationaccess",
			"settings": map[string]any{
				"allowSafariHistoryClearing": false,
				"allowSafariPrivateBrowsing": false,
			},
		},
	}
	payloads := blockPayloadsFrom(t, input)

	r.collectBlockLegacyPayloads(&components, &diags, payloads, "My Blueprint", "component_blocks[0]", nil)

	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}
	if components[0].Identifier != "com.jamf.ddm-configuration-profile" {
		t.Errorf("expected identifier 'com.jamf.ddm-configuration-profile', got %q", components[0].Identifier)
	}

	var config map[string]any
	if err := json.Unmarshal(components[0].Configuration, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	if config["payloadDisplayName"] != "My Blueprint" {
		t.Errorf("expected payloadDisplayName 'My Blueprint', got %v", config["payloadDisplayName"])
	}

	payloadContent, ok := config["payloadContent"].([]any)
	if !ok {
		t.Fatal("expected payloadContent to be an array")
	}
	if len(payloadContent) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(payloadContent))
	}

	payload := payloadContent[0].(map[string]any)
	if payload["payloadType"] != "com.apple.applicationaccess" {
		t.Errorf("expected payloadType 'com.apple.applicationaccess', got %v", payload["payloadType"])
	}
	if payload["allowSafariHistoryClearing"] != false {
		t.Errorf("expected allowSafariHistoryClearing false, got %v", payload["allowSafariHistoryClearing"])
	}
}

func TestCollectBlockLegacyPayloads_NoSettings(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	input := []any{
		map[string]any{
			"payload_type": "com.apple.wifi.managed",
		},
	}
	payloads := blockPayloadsFrom(t, input)

	r.collectBlockLegacyPayloads(&components, &diags, payloads, "Blueprint", "component_blocks[0]", nil)

	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}

	var config map[string]any
	if err := json.Unmarshal(components[0].Configuration, &config); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}
	payloadContent := config["payloadContent"].([]any)
	payload := payloadContent[0].(map[string]any)
	if payload["payloadType"] != "com.apple.wifi.managed" {
		t.Errorf("expected payloadType 'com.apple.wifi.managed', got %v", payload["payloadType"])
	}
}

func TestCollectBlockLegacyPayloads_MixedTypeSettings(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	input := []any{
		map[string]any{
			"payload_type": "com.apple.applicationaccess",
			"settings": map[string]any{
				"boolSetting":   true,
				"stringSetting": "hello",
				"numberSetting": float64(42),
			},
		},
	}
	payloads := blockPayloadsFrom(t, input)

	r.collectBlockLegacyPayloads(&components, &diags, payloads, "Test", "component_blocks[0]", nil)

	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}

	var config map[string]any
	if err := json.Unmarshal(components[0].Configuration, &config); err != nil {
		t.Fatalf("failed to unmarshal configuration: %v", err)
	}
	payloadContent := config["payloadContent"].([]any)
	payload := payloadContent[0].(map[string]any)

	if payload["boolSetting"] != true {
		t.Errorf("expected boolSetting true, got %v", payload["boolSetting"])
	}
	if payload["stringSetting"] != "hello" {
		t.Errorf("expected stringSetting 'hello', got %v", payload["stringSetting"])
	}
	if payload["numberSetting"] != float64(42) {
		t.Errorf("expected numberSetting 42, got %v", payload["numberSetting"])
	}
}

func TestCollectBlockLegacyPayloads_DuplicatePayloadType(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	input := []any{
		map[string]any{
			"payload_type": "com.apple.wifi.managed",
			"settings": map[string]any{
				"networkName": "Office",
			},
		},
		map[string]any{
			"payload_type": "com.apple.wifi.managed",
			"settings": map[string]any{
				"networkName": "Guest",
			},
		},
	}
	payloads := blockPayloadsFrom(t, input)

	r.collectBlockLegacyPayloads(&components, &diags, payloads, "Blueprint", "component_blocks[0]", nil)

	if !diags.HasError() {
		t.Fatal("expected error for duplicate payload_type, got none")
	}

	found := false
	for _, d := range diags.Errors() {
		if d.Summary() == "Duplicate payload_type" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'Duplicate payload_type' error, got: %v", diags)
	}
}

// legacyStepFixture builds one stored step whose legacy configuration profile payloads already
// carry a service-assigned identifier. The fixtures below are local to this file rather than shared
// with the identifier suite, so neither file's helpers can change what the other sends.
func legacyStepFixture(t *testing.T, name string, payloadTypeToIdentifier map[string]string) blueprints.BlueprintStep {
	t.Helper()

	payloads := make([]map[string]any, 0, len(payloadTypeToIdentifier))
	for payloadType, identifier := range payloadTypeToIdentifier {
		payloads = append(payloads, map[string]any{"payloadType": payloadType, "payloadIdentifier": identifier})
	}
	configuration, err := json.Marshal(map[string]any{"payloadContent": payloads})
	if err != nil {
		t.Fatalf("marshal stored step configuration: %v", err)
	}

	step := blueprints.BlueprintStep{
		Components: []blueprints.Component{{Identifier: legacyConfigProfileIdentifier, Configuration: configuration}},
	}
	if name != "" {
		step.Name = &name
	}
	return step
}

// sentLegacyPayloads decodes the legacy configuration profile component a built step carries, keyed
// by payload type, so a test asserts on the payload map rather than on a JSON string.
func sentLegacyPayloads(t *testing.T, step blueprints.BlueprintStep) map[string]map[string]any {
	t.Helper()

	sent := make(map[string]map[string]any)
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
			sent[payloadType] = payload
		}
	}
	return sent
}

// serverStampedKeysIn reports every key of a sent payload the service owns, whatever its case, so a
// test can assert on how many arrived rather than on one spelling.
func serverStampedKeysIn(payload map[string]any) map[string]any {
	stamped := make(map[string]any)
	for key, value := range payload {
		if strings.EqualFold(key, "payloadIdentifier") || strings.EqualFold(key, "payloadUUID") {
			stamped[key] = value
		}
	}
	return stamped
}

// TestBuildSteps_ServerStampedPayloadKeysAreDroppedWhateverTheCase covers the spelling the provider
// itself tells an author to use. appleprofiles.Validate rejects a lowercase payloadIdentifier as a
// miscased key and names Apple's PayloadIdentifier in the fix, so the capitalised form is the one an
// author ends up writing — and it must not ride to the wire beside the provider's own write-back.
// payloadUUID is dropped for a different reason: the service reassigns it on every write, so an
// authored value is never honoured.
func TestBuildSteps_ServerStampedPayloadKeysAreDroppedWhateverTheCase(t *testing.T) {
	t.Parallel()

	const payloadType = "com.apple.domains"
	settings := map[string]any{
		"EmailDomains":      []any{"example.com"},
		"PayloadIdentifier": "AUTHORED-APPLE-SPELLING",
		"payloadidentifier": "AUTHORED-LOWER",
		"PAYLOADIDENTIFIER": "AUTHORED-UPPER",
		"payloadUUID":       "AUTHORED-UUID",
		"PayloadUUID":       "AUTHORED-UUID-APPLE-SPELLING",
	}

	for _, mode := range []struct {
		name string
		data func(*testing.T) *BlueprintResourceModel
	}{
		{
			name: "block mode",
			data: func(t *testing.T) *BlueprintResourceModel {
				t.Helper()
				encoded, err := json.Marshal(settings)
				if err != nil {
					t.Fatalf("marshal settings: %v", err)
				}
				return &BlueprintResourceModel{
					Name: types.StringValue("BP"),
					ComponentBlocks: []ComponentBlockModel{{
						Name: types.StringValue("Block 1"),
						LegacyPayloads: []BlockLegacyPayloadModel{{
							PayloadType: types.StringValue(payloadType),
							Settings:    types.StringValue(string(encoded)),
						}},
					}},
				}
			},
		},
	} {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()

			t.Run("create sends no identifier at all", func(t *testing.T) {
				t.Parallel()

				steps, diags := (&BlueprintResource{}).buildSteps(context.Background(), mode.data(t), nil)
				if diags.HasError() {
					t.Fatalf("unexpected diagnostics: %v", diags)
				}

				payload := sentLegacyPayloads(t, steps[0])[payloadType]
				if stamped := serverStampedKeysIn(payload); len(stamped) != 0 {
					t.Errorf("create sent server-owned keys %v, want none", stamped)
				}
				if payload["EmailDomains"] == nil {
					t.Error("authored settings did not survive the drop")
				}
			})

			t.Run("update sends the stored identifier only", func(t *testing.T) {
				t.Parallel()

				stored := newStoredLegacyPayloadIdentifiers(&blueprints.BlueprintDetail{
					Steps: []blueprints.BlueprintStep{legacyStepFixture(t, "Block 1", map[string]string{payloadType: "STORED-IDENT"})},
				})

				steps, diags := (&BlueprintResource{}).buildSteps(context.Background(), mode.data(t), stored)
				if diags.HasError() {
					t.Fatalf("unexpected diagnostics: %v", diags)
				}

				stamped := serverStampedKeysIn(sentLegacyPayloads(t, steps[0])[payloadType])
				if len(stamped) != 1 {
					t.Fatalf("update sent server-owned keys %v, want only payloadIdentifier", stamped)
				}
				if got := stamped["payloadIdentifier"]; got != "STORED-IDENT" {
					t.Errorf("server-owned keys = %v, want payloadIdentifier STORED-IDENT", stamped)
				}
			})
		})
	}
}

// TestHasLegacyPayloads pins the gate that decides whether Update reads the blueprint before
// building its request. It has to agree with buildSteps on every input.
func TestHasLegacyPayloads(t *testing.T) {
	t.Parallel()

	blockWithPayload := ComponentBlockModel{
		Name: types.StringValue("Block 1"),
		LegacyPayloads: []BlockLegacyPayloadModel{{
			PayloadType: types.StringValue("com.apple.domains"),
			Settings:    types.StringValue(`{"EmailDomains":["example.com"]}`),
		}},
	}

	for _, tc := range []struct {
		name  string
		model BlueprintResourceModel
		want  bool
	}{
		{name: "empty model", model: BlueprintResourceModel{}},
		{
			name:  "payloads in a block",
			model: BlueprintResourceModel{ComponentBlocks: []ComponentBlockModel{blockWithPayload}},
			want:  true,
		},
		{
			name:  "blocks without payloads",
			model: BlueprintResourceModel{ComponentBlocks: []ComponentBlockModel{{Name: types.StringValue("Block 1")}}},
		},
		{
			name: "payloads in a later block only",
			model: BlueprintResourceModel{ComponentBlocks: []ComponentBlockModel{
				{Name: types.StringValue("Block 1")},
				blockWithPayload,
			}},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.model.hasLegacyPayloads(); got != tc.want {
				t.Errorf("hasLegacyPayloads() = %t, want %t", got, tc.want)
			}
		})
	}
}

// buildBlockLegacyPayloads runs one component block's legacy payloads through the builder and
// returns its diagnostics, which is where every duplicate is caught.
// duplicateTestBlockLocation is how describeBlockPosition renders the block buildBlockLegacyPayloads
// builds. Both duplicate diagnostics are raised below the validators and so carry no attribute path,
// which is why they have to name the block in their own text.
const duplicateTestBlockLocation = "component_blocks[0] (Block 1)"

func buildBlockLegacyPayloads(t *testing.T, payloads ...[2]string) diag.Diagnostics {
	t.Helper()

	data := &BlueprintResourceModel{
		Name:            types.StringValue("BP"),
		ComponentBlocks: []ComponentBlockModel{blockWithLegacyPayloads("Block 1", payloads...)},
	}
	_, diags := (&BlueprintResource{}).buildSteps(context.Background(), data, nil)
	return diags
}

// TestBuildSteps_SamePreferenceDomainTwiceIsRejected is the duplicate that survives the fix. Several
// custom settings payloads in one block are now legitimate, one per preference domain, so the check
// moved from the payload type onto the domain rather than being removed: two payloads setting the
// same domain leave the last write to decide what a Mac gets, which is not something to send.
func TestBuildSteps_SamePreferenceDomainTwiceIsRejected(t *testing.T) {
	t.Parallel()

	diags := buildBlockLegacyPayloads(t,
		[2]string{mcxPayloadType, mcxSettings(t, "com.example.repeated")},
		[2]string{mcxPayloadType, mcxSettings(t, "com.example.repeated")},
	)
	if !diags.HasError() {
		t.Fatal("the same preference domain in two payloads of one block was accepted")
	}

	reported := diags.Errors()[0]
	if reported.Summary() != "Duplicate preference domain" {
		t.Errorf("summary = %q, want the domain named rather than the payload type", reported.Summary())
	}
	if !strings.Contains(reported.Detail(), "com.example.repeated") {
		t.Errorf("detail does not name the repeated domain: %s", reported.Detail())
	}
	if !strings.Contains(reported.Detail(), duplicateTestBlockLocation) {
		t.Errorf("detail does not name the component block: %s", reported.Detail())
	}
}

// TestBuildSteps_TwoDomainlessCustomSettingsPayloadsAreRejected covers the payload an empty
// preference dictionary produces. No author can write one: appendMCXDomainProblems refuses it during
// plan, because Jamf discards an empty dictionary and the shape cannot round-trip. This exercises
// appendLegacyConfigProfile directly, below the validator, where the payloads can also come from the
// wire — a domainless payload has no domain to be identified by and falls back to its payload type,
// which is what leaves the duplicate check able to see two of them, where keying on a domain that is
// not there would let both through and send two payloads the service stamps as one.
func TestBuildSteps_TwoDomainlessCustomSettingsPayloadsAreRejected(t *testing.T) {
	t.Parallel()

	diags := buildBlockLegacyPayloads(t,
		[2]string{mcxPayloadType, mcxSettings(t)},
		[2]string{mcxPayloadType, mcxSettings(t)},
	)
	if !diags.HasError() {
		t.Fatal("two custom settings payloads with no preference domain were accepted")
	}
	reported := diags.Errors()[0]
	if reported.Summary() != "Duplicate payload_type" {
		t.Errorf("summary = %q, want the payload type named — there is no domain to name", reported.Summary())
	}
	if !strings.Contains(reported.Detail(), duplicateTestBlockLocation) {
		t.Errorf("detail does not name the component block: %s", reported.Detail())
	}
}

// TestBuildSteps_RepeatedPayloadTypeInABlockIsStillRejected keeps the rule the fix relaxed for one
// payload type from being relaxed for the rest. Every other type appears at most once in a block,
// since a second one carries no domain to tell it apart and would take the first's stored identifier.
func TestBuildSteps_RepeatedPayloadTypeInABlockIsStillRejected(t *testing.T) {
	t.Parallel()

	diags := buildBlockLegacyPayloads(t,
		[2]string{"com.apple.domains", `{"EmailDomains":["first.example"]}`},
		[2]string{"com.apple.domains", `{"EmailDomains":["second.example"]}`},
	)
	if !diags.HasError() {
		t.Fatal("a repeated payload type in one block was accepted")
	}

	reported := diags.Errors()[0]
	if reported.Summary() != "Duplicate payload_type" {
		t.Errorf("summary = %q", reported.Summary())
	}
	if !strings.Contains(reported.Detail(), "com.apple.domains") {
		t.Errorf("detail does not name the repeated type: %s", reported.Detail())
	}
	if !strings.Contains(reported.Detail(), duplicateTestBlockLocation) {
		t.Errorf("detail does not name the component block: %s", reported.Detail())
	}
}
