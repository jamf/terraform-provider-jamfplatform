// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jamf/jamfplatform-go-sdk/jamfplatform/blueprints"
)

// appleDeclaration builds one declaration model on the SYSTEM channel.
func appleDeclaration(declarationType, payload string) AppleDeclarationModel {
	return AppleDeclarationModel{
		ChannelType: types.StringValue("SYSTEM"),
		Payload:     types.StringValue(payload),
		Type:        types.StringValue(declarationType),
	}
}

// buildAppleDeclarationsComponent runs the write path and returns the one component it produced,
// failing on any diagnostic so each case stays about the configuration it built.
func buildAppleDeclarationsComponent(t *testing.T, declarations ...AppleDeclarationModel) blueprints.Component {
	t.Helper()

	var built []blueprints.Component
	var diags diag.Diagnostics
	(&BlueprintResource{}).appendAppleDeclarations(&built, &diags, declarations)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags.Errors())
	}
	if len(built) != 1 {
		t.Fatalf("expected exactly one component, got %d", len(built))
	}
	if built[0].Identifier != appleDeclarationsIdentifier {
		t.Fatalf("identifier = %q, want %q", built[0].Identifier, appleDeclarationsIdentifier)
	}
	return built[0]
}

// wireDeclarations decodes a built configuration back into the wire declarations.
func wireDeclarations(t *testing.T, configuration json.RawMessage) []blueprints.CustomDeclaration {
	t.Helper()

	var config blueprints.CustomDeclarationsConfiguration
	if err := json.Unmarshal(configuration, &config); err != nil {
		t.Fatalf("decoding the built configuration: %v", err)
	}
	return config.Declarations
}

// TestAppendAppleDeclarationsDerivesPayloadKeyFromPosition pins the derivation a $PAYLOAD_n
// reference resolves against: the 1-based position, so reordering the list reorders the keys.
func TestAppendAppleDeclarationsDerivesPayloadKeyFromPosition(t *testing.T) {
	component := buildAppleDeclarationsComponent(t,
		appleDeclaration("com.apple.configuration.a", `{}`),
		appleDeclaration("com.apple.configuration.b", `{}`),
		appleDeclaration("com.apple.configuration.c", `{}`),
	)

	declarations := wireDeclarations(t, component.Configuration)
	if len(declarations) != 3 {
		t.Fatalf("expected 3 declarations, got %d", len(declarations))
	}
	for i, declaration := range declarations {
		if declaration.PayloadKey != i+1 {
			t.Errorf("declaration[%d]: payloadKey = %d, want %d", i, declaration.PayloadKey, i+1)
		}
	}
}

// TestAppendAppleDeclarationsDerivesKindFromType covers all four kinds, including the two the SDK's
// own DeclarationKindValues() does not declare and the gateway was observed to accept.
func TestAppendAppleDeclarationsDerivesKindFromType(t *testing.T) {
	cases := map[string]string{
		"com.apple.configuration.passcode.settings": "CONFIGURATION",
		"com.apple.asset.data":                      "ASSET",
		"com.apple.activation.simple":               "ACTIVATION",
		"com.apple.management.organization-info":    "MANAGEMENT",
	}

	for declarationType, wantKind := range cases {
		t.Run(declarationType, func(t *testing.T) {
			component := buildAppleDeclarationsComponent(t, appleDeclaration(declarationType, `{}`))
			declarations := wireDeclarations(t, component.Configuration)
			if len(declarations) != 1 {
				t.Fatalf("expected 1 declaration, got %d", len(declarations))
			}
			if declarations[0].Kind != wantKind {
				t.Errorf("kind = %q, want %q", declarations[0].Kind, wantKind)
			}
		})
	}
}

// TestAppendAppleDeclarationsWritesNothingForAnEmptyList pins that an empty list writes no
// component. Writing one would create a component the configuration never asked for, which state
// would then have to keep.
func TestAppendAppleDeclarationsWritesNothingForAnEmptyList(t *testing.T) {
	for name, declarations := range map[string][]AppleDeclarationModel{
		"nil":   nil,
		"empty": {},
	} {
		t.Run(name, func(t *testing.T) {
			var built []blueprints.Component
			var diags diag.Diagnostics
			(&BlueprintResource{}).appendAppleDeclarations(&built, &diags, declarations)

			if len(diags) != 0 {
				t.Errorf("unexpected diagnostics: %v", diags)
			}
			if len(built) != 0 {
				t.Errorf("expected no component, got %+v", built)
			}
		})
	}
}

// TestAppendAppleDeclarationsRejectsAnUnparseablePayload checks the write path reports a payload
// that is not JSON, naming the declaration type. The plan-time validator catches a configured value
// first; this is the backstop for one it could not check, such as a payload known only at apply.
func TestAppendAppleDeclarationsRejectsAnUnparseablePayload(t *testing.T) {
	var built []blueprints.Component
	var diags diag.Diagnostics
	(&BlueprintResource{}).appendAppleDeclarations(&built, &diags,
		[]AppleDeclarationModel{appleDeclaration("com.apple.configuration.siri.settings", "not-valid-json")})

	if !diags.HasError() {
		t.Fatal("an unparseable payload produced no error")
	}
	if len(built) != 0 {
		t.Errorf("expected no component to be written, got %+v", built)
	}
	detail := diags.Errors()[0].Detail()
	for _, want := range []string{"com.apple.configuration.siri.settings", "jsonencode"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail does not name %q: %s", want, detail)
		}
	}
}

// declarationComponents keys one built component by its identifier, the shape the read path takes.
func declarationComponents(configuration string) map[string]blueprints.Component {
	return map[string]blueprints.Component{
		appleDeclarationsIdentifier: {
			Identifier:    appleDeclarationsIdentifier,
			Configuration: json.RawMessage(configuration),
		},
	}
}

// TestFlattenAppleDeclarationsKeepsTheAuthoredPayloadFormatting covers the reconciliation that
// makes file() usable. The platform re-serialises what it stored, compact with keys sorted, so
// without keeping the author's bytes an indented payload would be rewritten in state on the first
// read and diff on every plan after it.
func TestFlattenAppleDeclarationsKeepsTheAuthoredPayloadFormatting(t *testing.T) {
	authored := "{\n  \"ForceProfanityFilter\": true,\n  \"Enabled\": true\n}\n"
	prior := []AppleDeclarationModel{appleDeclaration("com.apple.configuration.siri.settings", authored)}

	var diags diag.Diagnostics
	declarations := flattenAppleDeclarations(&diags,
		prior,
		declarationComponents(`{"declarations":[{"channelType":"SYSTEM","kind":"CONFIGURATION","payloadKey":1,"type":"com.apple.configuration.siri.settings","payload":{"Enabled":true,"ForceProfanityFilter":true}}]}`),
		nil,
	)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags.Errors())
	}
	if len(declarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(declarations))
	}
	if got := declarations[0].Payload.ValueString(); got != authored {
		t.Errorf("payload was rewritten:\n want %q\n  got %q", authored, got)
	}
}

// TestFlattenAppleDeclarationsKeepsAnAuthoredExplicitNull pins the reconciliation against the
// declarations service's own treatment of a null. A legacy configuration profile payload's
// null-valued key is dropped by the service, but a declaration payload's is stored and echoed back
// verbatim — EU gateway, 2026-09-12 — so the comparison has to prune both sides. Pruning only the
// authored side mismatches here, state takes the canonical encoding instead of the authored bytes,
// and because payload is Required the framework rejects the apply as an inconsistent result.
func TestFlattenAppleDeclarationsKeepsAnAuthoredExplicitNull(t *testing.T) {
	authored := "{\n  \"Enabled\": true,\n  \"ForceProfanityFilter\": null\n}\n"
	prior := []AppleDeclarationModel{appleDeclaration("com.apple.configuration.siri.settings", authored)}

	var diags diag.Diagnostics
	declarations := flattenAppleDeclarations(&diags,
		prior,
		declarationComponents(`{"declarations":[{"channelType":"SYSTEM","kind":"CONFIGURATION","payloadKey":1,"type":"com.apple.configuration.siri.settings","payload":{"Enabled":true,"ForceProfanityFilter":null}}]}`),
		nil,
	)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags.Errors())
	}
	if len(declarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(declarations))
	}
	if got := declarations[0].Payload.ValueString(); got != authored {
		t.Errorf("payload was rewritten:\n want %q\n  got %q", authored, got)
	}
}

// TestFlattenAppleDeclarationsAlignsPriorByPosition pins that a prior payload is matched by
// position, not by declaration type. Two declarations of one type in a component are legal, so
// matching by type would reconcile the wrong one.
func TestFlattenAppleDeclarationsAlignsPriorByPosition(t *testing.T) {
	first := "{\n  \"Enabled\": true\n}"
	second := "{\n  \"Enabled\": false\n}"
	prior := []AppleDeclarationModel{
		appleDeclaration("com.apple.configuration.siri.settings", first),
		appleDeclaration("com.apple.configuration.siri.settings", second),
	}

	var diags diag.Diagnostics
	declarations := flattenAppleDeclarations(&diags,
		prior,
		declarationComponents(`{"declarations":[{"channelType":"SYSTEM","type":"com.apple.configuration.siri.settings","payload":{"Enabled":true}},{"channelType":"SYSTEM","type":"com.apple.configuration.siri.settings","payload":{"Enabled":false}}]}`),
		nil,
	)

	if len(declarations) != 2 {
		t.Fatalf("expected 2 declarations, got %d", len(declarations))
	}
	if got := declarations[0].Payload.ValueString(); got != first {
		t.Errorf("declaration[0] payload = %q, want %q", got, first)
	}
	if got := declarations[1].Payload.ValueString(); got != second {
		t.Errorf("declaration[1] payload = %q, want %q", got, second)
	}
}

// TestFlattenAppleDeclarationsCanonicalisesWithoutAPrior covers the import path, where there is no
// authored value to keep. The canonical encoding has to match what jsonencode() emits for the same
// object, HTML escaping included, or the first plan after an import diffs on formatting alone.
func TestFlattenAppleDeclarationsCanonicalisesWithoutAPrior(t *testing.T) {
	var diags diag.Diagnostics
	declarations := flattenAppleDeclarations(&diags,
		nil,
		declarationComponents(`{"declarations":[{"channelType":"USER","type":"com.apple.management.organization-info","payload":{"Name":"Example & Co <EMEA>"}}]}`),
		nil,
	)

	if len(declarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(declarations))
	}
	if got, want := declarations[0].Payload.ValueString(), `{"Name":"Example \u0026 Co \u003cEMEA\u003e"}`; got != want {
		t.Errorf("payload = %s, want %s", got, want)
	}
	if got := declarations[0].ChannelType.ValueString(); got != "USER" {
		t.Errorf("channel = %q, want USER", got)
	}
}

// TestFlattenAppleDeclarationsDropsDerivedWireFields pins that kind and payloadKey never reach
// state. Both are derived on write, so reading either back would let a value the platform echoed
// disagree with the position and type that produced it.
func TestFlattenAppleDeclarationsDropsDerivedWireFields(t *testing.T) {
	var diags diag.Diagnostics
	declarations := flattenAppleDeclarations(&diags,
		nil,
		declarationComponents(`{"declarations":[{"channelType":"SYSTEM","kind":"CONFIGURATION","payloadKey":7,"type":"com.apple.asset.data","payload":{"a":1}}]}`),
		nil,
	)

	if len(declarations) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(declarations))
	}

	component := buildAppleDeclarationsComponent(t, declarations...)
	wire := wireDeclarations(t, component.Configuration)
	if wire[0].Kind != "ASSET" {
		t.Errorf("kind = %q, want it re-derived as ASSET from the type", wire[0].Kind)
	}
	if wire[0].PayloadKey != 1 {
		t.Errorf("payloadKey = %d, want it re-derived as 1 from the position", wire[0].PayloadKey)
	}
}

// TestFlattenAppleDeclarationsAbsentAndEmpty pins the shapes that read as no declarations. An empty
// declaration list has to read as absent: an empty list writes no component, so a non-null empty
// list in state could only ever diff.
func TestFlattenAppleDeclarationsAbsentAndEmpty(t *testing.T) {
	cases := map[string]map[string]blueprints.Component{
		"component absent":   {},
		"no declarations":    declarationComponents(`{"declarations":[]}`),
		"empty object":       declarationComponents(`{}`),
		"null configuration": {appleDeclarationsIdentifier: {Identifier: appleDeclarationsIdentifier}},
	}

	for name, apiComponents := range cases {
		t.Run(name, func(t *testing.T) {
			var diags diag.Diagnostics
			if declarations := flattenAppleDeclarations(&diags, nil, apiComponents, nil); declarations != nil {
				t.Errorf("expected the attribute to read as absent, got %+v", declarations)
			}
			if len(diags) != 0 {
				t.Errorf("unexpected diagnostics: %v", diags)
			}
		})
	}
}

// TestFlattenAppleDeclarationsLeavesRawComponentAlone pins the escape hatch's read side. A
// component managed as a raw_component must not also appear here, or it would be written twice and
// the apply would fail as inconsistent.
func TestFlattenAppleDeclarationsLeavesRawComponentAlone(t *testing.T) {
	var diags diag.Diagnostics
	declarations := flattenAppleDeclarations(&diags,
		nil,
		declarationComponents(`{"declarations":[{"channelType":"SYSTEM","type":"com.apple.configuration.siri.settings","payload":{"Enabled":true}}]}`),
		map[string]struct{}{appleDeclarationsIdentifier: {}},
	)

	if declarations != nil {
		t.Errorf("a raw-managed component must not populate apple_declarations, got %+v", declarations)
	}
}

// TestFlattenAppleDeclarationsWarnsOnAnUndecodableConfiguration pins that a configuration this
// shape cannot read is reported. The attribute is Optional without being Computed, so leaving it
// silently absent makes a configuration that declares it propose adding it on every plan.
func TestFlattenAppleDeclarationsWarnsOnAnUndecodableConfiguration(t *testing.T) {
	var diags diag.Diagnostics
	declarations := flattenAppleDeclarations(&diags, nil, declarationComponents(`{"declarations":"not-a-list"}`), nil)

	if declarations != nil {
		t.Errorf("expected no declarations, got %+v", declarations)
	}
	if diags.HasError() {
		t.Fatalf("an undecodable configuration must not fail the read: %v", diags.Errors())
	}
	warnings := diags.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one", warnings)
	}
	if !strings.Contains(warnings[0].Detail(), appleDeclarationsIdentifier) {
		t.Errorf("warning does not name the component: %s", warnings[0].Detail())
	}
}
