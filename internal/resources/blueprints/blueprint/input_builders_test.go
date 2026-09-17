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
	"github.com/jamf/terraform-provider-jamfplatform/internal/common/helpers"
)

func TestCollectLegacyPayloads_ValidPayload(t *testing.T) {
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
	dynVal, _ := helpers.JSONToTerraformDynamic(input)

	r.collectLegacyPayloads(&components, &diags, dynVal, "My Blueprint", "legacy_payloads", nil)

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

func TestCollectLegacyPayloads_NoSettings(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	input := []any{
		map[string]any{
			"payload_type": "com.apple.wifi.managed",
		},
	}
	dynVal, _ := helpers.JSONToTerraformDynamic(input)

	r.collectLegacyPayloads(&components, &diags, dynVal, "Blueprint", "legacy_payloads", nil)

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

func TestCollectLegacyPayloads_MixedTypeSettings(t *testing.T) {
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
	dynVal, _ := helpers.JSONToTerraformDynamic(input)

	r.collectLegacyPayloads(&components, &diags, dynVal, "Test", "legacy_payloads", nil)

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

func TestCollectLegacyPayloads_EmptyList(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	dynVal, _ := helpers.JSONToTerraformDynamic([]any{})

	r.collectLegacyPayloads(&components, &diags, dynVal, "Empty Blueprint", "legacy_payloads", nil)

	if diags.HasError() {
		t.Fatalf("unexpected error: %v", diags)
	}
	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}

	var config map[string]any
	if err := json.Unmarshal(components[0].Configuration, &config); err != nil {
		t.Fatalf("failed to unmarshal configuration: %v", err)
	}
	payloadContent, ok := config["payloadContent"].([]any)
	if !ok {
		t.Fatal("expected payloadContent to be an array")
	}
	if len(payloadContent) != 0 {
		t.Errorf("expected empty payload content, got %d items", len(payloadContent))
	}
}

func TestCollectLegacyPayloads_DuplicatePayloadType(t *testing.T) {
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
	dynVal, _ := helpers.JSONToTerraformDynamic(input)

	r.collectLegacyPayloads(&components, &diags, dynVal, "Blueprint", "legacy_payloads", nil)

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

func TestCollectLegacyPayloads_NullDynamic(t *testing.T) {
	r := &BlueprintResource{}
	var components []blueprints.Component
	var diags diag.Diagnostics

	r.collectLegacyPayloads(&components, &diags, types.DynamicNull(), "Blueprint", "legacy_payloads", nil)

	if !diags.HasError() {
		t.Error("expected error for null dynamic value")
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

// flatModeLegacyPayloads builds the deprecated top-level legacy_payloads value for one payload.
func flatModeLegacyPayloads(t *testing.T, payloadType string, settings map[string]any) types.Dynamic {
	t.Helper()

	value, err := helpers.JSONToTerraformDynamic([]any{map[string]any{"payload_type": payloadType, "settings": settings}})
	if err != nil {
		t.Fatalf("building legacy_payloads value: %v", err)
	}
	return value
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

// TestBuildSteps_FlatModeTakesStepZeroIdentifiers pins flat mode's resolution to position. Flat mode
// writes one step and reads blueprint.Steps[0] back, while resolve's name pass matches a name at any
// index — so resolving by the flatStepName constant would hand the collapsed component the
// identifiers of a step flat mode does not manage as soon as any stored step happens to carry that
// name, and the read path allows that by only warning about the extra steps.
func TestBuildSteps_FlatModeTakesStepZeroIdentifiers(t *testing.T) {
	t.Parallel()

	const payloadType = "com.apple.domains"

	stepZero := legacyStepFixture(t, flatStepName, map[string]string{payloadType: "STEP-ZERO"})
	renamedStepZero := legacyStepFixture(t, "Renamed in the Jamf web app", map[string]string{payloadType: "STEP-ZERO"})
	laterStep := legacyStepFixture(t, flatStepName, map[string]string{payloadType: "WRONG-STEP"})

	for _, tc := range []struct {
		name  string
		steps []blueprints.BlueprintStep
	}{
		{name: "step zero carries the flat step name", steps: []blueprints.BlueprintStep{stepZero}},
		{name: "step zero is named something else", steps: []blueprints.BlueprintStep{renamedStepZero}},
		{name: "a later step carries the flat step name", steps: []blueprints.BlueprintStep{renamedStepZero, laterStep}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data := &BlueprintResourceModel{
				Name:           types.StringValue("BP"),
				LegacyPayloads: flatModeLegacyPayloads(t, payloadType, map[string]any{"EmailDomains": []any{"example.com"}}),
			}
			stored := newStoredLegacyPayloadIdentifiers(&blueprints.BlueprintDetail{Steps: tc.steps})

			steps, diags := (&BlueprintResource{}).buildSteps(context.Background(), data, stored)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if len(steps) != 1 {
				t.Fatalf("flat mode built %d steps, want 1", len(steps))
			}

			if got := sentLegacyPayloads(t, steps[0])[payloadType]["payloadIdentifier"]; got != "STEP-ZERO" {
				t.Errorf("payloadIdentifier = %v, want step zero's STEP-ZERO", got)
			}
		})
	}
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
		{
			name: "flat mode",
			data: func(t *testing.T) *BlueprintResourceModel {
				t.Helper()
				return &BlueprintResourceModel{
					Name:           types.StringValue("BP"),
					LegacyPayloads: flatModeLegacyPayloads(t, payloadType, settings),
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
// building its request. It has to agree with buildSteps on every input, including the precedence:
// component_blocks displaces the deprecated flat attribute rather than adding to it, so a model
// carrying blocks never consults the flat value.
func TestHasLegacyPayloads(t *testing.T) {
	t.Parallel()

	flatValue := flatModeLegacyPayloads(t, "com.apple.domains", map[string]any{"EmailDomains": []any{"example.com"}})
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
		{name: "flat mode with payloads", model: BlueprintResourceModel{LegacyPayloads: flatValue}, want: true},
		{name: "flat mode without payloads", model: BlueprintResourceModel{LegacyPayloads: types.DynamicNull()}},
		{name: "flat mode with an unknown value", model: BlueprintResourceModel{LegacyPayloads: types.DynamicUnknown()}},
		{
			name:  "block mode with payloads",
			model: BlueprintResourceModel{ComponentBlocks: []ComponentBlockModel{blockWithPayload}},
			want:  true,
		},
		{
			name:  "block mode without payloads",
			model: BlueprintResourceModel{ComponentBlocks: []ComponentBlockModel{{Name: types.StringValue("Block 1")}}},
		},
		{
			name: "block mode with payloads in a later block only",
			model: BlueprintResourceModel{ComponentBlocks: []ComponentBlockModel{
				{Name: types.StringValue("Block 1")},
				blockWithPayload,
			}},
			want: true,
		},
		{
			name: "block mode ignores a leftover flat value",
			model: BlueprintResourceModel{
				ComponentBlocks: []ComponentBlockModel{{Name: types.StringValue("Block 1")}},
				LegacyPayloads:  flatValue,
			},
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
