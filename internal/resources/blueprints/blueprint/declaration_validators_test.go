// Copyright Jamf Software LLC 2026
// SPDX-License-Identifier: MPL-2.0

package blueprint

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	appledeclarations "github.com/jamf/jamfplatform-go-sdk/appleschemas/declarations"
)

// declarationPath is a representative attribute path, so a failure message shows where a diagnostic
// would land in a real configuration.
func declarationPath() path.Path {
	return path.Root("component_blocks").AtListIndex(0).
		AtName("apple_declarations").AtListIndex(0).AtName("payload")
}

// TestValidateDeclarationPayloadAcceptsValid checks the happy path produces no diagnostics, so the
// validator cannot pass its other tests simply by rejecting everything.
func TestValidateDeclarationPayloadAcceptsValid(t *testing.T) {
	diags := validateDeclarationPayload(
		types.StringNull(),
		types.StringValue("com.apple.configuration.siri.settings"),
		types.StringValue(`{"Enabled":true,"ForceProfanityFilter":true}`),
		declarationPath(), "",
	)
	if diags.HasError() {
		t.Fatalf("valid declaration produced errors: %v", diags.Errors())
	}
	if len(diags.Warnings()) != 0 {
		t.Errorf("valid declaration produced warnings: %v", diags.Warnings())
	}
}

// TestValidateDeclarationPayloadSkipsUnresolvedValues pins the plan-time behaviour: a value
// Terraform has yet to compute must be left alone rather than guessed at, or an interpolated
// payload would fail every plan before its inputs exist. It must not pass silently either — the
// check never runs again, because the apply path marshals a payload without validating it — so an
// unknown value warns that the promised check did not happen. A null is a different case: there is
// nothing to check and nothing to warn about.
func TestValidateDeclarationPayloadSkipsUnresolvedValues(t *testing.T) {
	cases := map[string]struct {
		declarationType, payload types.String
		wantWarning              bool
	}{
		"unknown payload": {
			types.StringValue("com.apple.configuration.siri.settings"),
			types.StringUnknown(),
			true,
		},
		"unknown type": {
			types.StringUnknown(),
			types.StringValue(`{"Enabled":true}`),
			true,
		},
		"null payload": {
			types.StringValue("com.apple.configuration.siri.settings"),
			types.StringNull(),
			false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			diags := validateDeclarationPayload(
				types.StringNull(), tc.declarationType, tc.payload, declarationPath(), "",
			)
			if diags.HasError() {
				t.Errorf("unresolved value produced errors: %v", diags.Errors())
			}
			warnings := diags.Warnings()
			if tc.wantWarning != (len(warnings) == 1) {
				t.Fatalf("warnings = %v, want exactly one = %v", warnings, tc.wantWarning)
			}
			if !tc.wantWarning {
				return
			}
			if summary := warnings[0].Summary(); !strings.Contains(summary, "not checked") {
				t.Errorf("summary = %q, want it to say the check did not run", summary)
			}
			if detail := warnings[0].Detail(); !strings.Contains(detail, "not known until apply") {
				t.Errorf("detail = %q, want it to name the reason the check was skipped", detail)
			}
		})
	}
}

// TestValidateDeclarationPayloadRejectsMalformedJSON covers the one failure that is not a schema
// finding: a payload that is not a JSON object at all.
func TestValidateDeclarationPayloadRejectsMalformedJSON(t *testing.T) {
	diags := validateDeclarationPayload(
		types.StringNull(),
		types.StringValue("com.apple.configuration.siri.settings"),
		types.StringValue(`not json`),
		declarationPath(), "",
	)
	if !diags.HasError() {
		t.Fatal("malformed payload produced no error")
	}
	if summary := diags.Errors()[0].Summary(); !strings.Contains(summary, "not a JSON object") {
		t.Errorf("summary = %q, want it to name the decoding failure", summary)
	}
	if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "jsonencode") {
		t.Errorf("detail = %q, want it to name jsonencode as the fix", detail)
	}
}

// TestValidateDeclarationPayloadReportsSchemaFindings checks that each finding reaches the caller as
// an error on the payload's own path, with a summary a reader can act on.
func TestValidateDeclarationPayloadReportsSchemaFindings(t *testing.T) {
	cases := []struct {
		name            string
		declarationType string
		payload         string
		wantSummary     string
	}{
		{
			name:            "unknown key",
			declarationType: "com.apple.configuration.siri.settings",
			payload:         `{"ZzNotAKey":true}`,
			wantSummary:     "Unknown key in declaration payload",
		},
		{
			name:            "miscased key",
			declarationType: "com.apple.configuration.siri.settings",
			payload:         `{"forceprofanityfilter":true}`,
			wantSummary:     "Unknown key in declaration payload",
		},
		{
			name:            "unknown declaration type",
			declarationType: "com.apple.configuration.not.a.thing",
			payload:         `{"Enabled":true}`,
			wantSummary:     "Unknown Apple declaration type",
		},
		{
			name:            "wrong value type",
			declarationType: "com.apple.configuration.siri.settings",
			payload:         `{"Enabled":"yes"}`,
			wantSummary:     "Declaration does not match Apple's schema",
		},
		{
			name:            "out of range",
			declarationType: "com.apple.configuration.passcode.settings",
			payload:         `{"MinimumLength":999}`,
			wantSummary:     "Declaration does not match Apple's schema",
		},
		{
			name:            "missing required key",
			declarationType: "com.apple.asset.data",
			payload:         `{}`,
			wantSummary:     "Declaration payload is missing a required key",
		},
		{
			name:            "unknown status item",
			declarationType: "com.apple.configuration.management.status-subscriptions",
			payload:         `{"StatusItems":[{"Name":"device.identifier.not-a-thing"}]}`,
			wantSummary:     "Unknown status item",
		},
	}

	at := declarationPath()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := validateDeclarationPayload(
				types.StringNull(), types.StringValue(tc.declarationType), types.StringValue(tc.payload), at, "",
			)
			if !diags.HasError() {
				t.Fatal("no error reported")
			}
			first := diags.Errors()[0]
			if first.Summary() != tc.wantSummary {
				t.Errorf("summary = %q, want %q", first.Summary(), tc.wantSummary)
			}
			if first.Detail() == "" {
				t.Error("detail is empty; the diagnostic would say nothing")
			}
		})
	}
}

// TestValidateDeclarationPayloadChecksAuthoredKind covers the surface that asks for a kind. The
// platform accepts any pairing of kind and type and passes the mismatch to the device, so an
// authored kind is a value the provider has to check rather than one it can trust — and deriving the
// kind from the type here instead would compare the type with itself and could never disagree.
func TestValidateDeclarationPayloadChecksAuthoredKind(t *testing.T) {
	cases := map[string]struct {
		kind      types.String
		wantError bool
	}{
		"mismatched kind": {types.StringValue("ASSET"), true},
		"agreeing kind":   {types.StringValue("CONFIGURATION"), false},
		"derived kind":    {types.StringNull(), false},
		"unresolved kind": {types.StringUnknown(), false},
		"kind left empty": {types.StringValue(""), false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			diags := validateDeclarationPayload(
				tc.kind,
				types.StringValue("com.apple.configuration.passcode.settings"),
				types.StringValue(`{"MinimumLength":8}`),
				path.Root("custom_declarations").AtName("declaration"),
				"com.apple.configuration.passcode.settings",
			)
			if !tc.wantError {
				if diags.HasError() {
					t.Fatalf("unexpected errors: %v", diags.Errors())
				}
				return
			}
			if !diags.HasError() {
				t.Fatal("mismatched kind produced no error")
			}
			first := diags.Errors()[0]
			if first.Summary() != "Declaration kind does not match its type" {
				t.Errorf("summary = %q, want the kind mismatch summary", first.Summary())
			}
			for _, want := range []string{"ASSET", "CONFIGURATION", "com.apple.configuration.passcode.settings"} {
				if !strings.Contains(first.Detail(), want) {
					t.Errorf("detail does not name %q: %s", want, first.Detail())
				}
			}
		})
	}
}

// TestValidatePayloadReferencesChecksPosition covers the reference no schema can check: Apple types
// an asset reference as a plain string, and the position it names is the provider's own derivation
// from the list index, so a reference past the end of the list is caught here or not at all.
func TestValidatePayloadReferencesChecksPosition(t *testing.T) {
	cases := map[string]struct {
		payload      types.String
		declarations int
		wantErrors   int
	}{
		"in range": {
			types.StringValue(`{"Reference":{"DataURL":"$PAYLOAD_1"}}`), 2, 0,
		},
		"last position": {
			types.StringValue(`{"Reference":{"DataURL":"$PAYLOAD_2"}}`), 2, 0,
		},
		"past the end": {
			types.StringValue(`{"Reference":{"DataURL":"$PAYLOAD_3"}}`), 2, 1,
		},
		"zero is not a position": {
			types.StringValue(`{"Reference":{"DataURL":"$PAYLOAD_0"}}`), 2, 1,
		},
		"too large to parse": {
			types.StringValue(`{"Reference":{"DataURL":"$PAYLOAD_99999999999999999999999"}}`), 2, 1,
		},
		"repeated reference reported once": {
			types.StringValue(`{"A":"$PAYLOAD_3","B":"$PAYLOAD_3"}`), 2, 1,
		},
		"two distinct references": {
			types.StringValue(`{"A":"$PAYLOAD_3","B":"$PAYLOAD_4"}`), 2, 2,
		},
		"no reference": {
			types.StringValue(`{"Enabled":true}`), 2, 0,
		},
		"unknown payload": {
			types.StringUnknown(), 2, 0,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			diags := validatePayloadReferences(tc.payload, tc.declarations, declarationPath())
			if got := len(diags.Errors()); got != tc.wantErrors {
				t.Fatalf("errors = %d, want %d: %v", got, tc.wantErrors, diags.Errors())
			}
			if tc.wantErrors == 0 {
				return
			}
			first := diags.Errors()[0]
			if !strings.Contains(first.Summary(), "references a declaration that does not exist") {
				t.Errorf("summary = %q, want it to name the missing declaration", first.Summary())
			}
			if !strings.Contains(first.Detail(), "2 declarations") {
				t.Errorf("detail does not name the list length: %s", first.Detail())
			}
		})
	}
}

// TestValidatePayloadReferencesNamesTheReference checks the diagnostic quotes the reference the
// author wrote, which is what tells them which string to change.
func TestValidatePayloadReferencesNamesTheReference(t *testing.T) {
	diags := validatePayloadReferences(
		types.StringValue(`{"Reference":{"DataURL":"$PAYLOAD_7"}}`), 1, declarationPath(),
	)
	if !diags.HasError() {
		t.Fatal("out-of-range reference produced no error")
	}
	detail := diags.Errors()[0].Detail()
	if !strings.Contains(detail, "$PAYLOAD_7") {
		t.Errorf("detail does not name the reference: %s", detail)
	}
	if !strings.Contains(detail, "1 declaration") {
		t.Errorf("detail does not count the single declaration: %s", detail)
	}
}

// TestRenderDeclarationProblemNamesTheSnapshotOnlyWhenRelevant pins which findings mention the
// schema snapshot. Every finding whose truth depends on which revision of Apple's schemas the
// provider carries must say where the schemas came from and how to proceed: a name the table has
// never heard of, and equally a value Apple has since admitted to an enum or a range, or a key Apple
// has since stopped requiring. A wrong value type and a miscased name are wrong against every
// revision, so pointing at the snapshot there would only invite someone to dismiss a real mistake.
func TestRenderDeclarationProblemNamesTheSnapshotOnlyWhenRelevant(t *testing.T) {
	cases := map[appledeclarations.ProblemKind]bool{
		appledeclarations.UnknownKey:              true,
		appledeclarations.UnknownDeclarationType:  true,
		appledeclarations.UnknownStatusItem:       true,
		appledeclarations.NotInEnum:               true,
		appledeclarations.OutOfRange:              true,
		appledeclarations.MissingRequiredKey:      true,
		appledeclarations.WrongType:               false,
		appledeclarations.MiscasedKey:             false,
		appledeclarations.MiscasedDeclarationType: false,
		appledeclarations.KindMismatch:            false,
	}

	for kind, wantSnapshot := range cases {
		t.Run(kind.String(), func(t *testing.T) {
			_, detail := renderDeclarationProblem(
				appledeclarations.Problem{Kind: kind, Path: "Enabled", Detail: "something is wrong."}, "", "",
			)
			mentions := strings.Contains(detail, "raw_component")
			if mentions != wantSnapshot {
				t.Errorf("detail mentions the escape hatch = %v, want %v\ndetail: %s", mentions, wantSnapshot, detail)
			}
		})
	}
}

// TestRenderDeclarationProblemNamesTheSeedBranch checks a finding about a key Apple has published
// but not released says so, since the surrounding schema being newer than the released one is the
// context an operator needs to judge it.
func TestRenderDeclarationProblemNamesTheSeedBranch(t *testing.T) {
	_, detail := renderDeclarationProblem(
		appledeclarations.Problem{
			Kind: appledeclarations.WrongType, Path: "AllowSiriAI",
			Detail: "expected a boolean.", SeedOnly: true,
		},
		"com.apple.configuration.siri.settings", "",
	)
	if !strings.Contains(detail, "pre-release branch") {
		t.Errorf("detail does not name the pre-release branch: %s", detail)
	}
}

// TestRenderDeclarationProblemAttributesSetElements checks that a finding against a set-backed
// component, where no element index exists to address, still identifies which declaration it came
// from.
func TestRenderDeclarationProblemAttributesSetElements(t *testing.T) {
	_, detail := renderDeclarationProblem(
		appledeclarations.Problem{Kind: appledeclarations.UnknownKey, Path: "Foo", Detail: "unknown."},
		"com.apple.configuration.siri.settings",
		"com.apple.configuration.siri.settings",
	)
	if !strings.Contains(detail, "com.apple.configuration.siri.settings") {
		t.Errorf("detail does not name the declaration: %s", detail)
	}
}

// TestDeclaredKeyHintOffersAnAlternative checks an unknown top-level key is answered with the keys
// Apple does declare, and that the hint stays silent where it would have to guess which dictionary
// a nested path belongs to.
func TestDeclaredKeyHintOffersAnAlternative(t *testing.T) {
	_, detail := renderDeclarationProblem(
		appledeclarations.Problem{
			Kind: appledeclarations.UnknownKey, Path: "ZzNotAKey", Detail: "unknown.",
		},
		"com.apple.configuration.siri.settings", "",
	)
	if !strings.Contains(detail, "ForceProfanityFilter") {
		t.Errorf("detail does not name a declared key: %s", detail)
	}

	if hint := declaredKeyHint("com.apple.asset.data", "Reference.DataURL"); hint != "" {
		t.Errorf("nested path produced a hint for the wrong dictionary: %s", hint)
	}
	if hint := declaredKeyHint("com.apple.configuration.not.a.thing", "Anything"); hint != "" {
		t.Errorf("unknown declaration type produced a hint: %s", hint)
	}
}

// TestDeclarationValidatorDescription checks the schema description names the upstream release, so
// generated documentation states which schemas a given provider build validates against.
func TestDeclarationValidatorDescription(t *testing.T) {
	description := appleDeclarationsSchemaValidator().MarkdownDescription(context.Background())
	_, release := appledeclarations.Provenance()
	if release != "" && !strings.Contains(description, release) {
		t.Errorf("description %q does not name the upstream release %q", description, release)
	}
}

// declarationListValue builds an apple_declarations list value from (type, payload) pairs, all on
// the SYSTEM channel, so a test can drive ValidateList the way the framework does.
func declarationListValue(t *testing.T, declarations ...[2]string) types.List {
	t.Helper()

	elementType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"channel": types.StringType,
		"payload": types.StringType,
		"type":    types.StringType,
	}}

	elements := make([]attr.Value, 0, len(declarations))
	for _, declaration := range declarations {
		element, diags := types.ObjectValue(elementType.AttrTypes, map[string]attr.Value{
			"channel": types.StringValue("SYSTEM"),
			"payload": types.StringValue(declaration[1]),
			"type":    types.StringValue(declaration[0]),
		})
		if diags.HasError() {
			t.Fatalf("building a declaration element: %v", diags)
		}
		elements = append(elements, element)
	}

	list, diags := types.ListValue(elementType, elements)
	if diags.HasError() {
		t.Fatalf("building the declaration list: %v", diags)
	}
	return list
}

// appleDeclarationsListPath is the path the framework passes ValidateList.
func appleDeclarationsListPath() path.Path {
	return path.Root("component_blocks").AtListIndex(0).AtName("apple_declarations")
}

// validateDeclarationList runs the list validator over a value and returns its diagnostics.
func validateDeclarationList(t *testing.T, list types.List) diag.Diagnostics {
	t.Helper()

	var resp validator.ListResponse
	appleDeclarationsSchemaValidator().ValidateList(
		context.Background(),
		validator.ListRequest{Path: appleDeclarationsListPath(), ConfigValue: list},
		&resp,
	)
	return resp.Diagnostics
}

// TestAppleDeclarationsValidateListAddressesTheElement pins that a finding lands on the declaration
// that caused it: `apple_declarations[1].payload`, not the collection. A path built from the wrong
// nesting level would fail nothing else.
func TestAppleDeclarationsValidateListAddressesTheElement(t *testing.T) {
	diags := validateDeclarationList(t, declarationListValue(t,
		[2]string{"com.apple.configuration.siri.settings", `{"Enabled":true}`},
		[2]string{"com.apple.configuration.siri.settings", `{"ZzNotAKey":true}`},
	))

	if !diags.HasError() {
		t.Fatal("an undeclared key in the second declaration produced no error")
	}

	want := appleDeclarationsListPath().AtListIndex(1).AtName("payload")
	for _, reported := range diags.Errors() {
		withPath, ok := reported.(diag.DiagnosticWithPath)
		if !ok {
			t.Fatalf("diagnostic carries no path: %v", reported)
		}
		if !withPath.Path().Equal(want) {
			t.Errorf("path = %s, want %s", withPath.Path(), want)
		}
	}
}

// TestAppleDeclarationsValidateListChecksReferencesAgainstTheList checks a $PAYLOAD_n reference is
// range-checked against the list's own length. The positions are the provider's derivation from
// list order, so nothing else can catch a reference past the end.
func TestAppleDeclarationsValidateListChecksReferencesAgainstTheList(t *testing.T) {
	inRange := declarationListValue(t,
		[2]string{"com.apple.asset.data", `{"Reference":{"DataURL":"https://example.com/a.zip","ContentType":"application/zip","Hash-SHA-256":"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"},"Authentication":{"Type":"MDM"}}`},
		[2]string{"com.apple.configuration.services.configuration-files", `{"ServiceType":"com.apple.sudo","DataAssetReference":"$PAYLOAD_1"}`},
	)
	if diags := validateDeclarationList(t, inRange); diags.HasError() {
		t.Fatalf("a reference to the first declaration produced errors: %v", diags.Errors())
	}

	pastTheEnd := declarationListValue(t,
		[2]string{"com.apple.configuration.services.configuration-files", `{"ServiceType":"com.apple.sudo","DataAssetReference":"$PAYLOAD_2"}`},
	)
	diags := validateDeclarationList(t, pastTheEnd)
	if !diags.HasError() {
		t.Fatal("a reference past the end of the list produced no error")
	}
	if detail := diags.Errors()[0].Detail(); !strings.Contains(detail, "1 declaration") {
		t.Errorf("detail does not count the list: %s", detail)
	}
}

// TestAppleDeclarationsValidateListSkipsUnresolvedList checks an absent or not-yet-computed list is
// left alone rather than read as zero declarations, which would turn every `$PAYLOAD_n` in an
// interpolated configuration into a plan error.
func TestAppleDeclarationsValidateListSkipsUnresolvedList(t *testing.T) {
	elementType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"channel": types.StringType,
		"payload": types.StringType,
		"type":    types.StringType,
	}}

	for name, list := range map[string]types.List{
		"null":    types.ListNull(elementType),
		"unknown": types.ListUnknown(elementType),
	} {
		t.Run(name, func(t *testing.T) {
			if diags := validateDeclarationList(t, list); len(diags) != 0 {
				t.Errorf("an unresolved list produced diagnostics: %v", diags)
			}
		})
	}
}

// TestAppleDeclarationsValidateListToleratesAnUnknownElement checks a declaration Terraform has yet
// to compute is read without error, which is what lets a model/element-type divergence be the only
// thing ElementsAs still reports.
func TestAppleDeclarationsValidateListToleratesAnUnknownElement(t *testing.T) {
	elementType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"channel": types.StringType,
		"payload": types.StringType,
		"type":    types.StringType,
	}}

	list, diags := types.ListValue(elementType, []attr.Value{types.ObjectUnknown(elementType.AttrTypes)})
	if diags.HasError() {
		t.Fatalf("building the declaration list: %v", diags)
	}

	if diags := validateDeclarationList(t, list); diags.HasError() {
		t.Errorf("an unknown element produced errors: %v", diags.Errors())
	}
}
